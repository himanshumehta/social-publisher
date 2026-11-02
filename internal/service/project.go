package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ProjectService struct {
	pool *pgxpool.Pool
}

func NewProjectService(pool *pgxpool.Pool) *ProjectService {
	return &ProjectService{pool: pool}
}

type CreateProjectRequest struct {
	CategoryID     uuid.UUID       `json:"category_id"`
	Title          string          `json:"name"`
	Description    string          `json:"description"`
	SubjectName    string          `json:"subject_name"`
	SubjectType    string          `json:"subject_type"`
	SubjectRefID   string          `json:"subject_ref_id"`
	Requirements   json.RawMessage `json:"requirements,omitempty"`
	BudgetAmount   int64           `json:"budget_amount"`
	BudgetCurrency string          `json:"budget_currency"`
	DeadlineAt     *time.Time      `json:"deadline_at"`
	PublishTarget  *time.Time      `json:"publish_target"`
	Priority       int             `json:"priority"`
	Tags           []string        `json:"tags"`
	CreatorID      *uuid.UUID      `json:"creator_id,omitempty"`
}

// projectColumns is the shared SELECT column list for projects.
const projectColumns = `id, category_id, title, slug, description, COALESCE(subject_name, ''), COALESCE(subject_type, ''), COALESCE(subject_ref_id, ''),
		    requirements, budget_amount, budget_currency, deadline_at, publish_target, status,
		    creator_id, editor_id, created_by, priority, tags,
		    editing_requested, editing_notes, editing_completed_at,
		    project_type, publish_mode, claimed_at, agreement_accepted,
		    created_at, updated_at`

func generateProjectSlug(title string) string {
	slug := strings.ToLower(title)
	slug = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' {
			return '-'
		}
		return -1
	}, slug)
	parts := strings.Split(slug, "-")
	var filtered []string
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	slug = strings.Join(filtered, "-")
	suffix := uuid.New().String()[:8]
	return slug + "-" + suffix
}

func (s *ProjectService) Create(ctx context.Context, createdBy uuid.UUID, req CreateProjectRequest) (*model.Project, error) {
	if req.Title == "" || req.Description == "" || req.SubjectName == "" {
		return nil, fmt.Errorf("title, description, and subject_name are required")
	}
	currency := req.BudgetCurrency
	if currency == "" {
		currency = "INR"
	}

	// Determine initial status and project type
	projectType := "assignment"
	publishMode := "official"
	initialStatus := "open"
	if req.CreatorID != nil {
		// Pre-assigned to a creator → start as active
		initialStatus = "active"
	}

	reqJSON := req.Requirements
	if reqJSON == nil {
		reqJSON = json.RawMessage("{}")
	}

	var a model.Project
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (category_id, title, slug, description, subject_name, subject_type, subject_ref_id,
		    requirements, budget_amount, budget_currency, deadline_at, publish_target, status, creator_id, created_by, priority, tags,
		    project_type, publish_mode)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		 RETURNING `+projectColumns,
		req.CategoryID, req.Title, generateProjectSlug(req.Title), req.Description,
		req.SubjectName, req.SubjectType, req.SubjectRefID,
		reqJSON, req.BudgetAmount, currency, req.DeadlineAt, req.PublishTarget,
		initialStatus, req.CreatorID, createdBy, req.Priority, req.Tags,
		projectType, publishMode,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert project: %w", err)
	}
	return &a, nil
}

func (s *ProjectService) ListAll(ctx context.Context, status string, limit, offset int) ([]model.Project, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := ""
	argIdx := 1

	if status != "" {
		where = fmt.Sprintf("WHERE status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM projects"
	if where != "" {
		countQuery += " " + where
	}
	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT `+projectColumns+`
		 FROM projects %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	return scanProjects(rows, total)
}

func (s *ProjectService) GetByID(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	var a model.Project
	err := s.pool.QueryRow(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = $1`, id,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &a, nil
}

func (s *ProjectService) GetBySlug(ctx context.Context, slug string) (*model.Project, error) {
	var a model.Project
	err := s.pool.QueryRow(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE slug = $1`, slug,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project by slug: %w", err)
	}
	return &a, nil
}

func scanProjects(rows pgx.Rows, total int) ([]model.Project, int, error) {
	var projects []model.Project
	for rows.Next() {
		var a model.Project
		err := rows.Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
			&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
			&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
			&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
			&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
			&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
			&a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, a)
	}
	if projects == nil {
		projects = []model.Project{}
	}
	return projects, total, nil
}

// Delete hard-deletes a project and its related data.
func (s *ProjectService) Delete(ctx context.Context, projectID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Delete related data in dependency order
	cascadeTables := []string{
		"archive_tasks",
		"channel_targets",
		"publish_records",
		"reviews",
		"comments",
		"videos",
		"posts",
	}
	for _, table := range cascadeTables {
		if _, err := tx.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE project_id = $1", table), projectID); err != nil {
			// Ignore if table doesn't exist (some may not apply)
			continue
		}
	}

	tag, err := tx.Exec(ctx, "DELETE FROM projects WHERE id = $1", projectID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}

	return tx.Commit(ctx)
}
