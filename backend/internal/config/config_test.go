package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Clear env so defaults are used.
	for _, k := range []string{"APP_ENV", "APP_PORT", "DATABASE_URL", "REDIS_ADDR", "CORS_ORIGINS"} {
		os.Unsetenv(k)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("default port=%s", cfg.Port)
	}
	if cfg.AppEnv != "development" {
		t.Fatalf("default env=%s", cfg.AppEnv)
	}
	if cfg.RedisAddr != "127.0.0.1:6379" {
		t.Fatalf("default redis=%s", cfg.RedisAddr)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv("APP_PORT", "9999")
	os.Setenv("APP_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://x")
	os.Setenv("REDIS_ADDR", "redis:6379")
	os.Setenv("CORS_ORIGINS", "https://example.com")
	defer func() {
		for _, k := range []string{"APP_ENV", "APP_PORT", "DATABASE_URL", "REDIS_ADDR", "CORS_ORIGINS"} {
			os.Unsetenv(k)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != "9999" || cfg.AppEnv != "production" || cfg.CORSOrigins != "https://example.com" {
		t.Fatalf("env not applied: %+v", cfg)
	}
}
