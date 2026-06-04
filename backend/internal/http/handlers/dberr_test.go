package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	"ewallet/backend/internal/cache"
	httpapi "ewallet/backend/internal/http"
)

// deadApp returns an app whose DB pool is already closed, so every query path
// returns a DB_ERROR. This covers the otherwise-hard-to-reach error branches in
// List, Ledger, Get, Create, and Readyz.
func deadApp(t *testing.T) *fiber.App {
	t.Helper()
	dbURL := testPoolURL()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("dead pool: %v", err)
	}
	rdb, err := cache.New(ctx, redisAddrForTest())
	if err != nil {
		t.Fatalf("dead redis: %v", err)
	}
	app := httpapi.BuildApp(pool, rdb, httpapi.Options{})
	pool.Close() // kill it -> all queries fail
	_ = rdb.Close()
	return app
}

func doApp(t *testing.T, app *fiber.App, method, path string, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestDBErrorBranches(t *testing.T) {
	app := deadApp(t)

	cases := []struct {
		name, method, path string
	}{
		{"list", "GET", "/api/v1/wallets"},
		{"get", "GET", "/api/v1/wallets/00000000-0000-0000-0000-000000000000"},
		{"ledger", "GET", "/api/v1/wallets/00000000-0000-0000-0000-000000000000/ledger"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := doApp(t, app, tc.method, tc.path, nil)
			if code != fiber.StatusInternalServerError && code != http.StatusServiceUnavailable {
				t.Fatalf("%s: want 500/503, got %d", tc.name, code)
			}
		})
	}

	t.Run("readyz db down", func(t *testing.T) {
		code := doApp(t, app, "GET", "/readyz", nil)
		if code != fiber.StatusServiceUnavailable {
			t.Fatalf("want 503, got %d", code)
		}
	})
}
