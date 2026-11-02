package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ApprovalService struct {
	pool *pgxpool.Pool
}

func NewApprovalService(pool *pgxpool.Pool) *ApprovalService {
	return &ApprovalService{pool: pool}
}

// ── Workflow CRUD ───────────────────────────────────────────────────────

// CreateWorkflow validates the step list and inserts a new approval workflow.
func (s *ApprovalService) CreateWorkflow(ctx context.Context, ownerID uuid.UUID, req *model.CreateWorkflowRequest) (*model.ApprovalWorkflow, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(req.Steps) == 0 {
		return nil, fmt.Errorf("%w: at least one step is required", ErrInvalidInput)
	}

	// Normalize step numbers.
	for i := range req.Steps {
		req.Steps[i].StepNumber = i + 1
	}

	stepsJSON, err := json.Marshal(req.Steps)
	if err != nil {
		return nil, fmt.Errorf("marshal steps: %w", err)
	}

	var wf model.ApprovalWorkflow
	err = s.pool.QueryRow(ctx,
		`INSERT INTO approval_workflows (owner_id, name, steps, auto_apply)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, owner_id, name, steps, is_active, auto_apply, created_at, updated_at`,
		ownerID, req.Name, stepsJSON, req.AutoApply,
	).Scan(
		&wf.ID, &wf.OwnerID, &wf.Name, &stepsJSON,
		&wf.IsActive, &wf.AutoApply, &wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert workflow: %w", err)
	}

	if err := json.Unmarshal(stepsJSON, &wf.Steps); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}
	return &wf, nil
}

// GetWorkflow returns a single workflow owned by ownerID.
func (s *ApprovalService) GetWorkflow(ctx context.Context, ownerID, workflowID uuid.UUID) (*model.ApprovalWorkflow, error) {
	var wf model.ApprovalWorkflow
	var stepsJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, owner_id, name, steps, is_active, auto_apply, created_at, updated_at
		 FROM approval_workflows
		 WHERE id = $1 AND owner_id = $2`,
		workflowID, ownerID,
	).Scan(
		&wf.ID, &wf.OwnerID, &wf.Name, &stepsJSON,
		&wf.IsActive, &wf.AutoApply, &wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: workflow not found", ErrNotFound)
	}

	if err := json.Unmarshal(stepsJSON, &wf.Steps); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}
	return &wf, nil
}

// ListWorkflows returns all workflows for an owner.
func (s *ApprovalService) ListWorkflows(ctx context.Context, ownerID uuid.UUID) ([]model.ApprovalWorkflow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, owner_id, name, steps, is_active, auto_apply, created_at, updated_at
		 FROM approval_workflows
		 WHERE owner_id = $1
		 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}
	defer rows.Close()

	var workflows []model.ApprovalWorkflow
	for rows.Next() {
		var wf model.ApprovalWorkflow
		var stepsJSON []byte
		if err := rows.Scan(
			&wf.ID, &wf.OwnerID, &wf.Name, &stepsJSON,
			&wf.IsActive, &wf.AutoApply, &wf.CreatedAt, &wf.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workflow: %w", err)
		}
		if err := json.Unmarshal(stepsJSON, &wf.Steps); err != nil {
			return nil, fmt.Errorf("unmarshal steps: %w", err)
		}
		workflows = append(workflows, wf)
	}
	return workflows, nil
}

// UpdateWorkflow replaces the name, steps, and auto_apply flag of an existing workflow.
func (s *ApprovalService) UpdateWorkflow(ctx context.Context, ownerID, workflowID uuid.UUID, req *model.CreateWorkflowRequest) (*model.ApprovalWorkflow, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(req.Steps) == 0 {
		return nil, fmt.Errorf("%w: at least one step is required", ErrInvalidInput)
	}

	for i := range req.Steps {
		req.Steps[i].StepNumber = i + 1
	}

	stepsJSON, err := json.Marshal(req.Steps)
	if err != nil {
		return nil, fmt.Errorf("marshal steps: %w", err)
	}

	var wf model.ApprovalWorkflow
	err = s.pool.QueryRow(ctx,
		`UPDATE approval_workflows
		 SET name = $3, steps = $4, auto_apply = $5, updated_at = NOW()
		 WHERE id = $1 AND owner_id = $2
		 RETURNING id, owner_id, name, steps, is_active, auto_apply, created_at, updated_at`,
		workflowID, ownerID, req.Name, stepsJSON, req.AutoApply,
	).Scan(
		&wf.ID, &wf.OwnerID, &wf.Name, &stepsJSON,
		&wf.IsActive, &wf.AutoApply, &wf.CreatedAt, &wf.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: workflow not found", ErrNotFound)
	}

	if err := json.Unmarshal(stepsJSON, &wf.Steps); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}
	return &wf, nil
}

// DeleteWorkflow soft-deletes a workflow by setting is_active to false.
func (s *ApprovalService) DeleteWorkflow(ctx context.Context, ownerID, workflowID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE approval_workflows
		 SET is_active = FALSE, updated_at = NOW()
		 WHERE id = $1 AND owner_id = $2`,
		workflowID, ownerID,
	)
	if err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: workflow not found", ErrNotFound)
	}
	return nil
}

