package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/redact"
)

// UploadOrchestrator dispatches publish requests to the correct platform service
// and manages the upload queue.
type UploadOrchestrator struct {
	pool        *pgxpool.Pool
	youtube     *YouTubeUploadService
	instagram   *InstagramUploadService
	facebook    *FacebookUploadService
	threads     *ThreadsUploadService
	twitter     *TwitterUploadService
	linkedin    *LinkedInUploadService
	quota       *QuotaTracker
	tokenCrypto *TokenCrypto

	// dryRun, when set, replaces every platform upload with a log line.
	dryRun *DryRunAdapter
}

// EnableDryRun routes every platform upload to a DryRunAdapter: nothing is
// downloaded from storage or sent to a platform, and the publish record is
// marked published with a synthetic URL.
func (o *UploadOrchestrator) EnableDryRun(logger *slog.Logger) {
	o.dryRun = &DryRunAdapter{Logger: logger}
	logger.Info("upload orchestrator in DRY-RUN mode: no platform APIs will be called")
}

// ProcessingItem represents a publish record in 'processing' status that needs
// platform-side status polling.
type ProcessingItem struct {
	PublishRecordID   uuid.UUID
	ChannelID         uuid.UUID
	Platform          string
	PlatformContentID string
	VideoID           uuid.UUID
}

