package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ProjectAssetService struct {
	pool *pgxpool.Pool
}

func NewProjectAssetService(pool *pgxpool.Pool) *ProjectAssetService {
	return &ProjectAssetService{pool: pool}
}

const assetColumns = `id, project_id, category, COALESCE(subcategory, ''), filename, file_url,
	thumbnail_url, COALESCE(alt_text, ''), file_size, COALESCE(mime_type, ''),
	COALESCE(metadata, '{}'), sort_order, COALESCE(status, 'active'), version,
	uploaded_by, deleted_at, created_at, updated_at`

const changelogColumns = `id, project_id, action, COALESCE(category, ''), asset_count, COALESCE(summary, ''), COALESCE(triggered_by, ''), created_at`

// Request types

type CreateAssetRequest struct {
	Category     string          `json:"category"`
	Subcategory  string          `json:"subcategory"`
	Filename     string          `json:"filename"`
	FileURL      string          `json:"file_url"`
	ThumbnailURL string          `json:"thumbnail_url,omitempty"`
	AltText      string          `json:"alt_text,omitempty"`
	FileSize     int64           `json:"file_size"`
	MimeType     string          `json:"mime_type"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

type BulkDeleteAssetsRequest struct {
	AssetIDs []uuid.UUID `json:"asset_ids"`
}

type RenameAssetRequest struct {
	Filename string `json:"filename"`
}

type MoveAssetRequest struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}

type UpdateKitStatusRequest struct {
	Status   string          `json:"status"`
	Progress json.RawMessage `json:"progress,omitempty"`
}

type AddChangelogRequest struct {
	Action      string `json:"action"`
	Category    string `json:"category"`
	AssetCount  int    `json:"asset_count"`
	Summary     string `json:"summary"`
	TriggeredBy string `json:"triggered_by"`
}

type KitStatusResponse struct {
	Status      string          `json:"status"`
	Progress    json.RawMessage `json:"progress"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// ListAssets returns all active assets for a project, grouped by category.
func (s *ProjectAssetService) ListAssets(ctx context.Context, projectID uuid.UUID) ([]model.ProjectAssetTree, error) {
	query := `SELECT ` + assetColumns + ` FROM project_assets
		WHERE project_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY category, subcategory, sort_order, filename`

	rows, err := s.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()

	assets, err := scanAssets(rows)
	if err != nil {
		return nil, err
	}

	return groupByCategory(assets), nil
}

// SearchAssets searches assets by filename or type filter.
func (s *ProjectAssetService) SearchAssets(ctx context.Context, projectID uuid.UUID, query string, fileType string) ([]model.ProjectAsset, error) {
	sql := `SELECT ` + assetColumns + ` FROM project_assets
		WHERE project_id = $1 AND status = 'active' AND deleted_at IS NULL`
	args := []any{projectID}
	argN := 2

	if query != "" {
		sql += fmt.Sprintf(` AND filename ILIKE $%d`, argN)
		args = append(args, "%"+query+"%")
		argN++
	}
	if fileType != "" {
		sql += fmt.Sprintf(` AND mime_type LIKE $%d`, argN)
		args = append(args, fileType+"/%")
		argN++
	}
	sql += ` ORDER BY category, filename LIMIT 100`

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("search assets: %w", err)
	}
	defer rows.Close()

	return scanAssets(rows)
}

