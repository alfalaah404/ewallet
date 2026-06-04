package handlers_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestCreateWallet(t *testing.T) {
	truncate(t)

	t.Run("success", func(t *testing.T) {
		resp, raw := do(t, "POST", "/api/v1/wallets", map[string]any{"owner": "alice", "initial_balance": 5000}, nil)
		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		var w struct {
			ID      string `json:"id"`
			Owner   string `json:"owner"`
			Balance int64  `json:"balance"`
		}
		decodeData(t, raw, &w)
		if w.Owner != "alice" || w.Balance != 5000 || w.ID == "" {
			t.Fatalf("unexpected wallet: %+v", w)
		}
	})

	t.Run("missing owner", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets", map[string]any{"owner": "", "initial_balance": 100}, nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("negative initial balance", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets", map[string]any{"owner": "bob", "initial_balance": -1}, nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets", "not-json-but-string", nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestGetWallet(t *testing.T) {
	truncate(t)
	id := createWallet(t, "carol", 1234)

	t.Run("found", func(t *testing.T) {
		resp, raw := do(t, "GET", "/api/v1/wallets/"+id, nil, nil)
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		var w struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &w)
		if w.Balance != 1234 {
			t.Fatalf("balance=%d", w.Balance)
		}
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := do(t, "GET", "/api/v1/wallets/00000000-0000-0000-0000-000000000000", nil, nil)
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("bad uuid", func(t *testing.T) {
		resp, _ := do(t, "GET", "/api/v1/wallets/not-a-uuid", nil, nil)
		if resp.StatusCode != fiber.StatusInternalServerError && resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("want 500/404, got %d", resp.StatusCode)
		}
	})
}

func TestListWallets(t *testing.T) {
	truncate(t)
	createWallet(t, "w1", 10)
	createWallet(t, "w2", 20)

	resp, raw := do(t, "GET", "/api/v1/wallets", nil, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var list []map[string]any
	decodeData(t, raw, &list)
	if len(list) != 2 {
		t.Fatalf("want 2 wallets, got %d raw=%s", len(list), raw)
	}
}

func TestDeposit(t *testing.T) {
	truncate(t)
	id := createWallet(t, "dep", 0)

	t.Run("success", func(t *testing.T) {
		resp, raw := do(t, "POST", "/api/v1/wallets/"+id+"/deposit", map[string]any{"amount": 500},
			map[string]string{"Idempotency-Key": "dep-1"})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		var w struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &w)
		if w.Balance != 500 {
			t.Fatalf("balance=%d", w.Balance)
		}
	})

	t.Run("idempotent replay", func(t *testing.T) {
		// Same key — must NOT double the balance.
		resp, raw := do(t, "POST", "/api/v1/wallets/"+id+"/deposit", map[string]any{"amount": 500},
			map[string]string{"Idempotency-Key": "dep-1"})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		var w struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &w)
		if w.Balance != 500 {
			t.Fatalf("idempotency broken: balance=%d (want 500)", w.Balance)
		}
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/deposit", map[string]any{"amount": 100}, nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid amount", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/deposit", map[string]any{"amount": 0},
			map[string]string{"Idempotency-Key": "dep-bad"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/deposit", "garbage",
			map[string]string{"Idempotency-Key": "dep-body"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("deposit to missing wallet", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/00000000-0000-0000-0000-000000000000/deposit",
			map[string]any{"amount": 100}, map[string]string{"Idempotency-Key": "dep-missing"})
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("want 404, got %d", resp.StatusCode)
		}
	})
}

func TestWithdraw(t *testing.T) {
	truncate(t)
	id := createWallet(t, "wd", 1000)

	t.Run("success", func(t *testing.T) {
		resp, raw := do(t, "POST", "/api/v1/wallets/"+id+"/withdraw", map[string]any{"amount": 400},
			map[string]string{"Idempotency-Key": "wd-1"})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		var w struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &w)
		if w.Balance != 600 {
			t.Fatalf("balance=%d", w.Balance)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/withdraw", map[string]any{"amount": 999999},
			map[string]string{"Idempotency-Key": "wd-2"})
		if resp.StatusCode != fiber.StatusConflict {
			t.Fatalf("want 409, got %d", resp.StatusCode)
		}
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/withdraw", map[string]any{"amount": 10}, nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid amount", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/withdraw", map[string]any{"amount": -5},
			map[string]string{"Idempotency-Key": "wd-neg"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestTransfer(t *testing.T) {
	truncate(t)
	a := createWallet(t, "src", 1000)
	b := createWallet(t, "dst", 0)

	t.Run("success", func(t *testing.T) {
		resp, raw := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": b, "amount": 300},
			map[string]string{"Idempotency-Key": "tx-1"})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
	})

	t.Run("balances correct after transfer", func(t *testing.T) {
		_, raw := do(t, "GET", "/api/v1/wallets/"+a, nil, nil)
		var wa struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &wa)
		_, raw = do(t, "GET", "/api/v1/wallets/"+b, nil, nil)
		var wb struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &wb)
		if wa.Balance != 700 || wb.Balance != 300 {
			t.Fatalf("balances a=%d b=%d (want 700/300)", wa.Balance, wb.Balance)
		}
	})

	t.Run("idempotent replay", func(t *testing.T) {
		resp, raw := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": b, "amount": 300},
			map[string]string{"Idempotency-Key": "tx-1"})
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
		}
		// balance must be unchanged (still 700)
		_, raw = do(t, "GET", "/api/v1/wallets/"+a, nil, nil)
		var wa struct {
			Balance int64 `json:"balance"`
		}
		decodeData(t, raw, &wa)
		if wa.Balance != 700 {
			t.Fatalf("idempotency broken: a=%d", wa.Balance)
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": b, "amount": 999999},
			map[string]string{"Idempotency-Key": "tx-insuf"})
		if resp.StatusCode != fiber.StatusConflict {
			t.Fatalf("want 409, got %d", resp.StatusCode)
		}
	})

	t.Run("same wallet", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": a, "amount": 10},
			map[string]string{"Idempotency-Key": "tx-same"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("missing destination", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": "00000000-0000-0000-0000-000000000000", "amount": 10},
			map[string]string{"Idempotency-Key": "tx-nodst"})
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": b, "amount": 10}, nil)
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid amount", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers",
			map[string]any{"from_wallet_id": a, "to_wallet_id": b, "amount": 0},
			map[string]string{"Idempotency-Key": "tx-zero"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		resp, _ := do(t, "POST", "/api/v1/transfers", "junk",
			map[string]string{"Idempotency-Key": "tx-junk"})
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Fatalf("want 400, got %d", resp.StatusCode)
		}
	})
}