// QueuePublishRequest contains all the fields needed to queue a publish operation.
type QueuePublishRequest struct {
	VideoID   uuid.UUID `json:"video_id"`
	ChannelID uuid.UUID `json:"channel_id"`
	Platform  string    `json:"platform"`

	// Universal fields
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`

	// YouTube-specific
	Visibility string `json:"visibility"` // public, unlisted, private
	IsShort    bool   `json:"is_short"`
	CategoryID string `json:"youtube_category_id"`

	// Instagram/Threads
	Caption     string `json:"caption"`
	ShareToFeed bool   `json:"share_to_feed"`

	// Facebook
	IsReel bool `json:"is_reel"`

	// Twitter
	TweetText string `json:"tweet_text"`

	// LinkedIn
	PostText string `json:"post_text"`

	// Scheduling
	ScheduledAt *time.Time `json:"scheduled_at"`
}

func NewUploadOrchestrator(
	pool *pgxpool.Pool,
	youtube *YouTubeUploadService,
	instagram *InstagramUploadService,
	facebook *FacebookUploadService,
	threads *ThreadsUploadService,
	twitter *TwitterUploadService,
	linkedin *LinkedInUploadService,
	quota *QuotaTracker,
	tokenCrypto *TokenCrypto,
) *UploadOrchestrator {
	return &UploadOrchestrator{
		pool:        pool,
		youtube:     youtube,
		instagram:   instagram,
		facebook:    facebook,
		threads:     threads,
		twitter:     twitter,
		linkedin:    linkedin,
		quota:       quota,
		tokenCrypto: tokenCrypto,
	}
}

// QueuePublishResult is returned after queuing a publish request.
type QueuePublishResult struct {
	PublishRecordID uuid.UUID `json:"publish_record_id"`
	QueueItemID     uuid.UUID `json:"queue_item_id"`
	Status          string    `json:"status"`
	Platform        string    `json:"platform"`
}

// UploadQueueItem represents a row from the upload_queue table.
type UploadQueueItem struct {
	ID              uuid.UUID  `json:"id"`
	PublishRecordID uuid.UUID  `json:"publish_record_id"`
	ChannelID       uuid.UUID  `json:"channel_id"`
	VideoID         uuid.UUID  `json:"video_id"`
	Platform        string     `json:"platform"`
	Status          string     `json:"status"`
	Attempts        int        `json:"attempts"`
	MaxAttempts     int        `json:"max_attempts"`
	LastError       string     `json:"last_error"`
	ScheduledAt     *time.Time `json:"scheduled_at"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

// PickNextQueueItem fetches the next pending upload item for processing.
// Uses FOR UPDATE SKIP LOCKED to handle concurrent workers.
func (o *UploadOrchestrator) PickNextQueueItem(ctx context.Context) (*UploadQueueItem, error) {
	var item UploadQueueItem
	err := o.pool.QueryRow(ctx,
		`UPDATE upload_queue
		 SET status = 'downloading', started_at = NOW(), attempts = attempts + 1, updated_at = NOW()
		 WHERE id = (
			SELECT id FROM upload_queue
			WHERE status = 'pending'
			  AND (scheduled_at IS NULL OR scheduled_at <= NOW())
			  AND attempts < max_attempts
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, publish_record_id, channel_id, video_id, platform,
		           status, attempts, max_attempts, COALESCE(last_error, ''),
		           scheduled_at, started_at, completed_at`,
	).Scan(&item.ID, &item.PublishRecordID, &item.ChannelID, &item.VideoID,
		&item.Platform, &item.Status, &item.Attempts, &item.MaxAttempts, &item.LastError,
		&item.ScheduledAt, &item.StartedAt, &item.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pick queue item: %w", err)
	}

	// Update publish record status
	_, _ = o.pool.Exec(ctx,
		`UPDATE publish_records SET status = 'uploading', upload_started_at = NOW(), updated_at = NOW() WHERE id = $1`,
		item.PublishRecordID)

	return &item, nil
}

// ProcessUpload dispatches the upload to the correct platform service.
func (o *UploadOrchestrator) ProcessUpload(ctx context.Context, item *UploadQueueItem) error {
	var platformContentID string
	var err error

	// Get platform metadata from publish record
	var metaJSON []byte
	_ = o.pool.QueryRow(ctx,
		`SELECT COALESCE(platform_metadata, '{}') FROM publish_records WHERE id = $1`,
		item.PublishRecordID,
	).Scan(&metaJSON)

	var meta map[string]interface{}
	json.Unmarshal(metaJSON, &meta)

	// Get title/description from publish record
	var title, description string
	_ = o.pool.QueryRow(ctx,
		`SELECT COALESCE(title, ''), COALESCE(description, '') FROM publish_records WHERE id = $1`,
		item.PublishRecordID,
	).Scan(&title, &description)

	// Get thumbnail URL from publish record.
	var thumbnailURL string
	_ = o.pool.QueryRow(ctx,
		`SELECT COALESCE(thumbnail_url, '') FROM publish_records WHERE id = $1`,
		item.PublishRecordID,
	).Scan(&thumbnailURL)

	if o.dryRun != nil {
		return o.processDryRun(ctx, item, meta, title, description)
	}

	switch item.Platform {
	case "youtube":
		isShort := getBoolMeta(meta, "is_short")
		ytMeta := YouTubeUploadMeta{
			Title:        title,
			Description:  description,
			Visibility:   getStringMeta(meta, "visibility", "public"),
			IsShort:      isShort,
			CategoryID:   getStringMeta(meta, "category_id", ""),
			ThumbnailURL: thumbnailURL,
		}
		if tags, ok := meta["tags"]; ok {
			if tagSlice, ok := tags.([]interface{}); ok {
				for _, t := range tagSlice {
					if s, ok := t.(string); ok {
						ytMeta.Tags = append(ytMeta.Tags, s)
					}
				}
			}
		}
		platformContentID, err = o.youtube.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, ytMeta)

	case "instagram":
		igMeta := InstagramUploadMeta{
			Caption:      getStringMeta(meta, "caption", ""),
			ShareToFeed:  getBoolMeta(meta, "share_to_feed"),
			ThumbnailURL: thumbnailURL,
		}
		platformContentID, err = o.instagram.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, igMeta)

	case "facebook":
		fbMeta := FacebookUploadMeta{
			Title:        title,
			Description:  description,
			IsReel:       getBoolMeta(meta, "is_reel"),
			ThumbnailURL: thumbnailURL,
		}
		platformContentID, err = o.facebook.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, fbMeta)

	case "threads":
		thMeta := ThreadsUploadMeta{
			Text: getStringMeta(meta, "caption", ""),
		}
		platformContentID, err = o.threads.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, thMeta)

	case "twitter":
		twMeta := TwitterUploadMeta{
			TweetText: getStringMeta(meta, "tweet_text", ""),
		}
		platformContentID, err = o.twitter.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, twMeta)

	case "linkedin":
		liMeta := LinkedInUploadMeta{
			PostText:     getStringMeta(meta, "post_text", ""),
			ThumbnailURL: thumbnailURL,
		}
		platformContentID, err = o.linkedin.Upload(ctx, item.ChannelID, item.VideoID, item.PublishRecordID, liMeta)

	default:
		err = fmt.Errorf("unsupported platform: %s", item.Platform)
	}

	if err != nil {
		o.markFailed(ctx, item, err)
		return err
	}

	o.markCompleted(ctx, item, platformContentID)
	return nil
}

