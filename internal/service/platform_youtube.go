package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/netguard"
)

const (
	youtubeUploadURL  = "https://www.googleapis.com/upload/youtube/v3/videos"
	youtubeVideosURL  = "https://www.googleapis.com/youtube/v3/videos"
	youtubeTokenURL   = "https://oauth2.googleapis.com/token" // #nosec G101 -- endpoint URL, not a credential
	youtubeUploadCost = 1600                                  // quota units per videos.insert
	youtubeChunkSize  = 10 * 1024 * 1024                      // 10 MB
)

// YouTubeUploadService handles resumable uploads to YouTube using raw HTTP.
// It streams video data directly from S3 to YouTube in 10 MB chunks to stay
// within the worker's 256 MB RAM budget.
type YouTubeUploadService struct {
	pool         *pgxpool.Pool
	s3           *infra.S3Client
	crypto       *TokenCrypto
	quota        *QuotaTracker
	http         *http.Client
	media        *http.Client // thumbnail URLs (SSRF-guarded)
	clientID     string
	clientSecret string
}

// YouTubeUploadMeta holds video metadata for the YouTube upload.
type YouTubeUploadMeta struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	CategoryID   string   `json:"category_id"`
	Visibility   string   `json:"visibility"` // public, unlisted, private
	IsShort      bool     `json:"is_short"`
	ThumbnailURL string   `json:"thumbnail_url,omitempty"` // S3 presigned URL to set as custom thumbnail
}

const youtubeThumbnailURL = "https://www.googleapis.com/upload/youtube/v3/thumbnails/set"

// youtubeInitBody is the JSON body sent when initiating a resumable upload.
type youtubeInitBody struct {
	Snippet youtubeSnippet `json:"snippet"`
	Status  youtubeStatus  `json:"status"`
}

type youtubeSnippet struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	CategoryID  string   `json:"categoryId,omitempty"`
}

type youtubeStatus struct {
	PrivacyStatus string `json:"privacyStatus"`
}

// youtubeUploadResponse is the JSON returned when the upload completes.
type youtubeUploadResponse struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Snippet struct {
		Title string `json:"title"`
	} `json:"snippet"`
}

// youtubeVideoListResponse is the JSON returned by the videos.list endpoint.
type youtubeVideoListResponse struct {
	Items []struct {
		ProcessingDetails struct {
			ProcessingStatus string `json:"processingStatus"`
		} `json:"processingDetails"`
		Status struct {
			UploadStatus string `json:"uploadStatus"`
		} `json:"status"`
	} `json:"items"`
}

// youtubeErrorResponse represents a YouTube API error.
type youtubeErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// youtubeTokenRefreshResponse holds the response from a token refresh request.
type youtubeTokenRefreshResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func NewYouTubeUploadService(pool *pgxpool.Pool, s3 *infra.S3Client, crypto *TokenCrypto, quota *QuotaTracker, clientID, clientSecret string) *YouTubeUploadService {
	return &YouTubeUploadService{
		pool:         pool,
		s3:           s3,
		crypto:       crypto,
		quota:        quota,
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{
			// No global timeout: chunk uploads can take a while on large files.
			// Per-request timeouts are applied via context instead.
		},
		media: newMediaClient(false),
	}
}

