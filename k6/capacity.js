// Capacity / saturation test — finds the practical RPS ceiling per operation type
// by ramping the arrival rate until latency degrades. Separate scenarios so we get
// an honest number for READ (cheap) vs WRITE/TRANSFER (DB write + row locks).
//
// Run one at a time for a clean read:
//   OP=read     k6 run k6/capacity.js
//   OP=deposit  k6 run k6/capacity.js
//   OP=transfer k6 run k6/capacity.js
//
// The "ceiling" = the highest target stage where p95 stays acceptable and error
// rate stays ~0. k6 keeps the arrival rate fixed regardless of latency, so when
// the server can't keep up you'll see dropped_iterations climb — that's the wall.

import http from 'k6/http';
import { check } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const API = `${BASE}/api/v1`;
const OP = __ENV.OP || 'read';
const JSON_HEADERS = { 'Content-Type': 'application/json' };
function idem() { return { ...JSON_HEADERS, 'Idempotency-Key': uuidv4() }; }

// Each op type ramps through a staircase of arrival rates. Read can go much higher
// than transfer, so the staircases differ.
const STAIRS = {
  read:     [500, 1000, 2000, 4000, 6000, 8000],
  deposit:  [300, 600, 1200, 2000, 3000, 4000],
  transfer: [200, 400, 800, 1500, 2500, 3500],
};

function stages(targets) {
  // 15s at each rate; short ramp between. Long enough to expose saturation.
  const out = [];
  for (const t of targets) {
    out.push({ target: t, duration: '3s' });
    out.push({ target: t, duration: '15s' });
  }
  out.push({ target: 0, duration: '5s' });
  return out;
}

export const options = {
  scenarios: {
    capacity: {
      executor: 'ramping-arrival-rate',
      startRate: 100,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 400,
      stages: stages(STAIRS[OP]),
      exec: OP,
    },
  },
  thresholds: {
    // Informational — we read the per-stage breakdown, not pass/fail here.
    'http_req_duration': ['p(95)<2000'],
  },
};

export function setup() {
  // Two funded wallets for transfer/deposit; deep balance so transfers never drain.
  const ids = [];
  for (let i = 0; i < 200; i++) {
    const r = http.post(`${API}/wallets`, JSON.stringify({
      owner: `cap-${Date.now()}-${i}`, initial_balance: 1_000_000_000,
    }), { headers: JSON_HEADERS });
    ids.push(JSON.parse(r.body).data.id);
  }
  return { ids };
}

export function read(data) {
  const id = data.ids[Math.floor(Math.random() * data.ids.length)];
  check(http.get(`${API}/wallets/${id}`), { '200': (r) => r.status === 200 });
}

export function deposit(data) {
  const id = data.ids[Math.floor(Math.random() * data.ids.length)];
  check(http.post(`${API}/wallets/${id}/deposit`, JSON.stringify({ amount: 10 }),
    { headers: idem() }), { '200': (r) => r.status === 200 });
}

export function transfer(data) {
  // Spread across distinct pairs (ordered locking means no deadlock either way).
  let a = Math.floor(Math.random() * data.ids.length);
  let b = (a + 1 + Math.floor(Math.random() * (data.ids.length - 1))) % data.ids.length;
  const res = http.post(`${API}/transfers`, JSON.stringify({
    from_wallet_id: data.ids[a], to_wallet_id: data.ids[b], amount: 1,
  }), { headers: idem() });
  check(res, { 'ok': (r) => r.status === 200 || r.status === 409 });
}
