package handlers_test

import (
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestHealthz(t *testing.T) {
	resp, raw := do(t, "GET", "/healthz", nil, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
}

func TestReadyz(t *testing.T) {
	resp, raw := do(t, "GET", "/readyz", nil, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d raw=%s", resp.StatusCode, raw)
	}
}
