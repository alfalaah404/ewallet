import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

// High-contention e-wallet load test.
//
// Strategy: oversubscribe a SINGLE hot wallet to expose race conditions.
// One wallet is funded with exactly FUND units. Hundreds of concurrent VUs each
// try to withdraw STEP units. If the backend has a race (read-then-write without
// atomic guard), more than FUND/STEP withdrawals would succeed and balance would
// go negative. A correct backend lets exactly FUND/STEP succeed; the rest get 409.

const API = __ENV.API_BASE || 'http://localhost:8080';
const FUND = parseInt(__ENV.FUND || '1000000');   // hot wallet initial balance
const STEP = parseInt(__ENV.STEP || '100');        // amount per withdraw

const okWithdraw   = new Counter('withdraw_success');
const insufficient = new Counter('withdraw_insufficient');
const errors       = new Counter('withdraw_error');

export const options = {
  scenarios: {
    hot_wallet: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: 100 },
        { duration: '40s', target: 300 },
        { duration: '20s', target: 0 },
      ],
    },
  },
  thresholds: {
    // 409 insufficient-funds is correct business behaviour in a hot-wallet
    // stampede, so don't count it as a failure. Only true errors (5xx) matter.
    'withdraw_error': ['count<1'],
    http_req_duration: ['p(95)<400'],
  },
};

// setup() creates and funds the single hot wallet once before the test.
export function setup() {
  const res = http.post(`${API}/api/v1/wallets`, JSON.stringify({
    owner: 'hot-wallet', initial_balance: FUND,
  }), { headers: { 'Content-Type': 'application/json' } });
  const id = res.json('data.id');
  console.log(`hot wallet ${id} funded with ${FUND}`);
  return { walletId: id };
}

export default function (data) {
  // Unique idempotency key per attempt so each is a distinct withdraw.
  const key = `${__VU}-${__ITER}-${Date.now()}`;
  const res = http.post(
    `${API}/api/v1/wallets/${data.walletId}/withdraw`,
    JSON.stringify({ amount: STEP }),
    { headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key } },
  );

  if (res.status === 200) {
    okWithdraw.add(1);
    check(res, { 'balance never negative': (r) => r.json('data.balance') >= 0 });
  } else if (res.status === 409) {
    insufficient.add(1);
  } else {
    errors.add(1);
  }
}

// teardown() reports the final balance for the invariant check.
export function teardown(data) {
  const res = http.get(`${API}/api/v1/wallets/${data.walletId}`);
  console.log(`FINAL balance = ${res.json('data.balance')} (must be >= 0)`);
}
