package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// QuotaTracker manages daily platform quota via Redis.
type QuotaTracker struct {
	rdb *redis.Client
}

// DefaultQuotaLimits per platform per day (or per period).
var DefaultQuotaLimits = map[string]int{
	"youtube":   10000, // units (1600 per upload = ~6 uploads/day default)
	"instagram": 25,    // posts per account per 24h
	"threads":   250,   // posts per profile per 24h
	"twitter":   10000, // posts per month (Basic tier)
	"facebook":  0,     // no strict daily limit
	"linkedin":  100,   // approximate daily API calls
}

// QuotaCosts per upload operation.
var QuotaCosts = map[string]int{
	"youtube":   1600, // units per videos.insert
	"instagram": 1,    // 1 post
	"threads":   1,    // 1 post
	"twitter":   1,    // 1 tweet
	"facebook":  0,    // no cost tracking needed
	"linkedin":  1,    // 1 API call
}

func NewQuotaTracker(rdb *redis.Client) *QuotaTracker {
	return &QuotaTracker{rdb: rdb}
}

func (qt *QuotaTracker) quotaKey(platform string) string {
	date := time.Now().UTC().Format("2006-01-02")
	return fmt.Sprintf("quota:%s:%s", platform, date)
}

// ConsumeQuota records usage of quota units for a platform.
// Best-effort: logs warning and continues if Redis is unavailable.
func (qt *QuotaTracker) ConsumeQuota(ctx context.Context, platform string, units int) error {
	key := qt.quotaKey(platform)
	pipe := qt.rdb.Pipeline()
	pipe.IncrBy(ctx, key, int64(units))
	pipe.Expire(ctx, key, 48*time.Hour)
	_, err := pipe.Exec(ctx)
	if err != nil {
		slog.Warn("quota tracking unavailable (Redis down), skipping consume", "platform", platform, "units", units, "error", err)
		return nil
	}
	return nil
}

// GetUsed returns the current quota usage for a platform today.
// Returns 0 (optimistic) if Redis is unavailable — allows uploads to proceed.
func (qt *QuotaTracker) GetUsed(ctx context.Context, platform string) (int, error) {
	key := qt.quotaKey(platform)
	val, err := qt.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		slog.Warn("quota tracking unavailable (Redis down), assuming 0 usage", "platform", platform, "error", err)
		return 0, nil
	}
	used, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parse quota for %s: %w", platform, err)
	}
	return used, nil
}

// GetRemaining returns remaining quota for a platform today.
func (qt *QuotaTracker) GetRemaining(ctx context.Context, platform string, limitOverride int) (int, error) {
	limit := limitOverride
	if limit <= 0 {
		limit = DefaultQuotaLimits[platform]
	}
	if limit <= 0 {
		return 999999, nil // no limit
	}

	used, err := qt.GetUsed(ctx, platform)
	if err != nil {
		return 0, err
	}

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}

// CanUpload checks if there's enough quota for an upload.
func (qt *QuotaTracker) CanUpload(ctx context.Context, platform string, cost int, limitOverride int) (bool, error) {
	remaining, err := qt.GetRemaining(ctx, platform, limitOverride)
	if err != nil {
		return false, err
	}
	return remaining >= cost, nil
}

// QuotaStatus returns quota info for display.
type QuotaStatus struct {
	Platform     string `json:"platform"`
	Used         int    `json:"used"`
	Limit        int    `json:"limit"`
	ResetsAt     string `json:"resets_at"`
	Period       string `json:"period"`
	DisplayLabel string `json:"display_label"`
}

// GetQuotaStatus returns displayable quota info for a platform.
func (qt *QuotaTracker) GetQuotaStatus(ctx context.Context, platform string, limitOverride int) (*QuotaStatus, error) {
	limit := limitOverride
	if limit <= 0 {
		limit = DefaultQuotaLimits[platform]
	}

	used, err := qt.GetUsed(ctx, platform)
	if err != nil {
		return nil, err
	}

	// Calculate reset time (midnight UTC)
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)

	period := "daily"
	var label string
	switch platform {
	case "youtube":
		uploads := used / 1600
		maxUploads := limit / 1600
		label = fmt.Sprintf("%d/%d uploads used today", uploads, maxUploads)
	case "instagram":
		label = fmt.Sprintf("%d/%d posts today", used, limit)
	case "threads":
		period = "rolling_24h"
		label = fmt.Sprintf("%d/%d posts in last 24h", used, limit)
	case "twitter":
		period = "monthly"
		label = fmt.Sprintf("%d/%d posts this month", used, limit)
	case "facebook":
		period = "none"
		label = "No strict limit"
	case "linkedin":
		label = fmt.Sprintf("~%d/%d API calls today", used, limit)
	default:
		label = fmt.Sprintf("%d/%d used today", used, limit)
	}

	return &QuotaStatus{
		Platform:     platform,
		Used:         used,
		Limit:        limit,
		ResetsAt:     midnight.Format(time.RFC3339),
		Period:       period,
		DisplayLabel: label,
	}, nil
}