// Upload performs the full YouTube resumable upload flow:
//  1. Check daily quota (1600 units per upload)
//  2. Retrieve and decrypt channel tokens from the database
//  3. Auto-refresh the access token if it is expired or about to expire
//  4. Look up the video's S3 key and file size
//  5. Initiate a resumable upload session (POST with uploadType=resumable)
//  6. Stream the video from S3 to YouTube in 10 MB chunks using the resumable URI
//  7. Record progress in publish_records.upload_progress_pct as chunks complete
//  8. Consume quota on success
//  9. Return the YouTube video ID
func (s *YouTubeUploadService) Upload(
	ctx context.Context,
	channelID, versionID, publishRecordID uuid.UUID,
	meta YouTubeUploadMeta,
) (string, error) {
	// 1. Check quota before doing any work.
	canUpload, err := s.quota.CanUpload(ctx, "youtube", youtubeUploadCost, 0)
	if err != nil {
		return "", fmt.Errorf("check youtube quota: %w", err)
	}
	if !canUpload {
		return "", fmt.Errorf("youtube daily quota exceeded (need %d units)", youtubeUploadCost)
	}

	// 2. Get channel credentials.
	accessToken, refreshToken, tokenExpiresAt, err := s.getChannelTokens(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("get channel tokens: %w", err)
	}

	// 3. Refresh if token is expired or will expire within 5 minutes.
	if tokenExpiresAt != nil && time.Until(*tokenExpiresAt) < 5*time.Minute {
		if refreshToken == "" {
			return "", fmt.Errorf("access token expired and no refresh token available")
		}
		newToken, err := s.refreshAccessToken(ctx, channelID, refreshToken)
		if err != nil {
			return "", fmt.Errorf("refresh youtube token: %w", err)
		}
		accessToken = newToken
	}

	// 4. Get S3 key and file size.
	s3Key, dbFileSize, err := s.getVersionS3Info(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("get version s3 info: %w", err)
	}

	// 4b. Verify actual S3 file size before initiating the resumable upload.
	// The DB file size may be stale (e.g. from seed data). YouTube requires the
	// Content-Length in the resumable-upload init to match the actual upload size.
	s3Body, s3Size, err := s.s3.GetObjectStream(ctx, s3Key)
	if err != nil {
		return "", fmt.Errorf("open s3 stream for size check: %w", err)
	}
	fileSize := dbFileSize
	if s3Size > 0 && s3Size != dbFileSize {
		slog.Warn("s3 file size differs from database, using s3 size",
			"db_size", dbFileSize,
			"s3_size", s3Size,
		)
		fileSize = s3Size
	}

	// For Shorts, prepend #Shorts to the title if not already present.
	title := meta.Title
	if meta.IsShort && !strings.Contains(title, "#Shorts") {
		title = title + " #Shorts"
	}

	// 5. Initiate the resumable upload.
	resumableURI, err := s.initiateResumableUpload(ctx, accessToken, fileSize, youtubeInitBody{
		Snippet: youtubeSnippet{
			Title:       title,
			Description: meta.Description,
			Tags:        meta.Tags,
			CategoryID:  meta.CategoryID,
		},
		Status: youtubeStatus{
			PrivacyStatus: normalizeVisibility(meta.Visibility),
		},
	})
	if err != nil {
		s3Body.Close()
		return "", fmt.Errorf("initiate resumable upload: %w", err)
	}

	slog.Info("youtube resumable upload initiated",
		"channel_id", channelID,
		"version_id", versionID,
		"file_size", fileSize,
		"resumable_uri", resumableURI[:min(len(resumableURI), 80)]+"...",
	)

	// 6. Stream from S3 to YouTube in chunks (reusing the already-opened stream).
	videoID, err := s.streamUploadFromReader(ctx, resumableURI, s3Body, fileSize, publishRecordID)
	if err != nil {
		return "", fmt.Errorf("stream upload: %w", err)
	}

	// 7. Consume quota on success.
	if err := s.quota.ConsumeQuota(ctx, "youtube", youtubeUploadCost); err != nil {
		slog.Error("failed to consume youtube quota (upload succeeded)",
			"video_id", videoID,
			"error", err,
		)
		// Non-fatal: the upload already completed.
	}

	// Mark progress as 100%.
	_ = s.updateProgress(ctx, publishRecordID, 100)

	slog.Info("youtube upload completed",
		"video_id", videoID,
		"channel_id", channelID,
		"publish_record_id", publishRecordID,
	)

	// 8. Set custom thumbnail if provided (non-fatal on failure).
	if meta.ThumbnailURL != "" {
		if err := s.setThumbnail(ctx, videoID, meta.ThumbnailURL, accessToken); err != nil {
			slog.Error("failed to set youtube thumbnail (upload succeeded)",
				"video_id", videoID,
				"error", err,
			)
		} else {
			slog.Info("youtube thumbnail set", "video_id", videoID)
		}
	}

	return videoID, nil
}

// CheckProcessing polls the YouTube Data API for the processing status of a video.
// Returns the processing status string: "processing", "succeeded", "failed", or "terminated".
func (s *YouTubeUploadService) CheckProcessing(ctx context.Context, videoID, token string) (string, error) {
	reqURL, err := url.Parse(youtubeVideosURL)
	if err != nil {
		return "", fmt.Errorf("parse videos url: %w", err)
	}

	q := reqURL.Query()
	q.Set("id", videoID)
	q.Set("part", "processingDetails,status")
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("check processing: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read check response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("check processing returned %d: %s", resp.StatusCode, parseYouTubeError(body))
	}

	var result youtubeVideoListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode check response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", fmt.Errorf("video %s not found", videoID)
	}

	item := result.Items[0]

	// If upload itself failed, report that.
	if item.Status.UploadStatus == "failed" || item.Status.UploadStatus == "rejected" {
		return "failed", nil
	}

	status := item.ProcessingDetails.ProcessingStatus
	if status == "" {
		// Processing details not yet available; still uploading.
		return "processing", nil
	}

	return status, nil
}

