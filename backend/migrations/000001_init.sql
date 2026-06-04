-- E-wallet schema. Money stored as BIGINT minor units (e.g. cents/rupiah-sen).
-- PostgreSQL is the single source of truth. All invariants enforced in DB.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS wallets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner       TEXT        NOT NULL,
    balance     BIGINT      NOT NULL DEFAULT 0 CHECK (balance >= 0),
    version     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ledger: append-only record of every money movement. One row per leg.
CREATE TABLE IF NOT EXISTS ledger_entries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id      UUID        NOT NULL,
    wallet_id         UUID        NOT NULL REFERENCES wallets(id),
    related_wallet_id UUID        REFERENCES wallets(id),
    type              TEXT        NOT NULL CHECK (type IN ('deposit','withdraw','transfer_debit','transfer_credit')),
    amount            BIGINT      NOT NULL CHECK (amount > 0),
    balance_after     BIGINT      NOT NULL CHECK (balance_after >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Operations: idempotency anchor. Unique idempotency_key prevents duplicate money ops
-- even under concurrent retries (race-safe via UNIQUE constraint).
CREATE TABLE IF NOT EXISTS operations (
    idempotency_key TEXT        PRIMARY KEY,
    kind            TEXT        NOT NULL,
    payload         JSONB       NOT NULL,
    result          JSONB,
    status          TEXT        NOT NULL DEFAULT 'completed',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ledger_wallet      ON ledger_entries (wallet_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ledger_operation   ON ledger_entries (operation_id);
