// Package app holds the start-up steps shared by every binary: logging,
// configuration and the Redis client.
package app

import (
	"crypto/tls"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/redact"
)

// NewLogger returns the JSON logger every binary uses and installs it as the
// slog default. String and error attributes pass through redact, so OAuth
// tokens embedded in request URLs or error bodies never reach the log.
func NewLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: redact.ReplaceAttr, // tokens in URLs and API error bodies
	}))
	slog.SetDefault(logger)
	return logger
}

// MustLoadConfig loads configuration from the environment and exits the
// process if it does not validate. See config.Validate for the rules.
func MustLoadConfig() *config.Config {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if cfg.IsDev() {
		slog.Warn("running in development mode: relaxed configuration checks", "app_env", cfg.App.Env)
	}
	return cfg
}

// NewRedis builds the Redis client. TLS, when enabled, requires TLS 1.2+.
func NewRedis(cfg config.RedisConfig) *redis.Client {
	opts := &redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
	if cfg.TLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opts)
}
