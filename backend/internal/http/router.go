package httpapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"ewallet/backend/internal/http/handlers"
)

// Options controls optional features of the app (e.g. serving the SPA).
type Options struct {
	WebDir string // if non-empty and exists, serve the built frontend from here
}

// BuildApp wires the full Fiber app: middleware, health, wallet routes, and
// optional static SPA. Extracted from main so it can be exercised in tests.
func BuildApp(pool *pgxpool.Pool, rdb *redis.Client, opts Options) *fiber.App {
	app := fiber.New(fiber.Config{AppName: "ewallet-api"})

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Idempotency-Key"},
	}))

	health := handlers.NewHealthHandler(pool, rdb)
	app.Get("/healthz", health.Healthz)
	app.Get("/readyz", health.Readyz)

	wallet := handlers.NewWalletHandler(pool)
	api := app.Group("/api/v1")
	api.Get("/wallets", wallet.List)
	api.Post("/wallets", wallet.Create)
	api.Get("/wallets/:id", wallet.Get)
	api.Get("/wallets/:id/ledger", wallet.Ledger)
	api.Post("/wallets/:id/deposit", wallet.Deposit)
	api.Post("/wallets/:id/withdraw", wallet.Withdraw)
	api.Post("/transfers", wallet.Transfer)

	if opts.WebDir != "" {
		app.Use(static.New(opts.WebDir))
		app.Get("/*", func(c fiber.Ctx) error {
			return c.SendFile(opts.WebDir + "/index.html")
		})
	}

	return app
}
