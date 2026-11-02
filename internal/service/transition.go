package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type TransitionService struct {
	pool        *pgxpool.Pool
	projectPost *ProjectPostService
}

func NewTransitionService(pool *pgxpool.Pool) *TransitionService {
	return &TransitionService{pool: pool}
}

// SetProjectPostService injects the ProjectPostService (avoids circular init).
func (s *TransitionService) SetProjectPostService(pp *ProjectPostService) {
	s.projectPost = pp
}

type TransitionRequest struct {
	ToStatus string     `json:"to_status"`
	EditorID *uuid.UUID `json:"editor_id,omitempty"`
	Comment  string     `json:"comment,omitempty"`
}

type transitionRule struct {
	AllowedRoles   []string
	OwnershipCheck string // "creator_id", "editor_id", or "" (no check)
}

// transitionMap defines all allowed status transitions and their rules.
// Simplified to 4 statuses: active, in_review, published, archived.
var transitionMap = map[string]map[string]transitionRule{
	"open": {
		"active": {AllowedRoles: []string{"creator", "admin", "super_admin"}, OwnershipCheck: ""},
	},
	"active": {
		"in_review": {AllowedRoles: []string{"creator", "admin", "super_admin"}, OwnershipCheck: "creator_id"},
	},
	"in_review": {
		"active":    {AllowedRoles: []string{"admin", "super_admin"}, OwnershipCheck: ""},
		"published": {AllowedRoles: []string{"admin", "super_admin"}, OwnershipCheck: ""},
	},
	"published": {
		"archived": {AllowedRoles: []string{"admin", "super_admin"}, OwnershipCheck: ""},
	},
}

func roleAllowed(role string, allowed []string) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

// projectColumns is the shared SELECT column list for projects.
const projectColumns = `id, category_id, title, slug, description, COALESCE(subject_name, ''), COALESCE(subject_type, ''), COALESCE(subject_ref_id, ''),
		    requirements, budget_amount, budget_currency, deadline_at, publish_target, status,
		    creator_id, editor_id, created_by, priority, tags,
		    editing_requested, editing_notes, editing_completed_at,
		    project_type, publish_mode, claimed_at, agreement_accepted,
		    created_at, updated_at`

