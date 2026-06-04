package db

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// New builds a tuned pgx pool. Pool sizing matters under high traffic:
// too few conns = queueing, too many = PG context-switch thrash.
// MaxConns is overridable via DB_MAX_CONNS (default 50). Keep it well under
// PostgreSQL's max_connections (100 here) to leave headroom for psql/migrations.
func New(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = envInt("DB_MAX_CONNS", 50)
	cfg.MinConns = envInt("DB_MIN_CONNS", 10)
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// envInt reads an int from env, falling back to def if unset or invalid.
func envInt(key string, def int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return int32(n)
		}
	}
	return def
}
