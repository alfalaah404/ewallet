package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// New connects to Redis. Used here for rate-limiting counters and read-through
// cache of balances — NEVER as source of truth for money.
func New(ctx context.Context, addr string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     50,
		MinIdleConns: 5,
		DialTimeout:  3 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, err
	}
	return rdb, nil
}