// CreateAsset inserts a new project asset.
func (s *ProjectAssetService) CreateAsset(ctx context.Context, projectID uuid.UUID, uploadedBy uuid.UUID, req CreateAssetRequest) (*model.ProjectAsset, error) {
	if req.Category == "" || req.Filename == "" || req.FileURL == "" {
		return nil, fmt.Errorf("category, filename, and file_url are required")
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	var a model.ProjectAsset
	err := s.pool.QueryRow(ctx,
		`INSERT INTO project_assets (project_id, category, subcategory, filename, file_url,
			thumbnail_url, alt_text, file_size, mime_type, metadata, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+assetColumns,
		projectID, req.Category, req.Subcategory, req.Filename, req.FileURL,
		nilIfEmpty(req.ThumbnailURL), req.AltText, req.FileSize, req.MimeType,
		metadata, uploadedBy,
	).Scan(&a.ID, &a.ProjectID, &a.Category, &a.Subcategory, &a.Filename, &a.FileURL,
		&a.ThumbnailURL, &a.AltText, &a.FileSize, &a.MimeType,
		&a.Metadata, &a.SortOrder, &a.Status, &a.Version,
		&a.UploadedBy, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("create asset: %w", err)
	}
	return &a, nil
}

// DeleteAsset soft-deletes an asset.
func (s *ProjectAssetService) DeleteAsset(ctx context.Context, projectID, assetID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE project_assets SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
		WHERE id = $1 AND project_id = $2`, assetID, projectID)
	if err != nil {
		return fmt.Errorf("delete asset: %w", err)
	}
	return nil
}

// BulkDeleteAssets soft-deletes multiple assets.
func (s *ProjectAssetService) BulkDeleteAssets(ctx context.Context, projectID uuid.UUID, assetIDs []uuid.UUID) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE project_assets SET deleted_at = NOW(), status = 'deleted', updated_at = NOW()
		WHERE project_id = $1 AND id = ANY($2)`, projectID, assetIDs)
	if err != nil {
		return 0, fmt.Errorf("bulk delete: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// RestoreAsset restores a soft-deleted asset.
func (s *ProjectAssetService) RestoreAsset(ctx context.Context, projectID, assetID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE project_assets SET deleted_at = NULL, status = 'active', updated_at = NOW()
		WHERE id = $1 AND project_id = $2 AND deleted_at IS NOT NULL`, assetID, projectID)
	if err != nil {
		return fmt.Errorf("restore asset: %w", err)
	}
	return nil
}

// RenameAsset renames a file.
func (s *ProjectAssetService) RenameAsset(ctx context.Context, projectID, assetID uuid.UUID, req RenameAssetRequest) error {
	if req.Filename == "" {
		return fmt.Errorf("filename is required")
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE project_assets SET filename = $1, updated_at = NOW()
		WHERE id = $2 AND project_id = $3`, req.Filename, assetID, projectID)
	if err != nil {
		return fmt.Errorf("rename asset: %w", err)
	}
	return nil
}

// MoveAsset changes category/subcategory.
func (s *ProjectAssetService) MoveAsset(ctx context.Context, projectID, assetID uuid.UUID, req MoveAssetRequest) error {
	if req.Category == "" {
		return fmt.Errorf("category is required")
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE project_assets SET category = $1, subcategory = $2, updated_at = NOW()
		WHERE id = $3 AND project_id = $4`, req.Category, req.Subcategory, assetID, projectID)
	if err != nil {
		return fmt.Errorf("move asset: %w", err)
	}
	return nil
}

// ListDeletedAssets returns soft-deleted assets (trash).
func (s *ProjectAssetService) ListDeletedAssets(ctx context.Context, projectID uuid.UUID) ([]model.ProjectAsset, error) {
	query := `SELECT ` + assetColumns + ` FROM project_assets
		WHERE project_id = $1 AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC LIMIT 100`

	rows, err := s.pool.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("list deleted: %w", err)
	}
	defer rows.Close()

	return scanAssets(rows)
}

