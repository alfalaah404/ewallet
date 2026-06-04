package httpapi_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"

	httpapi "ewallet/backend/internal/http"
)

// TestBuildAppWithWebDir exercises the SPA static branch in BuildApp by
// pointing WebDir at a tmp dir containing an index.html. The static middleware
// + catch-all route serve that file for unknown paths.
func TestBuildAppWithWebDir(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pool/RDB are not used by static routes — pass nil; handlers won't be hit.
	app := httpapi.BuildApp(nil, nil, httpapi.Options{WebDir: dir})

	req := httptest.NewRequest("GET", "/some-spa-route", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
