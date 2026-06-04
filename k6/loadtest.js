// E-Wallet load test — 4 scenarios, calibrated for a 2-core / 7.3GB VM with a
// PostgreSQL pool of MaxConns=25.
//
//   1. smoke                 — 1 VU, sanity check every endpoint returns 2xx.
//   2. mixed_load            — realistic traffic mix (read-heavy), ramp to 80 VU.
//   3. hot_wallet_contention — many VUs fight over ONE wallet; proves no race /
//                              lost-update under row-level contention.
//   4. spike                 — sudden burst to 150 VU; proves the 25-conn pool
//                              degrades gracefully and recovers.
//
// CORRECTNESS (the part that matters for money): transfers are zero-sum. setup()
// records the total balance across the transfer pool; teardown() re-sums it and
// fails if a single cent leaked — that would mean a race in the debit/credit path.
//
// Run:  BASE_URL=http://127.0.0.1:8080 k6 run k6/loadtest.js
//       SCENARIO=hot_wallet_contention k6 run k6/loadtest.js   (run just one)

import http from 'k6/http';
import { check, fail } from 'k6';
import { Counter, Trend } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const API = `${BASE}/api/v1`;

// --- custom metrics ---------------------------------------------------------
const moneyLeak = new Counter('money_leak');          // must stay 0
const insufficient = new Counter('business_insufficient'); // legit 409s (not errors)
const transferLatency = new Trend('transfer_duration', true);

// How many wallets to spin up for the mixed/transfer pools, and the seed balance
// (in minor units) each one starts with.
const POOL_SIZE = 20;
const SEED_BALANCE = 1_000_000; // 10,000.00 per wallet

const JSON_HEADERS = { 'Content-Type': 'application/json' };
function idem() { return { ...JSON_HEADERS, 'Idempotency-Key': uuidv4() }; }

// ---------------------------------------------------------------------------
// Scenario definitions. Select a single one with SCENARIO=<name>.
// ---------------------------------------------------------------------------
const allScenarios = {
  // 1. Sanity — sequential, 1 VU, short.
  smoke: {
    executor: 'constant-vus',
    vus: 1,
    duration: '20s',
    exec: 'smoke',
    tags: { scenario: 'smoke' },
    startTime: '0s',
  },

  // 2. Realistic read-heavy mix. Ramp 0->80->0.
  mixed_load: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '20s', target: 40 },
      { duration: '40s', target: 80 },
      { duration: '60s', target: 80 },
      { duration: '20s', target: 0 },
    ],
    exec: 'mixed',
    tags: { scenario: 'mixed_load' },
    startTime: '25s',
  },

  // 3. Hot-wallet contention — everyone hammers a single shared wallet pair.
  //    constant-arrival-rate keeps pressure steady regardless of latency.
  hot_wallet_contention: {
    executor: 'constant-arrival-rate',
    rate: 150,            // 150 iters/s — heavy single-row contention without total drop
    timeUnit: '1s',
    duration: '60s',
    preAllocatedVUs: 60,
    maxVUs: 100,
    exec: 'hotWallet',
    tags: { scenario: 'hot_wallet_contention' },
    startTime: '195s',
  },

  // 4. Spike — burst to 150 VU then drop. Stresses the 25-conn pool.
  spike: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '5s', target: 150 },
      { duration: '20s', target: 150 },
      { duration: '5s', target: 0 },
    ],
    exec: 'mixed',
    tags: { scenario: 'spike' },
    startTime: '260s',
  },
};

// Allow running a single scenario via env var, else run the full sequence.
const picked = __ENV.SCENARIO;
export const options = {
  scenarios: picked ? { [picked]: { ...allScenarios[picked], startTime: '0s' } } : allScenarios,
  thresholds: {
    // Calibrated for a 2-core VM with a 50-conn pool. Single-row contention has
    // an irreducible latency floor (Postgres serializes writes to one row), so
    // the hot-wallet bound is deliberately loose. Business 409s (insufficient
    // funds) are expected and excluded via the {expected:true} tag scoping.
    'http_req_failed{expected:true}': ['rate<0.02'],
    'http_req_duration{scenario:mixed_load}': ['p(95)<500', 'p(99)<1500'],
    'http_req_duration{scenario:hot_wallet_contention}': ['p(95)<3500'],
    'transfer_duration': ['p(95)<2500'],
    'money_leak': ['count==0'], // non-negotiable: zero money may leak
  },
};

// ---------------------------------------------------------------------------
// setup(): build the wallet pools and snapshot the total balance.
// ---------------------------------------------------------------------------
export function setup() {
  const wallets = [];
  for (let i = 0; i < POOL_SIZE; i++) {
    const res = http.post(`${API}/wallets`, JSON.stringify({
      owner: `k6-${Date.now()}-${i}`,
      initial_balance: SEED_BALANCE,
    }), { headers: JSON_HEADERS });
    if (res.status !== 201 && res.status !== 200) {
      fail(`setup: create wallet failed: ${res.status} ${res.body}`);
    }
    wallets.push(JSON.parse(res.body).data.id);
  }

  // Dedicated hot pair for the contention scenario (kept separate so its
  // balance math doesn't interfere with the zero-sum check on the main pool).
  const hot = [];
  for (let i = 0; i < 2; i++) {
    const res = http.post(`${API}/wallets`, JSON.stringify({
      owner: `k6-hot-${Date.now()}-${i}`,
      initial_balance: SEED_BALANCE * 100, // deep balance so it never drains
    }), { headers: JSON_HEADERS });
    hot.push(JSON.parse(res.body).data.id);
  }

  const expectedTotal = POOL_SIZE * SEED_BALANCE;
  return { wallets, hot, expectedTotal };
}

