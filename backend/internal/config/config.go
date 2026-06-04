package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv      string
	Port        string
	DatabaseURL string
	RedisAddr   string
	CORSOrigins string
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load parses config from env once at startup. Fails fast on missing critical vars.
func Load() (*Config, error) {
	cfg := &Config{
		AppEnv:      get("APP_ENV", "development"),
		Port:        get("APP_PORT", "8080"),
		DatabaseURL: get("DATABASE_URL", "postgres://ewallet:ewallet_dev_pw@127.0.0.1:5432/ewallet?sslmode=disable"),
		RedisAddr:   get("REDIS_ADDR", "127.0.0.1:6379"),
		CORSOrigins: get("CORS_ORIGINS", "*"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL required")
	}
	return cfg, nil
}
