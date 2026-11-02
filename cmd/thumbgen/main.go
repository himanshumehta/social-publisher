package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/himanshumehta/social-publisher/internal/app"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// thumbgen is a one-time CLI tool to backfill thumbnails for existing videos.
// Usage: ./thumbgen                  (process all videos without thumbnails)
//        ./thumbgen <video-uuid>     (process a specific video)

func main() {
	app.NewLogger()

	cfg := app.MustLoadConfig()

	// Database
	pool, err := database.Connect(cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	rdb := app.NewRedis(cfg.Redis)
	defer rdb.Close()

	// S3
	hotS3 := infra.NewS3Client(cfg.S3)

	svc, err := service.New(cfg, pool, rdb, hotS3)
	if err != nil {
		slog.Error("failed to initialise services", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()

	// If a specific video ID is provided, process just that one
	if len(os.Args) > 1 {
		videoID, err := uuid.Parse(os.Args[1])
		if err != nil {
			slog.Error("invalid video UUID", "arg", os.Args[1], "error", err)
			os.Exit(1)
		}
		slog.Info("generating thumbnail for single video", "video_id", videoID)
		if err := svc.Thumbnail.GenerateThumbnail(ctx, videoID); err != nil {
			slog.Error("thumbnail generation failed", "video_id", videoID, "error", err)
			os.Exit(1)
		}
		slog.Info("thumbnail generated successfully", "video_id", videoID)
		return
	}

	// Otherwise, backfill all videos without thumbnails
	slog.Info("starting thumbnail backfill for all videos without thumbnails")

	rows, err := pool.Query(ctx,
		`SELECT id, file_name, mime_type FROM videos
		 WHERE thumbnail_key IS NULL AND is_deleted = FALSE
		 ORDER BY created_at ASC`)
	if err != nil {
		slog.Error("failed to query videos", "error", err)
		os.Exit(1)
	}
	defer rows.Close()

	var total, success, failed int
	for rows.Next() {
		var id uuid.UUID
		var fileName, mimeType string
		if err := rows.Scan(&id, &fileName, &mimeType); err != nil {
			slog.Error("failed to scan video row", "error", err)
			failed++
			continue
		}
		total++
		slog.Info("processing video", "video_id", id, "file_name", fileName, "mime_type", mimeType, "progress", fmt.Sprintf("%d/%d", total, total))

		if err := svc.Thumbnail.GenerateThumbnail(ctx, id); err != nil {
			slog.Error("thumbnail generation failed", "video_id", id, "error", err)
			failed++
		} else {
			slog.Info("thumbnail generated", "video_id", id)
			success++
		}
	}

	slog.Info("thumbnail backfill complete",
		"total", total,
		"success", success,
		"failed", failed,
	)
}