// processDryRun completes a queue item without contacting the platform.
func (o *UploadOrchestrator) processDryRun(ctx context.Context, item *UploadQueueItem, meta map[string]interface{}, title, description string) error {
	if !isSupportedVideoPlatform(item.Platform) {
		err := fmt.Errorf("unsupported platform: %s", item.Platform)
		o.markFailed(ctx, item, err)
		return err
	}
	caption := firstNonEmpty(
		getStringMeta(meta, "caption", ""),
		getStringMeta(meta, "tweet_text", ""),
		getStringMeta(meta, "post_text", ""),
		title,
		description,
	)
	contentID, platformURL, err := o.dryRun.Publish(ctx, PublishRequest{
		Platform: item.Platform,
		PostID:   item.PublishRecordID,
		Text:     caption,
	})
	if err != nil {
		o.markFailed(ctx, item, err)
		return err
	}
	o.markCompleted(ctx, item, contentID)
	return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
}

func isSupportedVideoPlatform(p string) bool {
	for _, s := range SupportedVideoPlatforms {
		if s == p {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (o *UploadOrchestrator) markFailed(ctx context.Context, item *UploadQueueItem, uploadErr error) {
	errMsg := redact.String(uploadErr.Error())
	adminMsg := MapPlatformError(errMsg)
	_, _ = o.pool.Exec(ctx,
		`UPDATE upload_queue SET status = 'failed', last_error = $1, completed_at = NOW(), updated_at = NOW() WHERE id = $2`,
		errMsg, item.ID)
	_, _ = o.pool.Exec(ctx,
		`UPDATE publish_records SET status = 'failed', error_message = $1, upload_completed_at = NOW(), updated_at = NOW(),
		 platform_metadata = platform_metadata || jsonb_build_object('admin_message', $2::text) WHERE id = $3`,
		errMsg, adminMsg, item.PublishRecordID)
}

func (o *UploadOrchestrator) markCompleted(ctx context.Context, item *UploadQueueItem, platformContentID string) {
	_, _ = o.pool.Exec(ctx,
		`UPDATE upload_queue SET status = 'completed', completed_at = NOW(), updated_at = NOW() WHERE id = $1`,
		item.ID)
	_, _ = o.pool.Exec(ctx,
		`UPDATE publish_records SET status = 'processing', platform_content_id = $1,
		 upload_progress_pct = 100, upload_completed_at = NOW(), processing_started_at = NOW(), updated_at = NOW()
		 WHERE id = $2`,
		platformContentID, item.PublishRecordID)
}

// MarkPublished updates the publish record when platform processing completes.
func (o *UploadOrchestrator) MarkPublished(ctx context.Context, publishRecordID uuid.UUID, platformURL string) error {
	now := time.Now()
	_, err := o.pool.Exec(ctx,
		`UPDATE publish_records SET status = 'published', platform_url = $1, published_at = $2, updated_at = NOW() WHERE id = $3`,
		platformURL, now, publishRecordID)
	return err
}

// GetProcessingItems returns all publish records currently in 'processing' status,
// joined with channel info for the processing poller.
func (o *UploadOrchestrator) GetProcessingItems(ctx context.Context) ([]ProcessingItem, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT pr.id, pr.channel_id, c.platform, COALESCE(pr.platform_content_id, ''), pr.video_id
		 FROM publish_records pr
		 JOIN channels c ON c.id = pr.channel_id
		 WHERE pr.status = 'processing'
		 ORDER BY pr.processing_started_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query processing items: %w", err)
	}
	defer rows.Close()

	var items []ProcessingItem
	for rows.Next() {
		var item ProcessingItem
		if err := rows.Scan(&item.PublishRecordID, &item.ChannelID, &item.Platform,
			&item.PlatformContentID, &item.VideoID); err != nil {
			return nil, fmt.Errorf("scan processing item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ProcessingPoll checks the platform-side processing status for all items
// currently in 'processing' state and transitions them to 'published' or 'failed'.
func (o *UploadOrchestrator) ProcessingPoll(ctx context.Context) error {
	items, err := o.GetProcessingItems(ctx)
	if err != nil {
		return fmt.Errorf("get processing items: %w", err)
	}
	if len(items) == 0 {
		return nil
	}

	for _, item := range items {
		if err := o.pollItem(ctx, item); err != nil {
			slog.Error("processing poll failed",
				"publish_record_id", item.PublishRecordID,
				"platform", item.Platform,
				"error", err,
			)
		}
	}
	return nil
}

func (o *UploadOrchestrator) pollItem(ctx context.Context, item ProcessingItem) error {
	switch item.Platform {
	case "youtube":
		return o.pollYouTube(ctx, item)
	case "instagram":
		return o.pollInstagram(ctx, item)
	case "threads":
		return o.pollThreads(ctx, item)
	case "facebook":
		return o.pollFacebook(ctx, item)
	case "twitter":
		// Twitter uploads create the tweet synchronously; mark published immediately.
		platformURL := fmt.Sprintf("https://x.com/i/status/%s", item.PlatformContentID)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	case "linkedin":
		// LinkedIn uploads create the post synchronously; mark published immediately.
		platformURL := fmt.Sprintf("https://www.linkedin.com/feed/update/%s", item.PlatformContentID)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	default:
		return fmt.Errorf("unsupported platform: %s", item.Platform)
	}
}

func (o *UploadOrchestrator) pollYouTube(ctx context.Context, item ProcessingItem) error {
	token, _, err := o.getChannelToken(ctx, item.ChannelID)
	if err != nil {
		return fmt.Errorf("get youtube token: %w", err)
	}

	status, err := o.youtube.CheckProcessing(ctx, item.PlatformContentID, token)
	if err != nil {
		return fmt.Errorf("youtube check processing: %w", err)
	}

	switch status {
	case "succeeded":
		platformURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.PlatformContentID)
		slog.Info("youtube processing complete",
			"publish_record_id", item.PublishRecordID,
			"video_id", item.PlatformContentID,
		)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	case "failed", "terminated":
		return o.markProcessingFailed(ctx, item.PublishRecordID, fmt.Sprintf("youtube processing %s", status))
	default:
		// Still processing; do nothing.
		return nil
	}
}

func (o *UploadOrchestrator) pollInstagram(ctx context.Context, item ProcessingItem) error {
	token, metadata, err := o.getChannelToken(ctx, item.ChannelID)
	if err != nil {
		return fmt.Errorf("get instagram token: %w", err)
	}

	status, err := o.instagram.PollStatus(ctx, item.PlatformContentID, token)
	if err != nil {
		return fmt.Errorf("instagram poll status: %w", err)
	}

	switch status {
	case "finished":
		// Extract the Instagram user ID from channel metadata for the publish call.
		userID, err := extractMetadataField(metadata, "instagram_business_account_id")
		if err != nil {
			return fmt.Errorf("extract instagram user id: %w", err)
		}

		result, err := o.instagram.Publish(ctx, userID, item.PlatformContentID, token)
		if err != nil {
			return fmt.Errorf("instagram publish: %w", err)
		}

		platformURL := result.Permalink
		if platformURL == "" {
			platformURL = fmt.Sprintf("https://www.instagram.com/reel/%s", result.MediaID)
		}

		// Update platform_content_id to the published media ID.
		_, _ = o.pool.Exec(ctx,
			`UPDATE publish_records SET platform_content_id = $1 WHERE id = $2`,
			result.MediaID, item.PublishRecordID)

		slog.Info("instagram publish complete",
			"publish_record_id", item.PublishRecordID,
			"media_id", result.MediaID,
		)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	case "error":
		return o.markProcessingFailed(ctx, item.PublishRecordID, "instagram container processing failed")
	default:
		return nil
	}
}

func (o *UploadOrchestrator) pollThreads(ctx context.Context, item ProcessingItem) error {
	token, metadata, err := o.getChannelToken(ctx, item.ChannelID)
	if err != nil {
		return fmt.Errorf("get threads token: %w", err)
	}

	status, err := o.threads.PollStatus(ctx, item.PlatformContentID, token)
	if err != nil {
		return fmt.Errorf("threads poll status: %w", err)
	}

	switch status {
	case "finished":
		userID, err := extractMetadataField(metadata, "threads_user_id")
		if err != nil {
			return fmt.Errorf("extract threads user id: %w", err)
		}

		result, err := o.threads.Publish(ctx, userID, item.PlatformContentID, token)
		if err != nil {
			return fmt.Errorf("threads publish: %w", err)
		}

		platformURL := result.Permalink
		if platformURL == "" {
			platformURL = fmt.Sprintf("https://www.threads.net/post/%s", result.MediaID)
		}

		_, _ = o.pool.Exec(ctx,
			`UPDATE publish_records SET platform_content_id = $1 WHERE id = $2`,
			result.MediaID, item.PublishRecordID)

		slog.Info("threads publish complete",
			"publish_record_id", item.PublishRecordID,
			"media_id", result.MediaID,
		)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	case "error":
		return o.markProcessingFailed(ctx, item.PublishRecordID, "threads container processing failed")
	default:
		return nil
	}
}

func (o *UploadOrchestrator) pollFacebook(ctx context.Context, item ProcessingItem) error {
	token, _, err := o.getChannelToken(ctx, item.ChannelID)
	if err != nil {
		return fmt.Errorf("get facebook token: %w", err)
	}

	status, err := o.facebook.CheckProcessing(ctx, item.PlatformContentID, token)
	if err != nil {
		return fmt.Errorf("facebook check processing: %w", err)
	}

	switch status {
	case "ready":
		platformURL := fmt.Sprintf("https://www.facebook.com/%s", item.PlatformContentID)
		slog.Info("facebook processing complete",
			"publish_record_id", item.PublishRecordID,
			"video_id", item.PlatformContentID,
		)
		return o.MarkPublished(ctx, item.PublishRecordID, platformURL)
	case "error":
		return o.markProcessingFailed(ctx, item.PublishRecordID, "facebook video processing failed")
	default:
		return nil
	}
}

// getChannelToken retrieves and decrypts the access token for a channel.
func (o *UploadOrchestrator) getChannelToken(ctx context.Context, channelID uuid.UUID) (string, []byte, error) {
	var accessToken string
	var tokenEncrypted bool
	var metadata []byte

	err := o.pool.QueryRow(ctx,
		`SELECT COALESCE(access_token, ''), COALESCE(token_encrypted, FALSE), metadata
		 FROM channels WHERE id = $1`, channelID,
	).Scan(&accessToken, &tokenEncrypted, &metadata)
	if err != nil {
		return "", nil, fmt.Errorf("query channel token: %w", err)
	}
	if accessToken == "" {
		return "", nil, fmt.Errorf("channel has no access token")
	}

	accessToken, err = o.tokenCrypto.Open(accessToken, tokenEncrypted)
	if err != nil {
		return "", nil, fmt.Errorf("decrypt access token: %w", err)
	}

	return accessToken, metadata, nil
}

// markProcessingFailed marks a publish record as failed during processing.
func (o *UploadOrchestrator) markProcessingFailed(ctx context.Context, publishRecordID uuid.UUID, errMsg string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE publish_records SET status = 'failed', error_message = $1, updated_at = NOW() WHERE id = $2`,
		redact.String(errMsg), publishRecordID)
	return err
}

func nilIfEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// RetryYouTubeThumbnail retries the thumbnail upload for published YouTube videos.
// It extracts the S3 key from the stored presigned thumbnail URL, generates a
// fresh presigned URL, and calls the YouTube API.
func (o *UploadOrchestrator) RetryYouTubeThumbnail(ctx context.Context, publishRecordID uuid.UUID) error {
	var channelID uuid.UUID
	var platformContentID, thumbnailURL string
	var platform string

	err := o.pool.QueryRow(ctx,
		`SELECT pr.channel_id, c.platform, COALESCE(pr.platform_content_id, ''), COALESCE(pr.thumbnail_url, '')
		 FROM publish_records pr
		 JOIN channels c ON c.id = pr.channel_id
		 WHERE pr.id = $1`,
		publishRecordID,
	).Scan(&channelID, &platform, &platformContentID, &thumbnailURL)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("publish record not found")
	}
	if err != nil {
		return fmt.Errorf("query publish record: %w", err)
	}

	if platform != "youtube" {
		return fmt.Errorf("retry-thumbnail only supports youtube (got %s)", platform)
	}
	if platformContentID == "" {
		return fmt.Errorf("publish record has no youtube video id")
	}
	if thumbnailURL == "" {
		return fmt.Errorf("publish record has no thumbnail url")
	}

	// Extract S3 key from the presigned URL: everything after /bucket/ and before ?
	s3Key := extractS3KeyFromURL(thumbnailURL)
	if s3Key == "" {
		return fmt.Errorf("could not extract s3 key from thumbnail url")
	}

	return o.youtube.RetryThumbnail(ctx, channelID, platformContentID, s3Key)
}

// extractS3KeyFromURL extracts the S3 object key from a path-style
// (presigned or public) S3 URL. The URL format is:
// https://<endpoint>/<bucket>/<key>?<query-params>
func extractS3KeyFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.TrimPrefix(u.Path, "/")
	// Drop the bucket segment; everything after it is the key.
	if idx := strings.Index(path, "/"); idx >= 0 && idx+1 < len(path) {
		return path[idx+1:]
	}
	return ""
}

func getStringMeta(meta map[string]interface{}, key, fallback string) string {
	if v, ok := meta[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func getBoolMeta(meta map[string]interface{}, key string) bool {
	if v, ok := meta[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
