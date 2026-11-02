// Command worker runs every background loop: the post pipeline, the schedule
// release loop, the video upload queue, the platform processing poller and
// housekeeping. Each loop claims work from Postgres with FOR UPDATE SKIP
// LOCKED, so any number of worker replicas can run side by side.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/himanshumehta/social-publisher/internal/app"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/service"
	"github.com/himanshumehta/social-publisher/internal/worker"
)

const (
	uploadQueueInterval     = 10 * time.Second
	processingPollInterval  = 30 * time.Second
	scheduleReleaseInterval = 60 * time.Second
	tokenExpiryInterval     = 24 * time.Hour
	cleanupInterval         = 6 * time.Hour
)

func main() {
	logger := app.NewLogger()

	cfg := app.MustLoadConfig()

	slog.Info("worker starting")

	pool, err := database.Connect(cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database", "database", cfg.DB.Name)

	rdb := app.NewRedis(cfg.Redis)
	defer rdb.Close()

	hotS3 := infra.NewS3Client(cfg.S3)
	slog.Info("s3 client initialized", "bucket", cfg.S3.Bucket)

	svc, err := service.New(cfg, pool, rdb, hotS3)
	if err != nil {
		slog.Error("failed to initialise services", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Text/image posts: scan → generate → review → publish.
	postPipeline := worker.NewPostPipeline(
		svc.Post,
		svc.PostScanner,
		svc.PostGenerator,
		svc.PostPublisher,
		worker.DefaultPostPipelineConfig(),
		logger,
	)
	go postPipeline.Start(ctx)

	// Scheduled posts: move due posts into the publish stage.
	go every(ctx, "schedule release", scheduleReleaseInterval, false, func(ctx context.Context) {
		releaseDuePosts(ctx, svc)
	})

	// Videos: drain the upload queue one item per tick.
	go every(ctx, "upload queue", uploadQueueInterval, false, func(ctx context.Context) {
		processNextUpload(ctx, svc)
	})

	// Videos: poll platforms that process uploads asynchronously.
	go every(ctx, "processing poller", processingPollInterval, false, func(ctx context.Context) {
		if err := svc.Orchestrator.ProcessingPoll(ctx); err != nil {
			slog.Error("processing poll error", "error", err)
		}
	})

	// Channels: warn about OAuth tokens that are close to expiry.
	go every(ctx, "token expiry checker", tokenExpiryInterval, true, func(ctx context.Context) {
		warnExpiringTokens(ctx, svc)
	})

	// Housekeeping: abandoned upload sessions, dead refresh tokens and expired
	// ingest idempotency keys.
	go every(ctx, "cleanup", cleanupInterval, false, func(ctx context.Context) {
		cleanup(ctx, pool)
	})

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	slog.Info("worker shutting down")
	cancel()
}

// every runs fn on a fixed interval until ctx is cancelled. When runAtStart
// is true fn also runs once immediately.
func every(ctx context.Context, name string, interval time.Duration, runAtStart bool, fn func(context.Context)) {
	slog.Info(name+" started", "interval", interval.String())
	if runAtStart {
		fn(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fn(ctx)
		}
	}
}

func releaseDuePosts(ctx context.Context, svc *service.Services) {
	dueIDs, err := svc.Calendar.PickDueScheduled(ctx)
	if err != nil {
		slog.Error("schedule worker: pick due failed", "error", err)
		return
	}
	if len(dueIDs) == 0 {
		return
	}
	slog.Info("schedule worker: found due posts", "count", len(dueIDs))
	for _, postID := range dueIDs {
		transitioned, err := svc.Calendar.TransitionToPublishing(ctx, postID)
		if err != nil {
			slog.Error("schedule worker: transition failed", "post_id", postID, "error", err)
			continue
		}
		if transitioned {
			slog.Info("schedule worker: post released for publishing", "post_id", postID)
		}
	}
}

func processNextUpload(ctx context.Context, svc *service.Services) {
	item, err := svc.Orchestrator.PickNextQueueItem(ctx)
	if err != nil {
		slog.Error("upload queue pick error", "error", err)
		return
	}
	if item == nil {
		return
	}
	slog.Info("processing upload",
		"queue_id", item.ID,
		"platform", item.Platform,
		"publish_record_id", item.PublishRecordID,
		"attempt", item.Attempts,
	)
	if err := svc.Orchestrator.ProcessUpload(ctx, item); err != nil {
		slog.Error("upload processing failed", "queue_id", item.ID, "platform", item.Platform, "error", err)
		return
	}
	slog.Info("upload processing completed", "queue_id", item.ID, "platform", item.Platform)
}

func warnExpiringTokens(ctx context.Context, svc *service.Services) {
	warnings, err := svc.ChannelOAuth.CheckExpiringTokens(ctx)
	if err != nil {
		slog.Error("token expiry check failed", "error", err)
		return
	}
	for _, w := range warnings {
		slog.Warn("channel token expiring soon",
			"channel_id", w.ChannelID,
			"channel_name", w.ChannelName,
			"platform", w.Platform,
			"days_remaining", w.DaysRemaining,
		)
	}
}

func cleanup(ctx context.Context, pool *pgxpool.Pool) {
	result, err := pool.Exec(ctx,
		`DELETE FROM upload_sessions WHERE created_at < NOW() - INTERVAL '24 hours' AND status != 'completed'`)
	if err != nil {
		slog.Error("upload session cleanup error", "error", err)
	} else {
		slog.Info("upload session cleanup completed", "deleted", result.RowsAffected())
	}

	result, err = pool.Exec(ctx,
		`DELETE FROM refresh_tokens WHERE (is_revoked = TRUE AND created_at < NOW() - INTERVAL '7 days') OR expires_at < NOW()`)
	if err != nil {
		slog.Error("expired token cleanup error", "error", err)
	} else {
		slog.Info("expired token cleanup completed", "deleted", result.RowsAffected())
	}

	if n, err := service.ExpireIngestRequests(ctx, pool); err != nil {
		slog.Error("ingest idempotency cleanup error", "error", err)
	} else {
		slog.Info("ingest idempotency cleanup completed", "deleted", n)
	}
}
