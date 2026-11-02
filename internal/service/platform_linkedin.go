package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
)

const (
	linkedInAPIBase    = "https://api.linkedin.com/rest"
	linkedInAPIVersion = "202601"
	linkedInRestLiVer  = "2.0.0"

	// LinkedIn recommends 4MB chunks for video uploads.
	linkedInChunkSize = 4 * 1024 * 1024 // 4 MB

	// Company pages support up to 200MB videos.
	linkedInMaxFileSize = 200 * 1024 * 1024 // 200 MB

	// Timeout for individual chunk uploads. LinkedIn upload URLs are
	// served by their CDN, so large chunks can take a while on slower links.
	linkedInChunkUploadTimeout = 2 * time.Minute

	// Timeout for LinkedIn API control-plane calls (init, finalize, post).
	linkedInAPITimeout = 30 * time.Second
)

// LinkedInUploadService implements the LinkedIn four-step video upload flow:
//
//  1. Initialize upload  -- registers the video with LinkedIn, gets chunk URLs
//  2. Upload chunks      -- PUT binary data to each upload URL
//  3. Finalize upload    -- signals all chunks are uploaded, provides ETags
//  4. Create post        -- publishes the video on the organization's feed
type LinkedInUploadService struct {
	pool   *pgxpool.Pool
	s3     *infra.S3Client
	crypto *TokenCrypto
	api    *http.Client // short-timeout client for API calls
	upload *http.Client // longer-timeout client for chunk uploads
}

// LinkedInUploadMeta carries the caller-supplied metadata for a LinkedIn post.
type LinkedInUploadMeta struct {
	PostText     string // commentary text for the post
	CompanyID    string // LinkedIn organization ID (numeric string)
	ThumbnailURL string // presigned S3 URL for thumbnail image
}

// NewLinkedInUploadService creates a service wired to the database, S3, and
// token encryption layer. Two separate HTTP clients are used: a fast one for
// API control-plane calls and a slower one that tolerates large chunk uploads.
func NewLinkedInUploadService(pool *pgxpool.Pool, s3 *infra.S3Client, crypto *TokenCrypto) *LinkedInUploadService {
	return &LinkedInUploadService{
		pool:   pool,
		s3:     s3,
		crypto: crypto,
		api: &http.Client{
			Timeout: linkedInAPITimeout,
		},
		upload: &http.Client{
			Timeout: linkedInChunkUploadTimeout,
		},
	}
}

