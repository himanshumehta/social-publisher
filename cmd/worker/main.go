package main

import (
	"context"
	"crypto/tls"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/database"
	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/service"
	"github.com/himanshumehta/social-publisher/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	slog.Info("worker starting")

	// Database
	pool, err := database.Connect(cfg.DB)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("connected to database", "database", cfg.DB.Name)

	// Redis (optional for worker)
	redisOpts := &redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}
	if cfg.Redis.TLS {
		redisOpts.TLSConfig = &tls.Config{}
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// S3 Clients
	hotS3 := infra.NewS3Client(cfg.S3)
	coldS3 := infra.NewS3Client(cfg.S3Cold)
	slog.Info("s3 clients initialized",
		"hot_bucket", cfg.S3.Bucket,
		"cold_bucket", cfg.S3Cold.Bucket,
	)

	// Services
	svc := service.New(cfg, pool, rdb, hotS3, coldS3)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// System user ID for auto-archive tasks (uses zero UUID as system actor)
	systemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	// Archive processor: poll every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("archive processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processed, err := svc.Archive.ProcessNextTask(ctx)
				if err != nil {
					slog.Error("archive processor error", "error", err)
				}
				if processed {
					slog.Info("archive task processed")
				}
			}
		}
	}()

	// Clip job processor: poll every 30 seconds (kept for backwards compatibility)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("clip job processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := svc.ClipJob.PickPending(ctx)
				if err != nil {
					slog.Error("clip job processor error", "error", err)
					continue
				}
				if job == nil {
					continue
				}
				slog.Info("processing clip job", "job_id", job.ID, "project_id", job.ProjectID)

				// Stub: real AI integration deferred
				// For now, complete with 0 clips
				if err := svc.ClipJob.Complete(ctx, job.ID, 0); err != nil {
					slog.Error("clip job complete error", "job_id", job.ID, "error", err)
				} else {
					slog.Info("clip job completed (stub)", "job_id", job.ID)
				}
			}
		}
	}()

	// AI job processor: poll every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		slog.Info("ai job processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := svc.AIJob.PickPending(ctx)
				if err != nil {
					slog.Error("ai job processor error", "error", err)
					continue
				}
				if job == nil {
					continue
				}
				slog.Info("processing ai job", "job_id", job.ID, "type", job.JobType, "project_id", job.ProjectID)

				// TODO: Route to actual AI provider based on job.JobType:
				// auto_clip: Whisper transcribe -> Claude segment -> FFmpeg cut
				// auto_subtitle: Whisper transcribe -> FFmpeg burn-in
				// ai_brief: Claude generate platform descriptions

				// Placeholder: complete with empty result
				creditsUsed := 1
				switch job.JobType {
				case "auto_clip":
					creditsUsed = 5
				case "auto_subtitle":
					creditsUsed = 3
				}

				if err := svc.AIJob.Complete(ctx, job.ID, []byte("{}"), creditsUsed); err != nil {
					slog.Error("ai job complete error", "job_id", job.ID, "error", err)
				} else {
					// Deduct credits (best effort; log error but do not fail the job)
					if err := svc.Credits.Deduct(ctx, job.CreatedBy, creditsUsed, "AI job: "+string(job.JobType), &job.ID); err != nil {
						slog.Warn("ai job credit deduction failed", "job_id", job.ID, "error", err)
					}
					slog.Info("ai job completed", "job_id", job.ID, "type", job.JobType, "credits", creditsUsed)
				}
			}
		}
	}()

	// Auto-archive: run every 6 hours
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		slog.Info("auto-archive scheduler started", "interval", "6h")

		// Run once at startup
		if err := svc.Archive.AutoArchiveStale(ctx, systemUserID); err != nil {
			slog.Error("auto-archive startup run failed", "error", err)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.Archive.AutoArchiveStale(ctx, systemUserID); err != nil {
					slog.Error("auto-archive error", "error", err)
				}
			}
		}
	}()

	// Session/temp file cleanup: run every 6 hours
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		slog.Info("temp file cleanup started", "interval", "6h")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Clean up expired upload sessions (older than 24 hours)
				result, err := pool.Exec(ctx,
					`DELETE FROM upload_sessions WHERE created_at < NOW() - INTERVAL '24 hours' AND status != 'completed'`)
				if err != nil {
					slog.Error("temp cleanup error", "error", err)
				} else {
					slog.Info("temp cleanup completed", "deleted", result.RowsAffected())
				}

				// Clean up expired refresh tokens (already revoked or expired)
				result2, err := pool.Exec(ctx,
					`DELETE FROM refresh_tokens WHERE (is_revoked = TRUE AND created_at < NOW() - INTERVAL '7 days') OR expires_at < NOW()`)
				if err != nil {
					slog.Error("expired token cleanup error", "error", err)
				} else {
					slog.Info("expired token cleanup completed", "deleted", result2.RowsAffected())
				}
			}
		}
	}()

	// Payout processing: check daily at midnight UTC (5:30 AM IST)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		slog.Info("payout processor started", "interval", "24h")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Find approved payouts
				rows, err := pool.Query(ctx,
					`SELECT id, creator_id, net_amount FROM payouts WHERE status = 'approved'`)
				if err != nil {
					slog.Error("payout query error", "error", err)
					continue
				}

				type pendingPayout struct {
					ID        uuid.UUID
					CreatorID uuid.UUID
					NetAmount int64
				}
				var payouts []pendingPayout
				for rows.Next() {
					var p pendingPayout
					if err := rows.Scan(&p.ID, &p.CreatorID, &p.NetAmount); err != nil {
						slog.Error("payout scan error", "error", err)
						continue
					}
					payouts = append(payouts, p)
				}
				rows.Close()

				if len(payouts) == 0 {
					continue
				}

				slog.Info("processing approved payouts", "count", len(payouts))

				for _, p := range payouts {
					// Mark as processing
					_, err := pool.Exec(ctx,
						`UPDATE payouts SET status = 'processing', updated_at = NOW() WHERE id = $1 AND status = 'approved'`,
						p.ID)
					if err != nil {
						slog.Error("payout status update error", "payout_id", p.ID, "error", err)
						continue
					}

					slog.Info("payout marked processing",
						"payout_id", p.ID,
						"creator_id", p.CreatorID,
						"net_amount_paise", p.NetAmount,
					)

					// TODO: Razorpay Payout API integration
					// 1. Look up creator's fund account (razorpay_fund_acc_id from users/banking table)
					// 2. POST https://api.razorpay.com/v1/payouts with:
					//    - fund_account_id, amount (paise), currency: "INR", mode: "NEFT"/"UPI"
					// 3. Store razorpay_payout_id on the payout record
					// 4. Poll for status or use webhook to move processing -> completed/failed
				}
			}
		}
	}()

	// YouTube analytics sync: run every 12 hours
	go func() {
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()

		slog.Info("analytics sync started", "interval", "12h")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Get channels with YouTube OAuth connected that need sync
				rows, err := pool.Query(ctx,
					`SELECT id FROM channels WHERE platform = 'youtube' AND is_active = TRUE
					 AND (last_analytics_sync IS NULL OR last_analytics_sync < NOW() - INTERVAL '12 hours')`)
				if err != nil {
					slog.Error("analytics sync query error", "error", err)
					continue
				}

				var channelIDs []uuid.UUID
				for rows.Next() {
					var id uuid.UUID
					if err := rows.Scan(&id); err != nil {
						continue
					}
					channelIDs = append(channelIDs, id)
				}
				rows.Close()

				if len(channelIDs) > 0 {
					slog.Info("syncing analytics for channels", "count", len(channelIDs))
				}

				for _, chID := range channelIDs {
					if err := svc.Revenue.SyncAnalytics(ctx, chID); err != nil {
						slog.Error("analytics sync failed", "channel_id", chID, "error", err)
						pool.Exec(ctx,
							`UPDATE channels SET analytics_sync_error = $1 WHERE id = $2`,
							err.Error(), chID)
					} else {
						slog.Info("analytics synced", "channel_id", chID)
					}
				}
			}
		}
	}()

	// V3: Post publishing pipeline
	postPipeline := worker.NewPostPipeline(
		svc.Post,
		svc.PostScanner,
		svc.PostGenerator,
		svc.PostPublisher,
		worker.DefaultPostPipelineConfig(),
		logger,
	)
	go postPipeline.Start(ctx)

	// V2: Processing poller: checks platform processing status every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("processing poller started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.Orchestrator.ProcessingPoll(ctx); err != nil {
					slog.Error("processing poll error", "error", err)
				}
			}
		}
	}()

	// V2: Upload queue processor: poll every 10 seconds
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		slog.Info("upload queue processor started", "interval", "10s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				item, err := svc.Orchestrator.PickNextQueueItem(ctx)
				if err != nil {
					slog.Error("upload queue pick error", "error", err)
					continue
				}
				if item == nil {
					continue
				}
				slog.Info("processing upload",
					"queue_id", item.ID,
					"platform", item.Platform,
					"publish_record_id", item.PublishRecordID,
					"attempt", item.Attempts,
				)
				if err := svc.Orchestrator.ProcessUpload(ctx, item); err != nil {
					slog.Error("upload processing failed",
						"queue_id", item.ID,
						"platform", item.Platform,
						"error", err,
					)
				} else {
					slog.Info("upload processing completed",
						"queue_id", item.ID,
						"platform", item.Platform,
					)
				}
			}
		}
	}()

	// V2: Token expiry checker: run every 24 hours
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		slog.Info("token expiry checker started", "interval", "24h")

		// Run once at startup
		warnings, err := svc.ChannelOAuth.CheckExpiringTokens(ctx)
		if err != nil {
			slog.Error("token expiry check failed", "error", err)
		} else if len(warnings) > 0 {
			for _, w := range warnings {
				slog.Warn("channel token expiring soon",
					"channel_id", w.ChannelID,
					"channel_name", w.ChannelName,
					"platform", w.Platform,
					"days_remaining", w.DaysRemaining,
				)
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				warnings, err := svc.ChannelOAuth.CheckExpiringTokens(ctx)
				if err != nil {
					slog.Error("token expiry check failed", "error", err)
					continue
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
		}
	}()

	// V8: Schedule worker: checks for due scheduled posts every 60 seconds
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		slog.Info("schedule worker started", "interval", "60s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dueIDs, err := svc.Calendar.PickDueScheduled(ctx)
				if err != nil {
					slog.Error("schedule worker: pick due failed", "error", err)
					continue
				}
				if len(dueIDs) == 0 {
					continue
				}

				slog.Info("schedule worker: found due posts", "count", len(dueIDs))

				for _, postID := range dueIDs {
					transitioned, err := svc.Calendar.TransitionToPublishing(ctx, postID)
					if err != nil {
						slog.Error("schedule worker: transition failed",
							"post_id", postID,
							"error", err,
						)
						continue
					}
					if transitioned {
						slog.Info("schedule worker: post released for publishing",
							"post_id", postID,
						)
					}
				}
			}
		}
	}()

	// V8: Virality scoring worker: score unscored posts every 15 minutes
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		slog.Info("virality scoring worker started", "interval", "15m")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				postIDs, err := svc.Virality.PickUnscoredPosts(ctx)
				if err != nil {
					slog.Error("virality pick unscored error", "error", err)
					continue
				}
				if len(postIDs) == 0 {
					continue
				}
				slog.Info("scoring unscored posts", "count", len(postIDs))
				if err := svc.Virality.ScoreBatch(ctx, postIDs); err != nil {
					slog.Error("virality batch score error", "error", err)
				} else {
					slog.Info("virality batch scoring complete", "count", len(postIDs))
				}
			}
		}
	}()

	// V8: A/B test auto-evaluator: run every 30 minutes
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()

		slog.Info("ab test evaluator started", "interval", "30m")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.ABTest.AutoEvaluate(ctx); err != nil {
					slog.Error("ab test auto-evaluate error", "error", err)
				}
			}
		}
	}()

	// V8: Goal update worker: recalculates revenue goal progress every 24 hours
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		slog.Info("goal update worker started", "interval", "24h")

		// Run once immediately on startup
		if err := svc.RevenueAttribution.UpdateGoalProgress(ctx); err != nil {
			slog.Error("goal update worker: initial run failed", "error", err)
		}

		for {
			select {
			case <-ctx.Done():
				slog.Info("goal update worker stopped")
				return
			case <-ticker.C:
				if err := svc.RevenueAttribution.UpdateGoalProgress(ctx); err != nil {
					slog.Error("goal update worker: update failed", "error", err)
				} else {
					slog.Info("goal update worker: cycle complete")
				}
			}
		}
	}()

	// V8: Push notification processor: poll every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("push notification processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processed, err := svc.Mobile.ProcessPushQueue(ctx)
				if err != nil {
					slog.Error("push notification processor error", "error", err)
				}
				if processed > 0 {
					slog.Info("push notifications processed", "count", processed)
				}
			}
		}
	}()

	// V9: Dub job processor: poll every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("dub job processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := svc.Dub.PickPending(ctx)
				if err != nil {
					continue
				}
				slog.Info("processing dub job", "job_id", job.ID, "target_language", job.TargetLanguage)
				// Simulate processing — in production this calls Sarvam AI or ElevenLabs
				creditsUsed := svc.Dub.EstimateCredits(300) // estimate 5 min video
				if err := svc.Dub.Complete(ctx, job.ID, "", "", creditsUsed); err != nil {
					slog.Error("dub job completion failed", "job_id", job.ID, "error", err)
					continue
				}
				if err := svc.Credits.Deduct(ctx, job.CreatedBy, creditsUsed, "dubbing: "+job.TargetLanguage, &job.ID); err != nil {
					slog.Warn("dub job credit deduction failed", "job_id", job.ID, "error", err)
				}
			}
		}
	}()

	// V9: Smart clip generator: poll every 30 seconds
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		slog.Info("smart clip processor started", "interval", "30s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				job, err := svc.SmartClip.PickPending(ctx)
				if err != nil {
					continue
				}
				slog.Info("processing smart clip job", "job_id", job.ID, "clip_count", job.ClipCount)
				// Simulate clip generation -- in production this does video analysis + cutting
				creditsUsed := svc.SmartClip.EstimateCredits(job.ClipCount)
				if err := svc.SmartClip.Complete(ctx, job.ID, creditsUsed); err != nil {
					slog.Error("smart clip job completion failed", "job_id", job.ID, "error", err)
					continue
				}
				if err := svc.Credits.Deduct(ctx, job.CreatedBy, creditsUsed, "smart clips", &job.ID); err != nil {
					slog.Warn("smart clip credit deduction failed", "job_id", job.ID, "error", err)
				}
			}
		}
	}()

	// V12: Commerce sync: run every 1 hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		slog.Info("commerce sync worker started", "interval", "1h")

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := svc.Commerce.SyncAllActive(ctx); err != nil {
					slog.Error("commerce sync error", "error", err)
				} else {
					slog.Info("commerce sync completed")
				}
			}
		}
	}()

	// V9: Newsletter scheduler: pick and send scheduled issues every 60 seconds
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		slog.Info("newsletter scheduler started", "interval", "60s")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				issues, err := svc.Newsletter.PickScheduledIssues(ctx)
				if err != nil {
					continue
				}
				for _, issue := range issues {
					slog.Info("sending scheduled newsletter issue", "issue_id", issue.ID, "subject", issue.Subject)
					// In production, this would send actual emails via SMTP to all active subscribers
				}
			}
		}
	}()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	<-done
	slog.Info("worker shutting down")
	cancel()
}
