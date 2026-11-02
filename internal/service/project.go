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

type CreateSelfProjectRequest struct {
	Title string `json:"name"`
}

type UpdateProjectRequest struct {
	Title       *string   `json:"name"`
	Description *string   `json:"description"`
	SubjectName *string   `json:"subject_name"`
	SubjectType *string   `json:"subject_type"`
	Tags        *[]string `json:"tags"`
}

type RequestEditingRequest struct {
	EditorID uuid.UUID `json:"editor_id"`
	Notes    string    `json:"notes"`
}

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

func (s *ProjectService) CreateSelfProject(ctx context.Context, creatorID uuid.UUID, req CreateSelfProjectRequest) (*model.Project, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("name is required")
	}

	var a model.Project
	err := s.pool.QueryRow(ctx,
		`INSERT INTO projects (title, slug, description, subject_name, subject_type,
		    budget_amount, budget_currency, status, creator_id, created_by,
		    project_type, publish_mode)
		 VALUES ($1, $2, '', '', '', 0, 'INR', 'active', $3, $3, 'self', 'own_channel')
		 RETURNING `+projectColumns,
		req.Title, generateProjectSlug(req.Title), creatorID,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert self project: %w", err)
	}
	return &a, nil
}

func (s *ProjectService) ListByCreator(ctx context.Context, creatorID uuid.UUID, status string, limit, offset int) ([]model.Project, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := "WHERE creator_id = $1"
	args = append(args, creatorID)
	argIdx := 2

	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM projects "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT `+projectColumns+`
		 FROM projects %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	return scanProjects(rows, total)
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

func (s *ProjectService) Update(ctx context.Context, projectID, userID uuid.UUID, isAdmin bool, req UpdateProjectRequest) (*model.Project, error) {
	current, err := s.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if !isAdmin && (current.CreatorID == nil || *current.CreatorID != userID) {
		return nil, fmt.Errorf("not authorized to edit this project")
	}

	// Only allow editing when project is active
	if current.Status != model.ProjectActive {
		return nil, fmt.Errorf("project cannot be edited in current status")
	}

	sets := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Title != nil && *req.Title != "" {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.SubjectName != nil {
		sets = append(sets, fmt.Sprintf("subject_name = $%d", argIdx))
		args = append(args, *req.SubjectName)
		argIdx++
	}
	if req.SubjectType != nil {
		sets = append(sets, fmt.Sprintf("subject_type = $%d", argIdx))
		args = append(args, *req.SubjectType)
		argIdx++
	}
	if req.Tags != nil {
		sets = append(sets, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, *req.Tags)
		argIdx++
	}

	if len(sets) == 0 {
		return current, nil
	}

	sets = append(sets, "updated_at = NOW()")
	query := fmt.Sprintf(
		`UPDATE projects SET %s WHERE id = $%d
		 RETURNING `+projectColumns,
		strings.Join(sets, ", "), argIdx)
	args = append(args, projectID)

	var a model.Project
	err = s.pool.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("update project: %w", err)
	}
	return &a, nil
}

func (s *ProjectService) ListByEditor(ctx context.Context, editorID uuid.UUID, status string, limit, offset int) ([]model.Project, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var args []interface{}
	where := "WHERE editor_id = $1"
	args = append(args, editorID)
	argIdx := 2

	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM projects "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT `+projectColumns+`
		 FROM projects %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	return scanProjects(rows, total)
}

// RequestEditing sets the editor on a project and marks editing as requested.
func (s *ProjectService) RequestEditing(ctx context.Context, projectID, creatorID, editorID uuid.UUID, notes string) (*model.Project, error) {
	current, err := s.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if current.CreatorID == nil || *current.CreatorID != creatorID {
		return nil, fmt.Errorf("not authorized: you are not the creator")
	}
	if current.Status != model.ProjectActive {
		return nil, fmt.Errorf("editing can only be requested on active projects")
	}

	var a model.Project
	err = s.pool.QueryRow(ctx,
		`UPDATE projects SET editor_id = $1, editing_requested = TRUE, editing_notes = $2, editing_completed_at = NULL, updated_at = NOW()
		 WHERE id = $3
		 RETURNING `+projectColumns,
		editorID, notes, projectID,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("request editing: %w", err)
	}
	return &a, nil
}

// MarkEditingDone marks editing as completed by the editor.
func (s *ProjectService) MarkEditingDone(ctx context.Context, projectID, editorID uuid.UUID) (*model.Project, error) {
	current, err := s.GetByID(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if current.EditorID == nil || *current.EditorID != editorID {
		return nil, fmt.Errorf("not authorized: you are not the assigned editor")
	}
	if current.Status != model.ProjectActive || !current.EditingRequested {
		return nil, fmt.Errorf("no active editing request on this project")
	}

	var a model.Project
	err = s.pool.QueryRow(ctx,
		`UPDATE projects SET editing_completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1
		 RETURNING `+projectColumns,
		projectID,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("mark editing done: %w", err)
	}
	return &a, nil
}

// ListReviewQueue returns projects in 'in_review' status for admin approval.
func (s *ProjectService) ListReviewQueue(ctx context.Context, limit, offset int) ([]model.Project, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE status = 'in_review'`,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count review queue: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+projectColumns+`
		 FROM projects WHERE status = 'in_review'
		 ORDER BY updated_at ASC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query review queue: %w", err)
	}
	defer rows.Close()

	return scanProjects(rows, total)
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

// ListOpenAssignments returns open assignment briefs for the content brief feed.
func (s *ProjectService) ListOpenAssignments(ctx context.Context, limit, offset int) ([]model.Project, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE status = 'open' AND project_type = 'assignment'`,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count open assignments: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+projectColumns+`
		 FROM projects WHERE status = 'open' AND project_type = 'assignment'
		 ORDER BY priority DESC, created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query open assignments: %w", err)
	}
	defer rows.Close()

	return scanProjects(rows, total)
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

// ClaimAssignment allows a creator to claim an open assignment.
func (s *ProjectService) ClaimAssignment(ctx context.Context, projectID, creatorID uuid.UUID) (*model.Project, error) {
	var a model.Project
	err := s.pool.QueryRow(ctx,
		`UPDATE projects
		 SET status = 'active', creator_id = $1, claimed_at = NOW(), agreement_accepted = TRUE, updated_at = NOW()
		 WHERE id = $2 AND status = 'open' AND project_type = 'assignment'
		 RETURNING `+projectColumns,
		creatorID, projectID,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("assignment not found or already claimed")
	}
	if err != nil {
		return nil, fmt.Errorf("claim assignment: %w", err)
	}
	return &a, nil
}