// Upload executes the full LinkedIn Register-Upload-Post flow.
//
// It streams the video from S3 to a temporary file (to stay within the 256MB
// worker RAM limit), uploads it in 4MB chunks to LinkedIn, then creates a
// published post. Progress is tracked in the publish_records table so the
// frontend can display an upload percentage.
//
// Returns the LinkedIn post URN on success.
func (s *LinkedInUploadService) Upload(
	ctx context.Context,
	channelID, versionID, publishRecordID uuid.UUID,
	meta LinkedInUploadMeta,
) (string, error) {
	// -- Resolve access token --------------------------------------------------
	accessToken, _, metadata, err := s.getChannelTokens(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("get channel tokens: %w", err)
	}

	// The organization ID can be provided explicitly in meta or extracted from
	// the channel's metadata JSONB (stored as "organization_id" or "company_id").
	orgID := meta.CompanyID
	if orgID == "" {
		orgID, err = resolveOrganizationID(metadata)
		if err != nil {
			return "", fmt.Errorf("resolve organization id: %w", err)
		}
	}

	// -- Resolve S3 key + file size -------------------------------------------
	s3Key, fileSize, err := s.getVersionFileInfo(ctx, versionID)
	if err != nil {
		return "", fmt.Errorf("get version file info: %w", err)
	}

	if fileSize > linkedInMaxFileSize {
		return "", fmt.Errorf("file size %d bytes exceeds LinkedIn company page limit of %d bytes", fileSize, linkedInMaxFileSize)
	}

	// -- Download to temp file (memory-safe for 256MB worker) -----------------
	tmpFile, err := s.downloadToTempFile(ctx, s3Key)
	if err != nil {
		return "", fmt.Errorf("download to temp file: %w", err)
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	// Mark the upload as started.
	s.setUploadProgress(ctx, publishRecordID, 0)
	s.updatePublishStatus(ctx, publishRecordID, "uploading", "")

	// -- Step 1: Initialize upload --------------------------------------------
	initResp, err := s.initializeUpload(ctx, accessToken, orgID, fileSize, meta)
	if err != nil {
		return "", fmt.Errorf("initialize upload: %w", err)
	}

	slog.Info("linkedin upload initialized",
		"video_urn", initResp.Video,
		"chunks", len(initResp.UploadInstructions),
		"publish_record_id", publishRecordID,
	)

	// -- Step 2: Upload chunks ------------------------------------------------
	etags, err := s.uploadChunks(ctx, accessToken, tmpFile, fileSize, initResp.UploadInstructions, publishRecordID)
	if err != nil {
		return "", fmt.Errorf("upload chunks: %w", err)
	}

	// -- Step 2b: Upload thumbnail (if provided) ------------------------------
	var thumbETags []string
	if meta.ThumbnailURL != "" && len(initResp.ThumbnailUploadInstructions) > 0 {
		thumbETags, err = s.uploadThumbnail(ctx, accessToken, meta.ThumbnailURL, initResp.ThumbnailUploadInstructions)
		if err != nil {
			slog.Error("linkedin thumbnail upload failed (video upload succeeded)",
				"error", err,
				"publish_record_id", publishRecordID,
			)
			// Non-fatal: continue without thumbnail.
		}
	}

	// -- Step 3: Finalize upload ----------------------------------------------
	if err := s.finalizeUpload(ctx, accessToken, initResp.Video, initResp.UploadToken, etags, thumbETags); err != nil {
		return "", fmt.Errorf("finalize upload: %w", err)
	}

	s.setUploadProgress(ctx, publishRecordID, 100)

	slog.Info("linkedin upload finalized",
		"video_urn", initResp.Video,
		"publish_record_id", publishRecordID,
	)

	// -- Step 4: Create post --------------------------------------------------
	postURN, err := s.createPost(ctx, accessToken, orgID, initResp.Video, meta.PostText)
	if err != nil {
		return "", fmt.Errorf("create post: %w", err)
	}

	// Mark as published.
	now := time.Now()
	s.updatePublishFinal(ctx, publishRecordID, "published", postURN, &now)

	slog.Info("linkedin post published",
		"post_urn", postURN,
		"video_urn", initResp.Video,
		"publish_record_id", publishRecordID,
	)

	return postURN, nil
}

// ---------------------------------------------------------------------------
// LinkedIn API request/response types
// ---------------------------------------------------------------------------

// liInitializeRequest is the JSON body for the initializeUpload action.
type liInitializeRequest struct {
	InitializeUploadRequest liInitializeBody `json:"initializeUploadRequest"`
}

type liInitializeBody struct {
	Owner           string `json:"owner"`
	FileSizeBytes   int64  `json:"fileSizeBytes"`
	UploadCaptions  bool   `json:"uploadCaptions"`
	UploadThumbnail bool   `json:"uploadThumbnail"`
}

// liInitializeResponse is the JSON envelope returned by initializeUpload.
type liInitializeResponse struct {
	Value liInitializeValue `json:"value"`
}

type liInitializeValue struct {
	UploadInstructions          []liUploadInstruction `json:"uploadInstructions"`
	ThumbnailUploadInstructions []liUploadInstruction `json:"thumbnailUploadInstructions,omitempty"`
	Video                       string                `json:"video"`
	UploadToken                 string                `json:"uploadToken"`
}

type liUploadInstruction struct {
	UploadURL string `json:"uploadUrl"`
	FirstByte int64  `json:"firstByte"`
	LastByte  int64  `json:"lastByte"`
}

// liFinalizeRequest is the JSON body for the finalizeUpload action.
type liFinalizeRequest struct {
	FinalizeUploadRequest liFinalizeBody `json:"finalizeUploadRequest"`
}

type liFinalizeBody struct {
	Video                    string   `json:"video"`
	UploadToken              string   `json:"uploadToken"`
	UploadedPartIDs          []string `json:"uploadedPartIds"`
	UploadedThumbnailPartIDs []string `json:"uploadedThumbnailPartIds,omitempty"`
}

// liCreatePostRequest is the JSON body for creating a published post.
type liCreatePostRequest struct {
	Author         string         `json:"author"`
	Commentary     string         `json:"commentary"`
	Visibility     string         `json:"visibility"`
	Distribution   liDistribution `json:"distribution"`
	Content        liPostContent  `json:"content"`
	LifecycleState string         `json:"lifecycleState"`
}

type liDistribution struct {
	FeedDistribution string `json:"feedDistribution"`
}

type liPostContent struct {
	Media liPostMedia `json:"media"`
}

type liPostMedia struct {
	ID string `json:"id"`
}

// liPostResponse is the response from creating a post, returned as a header.
type liPostResponse struct {
	ID string `json:"id"`
}

// liErrorResponse represents a LinkedIn REST API error envelope.
type liErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// ---------------------------------------------------------------------------
// Step 1: Initialize Upload
// ---------------------------------------------------------------------------

func (s *LinkedInUploadService) initializeUpload(
	ctx context.Context,
	token, orgID string,
	fileSizeBytes int64,
	meta LinkedInUploadMeta,
) (*liInitializeValue, error) {
	body := liInitializeRequest{
		InitializeUploadRequest: liInitializeBody{
			Owner:           fmt.Sprintf("urn:li:organization:%s", orgID),
			FileSizeBytes:   fileSizeBytes,
			UploadCaptions:  false,
			UploadThumbnail: meta.ThumbnailURL != "",
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal init request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		linkedInAPIBase+"/videos?action=initializeUpload",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create init request: %w", err)
	}
	setLinkedInHeaders(req, token)

	resp, err := s.api.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post init request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read init response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("initializeUpload returned %d: %s", resp.StatusCode, parseLinkedInError(respBody))
	}

	var initResp liInitializeResponse
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		return nil, fmt.Errorf("decode init response: %w", err)
	}

	if initResp.Value.Video == "" {
		return nil, fmt.Errorf("initializeUpload response missing video URN")
	}
	if len(initResp.Value.UploadInstructions) == 0 {
		return nil, fmt.Errorf("initializeUpload response has no upload instructions")
	}

	return &initResp.Value, nil
}

