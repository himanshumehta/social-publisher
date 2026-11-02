package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/model"
)

type RawAssetService struct {
	pool *pgxpool.Pool
	s3   *infra.S3Client
}

func NewRawAssetService(pool *pgxpool.Pool, s3 *infra.S3Client) *RawAssetService {
	return &RawAssetService{pool: pool, s3: s3}
}

// RawAssetWithURL embeds RawAsset and adds a presigned playback URL
type RawAssetWithURL struct {
	*model.RawAsset
	PlaybackURL string `json:"playback_url"`
}

// Create inserts a raw asset record and transitions the project from 'claimed' to 'in_production' on first upload
func (s *RawAssetService) Create(ctx context.Context, projectID, uploadedBy uuid.UUID, storageKey, storageType, fileName, mimeType string, fileSizeBytes int64) (*model.RawAsset, error) {
	// Use transaction for atomic insert and status update
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert raw asset record
	var rawAsset model.RawAsset
	err = tx.QueryRow(ctx,
		`INSERT INTO raw_assets (project_id, uploaded_by, storage_key, storage_type, file_name, file_size_bytes, mime_type, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, project_id, uploaded_by, storage_key, storage_type, file_name, file_size_bytes, mime_type, duration_seconds, width, height, status, notes, created_at, updated_at`,
		projectID, uploadedBy, storageKey, storageType, fileName, fileSizeBytes, mimeType, model.RawAssetProcessing,
	).Scan(&rawAsset.ID, &rawAsset.ProjectID, &rawAsset.UploadedBy, &rawAsset.StorageKey, &rawAsset.StorageType,
		&rawAsset.FileName, &rawAsset.FileSizeBytes, &rawAsset.MimeType, &rawAsset.DurationSeconds, &rawAsset.Width,
		&rawAsset.Height, &rawAsset.Status, &rawAsset.Notes, &rawAsset.CreatedAt, &rawAsset.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert raw asset: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &rawAsset, nil
}

// ListByProject returns paginated raw assets for an project with presigned playback URLs.
func (s *RawAssetService) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]RawAssetWithURL, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM raw_assets WHERE project_id = $1`, projectID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count raw assets: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, uploaded_by, storage_key, storage_type, file_name, file_size_bytes, mime_type, duration_seconds, width, height, status, notes, created_at, updated_at
		 FROM raw_assets
		 WHERE project_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query raw assets: %w", err)
	}
	defer rows.Close()

	var assets []RawAssetWithURL
	for rows.Next() {
		var asset model.RawAsset
		if err := rows.Scan(&asset.ID, &asset.ProjectID, &asset.UploadedBy, &asset.StorageKey, &asset.StorageType,
			&asset.FileName, &asset.FileSizeBytes, &asset.MimeType, &asset.DurationSeconds, &asset.Width,
			&asset.Height, &asset.Status, &asset.Notes, &asset.CreatedAt, &asset.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan raw asset: %w", err)
		}

		// Generate presigned playback URL
		playbackURL, err := s.s3.PresignGetObject(ctx, asset.StorageKey, 2*time.Hour)
		if err != nil {
			return nil, 0, fmt.Errorf("presign playback url for asset %s: %w", asset.ID, err)
		}

		assets = append(assets, RawAssetWithURL{
			RawAsset:    &asset,
			PlaybackURL: playbackURL,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate raw assets: %w", err)
	}

	if assets == nil {
		assets = []RawAssetWithURL{}
	}

	return assets, total, nil
}

// UpdateStatus updates the status of a raw asset
func (s *RawAssetService) UpdateStatus(ctx context.Context, rawAssetID uuid.UUID, status model.RawAssetStatus) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE raw_assets SET status = $1, updated_at = NOW() WHERE id = $2`,
		status, rawAssetID)
	if err != nil {
		return fmt.Errorf("update raw asset status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("raw asset not found")
	}

	return nil
}

// MarkReady marks a processing raw asset as ready
func (s *RawAssetService) MarkReady(ctx context.Context, rawAssetID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE raw_assets SET status = $1, updated_at = NOW() WHERE id = $2 AND status = $3`,
		model.RawAssetReady, rawAssetID, model.RawAssetProcessing)
	if err != nil {
		return fmt.Errorf("mark raw asset ready: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("raw asset not found or not in processing status")
	}

	return nil
}
