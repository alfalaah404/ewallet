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

The app image is self-contained: a single multi-stage `Dockerfile` builds the
SPA and the Go binary into one image that serves both the API and the UI on
port `8080` (single-origin). **Postgres and Redis are provisioned separately as
Coolify-managed services** — the compose file only runs the `api` plus a
one-shot `migrate` job, both pointed at those external services via env vars.

`deploy/docker-compose.yml` therefore needs two connection values:

- `DATABASE_URL` — full Postgres connection string from the Coolify Postgres resource
- `REDIS_ADDR`   — `host:port` from the Coolify Redis resource
- `CORS_ORIGINS` — your public origin (use `*` only for quick testing)

### Option A: Coolify (recommended)

1. **Create the databases first.** In Coolify add a **Postgres** resource and a
   **Redis** resource. Note their internal connection details (use the internal
   service hostnames so traffic stays on Coolify's private network).
2. **Add the app.** New Resource → **Docker Compose**, point it at this repo
   (branch `main`), compose path `deploy/docker-compose.yml`.
3. **Set environment variables** (Coolify → the resource → Environment):
   - `DATABASE_URL=postgres://USER:PASSWORD@<pg-internal-host>:5432/DBNAME?sslmode=disable`
   - `REDIS_ADDR=<redis-internal-host>:6379`
   - `CORS_ORIGINS=https://your-domain`
4. Coolify auto-detects the `api` service exposing port `8080`; attach your
   domain to it. Coolify terminates TLS and reverse-proxies for you — you do
   **not** need to publish ports yourself.
5. Deploy. Boot order is automatic: the `migrate` job runs `000001_init.sql`
   once against your Postgres, then `api` starts. The image healthcheck hits
   `/healthz`. The migration is idempotent (`IF NOT EXISTS`), so redeploys are safe.

### Option B: plain `docker compose` (self-host)

You supply your own Postgres + Redis (any host the containers can reach).

```bash
cd deploy
cp .env.example .env     # set DATABASE_URL + REDIS_ADDR + CORS_ORIGINS
docker compose up -d --build
# API + UI live on http://<host>:8080  (health: /healthz, ready: /readyz)
```

To apply future schema changes, add migration files under
`backend/migrations/` and re-run the `migrate` service.

> Verified locally with Docker 29 / Compose v5: image builds, `migrate` runs
> then exits 0, `api` starts and passes `/healthz`, and a full create-wallet →
> transfer → ledger round-trip works against external Postgres + Redis.