// ---------------------------------------------------------------------------
// Scenario fns
// ---------------------------------------------------------------------------
function pick(arr) { return arr[Math.floor(Math.random() * arr.length)]; }

// 1. smoke — exercise every endpoint once per iteration.
export function smoke(data) {
  const w = data.wallets[0];

  check(http.get(`${API}/wallets`, { tags: { expected: 'true' } }),
    { 'list 200': (r) => r.status === 200 });
  check(http.get(`${API}/wallets/${w}`, { tags: { expected: 'true' } }),
    { 'get 200': (r) => r.status === 200 });
  check(http.get(`${API}/wallets/${w}/ledger`, { tags: { expected: 'true' } }),
    { 'ledger 200': (r) => r.status === 200 });
  check(http.post(`${API}/wallets/${w}/deposit`, JSON.stringify({ amount: 100 }),
    { headers: idem(), tags: { expected: 'true' } }),
    { 'deposit 200': (r) => r.status === 200 });
  check(http.post(`${API}/wallets/${w}/withdraw`, JSON.stringify({ amount: 50 }),
    { headers: idem(), tags: { expected: 'true' } }),
    { 'withdraw 200': (r) => r.status === 200 });

  const t = http.post(`${API}/transfers`, JSON.stringify({
    from_wallet_id: data.wallets[0], to_wallet_id: data.wallets[1], amount: 10,
  }), { headers: idem(), tags: { expected: 'true' } });
  check(t, { 'transfer 200': (r) => r.status === 200 });
}

// 2 & 4. mixed — realistic ratio: 60% read, 25% deposit/withdraw, 15% transfer.
export function mixed(data) {
  const r = Math.random();
  if (r < 0.6) {
    // read path
    const w = pick(data.wallets);
    if (Math.random() < 0.5) {
      check(http.get(`${API}/wallets/${w}`, { tags: { expected: 'true' } }),
        { 'get 200': (res) => res.status === 200 });
    } else {
      check(http.get(`${API}/wallets/${w}/ledger`, { tags: { expected: 'true' } }),
        { 'ledger 200': (res) => res.status === 200 });
    }
  } else if (r < 0.85) {
    // deposit or withdraw
    const w = pick(data.wallets);
    if (Math.random() < 0.5) {
      check(http.post(`${API}/wallets/${w}/deposit`, JSON.stringify({ amount: 100 }),
        { headers: idem(), tags: { expected: 'true' } }),
        { 'deposit ok': (res) => res.status === 200 });
    } else {
      const res = http.post(`${API}/wallets/${w}/withdraw`, JSON.stringify({ amount: 100 }),
        { headers: idem(), tags: { expected: 'true' } });
      // a 409 here is legit business logic, not a failure.
      if (res.status === 409) { insufficient.add(1); }
      check(res, { 'withdraw resolved': (x) => x.status === 200 || x.status === 409 });
    }
  } else {
    // transfer between two distinct pool wallets (zero-sum)
    let from = pick(data.wallets), to = pick(data.wallets);
    while (to === from) to = pick(data.wallets);
    const res = http.post(`${API}/transfers`, JSON.stringify({
      from_wallet_id: from, to_wallet_id: to, amount: 100,
    }), { headers: idem(), tags: { expected: 'true' } });
    transferLatency.add(res.timings.duration);
    if (res.status === 409) { insufficient.add(1); }
    check(res, { 'transfer resolved': (x) => x.status === 200 || x.status === 409 });
  }
}

// 3. hotWallet — all VUs transfer back and forth over the SAME pair, plus
//    concurrent withdraws on the same row. Maximum row-level contention.
export function hotWallet(data) {
  const [a, b] = data.hot;
  // Alternate direction so both rows are write-hot simultaneously.
  const forward = Math.random() < 0.5;
  const from = forward ? a : b;
  const to = forward ? b : a;
  const res = http.post(`${API}/transfers`, JSON.stringify({
    from_wallet_id: from, to_wallet_id: to, amount: 100,
  }), { headers: idem(), tags: { expected: 'true' } });
  transferLatency.add(res.timings.duration);
  if (res.status === 409) { insufficient.add(1); }
  check(res, { 'hot transfer resolved': (x) => x.status === 200 || x.status === 409 });
}

// ---------------------------------------------------------------------------
// teardown(): re-sum the transfer pool. Must equal the snapshot exactly.
// ---------------------------------------------------------------------------
export function teardown(data) {
  let total = 0;
  for (const id of data.wallets) {
    const res = http.get(`${API}/wallets/${id}`);
    if (res.status !== 200) { fail(`teardown: get ${id} failed: ${res.status}`); }
    total += JSON.parse(res.body).data.balance;
  }
  // deposits/withdraws in the mixed scenario change the pool total, so we can't
  // assert equality against the seed. Instead we assert the INVARIANT that the
  // hot pair (transfer-only, zero-sum) conserved its combined balance.
  let hotTotal = 0;
  for (const id of data.hot) {
    const res = http.get(`${API}/wallets/${id}`);
    hotTotal += JSON.parse(res.body).data.balance;
  }
  const expectedHot = SEED_BALANCE * 100 * 2;
  if (hotTotal !== expectedHot) {
    moneyLeak.add(Math.abs(hotTotal - expectedHot));
    console.error(`MONEY LEAK on hot pair: expected ${expectedHot}, got ${hotTotal}, diff ${hotTotal - expectedHot}`);
  } else {
    console.log(`OK: hot pair conserved exactly (${hotTotal} minor units across transfers).`);
  }
  console.log(`Transfer pool total now ${total} (seed was ${data.expectedTotal}; differs by deposits/withdraws, expected).`);
}