// --- private helpers ---

// getChannelTokens retrieves and decrypts YouTube channel credentials.
func (s *YouTubeUploadService) getChannelTokens(ctx context.Context, channelID uuid.UUID) (accessToken, refreshToken string, expiresAt *time.Time, err error) {
	var encAccess, encRefresh string
	var tokenEncrypted bool

	err = s.pool.QueryRow(ctx,
		`SELECT access_token, refresh_token, token_encrypted, token_expires_at
		 FROM channels WHERE id = $1`,
		channelID,
	).Scan(&encAccess, &encRefresh, &tokenEncrypted, &expiresAt)
	if err == pgx.ErrNoRows {
		return "", "", nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("query channel: %w", err)
	}

	if encAccess == "" {
		return "", "", nil, fmt.Errorf("channel has no access token configured")
	}

	if accessToken, err = s.crypto.Open(encAccess, tokenEncrypted); err != nil {
		return "", "", nil, fmt.Errorf("decrypt access token: %w", err)
	}
	if refreshToken, err = s.crypto.Open(encRefresh, tokenEncrypted); err != nil {
		return "", "", nil, fmt.Errorf("decrypt refresh token: %w", err)
	}

	return accessToken, refreshToken, expiresAt, nil
}

// refreshAccessToken uses the refresh token to obtain a new access token from Google,
// then updates the channel record with the new token and expiry.
func (s *YouTubeUploadService) refreshAccessToken(ctx context.Context, channelID uuid.UUID, refreshToken string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, youtubeTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("send refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp youtubeTokenRefreshResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("decode refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("refresh response missing access_token")
	}

	// Compute new expiry.
	newExpiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Re-seal with the current key. The refresh token is re-sealed too, so a
	// refresh also completes a key rotation for this channel and the
	// token_encrypted flag stays true for both columns.
	storedToken, encrypted, err := s.crypto.Seal(tokenResp.AccessToken)
	if err != nil {
		return "", fmt.Errorf("encrypt new access token: %w", err)
	}
	storedRefresh, _, err := s.crypto.Seal(refreshToken)
	if err != nil {
		return "", fmt.Errorf("encrypt refresh token: %w", err)
	}

	// Update the channel record.
	_, err = s.pool.Exec(ctx,
		`UPDATE channels SET access_token = $1, refresh_token = NULLIF($2, ''), token_encrypted = $3,
		        token_expires_at = $4, updated_at = NOW() WHERE id = $5`,
		storedToken, storedRefresh, encrypted, newExpiry, channelID)
	if err != nil {
		slog.Error("failed to persist refreshed token",
			"channel_id", channelID,
			"error", err,
		)
		// Non-fatal: we can still use the token for this upload.
	}

	slog.Info("youtube access token refreshed",
		"channel_id", channelID,
		"expires_at", newExpiry.Format(time.RFC3339),
	)

	return tokenResp.AccessToken, nil
}

