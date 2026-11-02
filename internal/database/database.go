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

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
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