// PurgeDeleted permanently removes assets deleted more than 30 days ago.
func (s *ProjectAssetService) PurgeDeleted(ctx context.Context, projectID uuid.UUID) (int, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM project_assets
		WHERE project_id = $1 AND deleted_at IS NOT NULL AND deleted_at < NOW() - INTERVAL '30 days'`,
		projectID)
	if err != nil {
		return 0, fmt.Errorf("purge deleted: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// GetKitStatus returns the kit generation status for a project.
func (s *ProjectAssetService) GetKitStatus(ctx context.Context, projectID uuid.UUID) (*KitStatusResponse, error) {
	var status string
	var progress json.RawMessage
	var completedAt *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(kit_status, 'not_started'), COALESCE(kit_progress, '{}'), kit_completed_at
		FROM projects WHERE id = $1`, projectID,
	).Scan(&status, &progress, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("get kit status: %w", err)
	}

	return &KitStatusResponse{
		Status:      status,
		Progress:    progress,
		CompletedAt: completedAt,
	}, nil
}

// UpdateKitStatus updates the kit generation status.
func (s *ProjectAssetService) UpdateKitStatus(ctx context.Context, projectID uuid.UUID, req UpdateKitStatusRequest) error {
	progress := req.Progress
	if progress == nil {
		progress = json.RawMessage(`{}`)
	}

	sql := `UPDATE projects SET kit_status = $1, kit_progress = $2, updated_at = NOW()`
	args := []any{req.Status, progress}

	if req.Status == "complete" {
		sql += `, kit_completed_at = NOW()`
	}
	sql += fmt.Sprintf(` WHERE id = $%d`, len(args)+1)
	args = append(args, projectID)

	_, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("update kit status: %w", err)
	}
	return nil
}

// AddChangelog adds a changelog entry.
func (s *ProjectAssetService) AddChangelog(ctx context.Context, projectID uuid.UUID, req AddChangelogRequest) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO project_asset_changelog (project_id, action, category, asset_count, summary, triggered_by)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, req.Action, req.Category, req.AssetCount, req.Summary, req.TriggeredBy)
	if err != nil {
		return fmt.Errorf("add changelog: %w", err)
	}
	return nil
}

// ListChangelog returns recent changelog entries for a project.
func (s *ProjectAssetService) ListChangelog(ctx context.Context, projectID uuid.UUID, limit int) ([]model.ChangelogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+changelogColumns+` FROM project_asset_changelog
		WHERE project_id = $1 ORDER BY created_at DESC LIMIT $2`,
		projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list changelog: %w", err)
	}
	defer rows.Close()

	var entries []model.ChangelogEntry
	for rows.Next() {
		var e model.ChangelogEntry
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Action, &e.Category, &e.AssetCount,
			&e.Summary, &e.TriggeredBy, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan changelog: %w", err)
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []model.ChangelogEntry{}
	}
	return entries, nil
}

// Helpers

func scanAssets(rows pgx.Rows) ([]model.ProjectAsset, error) {
	var assets []model.ProjectAsset
	for rows.Next() {
		var a model.ProjectAsset
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Category, &a.Subcategory,
			&a.Filename, &a.FileURL, &a.ThumbnailURL, &a.AltText,
			&a.FileSize, &a.MimeType, &a.Metadata, &a.SortOrder,
			&a.Status, &a.Version, &a.UploadedBy, &a.DeletedAt,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}
	if assets == nil {
		assets = []model.ProjectAsset{}
	}
	return assets, nil
}

func groupByCategory(assets []model.ProjectAsset) []model.ProjectAssetTree {
	catMap := make(map[string]*model.ProjectAssetTree)
	var order []string

	for _, a := range assets {
		key := a.Category
		tree, ok := catMap[key]
		if !ok {
			displayName := model.CategoryDisplayNames[key]
			if displayName == "" {
				displayName = key
			}
			tree = &model.ProjectAssetTree{
				Category:    key,
				DisplayName: displayName,
				Assets:      []model.ProjectAsset{},
			}
			catMap[key] = tree
			order = append(order, key)
		}
		tree.Assets = append(tree.Assets, a)
		tree.TotalSize += a.FileSize
		tree.Count++
	}

	result := make([]model.ProjectAssetTree, 0, len(order))
	for _, key := range order {
		result = append(result, *catMap[key])
	}
	return result
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