// getVersionS3Info looks up the S3 storage key and file size for a video.
func (s *YouTubeUploadService) getVersionS3Info(ctx context.Context, videoID uuid.UUID) (s3Key string, fileSize int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT storage_key, file_size_bytes FROM videos WHERE id = $1 AND is_deleted = FALSE`, videoID,
	).Scan(&s3Key, &fileSize)
	if err == pgx.ErrNoRows {
		return "", 0, fmt.Errorf("video not found")
	}
	if err != nil {
		return "", 0, fmt.Errorf("query video: %w", err)
	}
	if s3Key == "" {
		return "", 0, fmt.Errorf("video has no storage key")
	}
	if fileSize <= 0 {
		return "", 0, fmt.Errorf("video has invalid file size: %d", fileSize)
	}
	return s3Key, fileSize, nil
}

// initiateResumableUpload sends the initial POST to YouTube to obtain a
// resumable upload URI.
func (s *YouTubeUploadService) initiateResumableUpload(
	ctx context.Context,
	token string,
	fileSize int64,
	body youtubeInitBody,
) (string, error) {
	reqURL, err := url.Parse(youtubeUploadURL)
	if err != nil {
		return "", fmt.Errorf("parse upload url: %w", err)
	}

	q := reqURL.Query()
	q.Set("uploadType", "resumable")
	q.Set("part", "snippet,status")
	reqURL.RawQuery = q.Encode()

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal init body: %w", err)
	}

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(initCtx, http.MethodPost, reqURL.String(), strings.NewReader(string(jsonBody)))
	if err != nil {
		return "", fmt.Errorf("create init request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/*")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(fileSize, 10))

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("send init request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("init upload returned %d: %s", resp.StatusCode, parseYouTubeError(respBody))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("init response missing Location header")
	}

	return location, nil
}

// streamUploadFromReader sends data from an already-opened reader to YouTube
// in 10 MB chunks. The caller is responsible for closing the reader on return.
// This is the core memory-efficient path: only one 10 MB buffer is held at a time.
func (s *YouTubeUploadService) streamUploadFromReader(
	ctx context.Context,
	resumableURI string,
	body io.ReadCloser,
	totalSize int64,
	publishRecordID uuid.UUID,
) (string, error) {
	defer body.Close()

	var offset int64
	buf := make([]byte, youtubeChunkSize)

	for offset < totalSize {
		// Determine chunk boundaries.
		remaining := totalSize - offset
		chunkLen := int64(youtubeChunkSize)
		if remaining < chunkLen {
			chunkLen = remaining
		}

		// Read exactly chunkLen bytes from the S3 stream.
		n, err := io.ReadFull(body, buf[:chunkLen])
		if err != nil && err != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("read s3 chunk at offset %d: %w", offset, err)
		}
		chunkLen = int64(n)
		if chunkLen == 0 {
			return "", fmt.Errorf("read 0 bytes from s3 at offset %d", offset)
		}

		endByte := offset + chunkLen - 1
		contentRange := fmt.Sprintf("bytes %d-%d/%d", offset, endByte, totalSize)

		videoID, done, err := s.uploadChunk(ctx, resumableURI, buf[:chunkLen], contentRange)
		if err != nil {
			return "", fmt.Errorf("upload chunk at offset %d: %w", offset, err)
		}

		offset += chunkLen

		// Update progress.
		pct := int((offset * 100) / totalSize)
		if pct > 99 {
			pct = 99 // Reserve 100 for the final confirmation.
		}
		_ = s.updateProgress(ctx, publishRecordID, pct)

		if done {
			return videoID, nil
		}
	}

	return "", fmt.Errorf("upload finished without receiving a completion response from YouTube")
}

// uploadChunk sends a single chunk to the resumable upload URI.
// Returns (videoID, done, error). When done is true the upload is complete
// and videoID contains the YouTube video ID.
func (s *YouTubeUploadService) uploadChunk(
	ctx context.Context,
	resumableURI string,
	data []byte,
	contentRange string,
) (string, bool, error) {
	// Use a generous per-chunk timeout: large chunks on slow connections.
	chunkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(chunkCtx, http.MethodPut, resumableURI, strings.NewReader(string(data)))
	if err != nil {
		return "", false, fmt.Errorf("create chunk request: %w", err)
	}
	req.Header.Set("Content-Type", "video/*")
	req.Header.Set("Content-Range", contentRange)
	req.ContentLength = int64(len(data))

	resp, err := s.http.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("send chunk: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		// Upload complete.
		var result youtubeUploadResponse
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", false, fmt.Errorf("decode upload response: %w", err)
		}
		if result.ID == "" {
			return "", false, fmt.Errorf("upload response missing video id")
		}
		return result.ID, true, nil

	case 308:
		// Resume Incomplete -- YouTube is asking for the next chunk.
		return "", false, nil

	default:
		return "", false, fmt.Errorf("chunk upload returned %d: %s", resp.StatusCode, parseYouTubeError(respBody))
	}
}

// updateProgress sets the upload_progress_pct on a publish record.
func (s *YouTubeUploadService) updateProgress(ctx context.Context, publishRecordID uuid.UUID, pct int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE publish_records SET upload_progress_pct = $1, updated_at = NOW() WHERE id = $2`,
		pct, publishRecordID)
	if err != nil {
		return fmt.Errorf("update progress: %w", err)
	}
	return nil
}

// normalizeVisibility maps user-facing visibility values to YouTube's
// privacyStatus enum.
func normalizeVisibility(v string) string {
	switch strings.ToLower(v) {
	case "public":
		return "public"
	case "unlisted":
		return "unlisted"
	case "private":
		return "private"
	default:
		return "private" // safe default
	}
}

// youtubeThumbnailMaxBytes is YouTube's 2 MB thumbnail limit.
const youtubeThumbnailMaxBytes = 2 * 1024 * 1024

