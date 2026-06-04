package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletHandler struct {
	db *pgxpool.Pool
}

func NewWalletHandler(db *pgxpool.Pool) *WalletHandler {
	return &WalletHandler{db: db}
}

// ---- DTOs ----

type createWalletReq struct {
	Owner          string `json:"owner"`
	InitialBalance int64  `json:"initial_balance"`
}

type walletResp struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	Balance int64  `json:"balance"`
	Version int64  `json:"version"`
}

type amountReq struct {
	Amount int64 `json:"amount"`
}

type transferReq struct {
	FromWalletID string `json:"from_wallet_id"`
	ToWalletID   string `json:"to_wallet_id"`
	Amount       int64  `json:"amount"`
}

func fail(c fiber.Ctx, status int, code, msg string) error {
	return c.Status(status).JSON(fiber.Map{
		"error": fiber.Map{"code": code, "message": msg},
	})
}

func ctxTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ---- Handlers ----

func (h *WalletHandler) Create(c fiber.Ctx) error {
	var req createWalletReq
	if err := c.Bind().Body(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid body")
	}
	if req.Owner == "" {
		return fail(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "owner required")
	}
	if req.InitialBalance < 0 {
		return fail(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "initial_balance must be >= 0")
	}
	ctx, cancel := ctxTimeout()
	defer cancel()

	var w walletResp
	err := h.db.QueryRow(ctx,
		`INSERT INTO wallets (owner, balance) VALUES ($1, $2)
		 RETURNING id, owner, balance, version`,
		req.Owner, req.InitialBalance,
	).Scan(&w.ID, &w.Owner, &w.Balance, &w.Version)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "could not create wallet")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": w})
}

func (h *WalletHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")
	ctx, cancel := ctxTimeout()
	defer cancel()

	var w walletResp
	err := h.db.QueryRow(ctx,
		`SELECT id, owner, balance, version FROM wallets WHERE id = $1`, id,
	).Scan(&w.ID, &w.Owner, &w.Balance, &w.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(c, fiber.StatusNotFound, "NOT_FOUND", "wallet not found")
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "query failed")
	}
	return c.JSON(fiber.Map{"data": w})
}

// Deposit: atomic increment. Idempotency-Key header dedupes retries.
func (h *WalletHandler) Deposit(c fiber.Ctx) error {
	return h.singleWalletMutation(c, "deposit")
}

// Withdraw: atomic conditional decrement. The WHERE balance >= amount guard is
// evaluated atomically with the UPDATE — concurrent requests cannot all pass a
// stale balance check, so balance can never go negative.
func (h *WalletHandler) Withdraw(c fiber.Ctx) error {
	return h.singleWalletMutation(c, "withdraw")
}