func TestLedger(t *testing.T) {
	truncate(t)
	id := createWallet(t, "led", 0)
	do(t, "POST", "/api/v1/wallets/"+id+"/deposit", map[string]any{"amount": 100},
		map[string]string{"Idempotency-Key": "led-dep"})

	resp, raw := do(t, "GET", "/api/v1/wallets/"+id+"/ledger", nil, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var entries []map[string]any
	decodeData(t, raw, &entries)
	if len(entries) != 1 {
		t.Fatalf("want 1 ledger entry, got %d raw=%s", len(entries), raw)
	}
	// Deposit has no counterparty.
	if cp, _ := entries[0]["counterparty"].(string); cp != "" {
		t.Fatalf("deposit counterparty should be empty, got %q", cp)
	}
}

// TestLedgerCounterparty proves a transfer's ledger legs carry the other party's
// owner name on both sides (sender sees recipient, recipient sees sender).
func TestLedgerCounterparty(t *testing.T) {
	truncate(t)
	src := createWallet(t, "alice", 100)
	dst := createWallet(t, "bob", 0)
	do(t, "POST", "/api/v1/transfers",
		map[string]any{"from_wallet_id": src, "to_wallet_id": dst, "amount": 40},
		map[string]string{"Idempotency-Key": "cp-xfer"})

	// Sender ledger: transfer_debit, counterparty = bob.
	_, raw := do(t, "GET", "/api/v1/wallets/"+src+"/ledger", nil, nil)
	var srcEntries []map[string]any
	decodeData(t, raw, &srcEntries)
	if len(srcEntries) != 1 {
		t.Fatalf("want 1 src entry, got %d raw=%s", len(srcEntries), raw)
	}
	if srcEntries[0]["type"] != "transfer_debit" {
		t.Fatalf("want transfer_debit, got %v", srcEntries[0]["type"])
	}
	if cp, _ := srcEntries[0]["counterparty"].(string); cp != "bob" {
		t.Fatalf("sender counterparty want bob, got %q", cp)
	}

	// Recipient ledger: transfer_credit, counterparty = alice.
	_, raw = do(t, "GET", "/api/v1/wallets/"+dst+"/ledger", nil, nil)
	var dstEntries []map[string]any
	decodeData(t, raw, &dstEntries)
	if len(dstEntries) != 1 {
		t.Fatalf("want 1 dst entry, got %d raw=%s", len(dstEntries), raw)
	}
	if dstEntries[0]["type"] != "transfer_credit" {
		t.Fatalf("want transfer_credit, got %v", dstEntries[0]["type"])
	}
	if cp, _ := dstEntries[0]["counterparty"].(string); cp != "alice" {
		t.Fatalf("recipient counterparty want alice, got %q", cp)
	}
}

// TestConcurrentWithdrawNoRace proves the race-safe invariant inside the test
// suite: many goroutines hammer one wallet; exactly fund/amount succeed and the
// balance never goes negative.
func TestConcurrentWithdrawNoRace(t *testing.T) {
	truncate(t)
	id := createWallet(t, "hot", 1000)

	const workers = 50
	const each = 20 // 50*20 = 1000 attempts of amount=10 -> exactly 100 should pass on 1000 balance? no: 1000/10=100
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	conflict := 0

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(wk int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				resp, _ := do(t, "POST", "/api/v1/wallets/"+id+"/withdraw",
					map[string]any{"amount": 10},
					map[string]string{"Idempotency-Key": fmt.Sprintf("hot-%d-%d", wk, i)})
				mu.Lock()
				switch resp.StatusCode {
				case fiber.StatusOK:
					success++
				case fiber.StatusConflict:
					conflict++
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()

	if success != 100 {
		t.Fatalf("expected exactly 100 successful withdrawals, got %d (conflict=%d)", success, conflict)
	}

	_, raw := do(t, "GET", "/api/v1/wallets/"+id, nil, nil)
	var w struct {
		Balance int64 `json:"balance"`
	}
	decodeData(t, raw, &w)
	if w.Balance != 0 {
		t.Fatalf("final balance must be 0, got %d", w.Balance)
	}
}
