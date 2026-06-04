package cache

import (
	"context"
	"testing"
	"time"
)

func TestNewSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rdb, err := New(ctx, "127.0.0.1:6379")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestNewFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 1 is reserved/unbound — connection must fail.
	_, err := New(ctx, "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error connecting to unreachable redis")
	}
}