func (h *WalletHandler) singleWalletMutation(c fiber.Ctx, kind string) error {
	walletID := c.Params("id")
	idemKey := c.Get("Idempotency-Key")
	if idemKey == "" {
		return fail(c, fiber.StatusBadRequest, "IDEMPOTENCY_REQUIRED", "Idempotency-Key header required")
	}
	var req amountReq
	if err := c.Bind().Body(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid body")
	}
	if req.Amount <= 0 {
		return fail(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "amount must be > 0")
	}

	ctx, cancel := ctxTimeout()
	defer cancel()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "tx begin failed")
	}
	defer tx.Rollback(ctx)

	// Idempotency: claim the key first. If it already exists, this op already ran.
	var existing string
	err = tx.QueryRow(ctx,
		`INSERT INTO operations (idempotency_key, kind, payload)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING idempotency_key`,
		idemKey, kind, map[string]any{"wallet_id": walletID, "amount": req.Amount},
	).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		// Key already used — return prior wallet state, idempotent success.
		var w walletResp
		_ = h.db.QueryRow(ctx, `SELECT id, owner, balance, version FROM wallets WHERE id=$1`, walletID).
			Scan(&w.ID, &w.Owner, &w.Balance, &w.Version)
		return c.JSON(fiber.Map{"data": w, "meta": fiber.Map{"idempotent_replay": true}})
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "idempotency claim failed")
	}

	var newBalance, version int64
	var q string
	if kind == "deposit" {
		q = `UPDATE wallets SET balance = balance + $2, version = version + 1, updated_at = now()
		     WHERE id = $1 RETURNING balance, version`
	} else {
		q = `UPDATE wallets SET balance = balance - $2, version = version + 1, updated_at = now()
		     WHERE id = $1 AND balance >= $2 RETURNING balance, version`
	}
	err = tx.QueryRow(ctx, q, walletID, req.Amount).Scan(&newBalance, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		if kind == "withdraw" {
			return fail(c, fiber.StatusConflict, "INSUFFICIENT_FUNDS", "insufficient balance")
		}
		return fail(c, fiber.StatusNotFound, "NOT_FOUND", "wallet not found")
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "balance update failed")
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO ledger_entries (operation_id, wallet_id, type, amount, balance_after)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4)`,
		walletID, kind, req.Amount, newBalance)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "ledger write failed")
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "commit failed")
	}
	return c.JSON(fiber.Map{"data": walletResp{ID: walletID, Balance: newBalance, Version: version}})
}

// Transfer: debit source (conditional) + credit dest + paired ledger rows, all in
// one transaction. Atomic conditional debit prevents negative balance under high
// contention. Total system balance is preserved (debit == credit).
func (h *WalletHandler) Transfer(c fiber.Ctx) error {
	idemKey := c.Get("Idempotency-Key")
	if idemKey == "" {
		return fail(c, fiber.StatusBadRequest, "IDEMPOTENCY_REQUIRED", "Idempotency-Key header required")
	}
	var req transferReq
	if err := c.Bind().Body(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "BAD_REQUEST", "invalid body")
	}
	if req.Amount <= 0 {
		return fail(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "amount must be > 0")
	}
	if req.FromWalletID == req.ToWalletID {
		return fail(c, fiber.StatusBadRequest, "VALIDATION_ERROR", "cannot transfer to same wallet")
	}

	ctx, cancel := ctxTimeout()
	defer cancel()

	tx, err := h.db.Begin(ctx)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "tx begin failed")
	}
	defer tx.Rollback(ctx)

	var existing string
	err = tx.QueryRow(ctx,
		`INSERT INTO operations (idempotency_key, kind, payload)
		 VALUES ($1, 'transfer', $2)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING idempotency_key`,
		idemKey, map[string]any{"from": req.FromWalletID, "to": req.ToWalletID, "amount": req.Amount},
	).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.JSON(fiber.Map{"data": fiber.Map{"status": "ok"}, "meta": fiber.Map{"idempotent_replay": true}})
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "idempotency claim failed")
	}

	// One operation_id shared by both ledger legs of this transfer.
	opUUID := uuid.NewString()

	// Deadlock avoidance: acquire row locks on BOTH wallets in a deterministic
	// order (sorted by id) before mutating. Without this, A->B and B->A transfers
	// running concurrently grab locks in opposite order and deadlock, producing a
	// lock convoy that spikes tail latency to multiple seconds under load. Locking
	// lowest-id first means every transaction agrees on ordering => no cycle.
	if _, err = tx.Exec(ctx,
		`SELECT id FROM wallets WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`,
		req.FromWalletID, req.ToWalletID); err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "lock acquire failed")
	}

	// Debit source — conditional. No row => insufficient funds or missing wallet.
	var srcBalance int64
	err = tx.QueryRow(ctx,
		`UPDATE wallets SET balance = balance - $2, version = version + 1, updated_at = now()
		 WHERE id = $1 AND balance >= $2 RETURNING balance`,
		req.FromWalletID, req.Amount).Scan(&srcBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(c, fiber.StatusConflict, "INSUFFICIENT_FUNDS", "insufficient balance or source not found")
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "debit failed")
	}

	// Credit destination.
	var dstBalance int64
	err = tx.QueryRow(ctx,
		`UPDATE wallets SET balance = balance + $2, version = version + 1, updated_at = now()
		 WHERE id = $1 RETURNING balance`,
		req.ToWalletID, req.Amount).Scan(&dstBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(c, fiber.StatusNotFound, "NOT_FOUND", "destination wallet not found")
	}
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "credit failed")
	}

	// Paired ledger rows sharing one operation_id.
	_, err = tx.Exec(ctx,
		`INSERT INTO ledger_entries (operation_id, wallet_id, related_wallet_id, type, amount, balance_after)
		 VALUES ($6::uuid, $1::uuid, $2::uuid, 'transfer_debit',  $5, $3),
		        ($6::uuid, $2::uuid, $1::uuid, 'transfer_credit', $5, $4)`,
		req.FromWalletID, req.ToWalletID, srcBalance, dstBalance, req.Amount, opUUID)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "ledger write failed")
	}

	if err := tx.Commit(ctx); err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "commit failed")
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"from_wallet_id": req.FromWalletID, "from_balance": srcBalance,
		"to_wallet_id": req.ToWalletID, "to_balance": dstBalance,
		"amount": req.Amount,
	}})
}

// Ledger lists recent entries for a wallet.
func (h *WalletHandler) Ledger(c fiber.Ctx) error {
	id := c.Params("id")
	ctx, cancel := ctxTimeout()
	defer cancel()

	rows, err := h.db.Query(ctx,
		`SELECT le.type, le.amount, le.balance_after, le.created_at,
		        COALESCE(w2.owner, '') AS counterparty
		 FROM ledger_entries le
		 LEFT JOIN wallets w2 ON w2.id = le.related_wallet_id
		 WHERE le.wallet_id = $1
		 ORDER BY le.created_at DESC LIMIT 50`, id)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "query failed")
	}
	defer rows.Close()

	type entry struct {
		Type         string    `json:"type"`
		Amount       int64     `json:"amount"`
		BalanceAfter int64     `json:"balance_after"`
		CreatedAt    time.Time `json:"created_at"`
		Counterparty string    `json:"counterparty"`
	}
	out := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.Type, &e.Amount, &e.BalanceAfter, &e.CreatedAt, &e.Counterparty); err != nil {
			return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "scan failed")
		}
		out = append(out, e)
	}
	return c.JSON(fiber.Map{"data": out})
}

// List returns all wallets (admin/demo convenience).
func (h *WalletHandler) List(c fiber.Ctx) error {
	ctx, cancel := ctxTimeout()
	defer cancel()
	rows, err := h.db.Query(ctx, `SELECT id, owner, balance, version FROM wallets ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "query failed")
	}
	defer rows.Close()
	out := []walletResp{}
	for rows.Next() {
		var w walletResp
		if err := rows.Scan(&w.ID, &w.Owner, &w.Balance, &w.Version); err != nil {
			return fail(c, fiber.StatusInternalServerError, "DB_ERROR", "scan failed")
		}
		out = append(out, w)
	}
	return c.JSON(fiber.Map{"data": out})
}
