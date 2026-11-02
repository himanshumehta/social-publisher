package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/netguard"
)

const (
	fbGraphBaseURL  = "https://graph.facebook.com"
	fbUploadBaseURL = "https://rupload.facebook.com"
	fbAPIVersion    = "v21.0"

	// fbChunkSize is the maximum bytes we hold in memory at once for chunked
	// feed-video uploads. 4 MB keeps peak RSS well under the 256 MB worker limit
	// even with HTTP buffers and runtime overhead.
	fbChunkSize = 4 * 1024 * 1024

	// fbPresignExpiry is the lifetime of the presigned S3 URL handed to Facebook
	// for reel URL-fetch uploads. One hour is generous enough for Facebook to
	// begin the download yet short enough to limit exposure.
	fbPresignExpiry = 1 * time.Hour
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// FacebookUploadService implements both the resumable-chunk feed-video flow
// and the URL-fetch reel flow against the Facebook Graph API.
type FacebookUploadService struct {
	pool   *pgxpool.Pool
	s3     *infra.S3Client
	crypto *TokenCrypto
	http   *http.Client
	media  *http.Client // thumbnail URLs (SSRF-guarded)
}

// FacebookUploadMeta carries the caller-supplied metadata for a publish.
type FacebookUploadMeta struct {
	Title        string
	Description  string
	IsReel       bool
	ThumbnailURL string // presigned S3 URL for cover image
}

// NewFacebookUploadService constructs the service. The http.Client should
// have a reasonable timeout configured by the caller (e.g. 30 s for API
// calls; chunk uploads set per-request deadlines via context).
func NewFacebookUploadService(pool *pgxpool.Pool, s3 *infra.S3Client, crypto *TokenCrypto) *FacebookUploadService {
	return &FacebookUploadService{
		pool:   pool,
		s3:     s3,
		crypto: crypto,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
		media: newMediaClient(false),
	}
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

// Upload routes to the correct flow based on meta.IsReel and returns the
// Facebook video ID on success.
func (s *FacebookUploadService) Upload(
	ctx context.Context,
	channelID, versionID, publishRecordID uuid.UUID,
	meta FacebookUploadMeta,
) (string, error) {
	if meta.IsReel {
		return s.UploadReel(ctx, channelID, versionID, publishRecordID, meta)
	}
	return s.UploadFeedVideo(ctx, channelID, versionID, publishRecordID, meta)
}

// ---------------------------------------------------------------------------
// Feed Video  (resumable chunked upload)
// ---------------------------------------------------------------------------

// UploadFeedVideo implements the three-phase resumable chunked upload:
//
//  1. Start  -- obtain upload_session_id and video_id.
//  2. Transfer -- stream the file from S3 in fbChunkSize pieces, posting
//     each as a multipart chunk. Repeat until Facebook says done.
//  3. Finish -- attach title/description and confirm publish.
//
// The S3 body is consumed as a stream (GetObjectStream) so at most one chunk
// lives in memory at a time.
func (s *FacebookUploadService) UploadFeedVideo(
	ctx context.Context,
	channelID, versionID, publishRecordID uuid.UUID,
	meta FacebookUploadMeta,
) (string, error) {
	// ---- resolve credentials & file metadata ---------------------------
	pageID, token, err := s.resolveChannel(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("facebook feed: resolve channel: %w", err)
	}

	s3Key, fileSize, err := s.resolveVersion(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("facebook feed: resolve version: %w", err)
	}

	// ---- phase 1: start ------------------------------------------------
	sessionID, videoID, err := s.feedStart(ctx, pageID, token, fileSize)
	if err != nil {
		return "", fmt.Errorf("facebook feed: start: %w", err)
	}

	s.setProgress(ctx, publishRecordID, 5)

	// ---- phase 2: transfer chunks from S3 stream -----------------------
	body, bodySize, err := s.s3.GetObjectStream(ctx, s3Key)
	if err != nil {
		return "", fmt.Errorf("facebook feed: s3 stream: %w", err)
	}
	defer body.Close()

	// Prefer the DB-recorded size; fall back to the S3 Content-Length.
	totalBytes := fileSize
	if totalBytes <= 0 {
		totalBytes = bodySize
	}

	startOffset := int64(0)
	for {
		chunkLen := fbChunkSize
		remaining := totalBytes - startOffset
		if remaining <= 0 {
			break
		}
		if int64(chunkLen) > remaining {
			chunkLen = int(remaining)
		}

		chunk := make([]byte, chunkLen)
		n, readErr := io.ReadFull(body, chunk)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return "", fmt.Errorf("facebook feed: read chunk at offset %d: %w", startOffset, readErr)
		}
		chunk = chunk[:n]

		nextOffset, endOffset, err := s.feedTransfer(ctx, pageID, token, sessionID, startOffset, chunk)
		if err != nil {
			return "", fmt.Errorf("facebook feed: transfer chunk offset %d: %w", startOffset, err)
		}

		startOffset = nextOffset

		// Update progress (5-90% range allocated to transfer).
		if totalBytes > 0 {
			pct := 5 + int(float64(startOffset)/float64(totalBytes)*85)
			if pct > 90 {
				pct = 90
			}
			s.setProgress(ctx, publishRecordID, pct)
		}

		if nextOffset >= endOffset {
			break
		}
	}

	// ---- phase 3: finish -----------------------------------------------
	if err := s.feedFinish(ctx, pageID, token, sessionID, meta); err != nil {
		return "", fmt.Errorf("facebook feed: finish: %w", err)
	}

	// ---- phase 4: set thumbnail (non-fatal) ----------------------------
	if meta.ThumbnailURL != "" {
		if err := s.setThumbnail(ctx, videoID, token, meta.ThumbnailURL); err != nil {
			slog.Error("facebook thumbnail failed (upload succeeded)", "video_id", videoID, "error", err)
		} else {
			slog.Info("facebook thumbnail set", "video_id", videoID)
		}
	}

	s.setProgress(ctx, publishRecordID, 100)
	return videoID, nil
}

// ---------------------------------------------------------------------------
// Reel Upload  (URL-fetch)
// ---------------------------------------------------------------------------

// UploadReel implements the three-phase URL-fetch reel upload:
//
//  1. Initialize -- obtain a video_id.
//  2. Upload via URL -- hand Facebook a presigned S3 GET URL.
//  3. Finish -- attach metadata and confirm.
func (s *FacebookUploadService) UploadReel(
	ctx context.Context,
	channelID, versionID, publishRecordID uuid.UUID,
	meta FacebookUploadMeta,
) (string, error) {
	pageID, token, err := s.resolveChannel(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("facebook reel: resolve channel: %w", err)
	}

	s3Key, _, err := s.resolveVersion(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("facebook reel: resolve version: %w", err)
	}

	// ---- phase 1: initialize -------------------------------------------
	videoID, err := s.reelInit(ctx, pageID, token)
	if err != nil {
		return "", fmt.Errorf("facebook reel: init: %w", err)
	}

	s.setProgress(ctx, publishRecordID, 10)

	// ---- phase 2: upload via presigned URL -----------------------------
	presignedURL, err := s.s3.PresignGetObject(ctx, s3Key, fbPresignExpiry)
	if err != nil {
		return "", fmt.Errorf("facebook reel: presign s3 url: %w", err)
	}

	if err := s.reelUploadURL(ctx, videoID, token, presignedURL); err != nil {
		return "", fmt.Errorf("facebook reel: upload url: %w", err)
	}

	s.setProgress(ctx, publishRecordID, 60)

	// ---- phase 3: finish -----------------------------------------------
	if err := s.reelFinish(ctx, pageID, token, videoID, meta); err != nil {
		return "", fmt.Errorf("facebook reel: finish: %w", err)
	}

	// Facebook Reels thumbnails are auto-generated from video frames.
	// The /{video-id}/thumbnails API is silently ignored for Reels.

	s.setProgress(ctx, publishRecordID, 100)
	return videoID, nil
}

// ---------------------------------------------------------------------------
// Processing status
// ---------------------------------------------------------------------------

// CheckProcessing polls the /video endpoint to determine transcode status.
// Returns one of: "processing", "ready", "error", or the raw status string.
func (s *FacebookUploadService) CheckProcessing(ctx context.Context, videoID, token string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=status&access_token=%s",
		fbGraphBaseURL, fbAPIVersion, videoID, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("facebook check processing: build request: %w", err)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("facebook check processing: request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status struct {
			VideoStatus string `json:"video_status"`
		} `json:"status"`
		Error *fbAPIError `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("facebook check processing: decode: %w", err)
	}
	if result.Error != nil {
		return "", result.Error
	}

	return result.Status.VideoStatus, nil
}

// ---------------------------------------------------------------------------
// Internal: Feed video phases
// ---------------------------------------------------------------------------

type fbStartResponse struct {
	UploadSessionID string `json:"upload_session_id"`
	VideoID         string `json:"video_id"`
}

func (s *FacebookUploadService) feedStart(ctx context.Context, pageID, token string, fileSize int64) (sessionID, videoID string, err error) {
	url := fmt.Sprintf("%s/%s/%s/videos", fbGraphBaseURL, fbAPIVersion, pageID)

	form := fmt.Sprintf("upload_phase=start&file_size=%d&access_token=%s", fileSize, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(form))
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result fbStartResponse
	if err := s.decodeOrError(resp, &result); err != nil {
		return "", "", err
	}

	if result.UploadSessionID == "" || result.VideoID == "" {
		return "", "", fmt.Errorf("empty session_id or video_id in start response")
	}
	return result.UploadSessionID, result.VideoID, nil
}

type fbTransferResponse struct {
	StartOffset string `json:"start_offset"`
	EndOffset   string `json:"end_offset"`
}

func (s *FacebookUploadService) feedTransfer(
	ctx context.Context,
	pageID, token, sessionID string,
	startOffset int64,
	chunk []byte,
) (nextOffset, endOffset int64, err error) {
	url := fmt.Sprintf("%s/%s/%s/videos", fbGraphBaseURL, fbAPIVersion, pageID)

	// Build multipart body. We write into a buffer because we need to know
	// the Content-Length for the POST and the chunk is already in memory.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("upload_phase", "transfer")
	_ = w.WriteField("upload_session_id", sessionID)
	_ = w.WriteField("start_offset", strconv.FormatInt(startOffset, 10))
	_ = w.WriteField("access_token", token)

	part, err := w.CreateFormFile("video_file_chunk", "chunk.dat")
	if err != nil {
		return 0, 0, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(chunk); err != nil {
		return 0, 0, fmt.Errorf("write chunk: %w", err)
	}
	if err := w.Close(); err != nil {
		return 0, 0, fmt.Errorf("close multipart: %w", err)
	}

	// Allow extra time for large chunk uploads.
	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(uploadCtx, http.MethodPost, url, &buf)
	if err != nil {
		return 0, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.http.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result fbTransferResponse
	if err := s.decodeOrError(resp, &result); err != nil {
		return 0, 0, err
	}

	nextOffset, err = strconv.ParseInt(result.StartOffset, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse start_offset %q: %w", result.StartOffset, err)
	}
	endOffset, err = strconv.ParseInt(result.EndOffset, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse end_offset %q: %w", result.EndOffset, err)
	}
	return nextOffset, endOffset, nil
}

func (s *FacebookUploadService) feedFinish(ctx context.Context, pageID, token, sessionID string, meta FacebookUploadMeta) error {
	url := fmt.Sprintf("%s/%s/%s/videos", fbGraphBaseURL, fbAPIVersion, pageID)

	form := fmt.Sprintf(
		"upload_phase=finish&upload_session_id=%s&title=%s&description=%s&access_token=%s",
		sessionID, urlEncode(meta.Title), urlEncode(meta.Description), token,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(form))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool        `json:"success"`
		Error   *fbAPIError `json:"error"`
	}
	if err := s.decodeOrError(resp, &result); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal: Reel phases
// ---------------------------------------------------------------------------

func (s *FacebookUploadService) reelInit(ctx context.Context, pageID, token string) (videoID string, err error) {
	url := fmt.Sprintf("%s/%s/%s/video_reels", fbGraphBaseURL, fbAPIVersion, pageID)

	form := fmt.Sprintf("upload_phase=start&access_token=%s", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(form))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		VideoID string      `json:"video_id"`
		Error   *fbAPIError `json:"error"`
	}
	if err := s.decodeOrError(resp, &result); err != nil {
		return "", err
	}
	if result.VideoID == "" {
		return "", fmt.Errorf("empty video_id in reel init response")
	}
	return result.VideoID, nil
}

func (s *FacebookUploadService) reelUploadURL(ctx context.Context, videoID, token, fileURL string) error {
	url := fmt.Sprintf("%s/video-upload/%s/%s", fbUploadBaseURL, fbAPIVersion, videoID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("file_url", fileURL)

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	// Facebook returns {"success": true} or an error object.
	var result struct {
		Success bool        `json:"success"`
		Error   *fbAPIError `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// Some FB endpoints return empty 200; treat as success.
		return nil
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func (s *FacebookUploadService) reelFinish(ctx context.Context, pageID, token, videoID string, meta FacebookUploadMeta) error {
	url := fmt.Sprintf("%s/%s/%s/video_reels", fbGraphBaseURL, fbAPIVersion, pageID)

	form := fmt.Sprintf(
		"upload_phase=finish&video_id=%s&title=%s&description=%s&access_token=%s",
		videoID, urlEncode(meta.Title), urlEncode(meta.Description), token,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(form))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool        `json:"success"`
		Error   *fbAPIError `json:"error"`
	}
	if err := s.decodeOrError(resp, &result); err != nil {
		return err
	}
	return nil
}

// setThumbnail downloads the image from thumbnailURL and uploads it to
// Facebook's video thumbnails endpoint via multipart POST.
func (s *FacebookUploadService) setThumbnail(ctx context.Context, videoID, token, thumbnailURL string) error {
	thumbData, _, err := netguard.Fetch(ctx, s.media, thumbnailURL, maxThumbnailBytes)
	if err != nil {
		return fmt.Errorf("download thumbnail: %w", err)
	}

	// Build multipart form with the thumbnail file.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("access_token", token)
	part, err := w.CreateFormFile("source", "thumbnail.jpg")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(thumbData); err != nil {
		return fmt.Errorf("write thumbnail data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close multipart: %w", err)
	}

	// POST to /{video-id}/thumbnails
	url := fmt.Sprintf("%s/%s/%s/thumbnails", fbGraphBaseURL, fbAPIVersion, videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("create thumbnail request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("post thumbnail: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Success bool        `json:"success"`
		Error   *fbAPIError `json:"error"`
	}
	if err := s.decodeOrError(resp, &result); err != nil {
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal: shared helpers
// ---------------------------------------------------------------------------

// channelMeta is the JSONB structure stored in channels.metadata for Facebook.
type channelMeta struct {
	PageID string `json:"page_id"`
}

// resolveChannel reads the channel row, decrypts the token, and extracts the
// page_id from the metadata JSONB.
func (s *FacebookUploadService) resolveChannel(ctx context.Context, channelID uuid.UUID) (pageID, token string, err error) {
	var (
		accessToken    string
		tokenEncrypted bool
		metadataRaw    []byte
	)

	err = s.pool.QueryRow(ctx,
		`SELECT access_token, token_encrypted, metadata
		 FROM channels WHERE id = $1 AND is_active = TRUE`,
		channelID,
	).Scan(&accessToken, &tokenEncrypted, &metadataRaw)
	if err != nil {
		return "", "", fmt.Errorf("query channel %s: %w", channelID, err)
	}

	// Decrypt if the token was stored encrypted.
	accessToken, err = s.crypto.Open(accessToken, tokenEncrypted)
	if err != nil {
		return "", "", fmt.Errorf("decrypt token for channel %s: %w", channelID, err)
	}
	if accessToken == "" {
		return "", "", fmt.Errorf("channel %s has no access token", channelID)
	}

	// Extract page_id from metadata JSONB.
	var meta channelMeta
	if err := json.Unmarshal(metadataRaw, &meta); err != nil {
		return "", "", fmt.Errorf("parse channel metadata for %s: %w", channelID, err)
	}
	if meta.PageID == "" {
		return "", "", fmt.Errorf("channel %s metadata missing page_id", channelID)
	}

	return meta.PageID, accessToken, nil
}

// resolveVersion looks up the video for the S3 key and recorded file size.
func (s *FacebookUploadService) resolveVersion(ctx context.Context, videoID uuid.UUID) (s3Key string, fileSize int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT storage_key, file_size_bytes FROM videos WHERE id = $1 AND is_deleted = FALSE`,
		videoID,
	).Scan(&s3Key, &fileSize)
	if err != nil {
		return "", 0, fmt.Errorf("query video %s: %w", videoID, err)
	}
	if s3Key == "" {
		return "", 0, fmt.Errorf("video %s has no storage key", videoID)
	}
	return s3Key, fileSize, nil
}

// setProgress updates the upload_progress_pct column on the publish_record.
// Errors are logged but not propagated because a progress-tracking failure
// must not abort an otherwise successful upload.
func (s *FacebookUploadService) setProgress(ctx context.Context, publishRecordID uuid.UUID, pct int) {
	_, err := s.pool.Exec(ctx,
		`UPDATE publish_records SET upload_progress_pct = $1 WHERE id = $2`,
		pct, publishRecordID,
	)
	if err != nil {
		slog.Warn("facebook: update progress", "pct", pct, "publish_record_id", publishRecordID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Facebook API error type
// ---------------------------------------------------------------------------

// fbAPIError represents the standard Graph API error envelope.
type fbAPIError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	FBTraceID string `json:"fbtrace_id"`
}

func (e *fbAPIError) Error() string {
	return fmt.Sprintf("facebook api error %d (%s): %s [trace: %s]", e.Code, e.Type, e.Message, e.FBTraceID)
}

// decodeOrError reads the response body and decodes into dst. If the HTTP
// status is not 2xx or the body contains an error object, it returns a
// descriptive error.
func (s *FacebookUploadService) decodeOrError(resp *http.Response, dst interface{}) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1 MB
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	// Try to detect an error envelope regardless of status code.
	var envelope struct {
		Error *fbAPIError `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	if envelope.Error != nil {
		return envelope.Error
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected http %d: %s", resp.StatusCode, truncate(string(body), 512))
	}

	if dst != nil {
		if err := json.Unmarshal(body, dst); err != nil {
			return fmt.Errorf("decode response: %w (body: %s)", err, truncate(string(body), 256))
		}
	}
	return nil
}

// urlEncode applies percent-encoding suitable for form values.
func urlEncode(s string) string {
	// net/url.QueryEscape encodes spaces as + which is correct for
	// application/x-www-form-urlencoded.
	var buf bytes.Buffer
	for _, b := range []byte(s) {
		if isUnreserved(b) {
			buf.WriteByte(b)
		} else {
			fmt.Fprintf(&buf, "%%%02X", b)
		}
	}
	return buf.String()
}

func isUnreserved(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') ||
		(b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~'
}

// truncate is defined in channel_oauth.go and shared across the package.
