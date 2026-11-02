package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
)

const (
	twitterAPIBase = "https://api.x.com/2"

	// Each APPEND chunk must be at most 1 MB (Twitter v2 requirement).
	twitterChunkSize = 1 * 1024 * 1024

	// Maximum video file size accepted by Twitter (512 MB).
	twitterMaxFileSize = 512 * 1024 * 1024

	// Processing poll limits.
	twitterMaxPollAttempts  = 120
	twitterDefaultPollDelay = 5 * time.Second
	twitterMaxPollDelay     = 30 * time.Second
)

// TwitterUploadService handles the full Twitter/X video upload and tweet
// creation flow using the v2 chunked media upload API.
type TwitterUploadService struct {
	pool   *pgxpool.Pool
	s3     *infra.S3Client
	crypto *TokenCrypto
	http   *http.Client
}

// TwitterUploadMeta contains metadata for the tweet to be created.
type TwitterUploadMeta struct {
	TweetText string
}

// twitterInitResponse is the JSON shape returned by the INITIALIZE endpoint.
type twitterInitResponse struct {
	ID              string `json:"id"`
	ExpiresAfterSec int    `json:"expires_after_secs"`
}

// twitterFinalizeResponse is the JSON shape returned by the FINALIZE endpoint.
type twitterFinalizeResponse struct {
	ID             string                 `json:"id"`
	ProcessingInfo *twitterProcessingInfo `json:"processing_info,omitempty"`
}

