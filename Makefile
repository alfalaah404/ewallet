.PHONY: help dev backend frontend build migrate up down logs test cover test-fe load load-smoke load-mixed load-hot load-spike

help:
	@echo "Targets: dev backend frontend build migrate up down logs test cover test-fe load load-smoke load-mixed load-hot load-spike"

# Run backend locally (needs native PostgreSQL + Redis + backend/.env)
backend:
	cd backend && go run ./cmd/api

# Run frontend dev server with /api proxy to localhost:8080
frontend:
	cd frontend && npm run dev

# Build backend binary + frontend dist, copy dist into backend/web (single-origin)
build:
	cd frontend && npm install && npm run build
	cp -r frontend/dist backend/web
	cd backend && go build -o bin/api ./cmd/api

# Apply DB migration via psql (uses backend/.env DATABASE_URL)
migrate:
	psql "$$(grep '^DATABASE_URL=' backend/.env | cut -d= -f2-)" -f backend/migrations/000001_init.sql

# Docker compose (production-like)
up:
	docker compose -f deploy/docker-compose.yml --env-file deploy/.env up -d --build

down:
	docker compose -f deploy/docker-compose.yml down

logs:
	docker compose -f deploy/docker-compose.yml logs -f --tail=100 api

# --- Standard quality gates (must exist in every serious project) ----------
# make test    -> backend unit/integration tests with race detector
# make cover   -> backend tests + coverage gate (>=90%)
# make test-fe -> frontend e2e (Playwright)
# make load    -> k6 load test (full 4-scenario sequence)
test:
	$(MAKE) -C backend test

cover:
	$(MAKE) -C backend cover

test-fe:
	cd frontend && npm run test

# k6 load tests. BASE_URL overrides the target (default http://127.0.0.1:8080).
# `make load` runs the full 4-scenario sequence (smoke -> mixed -> hot -> spike).
load:
	k6 run k6/loadtest.js

# Single scenarios (faster iteration).
load-smoke:
	SCENARIO=smoke k6 run k6/loadtest.js

load-mixed:
	SCENARIO=mixed_load k6 run k6/loadtest.js

# hot-wallet high-contention — proves no race / lost-update under stampede.
load-hot:
	SCENARIO=hot_wallet_contention k6 run k6/loadtest.js

# sudden burst — proves the 25-conn pool degrades gracefully and recovers.
load-spike:
	SCENARIO=spike k6 run k6/loadtest.js
