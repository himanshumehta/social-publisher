package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
)

// MetaContainerUploader handles the container-based upload flow shared by
// Instagram Reels and Threads. Both platforms use a three-phase approach:
// create container, poll until ready, then publish.
type MetaContainerUploader struct {
	pool   *pgxpool.Pool
	crypto *TokenCrypto
	s3     *infra.S3Client
	http   *http.Client
}

// MetaContainerConfig holds platform-specific parameters that differ between
// Instagram and Threads while sharing the same upload mechanics.
type MetaContainerConfig struct {
	BaseURL           string // "graph.facebook.com" for IG, "graph.threads.net" for Threads
	APIVersion        string // "v21.0" for IG, "v1.0" for Threads
	UserIDField       string // JSON key in channel metadata: "ig_user_id" or "threads_user_id"
	MediaTypeField    string // "REELS" for IG, "VIDEO" for Threads
	CaptionField      string // "caption" for IG, "text" for Threads
	ContainerEndpoint string // "media" for IG, "threads" for Threads
	StatusFields      string // "status_code,status" for IG, "status,error_message" for Threads
	PublishEndpoint   string // "media_publish" for IG, "threads_publish" for Threads
	Platform          string // "instagram" or "threads" (for logging)
}

// MetaPublishResult holds identifiers returned after a successful publish.
type MetaPublishResult struct {
	ContainerID string
	MediaID     string
	Permalink   string
}

// metaContainerResponse is the JSON shape returned by container creation
// and publish endpoints.
type metaContainerResponse struct {
	ID        string `json:"id"`
	Permalink string `json:"permalink"`
}

// metaStatusResponse is the JSON shape returned by the status polling endpoint.
type metaStatusResponse struct {
	StatusCode   string `json:"status_code"`   // IG: EXPIRED, ERROR, FINISHED, IN_PROGRESS, PUBLISHED
	Status       string `json:"status"`        // Threads: IN_PROGRESS, FINISHED, ERROR, PUBLISHED
	ErrorMessage string `json:"error_message"` // Threads-specific error detail
}

// metaErrorResponse represents a Graph API error envelope.
type metaErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func NewMetaContainerUploader(pool *pgxpool.Pool, crypto *TokenCrypto, s3 *infra.S3Client) *MetaContainerUploader {
	return &MetaContainerUploader{
		pool:   pool,
		crypto: crypto,
		s3:     s3,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Upload performs the full container creation phase of the Meta upload flow:
//  1. Retrieve and decrypt channel tokens from the database
//  2. Extract the platform user ID from channel metadata JSON
//  3. Look up the video's S3 storage key
//  4. Generate a presigned S3 URL (1 hour expiry) for the platform to fetch
//  5. POST a container creation request to the platform API
//  6. Update the publish_record status to 'processing'
//  7. Return the container ID for subsequent polling
func (u *MetaContainerUploader) Upload(
	ctx context.Context,
	cfg MetaContainerConfig,
	channelID, versionID, publishRecordID uuid.UUID,
	caption string,
	coverURL string,
) (string, error) {
	// 1. Get channel tokens and metadata.
	accessToken, _, metadata, err := u.getChannelTokens(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("get channel tokens: %w", err)
	}

	// 2. Extract platform user ID from metadata.
	userID, err := extractMetadataField(metadata, cfg.UserIDField)
	if err != nil {
		return "", fmt.Errorf("extract %s from channel metadata: %w", cfg.UserIDField, err)
	}

	// 3. Get the S3 key for this video.
	s3Key, err := u.getVersionS3Key(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("get version s3 key: %w", err)
	}

	// 4. Generate a presigned URL valid for 1 hour.
	presignedURL, err := u.s3.PresignGetObject(ctx, s3Key, time.Hour)
	if err != nil {
		return "", fmt.Errorf("presign get object: %w", err)
	}

	// 5. Create the container.
	containerID, err := u.createContainer(ctx, cfg, userID, presignedURL, caption, accessToken, coverURL)
	if err != nil {
		return "", fmt.Errorf("create %s container: %w", cfg.Platform, err)
	}

	// 6. Update publish record to 'processing'.
	if err := u.updatePublishStatus(ctx, publishRecordID, "processing", ""); err != nil {
		slog.Error("failed to update publish record status",
			"publish_record_id", publishRecordID,
			"error", err,
		)
		// Non-fatal: the container was created successfully.
	}

	slog.Info("meta container created",
		"platform", cfg.Platform,
		"container_id", containerID,
		"channel_id", channelID,
		"publish_record_id", publishRecordID,
	)

	return containerID, nil
}

// PollStatus checks the processing status of a container. It returns a
// normalized status string: "in_progress", "finished", or "error".
// The raw platform status codes differ between IG and Threads but are
// mapped to these three values.
func (u *MetaContainerUploader) PollStatus(
	ctx context.Context,
	cfg MetaContainerConfig,
	containerID, token string,
) (string, error) {
	endpoint := fmt.Sprintf("https://%s/%s/%s",
		cfg.BaseURL, cfg.APIVersion, containerID)

	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse poll url: %w", err)
	}

	q := reqURL.Query()
	q.Set("fields", cfg.StatusFields)
	q.Set("access_token", token)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create poll request: %w", err)
	}

	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("poll container status: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read poll response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("poll returned %d: %s", resp.StatusCode, parseMetaError(body))
	}

	var status metaStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return "", fmt.Errorf("decode poll response: %w", err)
	}

	return normalizeMetaStatus(cfg, status), nil
}