// twitterProcessingInfo describes the async processing state of a media upload.
type twitterProcessingInfo struct {
	State          string `json:"state"`
	CheckAfterSecs int    `json:"check_after_secs"`
	ProgressPct    int    `json:"progress_percent"`
	Error          *struct {
		Code    int    `json:"code"`
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// twitterStatusResponse is the JSON shape returned by the media status endpoint.
type twitterStatusResponse struct {
	ID             string                 `json:"id"`
	ProcessingInfo *twitterProcessingInfo `json:"processing_info,omitempty"`
}

// twitterTweetResponse is the JSON shape returned by the tweet creation endpoint.
type twitterTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// twitterErrorResponse captures error detail from the Twitter API.
type twitterErrorResponse struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Status int    `json:"status"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func NewTwitterUploadService(pool *pgxpool.Pool, s3 *infra.S3Client, crypto *TokenCrypto) *TwitterUploadService {
	return &TwitterUploadService{
		pool:   pool,
		s3:     s3,
		crypto: crypto,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Upload performs the complete Twitter video publish flow:
//  1. Fetch channel token and video storage key from the database
//  2. Download video from S3 to a temp file (memory-safe for 256 MB workers)
//  3. INITIALIZE chunked media upload
//  4. APPEND 1 MB chunks with base64-encoded multipart bodies
//  5. FINALIZE the media upload
//  6. Poll processing until succeeded
//  7. Create the tweet with the uploaded media
//
// Returns the tweet ID on success.
func (s *TwitterUploadService) Upload(ctx context.Context, channelID, versionID, publishRecordID uuid.UUID, meta TwitterUploadMeta) (string, error) {
	// --- Fetch the OAuth bearer token for this channel -----------------------
	token, err := s.getChannelToken(ctx, channelID)
	if err != nil {
		s.recordError(ctx, publishRecordID, "TOKEN_REFRESH_FAILED", err)
		return "", fmt.Errorf("get channel token: %w", err)
	}

	// --- Resolve the S3 storage key for the video --------------
	var storageKey string
	err = s.pool.QueryRow(ctx,
		`SELECT storage_key FROM videos WHERE id = $1`, versionID,
	).Scan(&storageKey)
	if err != nil {
		s.recordError(ctx, publishRecordID, "S3_DOWNLOAD_FAILED", err)
		return "", fmt.Errorf("get video storage key: %w", err)
	}

	// --- Download from S3 to a temp file (256 MB RAM constraint) -------------
	s.updateProgress(ctx, publishRecordID, 0, "downloading")

	tmpFile, err := os.CreateTemp("", "twitter-upload-*.mp4")
	if err != nil {
		s.recordError(ctx, publishRecordID, "S3_DOWNLOAD_FAILED", err)
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	reader, fileSize, err := s.s3.GetObjectStream(ctx, storageKey)
	if err != nil {
		s.recordError(ctx, publishRecordID, "S3_DOWNLOAD_FAILED", err)
		return "", fmt.Errorf("get object stream: %w", err)
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		reader.Close()
		s.recordError(ctx, publishRecordID, "S3_DOWNLOAD_FAILED", err)
		return "", fmt.Errorf("download to temp file: %w", err)
	}
	reader.Close()

	// Use the actual bytes written if S3 did not report content-length.
	if fileSize <= 0 {
		info, err := tmpFile.Stat()
		if err != nil {
			return "", fmt.Errorf("stat temp file: %w", err)
		}
		fileSize = info.Size()
	}

	if fileSize > twitterMaxFileSize {
		s.recordError(ctx, publishRecordID, "FILE_TOO_LARGE",
			fmt.Errorf("file size %d exceeds twitter limit %d", fileSize, twitterMaxFileSize))
		return "", fmt.Errorf("file size %d exceeds twitter limit of %d bytes", fileSize, twitterMaxFileSize)
	}

	// Seek back to the beginning for chunked reads.
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("seek temp file: %w", err)
	}

	s.updateProgress(ctx, publishRecordID, 5, "uploading")

	// --- STEP 1: INITIALIZE --------------------------------------------------
	mediaID, err := s.initializeUpload(ctx, token, fileSize)
	if err != nil {
		s.recordError(ctx, publishRecordID, "TWEET_CREATION_FAILED", err)
		return "", fmt.Errorf("twitter initialize: %w", err)
	}

	// Store the media ID as the upload session for potential recovery.
	_, _ = s.pool.Exec(ctx,
		`UPDATE publish_records SET upload_session_id = $1, upload_started_at = NOW() WHERE id = $2`,
		mediaID, publishRecordID)

	// --- STEP 2: APPEND chunks -----------------------------------------------
	totalChunks := int((fileSize + int64(twitterChunkSize) - 1) / int64(twitterChunkSize))
	if totalChunks < 1 {
		totalChunks = 1
	}

	buf := make([]byte, twitterChunkSize)
	for segmentIndex := 0; ; segmentIndex++ {
		n, readErr := tmpFile.Read(buf)
		if n == 0 {
			break
		}

		if err := s.appendChunk(ctx, token, mediaID, segmentIndex, buf[:n]); err != nil {
			s.recordError(ctx, publishRecordID, "TWEET_CREATION_FAILED", err)
			return "", fmt.Errorf("twitter append segment %d: %w", segmentIndex, err)
		}

		// Progress: 5-70% across all chunk uploads.
		pct := 5 + int(float64(segmentIndex+1)/float64(totalChunks)*65)
		if pct > 70 {
			pct = 70
		}
		s.updateProgress(ctx, publishRecordID, pct, "uploading")

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			s.recordError(ctx, publishRecordID, "TWEET_CREATION_FAILED", readErr)
			return "", fmt.Errorf("read temp file at segment %d: %w", segmentIndex, readErr)
		}
	}

	// --- STEP 3: FINALIZE ----------------------------------------------------
	s.updateProgress(ctx, publishRecordID, 72, "processing")

	finalResp, err := s.finalizeUpload(ctx, token, mediaID)
	if err != nil {
		s.recordError(ctx, publishRecordID, "MEDIA_PROCESSING_FAILED", err)
		return "", fmt.Errorf("twitter finalize: %w", err)
	}

	// --- STEP 4: Poll processing ---------------------------------------------
	if finalResp.ProcessingInfo != nil && finalResp.ProcessingInfo.State != "succeeded" {
		_, _ = s.pool.Exec(ctx,
			`UPDATE publish_records SET processing_started_at = NOW() WHERE id = $1`,
			publishRecordID)

		if err := s.pollProcessing(ctx, token, mediaID, publishRecordID); err != nil {
			s.recordError(ctx, publishRecordID, "MEDIA_PROCESSING_FAILED", err)
			return "", fmt.Errorf("twitter processing: %w", err)
		}
	}

	s.updateProgress(ctx, publishRecordID, 90, "publishing")

	// --- STEP 5: Create tweet ------------------------------------------------
	tweetID, err := s.createTweet(ctx, token, mediaID, meta.TweetText)
	if err != nil {
		s.recordError(ctx, publishRecordID, "TWEET_CREATION_FAILED", err)
		return "", fmt.Errorf("twitter create tweet: %w", err)
	}

	// Mark as published.
	tweetURL := fmt.Sprintf("https://x.com/i/status/%s", tweetID)
	now := time.Now()
	_, _ = s.pool.Exec(ctx,
		`UPDATE publish_records
		 SET status = 'published',
		     platform_content_id = $1,
		     platform_url = $2,
		     upload_progress_pct = 100,
		     upload_completed_at = $3,
		     published_at = $3,
		     error_message = NULL,
		     updated_at = NOW()
		 WHERE id = $4`,
		tweetID, tweetURL, now, publishRecordID)

	return tweetID, nil
}

// ---------------------------------------------------------------------------
// Internal: Twitter v2 API calls
// ---------------------------------------------------------------------------

// getChannelToken reads and decrypts the OAuth token for the given channel.
func (s *TwitterUploadService) getChannelToken(ctx context.Context, channelID uuid.UUID) (string, error) {
	var accessToken string
	var tokenEncrypted bool
	err := s.pool.QueryRow(ctx,
		`SELECT access_token, token_encrypted FROM channels WHERE id = $1`, channelID,
	).Scan(&accessToken, &tokenEncrypted)
	if err != nil {
		return "", fmt.Errorf("query channel token: %w", err)
	}
	if accessToken == "" {
		return "", fmt.Errorf("channel %s has no access token configured", channelID)
	}

	if tokenEncrypted {
		decrypted, err := s.crypto.Decrypt(accessToken)
		if err != nil {
			return "", fmt.Errorf("decrypt token: %w", err)
		}
		return decrypted, nil
	}
	return accessToken, nil
}

// initializeUpload calls the Twitter v2 media INITIALIZE endpoint.
func (s *TwitterUploadService) initializeUpload(ctx context.Context, token string, totalBytes int64) (string, error) {
	body := map[string]interface{}{
		"media_type":      "video/mp4",
		"media_category":  "tweet_video",
		"total_file_size": totalBytes,
	}

	var result twitterInitResponse
	if err := s.doJSON(ctx, http.MethodPost, twitterAPIBase+"/media/upload/initialize", token, body, &result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("initialize returned empty media id")
	}
	return result.ID, nil
}

// appendChunk sends one base64-encoded chunk via the APPEND endpoint as
// multipart/form-data.
func (s *TwitterUploadService) appendChunk(ctx context.Context, token, mediaID string, segmentIndex int, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("media_data", encoded); err != nil {
		return fmt.Errorf("write media_data field: %w", err)
	}
	if err := writer.WriteField("segment_index", fmt.Sprintf("%d", segmentIndex)); err != nil {
		return fmt.Errorf("write segment_index field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	url := fmt.Sprintf("%s/media/upload/%s/append", twitterAPIBase, mediaID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("create append request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("append request: %w", err)
	}
	defer resp.Body.Close()

	// Twitter APPEND returns 204 No Content on success.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}

	return s.parseAPIError(resp, "append")
}

// finalizeUpload calls the FINALIZE endpoint.
func (s *TwitterUploadService) finalizeUpload(ctx context.Context, token, mediaID string) (*twitterFinalizeResponse, error) {
	url := fmt.Sprintf("%s/media/upload/%s/finalize", twitterAPIBase, mediaID)

	var result twitterFinalizeResponse
	if err := s.doJSON(ctx, http.MethodPost, url, token, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// getMediaStatus calls the GET status endpoint for a media upload.
func (s *TwitterUploadService) getMediaStatus(ctx context.Context, token, mediaID string) (*twitterStatusResponse, error) {
	url := fmt.Sprintf("%s/media/upload/%s", twitterAPIBase, mediaID)

	var result twitterStatusResponse
	if err := s.doJSON(ctx, http.MethodGet, url, token, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// pollProcessing repeatedly checks the media processing status until it
// reaches "succeeded" or "failed", or the attempt limit is exceeded.
func (s *TwitterUploadService) pollProcessing(ctx context.Context, token, mediaID string, publishRecordID uuid.UUID) error {
	for attempt := 0; attempt < twitterMaxPollAttempts; attempt++ {
		resp, err := s.getMediaStatus(ctx, token, mediaID)
		if err != nil {
			return fmt.Errorf("poll attempt %d: %w", attempt, err)
		}

		if resp.ProcessingInfo == nil {
			// No processing info means processing is complete.
			return nil
		}

		switch resp.ProcessingInfo.State {
		case "succeeded":
			return nil

		case "failed":
			errMsg := "unknown processing failure"
			if resp.ProcessingInfo.Error != nil {
				errMsg = fmt.Sprintf("%s: %s", resp.ProcessingInfo.Error.Name, resp.ProcessingInfo.Error.Message)
			}
			return fmt.Errorf("media processing failed: %s", errMsg)

		case "pending", "in_progress":
			// Update progress: 72-88% during processing phase.
			if resp.ProcessingInfo.ProgressPct > 0 {
				pct := 72 + int(float64(resp.ProcessingInfo.ProgressPct)/100.0*16)
				if pct > 88 {
					pct = 88
				}
				s.updateProgress(ctx, publishRecordID, pct, "processing")
			}

			delay := time.Duration(resp.ProcessingInfo.CheckAfterSecs) * time.Second
			if delay <= 0 {
				delay = twitterDefaultPollDelay
			}
			if delay > twitterMaxPollDelay {
				delay = twitterMaxPollDelay
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// continue polling
			}

		default:
			return fmt.Errorf("unexpected processing state: %s", resp.ProcessingInfo.State)
		}
	}

	return fmt.Errorf("media processing timed out after %d poll attempts", twitterMaxPollAttempts)
}

// createTweet creates a tweet with the uploaded media attached.
func (s *TwitterUploadService) createTweet(ctx context.Context, token, mediaID, text string) (string, error) {
	body := map[string]interface{}{
		"text": text,
		"media": map[string]interface{}{
			"media_ids": []string{mediaID},
		},
	}

	var result twitterTweetResponse
	if err := s.doJSON(ctx, http.MethodPost, twitterAPIBase+"/tweets", token, body, &result); err != nil {
		return "", err
	}
	if result.Data.ID == "" {
		return "", fmt.Errorf("tweet creation returned empty id")
	}
	return result.Data.ID, nil
}

// ---------------------------------------------------------------------------
// Internal: HTTP and DB helpers
// ---------------------------------------------------------------------------

// doJSON performs a JSON API request with Bearer auth. If reqBody is nil,
// no request body is sent (suitable for GET requests). The response is
// decoded into dest.
func (s *TwitterUploadService) doJSON(ctx context.Context, method, url, token string, reqBody interface{}, dest interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.parseAPIError(resp, method+" "+url)
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode response from %s: %w", url, err)
		}
	}
	return nil
}

// parseAPIError reads an error response body and returns a structured error.
func (s *TwitterUploadService) parseAPIError(resp *http.Response, context string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var apiErr twitterErrorResponse
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Detail != "" {
		return fmt.Errorf("twitter API error (%s) %d: %s - %s",
			context, resp.StatusCode, apiErr.Title, apiErr.Detail)
	}

	return fmt.Errorf("twitter API error (%s) %d: %s",
		context, resp.StatusCode, string(body))
}

// updateProgress updates the upload progress percentage and status on the
// publish record. Errors are intentionally swallowed to avoid failing the
// upload over a progress tracking issue.
func (s *TwitterUploadService) updateProgress(ctx context.Context, publishRecordID uuid.UUID, pct int, status string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE publish_records SET upload_progress_pct = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		pct, status, publishRecordID)
}

// recordError stores an error on the publish record and maps it to an
// admin-friendly message via the platform error map.
func (s *TwitterUploadService) recordError(ctx context.Context, publishRecordID uuid.UUID, errorCode string, originalErr error) {
	friendlyMsg := MapPlatformError(errorCode)
	_, _ = s.pool.Exec(ctx,
		`UPDATE publish_records
		 SET status = 'failed',
		     error_message = $1,
		     platform_metadata = platform_metadata || $2::jsonb,
		     updated_at = NOW()
		 WHERE id = $3`,
		friendlyMsg,
		fmt.Sprintf(`{"error_code":"%s","technical_error":"%s"}`, errorCode, escapeJSONString(originalErr.Error())),
		publishRecordID)
}

// escapeJSONString performs minimal escaping so the technical error can be
// embedded safely in a JSON string literal written via Sprintf.
func escapeJSONString(s string) string {
	var buf bytes.Buffer
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
