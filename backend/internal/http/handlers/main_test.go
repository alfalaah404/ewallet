package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"ewallet/backend/internal/cache"
	"ewallet/backend/internal/db"
	httpapi "ewallet/backend/internal/http"
)

var (
	testPool *pgxpool.Pool
	testRDB  *redis.Client
	testApp  *fiber.App
)

const defaultTestDBURL = "postgres://ewallet:ewallet_dev_pw@127.0.0.1:5432/ewallet_test?sslmode=disable"
const defaultTestRedis = "127.0.0.1:6379"

// TestMain sets up shared DB/Redis connections and the Fiber app once, then
// runs every test. Each test calls truncate() to start from a clean slate.
func TestMain(m *testing.M) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultTestDBURL
	}
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = defaultTestRedis
	}

	ctx := context.Background()
	var err error
	testPool, err = db.New(ctx, dbURL)
	if err != nil {
		panic("test db connect: " + err.Error())
	}
	testRDB, err = cache.New(ctx, redisAddr)
	if err != nil {
		panic("test redis connect: " + err.Error())
	}
	testApp = httpapi.BuildApp(testPool, testRDB, httpapi.Options{})

	code := m.Run()

	testPool.Close()
	_ = testRDB.Close()
	os.Exit(code)
}

func testPoolURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return defaultTestDBURL
}

func redisAddrForTest() string {
	if v := os.Getenv("TEST_REDIS_ADDR"); v != "" {
		return v
	}
	return defaultTestRedis
}

func truncate(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := testPool.Exec(ctx, "TRUNCATE ledger_entries, operations, wallets CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// do issues an in-process HTTP request through the Fiber app (no network).
func do(t *testing.T, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := testApp.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("app.Test %s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, data
}

// decodeData unmarshals the {"data": ...} envelope into dst.
func decodeData(t *testing.T, raw []byte, dst any) {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v (raw=%s)", err, raw)
	}
	if err := json.Unmarshal(env.Data, dst); err != nil {
		t.Fatalf("decode data: %v (raw=%s)", err, env.Data)
	}
}

// createWallet is a helper that returns the new wallet id.
func createWallet(t *testing.T, owner string, initial int64) string {
	t.Helper()
	resp, raw := do(t, "POST", "/api/v1/wallets", map[string]any{"owner": owner, "initial_balance": initial}, nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("createWallet status=%d raw=%s", resp.StatusCode, raw)
	}
	var w struct {
		ID string `json:"id"`
	}
	decodeData(t, raw, &w)
	return w.ID
}
