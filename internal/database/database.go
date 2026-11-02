package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/config"
)

func Connect(cfg config.DBConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	poolCfg.MaxConns = clampConns(cfg.MaxConns, 25)
	poolCfg.MinConns = clampConns(cfg.MinConns, 0)
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	const maxRetries = 5
	backoff := 1 * time.Second

	var pool *pgxpool.Pool
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
		if err != nil {
			cancel()
			slog.Warn("database connection failed, retrying",
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff", backoff,
				"error", err,
			)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if err = pool.Ping(ctx); err != nil {
			cancel()
			pool.Close()
			slog.Warn("database ping failed, retrying",
				"attempt", attempt,
				"max_retries", maxRetries,
				"backoff", backoff,
				"error", err,
			)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		cancel()
		if attempt > 1 {
			slog.Info("database connected after retry", "attempt", attempt)
		}
		return pool, nil
	}

	return nil, fmt.Errorf("database unreachable after %d attempts: %w", maxRetries, err)
}

// maxPoolConns bounds DB_MAX_CONNS/DB_MIN_CONNS to a sane range.
const maxPoolConns = 1000

// clampConns converts a configured pool size to int32, falling back to def
// for values outside 0..maxPoolConns.
func clampConns(n, def int) int32 {
	if n < 0 || n > maxPoolConns {
		n = def
	}
	return int32(n) // #nosec G115 -- bounded above
}