// ---------------------------------------------------------------------------
// Step 2: Upload chunks
// ---------------------------------------------------------------------------

func (s *LinkedInUploadService) uploadChunks(
	ctx context.Context,
	token string,
	file *os.File,
	fileSize int64,
	instructions []liUploadInstruction,
	publishRecordID uuid.UUID,
) ([]string, error) {
	totalChunks := len(instructions)
	etags := make([]string, 0, totalChunks)

	// Pre-allocate a single read buffer. At 4MB this is well within the
	// 256MB worker limit even when combined with the temp file.
	buf := make([]byte, linkedInChunkSize)

	for i, inst := range instructions {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		chunkSize := inst.LastByte - inst.FirstByte + 1

		// Seek to the correct position.
		if _, err := file.Seek(inst.FirstByte, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek to byte %d for chunk %d: %w", inst.FirstByte, i+1, err)
		}

		// Read exactly the bytes for this chunk.
		readBuf := buf[:chunkSize]
		if _, err := io.ReadFull(file, readBuf); err != nil {
			return nil, fmt.Errorf("read chunk %d (%d bytes): %w", i+1, chunkSize, err)
		}

		etag, err := s.uploadSingleChunk(ctx, token, inst.UploadURL, readBuf)
		if err != nil {
			return nil, fmt.Errorf("upload chunk %d/%d: %w", i+1, totalChunks, err)
		}

		etags = append(etags, etag)

		// Update progress: scale from 0-95% (reserve 5% for finalize + post).
		pct := int(float64(i+1) / float64(totalChunks) * 95)
		s.setUploadProgress(ctx, publishRecordID, pct)

		slog.Debug("linkedin chunk uploaded",
			"chunk", i+1,
			"total", totalChunks,
			"bytes", chunkSize,
			"etag", etag,
		)
	}

	return etags, nil
}

// uploadSingleChunk PUTs a single binary chunk to the LinkedIn upload URL
// and returns the ETag from the response headers.
func (s *LinkedInUploadService) uploadSingleChunk(
	ctx context.Context,
	token, uploadURL string,
	data []byte,
) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create chunk request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	// LinkedIn requires versioned headers even on upload URLs.
	req.Header.Set("LinkedIn-Version", linkedInAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", linkedInRestLiVer)

	resp, err := s.upload.Do(req)
	if err != nil {
		return "", fmt.Errorf("put chunk: %w", err)
	}
	defer resp.Body.Close()

	// Drain the body so the connection can be reused.
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("chunk upload returned HTTP %d", resp.StatusCode)
	}

	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", fmt.Errorf("chunk upload response missing ETag header")
	}

	return etag, nil
}