// setThumbnail downloads an image from thumbnailURL and uploads it to YouTube
// as the custom thumbnail for the given video. If the downloaded image exceeds
// YouTube's 2 MB limit it is re-encoded as JPEG at reduced quality.
func (s *YouTubeUploadService) setThumbnail(ctx context.Context, ytVideoID, thumbnailURL, accessToken string) error {
	// 1. Download the thumbnail image (guarded, size-capped) and check its
	// declared dimensions before any full decode.
	raw, contentType, err := netguard.Fetch(ctx, s.media, thumbnailURL, maxThumbnailBytes)
	if err != nil {
		return fmt.Errorf("download thumbnail: %w", err)
	}
	if err := checkImageDimensions(raw); err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}

	uploadBody := raw
	if contentType == "" {
		contentType = "image/png"
	}

	// 2. If image exceeds YouTube's 2 MB limit, re-encode as JPEG.
	if len(raw) > youtubeThumbnailMaxBytes {
		slog.Info("thumbnail exceeds 2 MB, re-encoding as JPEG", "original_bytes", len(raw))

		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("decode thumbnail for compression: %w", err)
		}

		// Try quality levels until we fit under the limit.
		for _, q := range []int{85, 70, 55, 40} {
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
				return fmt.Errorf("jpeg encode at quality %d: %w", q, err)
			}
			if buf.Len() <= youtubeThumbnailMaxBytes {
				uploadBody = buf.Bytes()
				contentType = "image/jpeg"
				slog.Info("thumbnail compressed", "quality", q, "compressed_bytes", len(uploadBody))
				break
			}
		}
		if len(uploadBody) > youtubeThumbnailMaxBytes {
			return fmt.Errorf("thumbnail still %d bytes after compression (limit %d)", len(uploadBody), youtubeThumbnailMaxBytes)
		}
	}

	slog.Info("setThumbnail: downloaded",
		"video_id", ytVideoID,
		"download_bytes", len(raw),
		"upload_bytes", len(uploadBody),
		"content_type", contentType,
	)

	// 3. Upload to YouTube thumbnails.set endpoint.
	reqURL, err := url.Parse(youtubeThumbnailURL)
	if err != nil {
		return fmt.Errorf("parse thumbnail url: %w", err)
	}
	q := reqURL.Query()
	q.Set("videoId", ytVideoID)
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(uploadBody))
	if err != nil {
		return fmt.Errorf("create thumbnail upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("upload thumbnail to youtube: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("youtube thumbnails.set returned %d: %s", resp.StatusCode, parseYouTubeError(respBody))
	}

	slog.Info("setThumbnail: youtube response",
		"video_id", ytVideoID,
		"status", resp.StatusCode,
		"response", string(respBody[:min(len(respBody), 500)]),
	)

	return nil
}

// RetryThumbnail sets a custom thumbnail on an already-published YouTube video.
// It generates a fresh presigned URL for the thumbnail and refreshes the OAuth
// token if needed.
func (s *YouTubeUploadService) RetryThumbnail(ctx context.Context, channelID uuid.UUID, ytVideoID, thumbnailS3Key string) error {
	// 1. Get channel credentials.
	accessToken, refreshToken, tokenExpiresAt, err := s.getChannelTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get channel tokens: %w", err)
	}

	// 2. Refresh if token is expired or about to expire.
	if tokenExpiresAt != nil && time.Until(*tokenExpiresAt) < 5*time.Minute {
		if refreshToken == "" {
			return fmt.Errorf("access token expired and no refresh token available")
		}
		newToken, err := s.refreshAccessToken(ctx, channelID, refreshToken)
		if err != nil {
			return fmt.Errorf("refresh youtube token: %w", err)
		}
		accessToken = newToken
	}

	// 3. Generate a fresh presigned URL for the thumbnail.
	freshURL, err := s.s3.PresignGetObject(ctx, thumbnailS3Key, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("presign thumbnail: %w", err)
	}

	slog.Info("retry thumbnail: presigned url generated",
		"video_id", ytVideoID,
		"s3_key", thumbnailS3Key,
		"presigned_url_prefix", freshURL[:min(len(freshURL), 120)],
	)

	// 4. Set the thumbnail.
	if err := s.setThumbnail(ctx, ytVideoID, freshURL, accessToken); err != nil {
		return err
	}
	slog.Info("retry thumbnail: youtube accepted", "video_id", ytVideoID)
	return nil
}

// parseYouTubeError extracts a human-readable message from a YouTube API error body.
func parseYouTubeError(body []byte) string {
	var e youtubeErrorResponse
	if err := json.Unmarshal(body, &e); err == nil && e.Error.Message != "" {
		reason := ""
		if len(e.Error.Errors) > 0 {
			reason = e.Error.Errors[0].Reason
		}
		if reason != "" {
			return fmt.Sprintf("%s (reason: %s, code: %d)", e.Error.Message, reason, e.Error.Code)
		}
		return fmt.Sprintf("%s (code: %d)", e.Error.Message, e.Error.Code)
	}
	if len(body) > 512 {
		return string(body[:512]) + "..."
	}
	return string(body)
}
