package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type EditorAlignmentService struct {
	pool *pgxpool.Pool
}

func NewEditorAlignmentService(pool *pgxpool.Pool) *EditorAlignmentService {
	return &EditorAlignmentService{pool: pool}
}

type CreateAlignmentRequest struct {
	CreatorID uuid.UUID `json:"creator_id"`
	EditorID  uuid.UUID `json:"editor_id"`
}

func (s *EditorAlignmentService) Create(ctx context.Context, alignedBy uuid.UUID, req CreateAlignmentRequest) (*model.EditorAlignmentWithNames, error) {
	var ea model.EditorAlignment
	err := s.pool.QueryRow(ctx,
		`INSERT INTO editor_alignments (creator_id, editor_id, aligned_by)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (creator_id, editor_id) DO UPDATE SET is_active = TRUE
		 RETURNING id, creator_id, editor_id, aligned_by, is_active, created_at`,
		req.CreatorID, req.EditorID, alignedBy,
	).Scan(&ea.ID, &ea.CreatorID, &ea.EditorID, &ea.AlignedBy, &ea.IsActive, &ea.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create editor alignment: %w", err)
	}

	return s.withNames(ctx, ea)
}

func (s *EditorAlignmentService) Delete(ctx context.Context, alignmentID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE editor_alignments SET is_active = FALSE WHERE id = $1`, alignmentID)
	if err != nil {
		return fmt.Errorf("deactivate alignment: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("alignment not found")
	}
	return nil
}

func (s *EditorAlignmentService) ListByCreator(ctx context.Context, creatorID uuid.UUID) ([]model.EditorAlignmentWithNames, error) {
	return s.list(ctx, "creator_id", creatorID)
}

func (s *EditorAlignmentService) ListByEditor(ctx context.Context, editorID uuid.UUID) ([]model.EditorAlignmentWithNames, error) {
	return s.list(ctx, "editor_id", editorID)
}

func (s *EditorAlignmentService) list(ctx context.Context, filterCol string, filterID uuid.UUID) ([]model.EditorAlignmentWithNames, error) {
	query := fmt.Sprintf(`
		SELECT ea.id, ea.creator_id, ea.editor_id, ea.aligned_by, ea.is_active, ea.created_at,
		       COALESCE(cu.display_name, cu.name) AS creator_name,
		       COALESCE(eu.display_name, eu.name) AS editor_name,
		       COALESCE(au.display_name, au.name) AS aligned_by_name
		FROM editor_alignments ea
		JOIN users cu ON cu.id = ea.creator_id
		JOIN users eu ON eu.id = ea.editor_id
		JOIN users au ON au.id = ea.aligned_by
		WHERE ea.%s = $1 AND ea.is_active = TRUE
		ORDER BY ea.created_at DESC`, filterCol)

	rows, err := s.pool.Query(ctx, query, filterID)
	if err != nil {
		return nil, fmt.Errorf("list alignments: %w", err)
	}
	defer rows.Close()

	var results []model.EditorAlignmentWithNames
	for rows.Next() {
		var r model.EditorAlignmentWithNames
		if err := rows.Scan(&r.ID, &r.CreatorID, &r.EditorID, &r.AlignedBy, &r.IsActive, &r.CreatedAt,
			&r.CreatorName, &r.EditorName, &r.AlignedByName); err != nil {
			return nil, fmt.Errorf("scan alignment: %w", err)
		}
		results = append(results, r)
	}
	if results == nil {
		results = []model.EditorAlignmentWithNames{}
	}
	return results, nil
}

func (s *EditorAlignmentService) withNames(ctx context.Context, ea model.EditorAlignment) (*model.EditorAlignmentWithNames, error) {
	r := &model.EditorAlignmentWithNames{EditorAlignment: ea}

	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, name) FROM users WHERE id = $1`, ea.CreatorID,
	).Scan(&r.CreatorName)

	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, name) FROM users WHERE id = $1`, ea.EditorID,
	).Scan(&r.EditorName)

	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(display_name, name) FROM users WHERE id = $1`, ea.AlignedBy,
	).Scan(&r.AlignedByName)

	return r, nil
}

// GetByID returns a single alignment by its ID.
func (s *EditorAlignmentService) GetByID(ctx context.Context, id uuid.UUID) (*model.EditorAlignment, error) {
	var ea model.EditorAlignment
	err := s.pool.QueryRow(ctx,
		`SELECT id, creator_id, editor_id, aligned_by, is_active, created_at
		 FROM editor_alignments WHERE id = $1`, id,
	).Scan(&ea.ID, &ea.CreatorID, &ea.EditorID, &ea.AlignedBy, &ea.IsActive, &ea.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("alignment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get alignment: %w", err)
	}
	return &ea, nil
}