// uploadThumbnail downloads a thumbnail from the presigned URL and uploads it
// to LinkedIn's thumbnail upload endpoint. Returns the ETags for finalization.
func (s *LinkedInUploadService) uploadThumbnail(
	ctx context.Context,
	token, thumbnailURL string,
	instructions []liUploadInstruction,
) ([]string, error) {
	// Download thumbnail from presigned URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, thumbnailURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create thumbnail download request: %w", err)
	}

	resp, err := s.api.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download thumbnail: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thumbnail download returned HTTP %d", resp.StatusCode)
	}

	thumbData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail data: %w", err)
	}

	// Upload to each thumbnail instruction URL.
	etags := make([]string, 0, len(instructions))
	for i, inst := range instructions {
		chunkSize := inst.LastByte - inst.FirstByte + 1
		start := inst.FirstByte
		end := start + chunkSize
		if end > int64(len(thumbData)) {
			end = int64(len(thumbData))
		}
		if start >= int64(len(thumbData)) {
			break
		}

		etag, err := s.uploadSingleChunk(ctx, token, inst.UploadURL, thumbData[start:end])
		if err != nil {
			return nil, fmt.Errorf("upload thumbnail chunk %d: %w", i+1, err)
		}
		etags = append(etags, etag)
	}

	slog.Info("linkedin thumbnail uploaded", "chunks", len(etags))
	return etags, nil
}

// ---------------------------------------------------------------------------
// Step 3: Finalize Upload
// ---------------------------------------------------------------------------

func (s *LinkedInUploadService) finalizeUpload(
	ctx context.Context,
	token, videoURN, uploadToken string,
	etags []string,
	thumbETags []string,
) error {
	finalizeBody := liFinalizeBody{
		Video:           videoURN,
		UploadToken:     uploadToken,
		UploadedPartIDs: etags,
	}
	if len(thumbETags) > 0 {
		finalizeBody.UploadedThumbnailPartIDs = thumbETags
	}
	body := liFinalizeRequest{
		FinalizeUploadRequest: finalizeBody,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal finalize request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		linkedInAPIBase+"/videos?action=finalizeUpload",
		bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create finalize request: %w", err)
	}
	setLinkedInHeaders(req, token)

	resp, err := s.api.Do(req)
	if err != nil {
		return fmt.Errorf("post finalize request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read finalize response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("finalizeUpload returned %d: %s", resp.StatusCode, parseLinkedInError(respBody))
	}

	return nil
}

// ---------------------------------------------------------------------------
// Step 4: Create Post
// ---------------------------------------------------------------------------

func (s *LinkedInUploadService) createPost(
	ctx context.Context,
	token, orgID, videoURN, commentary string,
) (string, error) {
	body := liCreatePostRequest{
		Author:       fmt.Sprintf("urn:li:organization:%s", orgID),
		Commentary:   commentary,
		Visibility:   "PUBLIC",
		Distribution: liDistribution{FeedDistribution: "MAIN_FEED"},
		Content: liPostContent{
			Media: liPostMedia{ID: videoURN},
		},
		LifecycleState: "PUBLISHED",
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal post request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		linkedInAPIBase+"/posts",
		bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create post request: %w", err)
	}
	setLinkedInHeaders(req, token)

	resp, err := s.api.Do(req)
	if err != nil {
		return "", fmt.Errorf("post create request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read post response: %w", err)
	}

	// LinkedIn returns 201 Created for successful post creation.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create post returned %d: %s", resp.StatusCode, parseLinkedInError(respBody))
	}

	// The post URN is returned in the x-restli-id header.
	postURN := resp.Header.Get("x-restli-id")
	if postURN == "" {
		// Fall back to parsing the response body.
		var postResp liPostResponse
		if err := json.Unmarshal(respBody, &postResp); err == nil && postResp.ID != "" {
			postURN = postResp.ID
		}
	}

	if postURN == "" {
		// Even without a URN the post was created (2xx status). Log a warning
		// but return the video URN as a fallback identifier.
		slog.Warn("linkedin post created but no URN returned, using video URN as fallback",
			"video_urn", videoURN,
		)
		postURN = videoURN
	}

	return postURN, nil
}

// ---------------------------------------------------------------------------
// Database helpers
// ---------------------------------------------------------------------------