// PublishContainer publishes a finished container and returns the resulting
// media ID and permalink.
func (u *MetaContainerUploader) PublishContainer(
	ctx context.Context,
	cfg MetaContainerConfig,
	userID, containerID, token string,
) (*MetaPublishResult, error) {
	endpoint := fmt.Sprintf("https://%s/%s/%s/%s",
		cfg.BaseURL, cfg.APIVersion, userID, cfg.PublishEndpoint)

	form := url.Values{}
	form.Set("creation_id", containerID)
	form.Set("access_token", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create publish request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publish container: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read publish response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("publish returned %d: %s", resp.StatusCode, parseMetaError(body))
	}

	var result metaContainerResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode publish response: %w", err)
	}

	if result.ID == "" {
		return nil, fmt.Errorf("publish response missing media id")
	}

	return &MetaPublishResult{
		ContainerID: containerID,
		MediaID:     result.ID,
		Permalink:   result.Permalink,
	}, nil
}

// --- private helpers ---

// getChannelTokens retrieves and decrypts channel credentials from the database.
func (u *MetaContainerUploader) getChannelTokens(ctx context.Context, channelID uuid.UUID) (accessToken, refreshToken string, metadata []byte, err error) {
	var encAccessToken string
	var encRefreshToken *string
	var tokenEncrypted bool

	err = u.pool.QueryRow(ctx,
		`SELECT access_token, refresh_token, token_encrypted, metadata
		 FROM channels WHERE id = $1`,
		channelID,
	).Scan(&encAccessToken, &encRefreshToken, &tokenEncrypted, &metadata)
	if err == pgx.ErrNoRows {
		return "", "", nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("query channel: %w", err)
	}

	if encAccessToken == "" {
		return "", "", nil, fmt.Errorf("channel has no access token configured")
	}

	encRefreshStr := ""
	if encRefreshToken != nil {
		encRefreshStr = *encRefreshToken
	}

	if tokenEncrypted && u.crypto != nil {
		accessToken, err = u.crypto.Decrypt(encAccessToken)
		if err != nil {
			return "", "", nil, fmt.Errorf("decrypt access token: %w", err)
		}
		if encRefreshStr != "" {
			refreshToken, err = u.crypto.Decrypt(encRefreshStr)
			if err != nil {
				return "", "", nil, fmt.Errorf("decrypt refresh token: %w", err)
			}
		}
	} else {
		accessToken = encAccessToken
		refreshToken = encRefreshStr
	}

	return accessToken, refreshToken, metadata, nil
}

