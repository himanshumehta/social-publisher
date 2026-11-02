package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/model"
)

const (
	partSizeBytes       = 100 * 1024 * 1024 // 100 MB
	presignExpiry       = 2 * time.Hour
	avatarPresignExpiry = 15 * time.Minute
)

// DuplicateFileError is returned when a file with the same name already
// exists for the given project+format. The frontend uses this to show
// a targeted "please rename" message.
type DuplicateFileError struct {
	FileName string
}

func (e *DuplicateFileError) Error() string {
	return fmt.Sprintf("duplicate file: '%s' already exists for this project — please rename the file and try again", e.FileName)
}

// IsDuplicateFileError checks whether an error is a DuplicateFileError.
func IsDuplicateFileError(err error) bool {
	var dup *DuplicateFileError
	return errors.As(err, &dup)
}

type UploadService struct {
	pool *pgxpool.Pool
	s3   *infra.S3Client
}

func NewUploadService(pool *pgxpool.Pool, s3 *infra.S3Client) *UploadService {
	return &UploadService{pool: pool, s3: s3}
}

type InitiateUploadRequest struct {
	ProjectID     uuid.UUID `json:"project_id"`
	FileName      string    `json:"file_name"`
	FileSizeBytes int64     `json:"file_size_bytes"`
	MimeType      string    `json:"mime_type"`
	Format        string    `json:"format,omitempty"`
}

type PresignedPart struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

type InitiateUploadResponse struct {
	UploadID       string          `json:"upload_id"`
	StorageKey     string          `json:"storage_key"`
	PresignedParts []PresignedPart `json:"presigned_parts"`
}

type CompleteUploadRequest struct {
	UploadID string `json:"upload_id"`
}

type RefreshURLsRequest struct {
	UploadID    string `json:"upload_id"`
	PartNumbers []int  `json:"part_numbers"`
}

type RefreshURLsResponse struct {
	PresignedParts []PresignedPart `json:"presigned_parts"`
}