// getChannelTokens retrieves and decrypts LinkedIn channel credentials.
func (s *LinkedInUploadService) getChannelTokens(ctx context.Context, channelID uuid.UUID) (accessToken, refreshToken string, metadata []byte, err error) {
	var encAccessToken string
	var encRefreshToken *string
	var tokenEncrypted bool

	err = s.pool.QueryRow(ctx,
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

	if tokenEncrypted && s.crypto != nil {
		accessToken, err = s.crypto.Decrypt(encAccessToken)
		if err != nil {
			return "", "", nil, fmt.Errorf("decrypt access token: %w", err)
		}
		if encRefreshStr != "" {
			refreshToken, err = s.crypto.Decrypt(encRefreshStr)
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

// getVersionFileInfo retrieves the S3 key and file size for a video.
func (s *LinkedInUploadService) getVersionFileInfo(ctx context.Context, videoID uuid.UUID) (string, int64, error) {
	var s3Key string
	var fileSize int64

	err := s.pool.QueryRow(ctx,
		`SELECT storage_key, file_size_bytes FROM videos WHERE id = $1 AND is_deleted = FALSE`,
		videoID,
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
		return "", 0, fmt.Errorf("deliverable version has invalid file size: %d", fileSize)
	}

	return s3Key, fileSize, nil
}

// downloadToTempFile streams a file from S3 to a local temp file, keeping
// memory usage bounded regardless of file size.
func (s *LinkedInUploadService) downloadToTempFile(ctx context.Context, s3Key string) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "linkedin-upload-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	reader, _, err := s.s3.GetObjectStream(ctx, s3Key)
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("get s3 object stream: %w", err)
	}
	defer reader.Close()

	if _, err := io.Copy(tmpFile, reader); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("copy s3 stream to temp file: %w", err)
	}

	// Seek back to start for subsequent reads.
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("seek temp file to start: %w", err)
	}

	return tmpFile, nil
}

// setUploadProgress updates the upload percentage on the publish record.
// Errors are logged but do not abort the upload.
func (s *LinkedInUploadService) setUploadProgress(ctx context.Context, publishRecordID uuid.UUID, pct int) {
	_, err := s.pool.Exec(ctx,
		`UPDATE publish_records SET upload_progress_pct = $1 WHERE id = $2`,
		pct, publishRecordID)
	if err != nil {
		slog.Error("failed to update upload progress",
			"publish_record_id", publishRecordID,
			"pct", pct,
			"error", err,
		)
	}
}

// updatePublishStatus sets the status and optional error message on a publish record.
func (s *LinkedInUploadService) updatePublishStatus(ctx context.Context, publishRecordID uuid.UUID, status, errMsg string) {
	_, err := s.pool.Exec(ctx,
		`UPDATE publish_records SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
		status, nilIfEmptyStr(errMsg), publishRecordID)
	if err != nil {
		slog.Error("failed to update publish status",
			"publish_record_id", publishRecordID,
			"status", status,
			"error", err,
		)
	}
}

// updatePublishFinal marks a publish record as complete with the platform URL
// and published timestamp.
func (s *LinkedInUploadService) updatePublishFinal(ctx context.Context, publishRecordID uuid.UUID, status, platformContentID string, publishedAt *time.Time) {
	_, err := s.pool.Exec(ctx,
		`UPDATE publish_records
		 SET status = $1, platform_content_id = $2, published_at = $3,
		     upload_progress_pct = 100, upload_completed_at = NOW(), updated_at = NOW()
		 WHERE id = $4`,
		status, platformContentID, publishedAt, publishRecordID)
	if err != nil {
		slog.Error("failed to update publish record final status",
			"publish_record_id", publishRecordID,
			"status", status,
			"error", err,
		)
	}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// setLinkedInHeaders applies the mandatory LinkedIn REST API headers to a request.
func setLinkedInHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", linkedInAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", linkedInRestLiVer)
}

// resolveOrganizationID extracts the LinkedIn organization ID from the
// channel's metadata JSONB. It checks for "organization_id" first, then
// falls back to "company_id".
func resolveOrganizationID(metadata []byte) (string, error) {
	orgID, err := extractMetadataField(metadata, "organization_id")
	if err == nil && orgID != "" {
		return orgID, nil
	}

	orgID, err = extractMetadataField(metadata, "company_id")
	if err == nil && orgID != "" {
		return orgID, nil
	}

	return "", fmt.Errorf("neither 'organization_id' nor 'company_id' found in channel metadata")
}

// parseLinkedInError extracts a human-readable error from a LinkedIn API
// error response. LinkedIn REST errors follow a consistent JSON shape:
//
//	{"status": 403, "message": "...", "code": "..."}
func parseLinkedInError(body []byte) string {
	var e liErrorResponse
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if e.Code != "" {
			return fmt.Sprintf("[%s] %s (HTTP %d)", e.Code, e.Message, e.Status)
		}
		return fmt.Sprintf("%s (HTTP %d)", e.Message, e.Status)
	}
	// Truncate raw body for log safety.
	if len(body) > 512 {
		return string(body[:512]) + "..."
	}
	return string(body)
}