// ── Approval Request Lifecycle ──────────────────────────────────────────

// SubmitForApproval creates a new approval request for a post. If no
// WorkflowID is provided, it looks for the owner's auto-apply workflow.
func (s *ApprovalService) SubmitForApproval(ctx context.Context, submitterID, postID uuid.UUID, req *model.SubmitApprovalRequest) (*model.ApprovalRequest, error) {
	// Verify the post exists and get the author.
	var authorID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT author_id FROM posts WHERE id = $1`, postID,
	).Scan(&authorID)
	if err != nil {
		return nil, fmt.Errorf("%w: post not found", ErrNotFound)
	}

	// Determine which workflow to use.
	var workflowID uuid.UUID
	if req != nil && req.WorkflowID != nil {
		workflowID = *req.WorkflowID
	} else {
		// Find the owner's default (auto-apply) active workflow.
		err = s.pool.QueryRow(ctx,
			`SELECT id FROM approval_workflows
			 WHERE owner_id = $1 AND is_active = TRUE AND auto_apply = TRUE
			 ORDER BY created_at DESC LIMIT 1`,
			authorID,
		).Scan(&workflowID)
		if err != nil {
			return nil, fmt.Errorf("%w: no default workflow found; specify workflow_id", ErrInvalidInput)
		}
	}

	// Validate the workflow exists and is active.
	var stepsJSON []byte
	err = s.pool.QueryRow(ctx,
		`SELECT steps FROM approval_workflows WHERE id = $1 AND is_active = TRUE`,
		workflowID,
	).Scan(&stepsJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: workflow not found or inactive", ErrNotFound)
	}

	var steps []model.WorkflowStep
	if err := json.Unmarshal(stepsJSON, &steps); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: workflow has no steps", ErrInvalidInput)
	}

	// Create the approval request.
	var ar model.ApprovalRequest
	err = s.pool.QueryRow(ctx,
		`INSERT INTO approval_requests (post_id, workflow_id, submitted_by, current_step, status)
		 VALUES ($1, $2, $3, 1, 'in_review')
		 RETURNING id, post_id, workflow_id, submitted_by, current_step, status, created_at, updated_at`,
		postID, workflowID, submitterID,
	).Scan(
		&ar.ID, &ar.PostID, &ar.WorkflowID, &ar.SubmittedBy,
		&ar.CurrentStep, &ar.Status, &ar.CreatedAt, &ar.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert approval request: %w", err)
	}

	ar.Actions = []model.ApprovalActionRecord{}
	return &ar, nil
}

// TakeAction records an approve/reject/request_changes action on the current step
// of an approval request. It advances the step (or sets terminal status) as needed.
func (s *ApprovalService) TakeAction(ctx context.Context, actorID, requestID uuid.UUID, req *model.ApprovalActionRequest) (*model.ApprovalRequest, error) {
	if !model.ValidApprovalActions[req.Action] {
		return nil, fmt.Errorf("%w: invalid action %q", ErrInvalidInput, req.Action)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the approval request row for update.
	var ar model.ApprovalRequest
	var workflowID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT id, post_id, workflow_id, submitted_by, current_step, status, created_at, updated_at
		 FROM approval_requests
		 WHERE id = $1
		 FOR UPDATE`,
		requestID,
	).Scan(
		&ar.ID, &ar.PostID, &ar.WorkflowID, &ar.SubmittedBy,
		&ar.CurrentStep, &ar.Status, &ar.CreatedAt, &ar.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: approval request not found", ErrNotFound)
	}
	workflowID = ar.WorkflowID

	// Cannot act on terminal statuses.
	if ar.Status == model.ApprovalApproved || ar.Status == model.ApprovalRejected {
		return nil, fmt.Errorf("%w: request is already %s", ErrInvalidInput, ar.Status)
	}

	// Load workflow steps.
	var stepsJSON []byte
	err = tx.QueryRow(ctx,
		`SELECT steps FROM approval_workflows WHERE id = $1`,
		workflowID,
	).Scan(&stepsJSON)
	if err != nil {
		return nil, fmt.Errorf("load workflow: %w", err)
	}

	var steps []model.WorkflowStep
	if err := json.Unmarshal(stepsJSON, &steps); err != nil {
		return nil, fmt.Errorf("unmarshal steps: %w", err)
	}

	// Validate the actor can act on the current step.
	// We need the actor's role on the team. Look it up from team_members.
	var actorRole string
	err = tx.QueryRow(ctx,
		`SELECT role FROM team_members
		 WHERE owner_id = (SELECT author_id FROM posts WHERE id = $1)
		   AND member_id = $2 AND status = 'active'`,
		ar.PostID, actorID,
	).Scan(&actorRole)
	if err != nil {
		// Also allow the post author (owner) to act.
		var authorID uuid.UUID
		_ = tx.QueryRow(ctx, `SELECT author_id FROM posts WHERE id = $1`, ar.PostID).Scan(&authorID)
		if authorID == actorID {
			actorRole = "admin" // owner has full privileges
		} else {
			return nil, fmt.Errorf("%w: you are not a team member", ErrForbidden)
		}
	}

	if !canActOnStep(steps, ar.CurrentStep, actorID, actorRole) {
		return nil, fmt.Errorf("%w: you cannot act on step %d", ErrForbidden, ar.CurrentStep)
	}

	// Record the action.
	_, err = tx.Exec(ctx,
		`INSERT INTO approval_actions (request_id, step_number, actor_id, action, comment)
		 VALUES ($1, $2, $3, $4, $5)`,
		requestID, ar.CurrentStep, actorID, req.Action, req.Comment,
	)
	if err != nil {
		return nil, fmt.Errorf("insert action: %w", err)
	}

	// Determine new status based on action.
	switch req.Action {
	case model.ActionApprove:
		if advanceStep(&ar, steps) {
			ar.Status = model.ApprovalApproved
		} else {
			ar.Status = model.ApprovalInReview
		}
	case model.ActionReject:
		ar.Status = model.ApprovalRejected
	case model.ActionRequestChanges:
		ar.Status = model.ApprovalChangesRequested
	}

	// Update the request.
	_, err = tx.Exec(ctx,
		`UPDATE approval_requests
		 SET current_step = $2, status = $3, updated_at = NOW()
		 WHERE id = $1`,
		requestID, ar.CurrentStep, ar.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("update request: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Load actions for the response.
	ar.Actions, _ = s.loadActions(ctx, requestID)
	return &ar, nil
}

// GetApprovalStatus returns the most recent approval request for a post.
func (s *ApprovalService) GetApprovalStatus(ctx context.Context, postID uuid.UUID) (*model.ApprovalRequest, error) {
	var ar model.ApprovalRequest
	err := s.pool.QueryRow(ctx,
		`SELECT id, post_id, workflow_id, submitted_by, current_step, status, created_at, updated_at
		 FROM approval_requests
		 WHERE post_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		postID,
	).Scan(
		&ar.ID, &ar.PostID, &ar.WorkflowID, &ar.SubmittedBy,
		&ar.CurrentStep, &ar.Status, &ar.CreatedAt, &ar.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: no approval request for this post", ErrNotFound)
	}

	ar.Actions, _ = s.loadActions(ctx, ar.ID)
	return &ar, nil
}

// ListPendingApprovals returns all in_review requests where the given user is
// a valid approver for the current step.
func (s *ApprovalService) ListPendingApprovals(ctx context.Context, approverID uuid.UUID) ([]model.ApprovalRequest, error) {
	// Find all requests in "in_review" status, then filter client-side based
	// on step matching. This approach is simpler than a complex JSONB query.
	rows, err := s.pool.Query(ctx,
		`SELECT ar.id, ar.post_id, ar.workflow_id, ar.submitted_by,
		        ar.current_step, ar.status, ar.created_at, ar.updated_at,
		        aw.steps,
		        COALESCE(tm.role, '') AS member_role
		 FROM approval_requests ar
		 JOIN approval_workflows aw ON aw.id = ar.workflow_id
		 JOIN posts p ON p.id = ar.post_id
		 LEFT JOIN team_members tm ON tm.owner_id = p.author_id
		   AND tm.member_id = $1 AND tm.status = 'active'
		 WHERE ar.status = 'in_review'
		 ORDER BY ar.created_at DESC`,
		approverID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending approvals: %w", err)
	}
	defer rows.Close()

	var results []model.ApprovalRequest
	for rows.Next() {
		var ar model.ApprovalRequest
		var stepsJSON []byte
		var memberRole string
		if err := rows.Scan(
			&ar.ID, &ar.PostID, &ar.WorkflowID, &ar.SubmittedBy,
			&ar.CurrentStep, &ar.Status, &ar.CreatedAt, &ar.UpdatedAt,
			&stepsJSON, &memberRole,
		); err != nil {
			return nil, fmt.Errorf("scan pending approval: %w", err)
		}

		var steps []model.WorkflowStep
		if err := json.Unmarshal(stepsJSON, &steps); err != nil {
			continue
		}

		if canActOnStep(steps, ar.CurrentStep, approverID, memberRole) {
			results = append(results, ar)
		}
	}
	return results, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────

// canActOnStep returns true if the actor (identified by ID and role) can
// take action on the given step of a workflow.
func canActOnStep(steps []model.WorkflowStep, stepNumber int, actorID uuid.UUID, actorRole string) bool {
	for _, step := range steps {
		if step.StepNumber != stepNumber {
			continue
		}

		// If a specific approver ID is set, check that first.
		if step.ApproverID != nil && *step.ApproverID == actorID {
			return true
		}

		// Otherwise match by role.
		if step.ApproverRole != "" && step.ApproverRole == actorRole {
			return true
		}

		return false
	}
	return false // step not found
}

// advanceStep increments the current_step on the request, skipping optional
// steps. Returns true if all steps are completed (i.e. the request should
// be approved).
func advanceStep(request *model.ApprovalRequest, steps []model.WorkflowStep) bool {
	nextStep := request.CurrentStep + 1

	for nextStep <= len(steps) {
		step := steps[nextStep-1]
		if step.Required {
			request.CurrentStep = nextStep
			return false // more required steps remain
		}
		// Skip optional step.
		nextStep++
	}

	// All remaining steps were optional (or no more steps).
	request.CurrentStep = len(steps)
	return true
}

// loadActions fetches all action records for a given request.
func (s *ApprovalService) loadActions(ctx context.Context, requestID uuid.UUID) ([]model.ApprovalActionRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, request_id, step_number, actor_id, action, comment, created_at
		 FROM approval_actions
		 WHERE request_id = $1
		 ORDER BY created_at ASC`,
		requestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []model.ApprovalActionRecord
	for rows.Next() {
		var a model.ApprovalActionRecord
		if err := rows.Scan(
			&a.ID, &a.RequestID, &a.StepNumber, &a.ActorID,
			&a.Action, &a.Comment, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		actions = append(actions, a)
	}
	return actions, nil
}