func (s *UploadService) InitiateUpload(ctx context.Context, userID uuid.UUID, req InitiateUploadRequest) (*InitiateUploadResponse, error) {
	// Determine format (default to raw)
	format := req.Format
	if format == "" {
		format = "raw"
	}

	// Validate user permissions based on format
	var projectStatus model.ProjectStatus
	var creatorID, editorID *uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT status, creator_id, editor_id FROM projects WHERE id = $1`, req.ProjectID,
	).Scan(&projectStatus, &creatorID, &editorID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	// Uploads are allowed while project is active
	if projectStatus != model.ProjectActive {
		return nil, fmt.Errorf("uploads are only allowed on active projects")
	}

	if format == "raw" {
		// Creator uploads raw footage
		if creatorID == nil || *creatorID != userID {
			return nil, fmt.Errorf("you are not the assigned creator")
		}
	} else {
		// Non-raw uploads: creator or assigned editor can upload
		isCreator := creatorID != nil && *creatorID == userID
		isEditor := editorID != nil && *editorID == userID
		if !isCreator && !isEditor {
			return nil, fmt.Errorf("you are not the creator or assigned editor")
		}
	}

	if req.FileName == "" || req.FileSizeBytes <= 0 || req.MimeType == "" {
		return nil, fmt.Errorf("file_name, file_size_bytes, and mime_type are required")
	}

	// ── Reject duplicate filenames ──────────────────────────────────────
	// If a non-deleted video with the same filename already exists for
	// this project+format, block the upload and ask the user to rename.
	var existingCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos
		 WHERE project_id = $1 AND format = $2 AND file_name = $3 AND is_deleted = FALSE`,
		req.ProjectID, format, req.FileName,
	).Scan(&existingCount)
	if err != nil {
		return nil, fmt.Errorf("check duplicate: %w", err)
	}
	if existingCount > 0 {
		return nil, &DuplicateFileError{FileName: req.FileName}
	}

	// Also check in-progress multipart uploads to prevent concurrent
	// duplicate uploads of the same file.
	var inProgressCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM multipart_uploads
		 WHERE project_id = $1 AND format = $2 AND file_name = $3 AND status = 'in_progress'`,
		req.ProjectID, format, req.FileName,
	).Scan(&inProgressCount)
	if err != nil {
		return nil, fmt.Errorf("check in-progress duplicate: %w", err)
	}
	if inProgressCount > 0 {
		return nil, &DuplicateFileError{FileName: req.FileName}
	}

	// Fetch project slug for human-readable storage key
	var projectSlug string
	err = s.pool.QueryRow(ctx,
		`SELECT slug FROM projects WHERE id = $1`, req.ProjectID,
	).Scan(&projectSlug)
	if err != nil {
		return nil, fmt.Errorf("get project slug: %w", err)
	}

	// Get next version number to include in the storage key.
	// This prevents S3 key collisions even if duplicate check is
	// somehow bypassed (e.g. race condition between two requests).
	var nextVersion int
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM videos WHERE project_id = $1 AND format = $2`,
		req.ProjectID, format,
	).Scan(&nextVersion)
	if err != nil {
		return nil, fmt.Errorf("get next version: %w", err)
	}

	// Storage key: {project-slug}/v{version}-{file-name}
	storageKey := fmt.Sprintf("%s/v%d-%s", projectSlug, nextVersion, req.FileName)

	// Create S3 multipart upload
	s3UploadID, err := s.s3.CreateMultipartUpload(ctx, storageKey, req.MimeType)
	if err != nil {
		return nil, fmt.Errorf("create multipart upload: %w", err)
	}

	totalParts := int(math.Ceil(float64(req.FileSizeBytes) / float64(partSizeBytes)))
	if totalParts < 1 {
		totalParts = 1
	}

	// Generate presigned URLs for all parts
	presignedParts := make([]PresignedPart, totalParts)
	for i := 0; i < totalParts; i++ {
		partNum := i + 1
		url, err := s.s3.PresignUploadPart(ctx, storageKey, s3UploadID, partNum, presignExpiry)
		if err != nil {
			// Abort if we fail to generate presigned URLs
			_ = s.s3.AbortMultipartUpload(ctx, storageKey, s3UploadID)
			return nil, fmt.Errorf("presign part %d: %w", partNum, err)
		}
		presignedParts[i] = PresignedPart{PartNumber: partNum, URL: url}
	}

	// Track in database
	_, err = s.pool.Exec(ctx,
		`INSERT INTO multipart_uploads (upload_id, project_id, user_id, storage_key, file_name, file_size_bytes, mime_type, total_parts, format)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		s3UploadID, req.ProjectID, userID, storageKey, req.FileName, req.FileSizeBytes, req.MimeType, totalParts, format)
	if err != nil {
		_ = s.s3.AbortMultipartUpload(ctx, storageKey, s3UploadID)
		return nil, fmt.Errorf("track upload: %w", err)
	}

	return &InitiateUploadResponse{
		UploadID:       s3UploadID,
		StorageKey:     storageKey,
		PresignedParts: presignedParts,
	}, nil
}

func (s *UploadService) CompleteUpload(ctx context.Context, userID uuid.UUID, req CompleteUploadRequest) (*model.Video, error) {
	// Get upload record
	var mu model.MultipartUpload
	err := s.pool.QueryRow(ctx,
		`SELECT id, upload_id, project_id, user_id, storage_key, file_name, file_size_bytes, mime_type, total_parts, format, status
		 FROM multipart_uploads WHERE upload_id = $1`, req.UploadID,
	).Scan(&mu.ID, &mu.UploadID, &mu.ProjectID, &mu.UserID, &mu.StorageKey,
		&mu.FileName, &mu.FileSizeBytes, &mu.MimeType, &mu.TotalParts, &mu.Format, &mu.Status)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("upload not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get upload: %w", err)
	}

	if mu.UserID != userID {
		return nil, fmt.Errorf("not your upload")
	}
	if mu.Status != "in_progress" {
		return nil, fmt.Errorf("upload is not in progress")
	}

	// Fetch part ETags from S3 (avoids CORS Expose-Headers requirement on the browser)
	completedParts, err := s.s3.ListParts(ctx, mu.StorageKey, mu.UploadID)
	if err != nil {
		return nil, fmt.Errorf("list parts: %w", err)
	}
	if len(completedParts) != mu.TotalParts {
		return nil, fmt.Errorf("expected %d parts, got %d", mu.TotalParts, len(completedParts))
	}

	if err := s.s3.CompleteMultipartUpload(ctx, mu.StorageKey, mu.UploadID, completedParts); err != nil {
		return nil, fmt.Errorf("complete s3 upload: %w", err)
	}

	// Use transaction for DB updates
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Mark upload complete
	_, err = tx.Exec(ctx,
		`UPDATE multipart_uploads SET status = 'completed', updated_at = NOW() WHERE upload_id = $1`,
		req.UploadID)
	if err != nil {
		return nil, fmt.Errorf("update upload status: %w", err)
	}

	// Use the format stored during initiation
	videoFormat := mu.Format

	// Get current max version for this project+format
	var maxVersion int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM videos WHERE project_id = $1 AND format = $2`,
		mu.ProjectID, videoFormat).Scan(&maxVersion)
	if err != nil {
		return nil, fmt.Errorf("get max version: %w", err)
	}

	// Mark previous videos of same format as not current
	_, err = tx.Exec(ctx,
		`UPDATE videos SET is_current = FALSE, updated_at = NOW()
		 WHERE project_id = $1 AND format = $2 AND is_current = TRUE`,
		mu.ProjectID, videoFormat)
	if err != nil {
		return nil, fmt.Errorf("update previous videos: %w", err)
	}

	// Insert video record
	var video model.Video
	err = tx.QueryRow(ctx,
		`INSERT INTO videos (project_id, uploaded_by, format, version, storage_key, storage_type, file_name, file_size_bytes, mime_type)
		 VALUES ($1, $2, $3, $4, $5, 'spaces', $6, $7, $8)
		 RETURNING id, project_id, uploaded_by, format, version, storage_key, storage_type, file_name, file_size_bytes, mime_type,
		    duration_seconds, width, height, status, source_video_id, clip_job_id, virality_score, is_current, created_at, updated_at`,
		mu.ProjectID, userID, videoFormat, maxVersion+1, mu.StorageKey, mu.FileName, mu.FileSizeBytes, mu.MimeType,
	).Scan(&video.ID, &video.ProjectID, &video.UploadedBy, &video.Format, &video.Version,
		&video.StorageKey, &video.StorageType, &video.FileName, &video.FileSizeBytes, &video.MimeType,
		&video.DurationSeconds, &video.Width, &video.Height, &video.Status,
		&video.SourceVideoID, &video.ClipJobID, &video.ViralityScore, &video.IsCurrent, &video.CreatedAt, &video.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert video: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &video, nil
}

