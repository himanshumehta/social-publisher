package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/model"
)

const playbackURLExpiry = 2 * time.Hour

type VideoService struct {
	pool *pgxpool.Pool
	s3   *infra.S3Client
}

func NewVideoService(pool *pgxpool.Pool, s3 *infra.S3Client) *VideoService {
	return &VideoService{pool: pool, s3: s3}
}

type VideoWithURL struct {
	model.Video
	PlaybackURL  string `json:"playback_url"`
	ThumbnailURL string `json:"thumbnail_url,omitempty"`
}

func (s *VideoService) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]VideoWithURL, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM videos WHERE project_id = $1 AND is_deleted = FALSE`, projectID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count videos: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, uploaded_by, format, version, storage_key, storage_type,
		        file_name, file_size_bytes, mime_type, duration_seconds, width, height,
		        status, source_video_id, clip_job_id, virality_score, thumbnail_key,
		        is_current, created_at, updated_at
		 FROM videos
		 WHERE project_id = $1 AND is_deleted = FALSE
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`, projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query videos: %w", err)
	}
	defer rows.Close()

	var videos []VideoWithURL
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.UploadedBy, &v.Format, &v.Version,
			&v.StorageKey, &v.StorageType, &v.FileName, &v.FileSizeBytes, &v.MimeType,
			&v.DurationSeconds, &v.Width, &v.Height, &v.Status,
			&v.SourceVideoID, &v.ClipJobID, &v.ViralityScore, &v.ThumbnailKey,
			&v.IsCurrent, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan video: %w", err)
		}
		url, err := s.s3.PresignGetObject(ctx, v.StorageKey, playbackURLExpiry)
		if err != nil {
			url = "" // non-fatal: video may still be processing
		}
		vWithURL := VideoWithURL{Video: v, PlaybackURL: url}
		if v.ThumbnailKey != nil && *v.ThumbnailKey != "" {
			thumbURL, err := s.s3.PresignGetObject(ctx, *v.ThumbnailKey, playbackURLExpiry)
			if err == nil {
				vWithURL.ThumbnailURL = thumbURL
			}
		}
		videos = append(videos, vWithURL)
	}
	if videos == nil {
		videos = []VideoWithURL{}
	}
	return videos, total, nil
}

func (s *VideoService) GetPlaybackURL(ctx context.Context, videoID uuid.UUID) (string, error) {
	var storageKey string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_key FROM videos WHERE id = $1 AND is_deleted = FALSE`, videoID,
	).Scan(&storageKey)
	if err == pgx.ErrNoRows {
		return "", fmt.Errorf("video not found")
	}
	if err != nil {
		return "", fmt.Errorf("get video: %w", err)
	}
	url, err := s.s3.PresignGetObject(ctx, storageKey, playbackURLExpiry)
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return url, nil
}

func (s *VideoService) SoftDelete(ctx context.Context, videoID, userID uuid.UUID, userRole string) error {
	// Fetch video and its project status
	var uploadedBy uuid.UUID
	var projectStatus string
	var thumbnailKey *string
	err := s.pool.QueryRow(ctx,
		`SELECT v.uploaded_by, a.status, v.thumbnail_key
		 FROM videos v
		 JOIN projects a ON a.id = v.project_id
		 WHERE v.id = $1 AND v.is_deleted = FALSE`, videoID,
	).Scan(&uploadedBy, &projectStatus, &thumbnailKey)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("video not found")
	}
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}

	isAdmin := userRole == "admin" || userRole == "super_admin"

	// Non-admin users can only delete while project is active
	if !isAdmin {
		if uploadedBy != userID {
			return fmt.Errorf("forbidden: only the uploader or admin can delete this video")
		}
		if projectStatus != string(model.ProjectActive) {
			return fmt.Errorf("cannot delete files once project is submitted for review")
		}
	}

	// Delete thumbnail from S3 if it exists
	if thumbnailKey != nil && *thumbnailKey != "" {
		if err := s.s3.DeleteObject(ctx, *thumbnailKey); err != nil {
			// Log error but don't fail the delete operation
			fmt.Printf("warning: failed to delete thumbnail %s: %v\n", *thumbnailKey, err)
		}
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE videos SET is_deleted = TRUE, updated_at = NOW() WHERE id = $1`, videoID)
	if err != nil {
		return fmt.Errorf("soft delete video: %w", err)
	}
	return nil
}
