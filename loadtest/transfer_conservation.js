import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

// Transfer conservation test.
//
// Two wallets A and B. VUs concurrently transfer STEP from A->B. With high
// contention on A's row, a racy backend could double-spend or lose money.
// Invariant: A.balance + B.balance == FUND at all times and at the end.
// A.balance never < 0.

const API = __ENV.API_BASE || 'http://localhost:8080';
const FUND = parseInt(__ENV.FUND || '500000');
const STEP = parseInt(__ENV.STEP || '50');

const ok           = new Counter('transfer_success');
const insufficient = new Counter('transfer_insufficient');
const errors       = new Counter('transfer_error');

export const options = {
  scenarios: {
    transfer_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '15s', target: 100 },
        { duration: '30s', target: 250 },
        { duration: '15s', target: 0 },
      ],
    },
  },
  thresholds: {
    'transfer_error': ['count<1'],
    http_req_duration: ['p(95)<400'],
  },
};

export function setup() {
  const h = { headers: { 'Content-Type': 'application/json' } };
  const a = http.post(`${API}/api/v1/wallets`, JSON.stringify({ owner: 'src', initial_balance: FUND }), h).json('data.id');
  const b = http.post(`${API}/api/v1/wallets`, JSON.stringify({ owner: 'dst', initial_balance: 0 }), h).json('data.id');
  console.log(`A=${a} B=${b} FUND=${FUND}`);
  return { a, b };
}

export default function (data) {
  const key = `${__VU}-${__ITER}-${Date.now()}`;
  const res = http.post(`${API}/api/v1/transfers`, JSON.stringify({
    from_wallet_id: data.a, to_wallet_id: data.b, amount: STEP,
  }), { headers: { 'Content-Type': 'application/json', 'Idempotency-Key': key } });

  if (res.status === 200) ok.add(1);
  else if (res.status === 409) insufficient.add(1);
  else errors.add(1);
}

export function teardown(data) {
  const a = http.get(`${API}/api/v1/wallets/${data.a}`).json('data.balance');
  const b = http.get(`${API}/api/v1/wallets/${data.b}`).json('data.balance');
  console.log(`FINAL A=${a} B=${b} total=${a + b} (must equal ${FUND}); A>=0 -> ${a >= 0}`);
  check(null, {
    'total conserved': () => a + b === FUND,
    'source not negative': () => a >= 0,
    'B equals transferred sum': () => b === FUND - a,
  });
}
