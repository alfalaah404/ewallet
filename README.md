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

## Deploy

The whole stack (API + Postgres + Redis + one-shot migration) is containerized.
A single multi-stage `Dockerfile` builds the SPA and the Go binary into one
image that serves both the API and the UI on port `8080` (single-origin).

### Option A: Coolify (recommended)

1. In Coolify: **New Resource → Docker Compose**, point it at this repo
   (branch `main`), and set the compose path to `deploy/docker-compose.yml`.
2. Set the environment variables (Coolify → the resource → Environment):
   - `POSTGRES_PASSWORD` — a long random string
   - `CORS_ORIGINS` — your public domain, e.g. `https://ewallet.example.com`
     (use `*` only for quick testing)
3. Coolify auto-detects the `api` service exposing port `8080`; attach your
   domain to it. Coolify terminates TLS and reverse-proxies for you — you do
   **not** need to publish ports yourself.
4. Deploy. Boot order is handled automatically: Postgres/Redis come up healthy,
   the `migrate` job runs `000001_init.sql` once, then `api` starts. The image
   healthcheck hits `/healthz`.

Postgres and Redis have **no public ports** (internal network only); their data
persists in named volumes (`postgres_data`, `redis_data`).

### Option B: plain `docker compose`

```bash
cd deploy
cp .env.example .env          # set POSTGRES_PASSWORD + CORS_ORIGINS
docker compose up -d --build
# API + UI live on http://<host>:8080  (health: /healthz, ready: /readyz)
```

To apply future schema changes, add migration files under
`backend/migrations/` and re-run the `migrate` service.

