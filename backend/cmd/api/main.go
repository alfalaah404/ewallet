package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ewallet/backend/internal/cache"
	"ewallet/backend/internal/config"
	"ewallet/backend/internal/db"
	httpapi "ewallet/backend/internal/http"

	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	rdb, err := cache.New(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	// Serve the built frontend from ./web if present (single-origin API + UI).
	webDir := ""
	if st, err := os.Stat("./web"); err == nil && st.IsDir() {
		webDir = "./web"
	}

	app := httpapi.BuildApp(pool, rdb, httpapi.Options{WebDir: webDir})

	go func() {
		if err := app.Listen(":"+cfg.Port, fiber.ListenConfig{DisableStartupMessage: false}); err != nil {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("stopped")
}
