# E-Wallet Simulator

Interactive e-wallet demo for live multi-user transaction simulation.

- **Backend:** Go + Fiber, PostgreSQL, Redis. Money stored in minor units (integer cents). Transfers are idempotent (`Idempotency-Key`) and use row-locked, ordered updates to stay deadlock-free under contention.
- **Frontend:** Svelte 5 + Tailwind CSS, single-origin SPA served by the backend from `./web`.

## Features

Onboarding (name + initial balance), then 3 one-click actions:

1. **Topup** — add a fixed amount.
2. **Kirim** — transfer to another user via a searchable (Select2-style) recipient picker; the recipient stays selected after sending so you can fire repeated transfers. History labels each leg with the counterparty name.
3. **Belanja** — spend a fixed amount.

Balances and history update in near-realtime (≤2s poll), so a recipient sees incoming transfers without reloading.

## Layout

```
backend/    Go + Fiber API (serves built SPA from ./web)
frontend/   Svelte + Tailwind SPA
k6/         Load tests (smoke, realistic mix, hot-wallet contention, spike)
Makefile    Quality contract: make test / cover / load + npm run test
```

## Quality contract

From the repo root:

- `make test`  — backend tests with the Go race detector
- `make cover` — coverage gate (≥90%)
- `make load`  — k6 multi-scenario load + money/state conservation assertions
- `cd frontend && npm run test` — Playwright e2e (error/edge/validation paths)

## Dev setup

```bash
cp backend/.env.example backend/.env   # fill in real values
# build SPA + copy into backend/web, then run the API
make build && cd backend && ./bin/api
```
