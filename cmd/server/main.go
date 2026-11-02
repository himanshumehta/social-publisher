package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/himanshumehta/social-publisher/internal/app"

	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/router"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func main() {
	logger := app.NewLogger()

	cfg := app.MustLoadConfig()

	// Database
	pool, err := database.Connect(cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database", "database", cfg.DB.Name)

	rdb := app.NewRedis(cfg.Redis)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis not available, continuing without cache", "error", err)
	} else {
		slog.Info("connected to redis")
	}

	s3Client := infra.NewS3Client(cfg.S3)
	slog.Info("s3 client initialized", "endpoint", cfg.S3.Endpoint, "bucket", cfg.S3.Bucket)

	svc := service.New(cfg, pool, rdb, s3Client)

	// Dev-only: bootstrap a local admin so the API can be used without signup/OAuth.
	if cfg.IsDev() && cfg.DevAdmin.Email != "" && cfg.DevAdmin.Password != "" {
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := service.EnsureDevAdmin(seedCtx, pool, cfg.DevAdmin.Email, cfg.DevAdmin.Password); err != nil {
			slog.Error("dev admin bootstrap failed", "error", err)
		} else {
			slog.Info("dev admin ready", "email", cfg.DevAdmin.Email)
		}
		seedCancel()
	}

	// Router
	handler := router.New(cfg, svc, logger, pool, rdb)

	// Server
	addr := fmt.Sprintf("%s:%s", cfg.App.Host, cfg.App.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", addr, "env", cfg.App.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("server shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