func (s *UploadService) AbortUpload(ctx context.Context, userID uuid.UUID, uploadID string) error {
	var mu model.MultipartUpload
	err := s.pool.QueryRow(ctx,
		`SELECT id, upload_id, project_id, user_id, storage_key, status
		 FROM multipart_uploads WHERE upload_id = $1`, uploadID,
	).Scan(&mu.ID, &mu.UploadID, &mu.ProjectID, &mu.UserID, &mu.StorageKey, &mu.Status)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("upload not found")
	}
	if err != nil {
		return fmt.Errorf("get upload: %w", err)
	}

	if mu.UserID != userID {
		return fmt.Errorf("not your upload")
	}
	if mu.Status != "in_progress" {
		return fmt.Errorf("upload is not in progress")
	}

	_ = s.s3.AbortMultipartUpload(ctx, mu.StorageKey, mu.UploadID)

	_, err = s.pool.Exec(ctx,
		`UPDATE multipart_uploads SET status = 'aborted', updated_at = NOW() WHERE upload_id = $1`,
		uploadID)
	if err != nil {
		return fmt.Errorf("update upload status: %w", err)
	}

	return nil
}

func (s *UploadService) GetPresignedURLsForParts(ctx context.Context, userID uuid.UUID, req RefreshURLsRequest) (*RefreshURLsResponse, error) {
	var mu model.MultipartUpload
	err := s.pool.QueryRow(ctx,
		`SELECT id, upload_id, user_id, storage_key, status
		 FROM multipart_uploads WHERE upload_id = $1`, req.UploadID,
	).Scan(&mu.ID, &mu.UploadID, &mu.UserID, &mu.StorageKey, &mu.Status)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("upload not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get upload: %w", err)
	}

	if mu.UserID != userID {
		return nil, fmt.Errorf("not your upload")
	}
	if mu.Status != "in_progress" {
		return nil, fmt.Errorf("upload is not in progress")
	}

	presignedParts := make([]PresignedPart, len(req.PartNumbers))
	for i, partNum := range req.PartNumbers {
		url, err := s.s3.PresignUploadPart(ctx, mu.StorageKey, mu.UploadID, partNum, presignExpiry)
		if err != nil {
			return nil, fmt.Errorf("presign part %d: %w", partNum, err)
		}
		presignedParts[i] = PresignedPart{PartNumber: partNum, URL: url}
	}

	return &RefreshURLsResponse{PresignedParts: presignedParts}, nil
}
