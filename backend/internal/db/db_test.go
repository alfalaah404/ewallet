package db

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNewSuccess(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := New(ctx, url)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestNewParseError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := New(ctx, "this-is-not-a-valid-url")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestNewPingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Valid DSN syntax but unreachable host so Ping fails.
	_, err := New(ctx, "postgres://nobody:nope@127.0.0.1:1/none?sslmode=disable")
	if err == nil {
		t.Fatal("expected ping error")
	}
}
