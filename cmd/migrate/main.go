// CLI tool for database migrations.
//
// Usage:
//
//	go run cmd/migrate/main.go              # apply all pending (up)
//	go run cmd/migrate/main.go up           # apply all pending
//	go run cmd/migrate/main.go down         # roll back one migration
//	go run cmd/migrate/main.go status       # show applied & pending
//	go run cmd/migrate/main.go reset        # clear dirty state
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/migrate"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()
	pool, err := database.Connect(cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	m := migrate.New(pool, "migrations")

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		err = m.Up(ctx)
	case "down":
		err = m.Down(ctx)
	case "status":
		err = m.Status(ctx)
	case "reset":
		err = m.Reset(ctx)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\nusage: migrate [up|down|status|reset]\n", cmd)
		os.Exit(1)
	}

	if err != nil {
		slog.Error("migration failed", "command", cmd, "error", err)
		os.Exit(1)
	}

	if cmd == "up" {
		slog.Info("migrations completed successfully")
	}
}