func (s *TransitionService) Transition(ctx context.Context, userID uuid.UUID, userRole string, projectID uuid.UUID, req TransitionRequest) (*model.Project, error) {
	// Get current project
	var a model.Project
	err := s.pool.QueryRow(ctx,
		`SELECT `+projectColumns+` FROM projects WHERE id = $1`, projectID,
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

	fromStatus := string(a.Status)

	// Handle archive from any status (admin only)
	if req.ToStatus == "archived" {
		if !roleAllowed(userRole, []string{"admin", "super_admin"}) {
			return nil, fmt.Errorf("role '%s' cannot archive projects", userRole)
		}
	} else {
		// Check transition is allowed via transition map
		targets, ok := transitionMap[fromStatus]
		if !ok {
			return nil, fmt.Errorf("no transitions allowed from status '%s'", fromStatus)
		}
		rule, ok := targets[req.ToStatus]
		if !ok {
			return nil, fmt.Errorf("transition from '%s' to '%s' is not allowed", fromStatus, req.ToStatus)
		}

		// Check role
		if !roleAllowed(userRole, rule.AllowedRoles) {
			return nil, fmt.Errorf("role '%s' cannot perform this transition", userRole)
		}

		// Ownership check
		switch rule.OwnershipCheck {
		case "creator_id":
			if a.CreatorID == nil || *a.CreatorID != userID {
				return nil, fmt.Errorf("you are not the creator of this project")
			}
		case "editor_id":
			if a.EditorID == nil || *a.EditorID != userID {
				return nil, fmt.Errorf("you are not the editor of this project")
			}
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Validate prerequisites before transitioning
	if err := s.validatePrerequisites(ctx, tx, projectID, fromStatus, req.ToStatus); err != nil {
		return nil, err
	}

	// Build update query
	err = tx.QueryRow(ctx,
		`UPDATE projects SET status = $1, updated_at = NOW()
		 WHERE id = $2 AND status = $3
		 RETURNING `+projectColumns,
		req.ToStatus, projectID, fromStatus,
	).Scan(&a.ID, &a.CategoryID, &a.Title, &a.Slug, &a.Description,
		&a.SubjectName, &a.SubjectType, &a.SubjectRefID, &a.Requirements,
		&a.BudgetAmount, &a.BudgetCurrency, &a.DeadlineAt, &a.PublishTarget,
		&a.Status, &a.CreatorID, &a.EditorID, &a.CreatedBy, &a.Priority,
		&a.Tags, &a.EditingRequested, &a.EditingNotes, &a.EditingCompletedAt,
		&a.ProjectType, &a.PublishMode, &a.ClaimedAt, &a.AgreementAccepted,
		&a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project status changed concurrently")
	}
	if err != nil {
		return nil, fmt.Errorf("update project status: %w", err)
	}

	// Auto-publish: convert channel targets to publish records + upload queue on approval
	if req.ToStatus == "published" {
		creatorID := uuid.Nil
		if a.CreatorID != nil {
			creatorID = *a.CreatorID
		}
		if err := s.autoPublish(ctx, tx, projectID, creatorID); err != nil {
			return nil, fmt.Errorf("auto-publish: %w", err)
		}
	}

	// Insert audit review row
	action := "transition"
	_, err = tx.Exec(ctx,
		`INSERT INTO reviews (project_id, reviewer_id, action, comment, from_status, to_status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, userID, action, req.Comment, fromStatus, req.ToStatus)
	if err != nil {
		return nil, fmt.Errorf("insert transition review: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// After transitioning to in_review, auto-generate posts from project content
	if req.ToStatus == "in_review" && s.projectPost != nil {
		go s.projectPost.AutoGeneratePostsForProject(context.Background(), projectID)
	}

	return &a, nil
}

// validatePrerequisites checks that preconditions are met before allowing transitions.
func (s *TransitionService) validatePrerequisites(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, fromStatus, toStatus string) error {
	// Before submitting for review: require at least 1 non-deleted video and at least 1 channel target
	if fromStatus == "active" && toStatus == "in_review" {
		var videoCount int
		err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM videos
			WHERE project_id = $1 AND is_deleted = FALSE
		`, projectID).Scan(&videoCount)
		if err != nil {
			return fmt.Errorf("validate videos: %w", err)
		}
		if videoCount == 0 {
			return fmt.Errorf("cannot submit for review: at least 1 video is required")
		}

		var targetCount int
		err = tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM channel_targets WHERE project_id = $1
		`, projectID).Scan(&targetCount)
		if err != nil {
			return fmt.Errorf("validate channel targets: %w", err)
		}
		if targetCount == 0 {
			return fmt.Errorf("cannot submit for review: at least 1 channel target is required")
		}
	}

	// Before publishing: require at least 1 channel target
	if toStatus == "published" {
		var targetCount int
		err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM channel_targets WHERE project_id = $1
		`, projectID).Scan(&targetCount)
		if err != nil {
			return fmt.Errorf("validate channel targets: %w", err)
		}
		if targetCount == 0 {
			return fmt.Errorf("cannot publish: no channel targets configured")
		}
	}

	return nil
}

// autoPublish converts channel_targets to publish_records and upload_queue entries
// when transitioning to 'published' (admin approves → system auto-publishes).
func (s *TransitionService) autoPublish(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, creatorID uuid.UUID) error {
	var targetCount int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM channel_targets WHERE project_id = $1`, projectID,
	).Scan(&targetCount)
	if err != nil {
		return fmt.Errorf("count channel targets: %w", err)
	}
	if targetCount == 0 {
		return nil
	}

	// Convert channel_targets to publish_records.
	// When video_id is NULL (project-level target), cross-join with the project's videos.
	_, err = tx.Exec(ctx, `
		INSERT INTO publish_records (
			project_id, video_id,
			channel_id, visibility, status, created_at, updated_at
		)
		SELECT ct.project_id, COALESCE(ct.video_id, v.id),
			ct.channel_id, 'public', 'pending', NOW(), NOW()
		FROM channel_targets ct
		LEFT JOIN videos v ON v.project_id = ct.project_id AND v.is_deleted = FALSE
			AND ct.video_id IS NULL
		WHERE ct.project_id = $1
		  AND (ct.video_id IS NOT NULL OR v.id IS NOT NULL)
	`, projectID)
	if err != nil {
		return fmt.Errorf("create publish records from targets: %w", err)
	}

	// Create upload_queue entries for the background worker to pick up
	_, err = tx.Exec(ctx, `
		INSERT INTO upload_queue (publish_record_id, channel_id, video_id, platform, status)
		SELECT pr.id, pr.channel_id, pr.video_id, ch.platform, 'pending'
		FROM publish_records pr
		JOIN channels ch ON ch.id = pr.channel_id
		WHERE pr.project_id = $1 AND pr.status = 'pending'
	`, projectID)
	if err != nil {
		return fmt.Errorf("create upload queue entries: %w", err)
	}

	// Delete processed channel_targets
	_, err = tx.Exec(ctx,
		`DELETE FROM channel_targets WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("delete processed channel targets: %w", err)
	}

	// Auto-approve posts linked to this project
	_, err = tx.Exec(ctx, `
		UPDATE posts SET status = 'approved'::post_status, updated_at = NOW()
		WHERE project_id = $1 AND status IN ('pending_review', 'flagged', 'generated')
	`, projectID)
	if err != nil {
		return fmt.Errorf("auto-approve project posts: %w", err)
	}

	// Notify creator about auto-publish
	if creatorID != uuid.Nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO notifications (user_id, title, body, link, type, is_read)
			VALUES ($1, 'Project Approved & Publishing',
				'Your project has been approved! Publishing to ' || $2 || ' channel(s).',
				'/projects/' || (SELECT slug FROM projects WHERE id = $3),
				'status_change', FALSE)
		`, creatorID, fmt.Sprintf("%d", targetCount), projectID)
		if err != nil {
			return fmt.Errorf("create publish notification: %w", err)
		}
	}

	return nil
}

func (s *TransitionService) GetAllowedTransitions(ctx context.Context, projectID, userID uuid.UUID, userRole string) ([]string, error) {
	var a model.Project
	err := s.pool.QueryRow(ctx,
		`SELECT status, creator_id, editor_id FROM projects WHERE id = $1`, projectID,
	).Scan(&a.Status, &a.CreatorID, &a.EditorID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	fromStatus := string(a.Status)
	targets, ok := transitionMap[fromStatus]
	if !ok {
		// Admin can always archive
		if roleAllowed(userRole, []string{"admin", "super_admin"}) {
			return []string{"archived"}, nil
		}
		return []string{}, nil
	}

	var allowed []string
	for toStatus, rule := range targets {
		if !roleAllowed(userRole, rule.AllowedRoles) {
			continue
		}
		switch rule.OwnershipCheck {
		case "creator_id":
			if a.CreatorID == nil || *a.CreatorID != userID {
				continue
			}
		case "editor_id":
			if a.EditorID == nil || *a.EditorID != userID {
				continue
			}
		}
		allowed = append(allowed, toStatus)
	}

	// Admin can always archive from any non-archived status
	if roleAllowed(userRole, []string{"admin", "super_admin"}) && fromStatus != "archived" {
		allowed = append(allowed, "archived")
	}

	if allowed == nil {
		allowed = []string{}
	}
	return allowed, nil
}