// getVersionS3Key looks up the S3 storage key for a video.
func (u *MetaContainerUploader) getVersionS3Key(ctx context.Context, videoID uuid.UUID) (string, error) {
	var s3Key string
	err := u.pool.QueryRow(ctx,
		`SELECT storage_key FROM videos WHERE id = $1`, videoID,
	).Scan(&s3Key)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("video not found")
	}
	if err != nil {
		return "", fmt.Errorf("query video: %w", err)
	}
	if s3Key == "" {
		return "", fmt.Errorf("video has no storage key")
	}
	return s3Key, nil
}

// createContainer POSTs a new media container to the platform API.
func (u *MetaContainerUploader) createContainer(
	ctx context.Context,
	cfg MetaContainerConfig,
	userID, presignedURL, caption, token string,
	coverURL string,
) (string, error) {
	endpoint := fmt.Sprintf("https://%s/%s/%s/%s",
		cfg.BaseURL, cfg.APIVersion, userID, cfg.ContainerEndpoint)

	form := url.Values{}
	form.Set("media_type", cfg.MediaTypeField)
	form.Set("video_url", presignedURL)
	form.Set(cfg.CaptionField, caption)
	form.Set("access_token", token)

	// Instagram Reels require share_to_feed=true.
	if cfg.Platform == "instagram" {
		form.Set("share_to_feed", "true")
	}

	// Set cover image if provided (Instagram supports cover_url for Reels).
	if coverURL != "" {
		form.Set("cover_url", coverURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create container request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("post container: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read container response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("container creation returned %d: %s", resp.StatusCode, parseMetaError(body))
	}

	var result metaContainerResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode container response: %w", err)
	}

	if result.ID == "" {
		return "", fmt.Errorf("container response missing id")
	}

	return result.ID, nil
}

// updatePublishStatus sets the status (and optional error message) on a publish record.
func (u *MetaContainerUploader) updatePublishStatus(ctx context.Context, publishRecordID uuid.UUID, status, errMsg string) error {
	_, err := u.pool.Exec(ctx,
		`UPDATE publish_records SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
		status, nilIfEmptyStr(errMsg), publishRecordID)
	if err != nil {
		return fmt.Errorf("update publish record: %w", err)
	}
	return nil
}

// extractMetadataField parses a JSON metadata blob and returns the string
// value at the given top-level key.
func extractMetadataField(metadata []byte, field string) (string, error) {
	if len(metadata) == 0 {
		return "", fmt.Errorf("channel metadata is empty")
	}

	var m map[string]interface{}
	if err := json.Unmarshal(metadata, &m); err != nil {
		return "", fmt.Errorf("parse metadata JSON: %w", err)
	}

	val, ok := m[field]
	if !ok {
		return "", fmt.Errorf("field %q not found in metadata", field)
	}

	str, ok := val.(string)
	if !ok {
		// Handle numeric IDs stored as float64 by JSON.
		if num, ok := val.(float64); ok {
			return fmt.Sprintf("%.0f", num), nil
		}
		return "", fmt.Errorf("field %q is not a string", field)
	}

	if str == "" {
		return "", fmt.Errorf("field %q is empty", field)
	}

	return str, nil
}

// normalizeMetaStatus maps platform-specific status values to a common
// set: "in_progress", "finished", or "error".
func normalizeMetaStatus(cfg MetaContainerConfig, s metaStatusResponse) string {
	// Instagram uses StatusCode; Threads uses Status.
	raw := s.StatusCode
	if raw == "" {
		raw = s.Status
	}

	switch strings.ToUpper(raw) {
	case "FINISHED":
		return "finished"
	case "IN_PROGRESS":
		return "in_progress"
	case "ERROR", "EXPIRED":
		return "error"
	case "PUBLISHED":
		return "finished"
	default:
		slog.Warn("unknown meta container status",
			"platform", cfg.Platform,
			"raw_status", raw,
		)
		return "in_progress"
	}
}

// parseMetaError extracts a human-readable error from a Graph API error response body.
func parseMetaError(body []byte) string {
	var e metaErrorResponse
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Message != "" {
		return fmt.Sprintf("[%s] %s (code %d)", e.Error.Type, e.Error.Message, e.Error.Code)
	}
	// Truncate raw body for log safety.
	if len(body) > 512 {
		return string(body[:512]) + "..."
	}
	return string(body)
}
