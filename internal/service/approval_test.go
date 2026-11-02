package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Unit tests for helpers (no DB) ──────────────────────────────────────

func TestCanActOnStep_MatchingRole(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "manager", Required: true},
		{StepNumber: 2, ApproverRole: "admin", Required: true},
	}

	if !canActOnStep(steps, 1, uuid.New(), "manager") {
		t.Error("expected manager to be able to act on step 1")
	}

	if canActOnStep(steps, 1, uuid.New(), "viewer") {
		t.Error("expected viewer to NOT be able to act on step 1")
	}
}

func TestCanActOnStep_MatchingUserID(t *testing.T) {
	specificUser := uuid.New()
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverID: &specificUser, Required: true},
	}

	if !canActOnStep(steps, 1, specificUser, "viewer") {
		t.Error("expected specific user to be able to act on step 1")
	}

	if canActOnStep(steps, 1, uuid.New(), "viewer") {
		t.Error("expected random user to NOT be able to act on step 1")
	}
}

func TestCanActOnStep_NoPermission(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "admin", Required: true},
	}

	if canActOnStep(steps, 1, uuid.New(), "viewer") {
		t.Error("expected viewer to not have permission")
	}

	if canActOnStep(steps, 1, uuid.New(), "contributor") {
		t.Error("expected contributor to not have permission")
	}

	// Non-existent step.
	if canActOnStep(steps, 99, uuid.New(), "admin") {
		t.Error("expected no permission on non-existent step")
	}
}

func TestAdvanceStep_AllCompleted(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "manager", Required: true},
	}
	ar := &model.ApprovalRequest{CurrentStep: 1}

	done := advanceStep(ar, steps)
	if !done {
		t.Error("expected all steps to be completed")
	}
}

func TestAdvanceStep_MoreStepsRemaining(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "contributor", Required: true},
		{StepNumber: 2, ApproverRole: "manager", Required: true},
		{StepNumber: 3, ApproverRole: "admin", Required: true},
	}
	ar := &model.ApprovalRequest{CurrentStep: 1}

	done := advanceStep(ar, steps)
	if done {
		t.Error("expected more steps remaining")
	}
	if ar.CurrentStep != 2 {
		t.Errorf("expected current_step=2, got %d", ar.CurrentStep)
	}
}

func TestAdvanceStep_SkipOptionalStep(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "contributor", Required: true},
		{StepNumber: 2, ApproverRole: "viewer", Required: false}, // optional
		{StepNumber: 3, ApproverRole: "admin", Required: true},
	}
	ar := &model.ApprovalRequest{CurrentStep: 1}

	done := advanceStep(ar, steps)
	if done {
		t.Error("expected more steps remaining (step 3 is required)")
	}
	if ar.CurrentStep != 3 {
		t.Errorf("expected current_step=3 (skipping optional step 2), got %d", ar.CurrentStep)
	}
}

func TestAdvanceStep_AllOptionalAfterCurrent(t *testing.T) {
	steps := []model.WorkflowStep{
		{StepNumber: 1, ApproverRole: "manager", Required: true},
		{StepNumber: 2, ApproverRole: "viewer", Required: false},
		{StepNumber: 3, ApproverRole: "viewer", Required: false},
	}
	ar := &model.ApprovalRequest{CurrentStep: 1}

	done := advanceStep(ar, steps)
	if !done {
		t.Error("expected completion when remaining steps are all optional")
	}
}

// ── Integration-style tests (require DB) ────────────────────────────────

func TestSubmitForApproval_InvalidPost(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewApprovalService(pool)
	ctx := context.Background()

	_, err := svc.SubmitForApproval(ctx, uuid.New(), uuid.New(), &model.SubmitApprovalRequest{})
	if err == nil {
		t.Fatal("expected error for invalid post, got nil")
	}
}

func TestTakeAction_Approve_AdvancesStep(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Create test users.
	ownerID := uuid.New()
	managerID := uuid.New()
	adminID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Approval Owner", "appr-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		managerID, "Approval Manager", "appr-mgr-"+managerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert manager: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		adminID, "Approval Admin", "appr-admin-"+adminID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	// Create team members.
	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
		 VALUES ($1, $2, 'manager', $1, 'active')`,
		ownerID, managerID)
	if err != nil {
		t.Fatalf("insert manager team member: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
		 VALUES ($1, $2, 'admin', $1, 'active')`,
		ownerID, adminID)
	if err != nil {
		t.Fatalf("insert admin team member: %v", err)
	}

	// Create a post.
	var postID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Test Post', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "test-post-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	// Create a 2-step workflow.
	approvalSvc := NewApprovalService(pool)
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name: "Two Step Review",
		Steps: []model.WorkflowStep{
			{ApproverRole: "manager", Required: true},
			{ApproverRole: "admin", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// Submit for approval.
	ar, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit for approval: %v", err)
	}
	if ar.CurrentStep != 1 {
		t.Errorf("expected current_step=1, got %d", ar.CurrentStep)
	}
	if ar.Status != model.ApprovalInReview {
		t.Errorf("expected status=in_review, got %s", ar.Status)
	}

	// Manager approves step 1.
	ar, err = approvalSvc.TakeAction(ctx, managerID, ar.ID, &model.ApprovalActionRequest{
		Action:  model.ActionApprove,
		Comment: "Looks good from manager perspective",
	})
	if err != nil {
		t.Fatalf("manager approve: %v", err)
	}
	if ar.CurrentStep != 2 {
		t.Errorf("expected current_step=2 after manager approve, got %d", ar.CurrentStep)
	}
	if ar.Status != model.ApprovalInReview {
		t.Errorf("expected status=in_review, got %s", ar.Status)
	}

	// Admin approves step 2 -- should complete.
	ar, err = approvalSvc.TakeAction(ctx, adminID, ar.ID, &model.ApprovalActionRequest{
		Action:  model.ActionApprove,
		Comment: "Final approval granted",
	})
	if err != nil {
		t.Fatalf("admin approve: %v", err)
	}
	if ar.Status != model.ApprovalApproved {
		t.Errorf("expected status=approved, got %s", ar.Status)
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2, $3)`, ownerID, managerID, adminID)
}

func TestTakeAction_Reject_SetsStatus(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	ownerID := uuid.New()
	managerID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Reject Owner", "rej-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		managerID, "Reject Manager", "rej-mgr-"+managerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert manager: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
		 VALUES ($1, $2, 'manager', $1, 'active')`,
		ownerID, managerID)
	if err != nil {
		t.Fatalf("insert team member: %v", err)
	}

	var postID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Reject Test', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "reject-test-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	approvalSvc := NewApprovalService(pool)
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name:  "Reject Workflow",
		Steps: []model.WorkflowStep{{ApproverRole: "manager", Required: true}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	ar, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit for approval: %v", err)
	}

	// Manager rejects.
	ar, err = approvalSvc.TakeAction(ctx, managerID, ar.ID, &model.ApprovalActionRequest{
		Action:  model.ActionReject,
		Comment: "Not ready for publication",
	})
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if ar.Status != model.ApprovalRejected {
		t.Errorf("expected status=rejected, got %s", ar.Status)
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, managerID)
}

func TestTakeAction_RequestChanges(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	ownerID := uuid.New()
	managerID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Changes Owner", "chg-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		managerID, "Changes Manager", "chg-mgr-"+managerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert manager: %v", err)
	}

	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
		 VALUES ($1, $2, 'manager', $1, 'active')`,
		ownerID, managerID)
	if err != nil {
		t.Fatalf("insert team member: %v", err)
	}

	var postID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Changes Test', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "changes-test-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	approvalSvc := NewApprovalService(pool)
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name:  "Changes Workflow",
		Steps: []model.WorkflowStep{{ApproverRole: "manager", Required: true}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	ar, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit for approval: %v", err)
	}

	ar, err = approvalSvc.TakeAction(ctx, managerID, ar.ID, &model.ApprovalActionRequest{
		Action:  model.ActionRequestChanges,
		Comment: "Please fix the headline",
	})
	if err != nil {
		t.Fatalf("request changes failed: %v", err)
	}
	if ar.Status != model.ApprovalChangesRequested {
		t.Errorf("expected status=changes_requested, got %s", ar.Status)
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, managerID)
}

func TestListPendingApprovals_FiltersCorrectly(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	ownerID := uuid.New()
	managerID := uuid.New()
	viewerID := uuid.New()

	for _, u := range []struct {
		id   uuid.UUID
		name string
	}{
		{ownerID, "Filter Owner"},
		{managerID, "Filter Manager"},
		{viewerID, "Filter Viewer"},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
			u.id, u.name, "filter-"+u.id.String()[:8]+"@test.local")
		if err != nil {
			t.Fatalf("insert user %s: %v", u.name, err)
		}
	}

	// Manager is on the team; viewer is also on the team but with viewer role.
	_, _ = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status) VALUES ($1, $2, 'manager', $1, 'active')`,
		ownerID, managerID)
	_, _ = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status) VALUES ($1, $2, 'viewer', $1, 'active')`,
		ownerID, viewerID)

	var postID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Filter Test', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "filter-test-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	approvalSvc := NewApprovalService(pool)
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name:  "Filter Workflow",
		Steps: []model.WorkflowStep{{ApproverRole: "manager", Required: true}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	_, err = approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit for approval: %v", err)
	}

	// Manager should see the pending approval.
	pending, err := approvalSvc.ListPendingApprovals(ctx, managerID)
	if err != nil {
		t.Fatalf("list pending (manager): %v", err)
	}
	if len(pending) == 0 {
		t.Error("expected manager to see pending approvals, got none")
	}

	// Viewer should NOT see it (wrong role for the step).
	pending, err = approvalSvc.ListPendingApprovals(ctx, viewerID)
	if err != nil {
		t.Fatalf("list pending (viewer): %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected viewer to see 0 pending approvals, got %d", len(pending))
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2, $3)`, ownerID, managerID, viewerID)
}

// ── Integration tests ───────────────────────────────────────────────────

// TestIntegration_InviteAcceptListMembers exercises the invite -> accept ->
// list flow end-to-end.
func TestIntegration_InviteAcceptListMembers(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	ownerID := uuid.New()
	memberID := uuid.New()
	memberEmail := "integ-member-" + uuid.New().String()[:8] + "@test.local"

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Integ Owner", "integ-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		memberID, "Integ Member", memberEmail)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, memberID)

	teamSvc := NewTeamService(pool)

	// 1. Invite member.
	tm, err := teamSvc.InviteMember(ctx, ownerID, &model.InviteMemberRequest{
		Email: memberEmail,
		Role:  model.TeamRoleContributor,
	})
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if tm.Status != model.TeamMemberPending {
		t.Errorf("expected pending, got %s", tm.Status)
	}

	// 2. Accept invite.
	if err := teamSvc.AcceptInvite(ctx, memberID, tm.ID); err != nil {
		t.Fatalf("accept: %v", err)
	}

	// 3. List members -- should include accepted member.
	members, err := teamSvc.ListMembers(ctx, ownerID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, m := range members {
		if m.MemberID == memberID && m.Status == model.TeamMemberActive {
			found = true
		}
	}
	if !found {
		t.Error("expected to find active member in list")
	}

	// 4. Check membership.
	isMember, err := teamSvc.IsTeamMember(ctx, ownerID, memberID)
	if err != nil {
		t.Fatalf("is team member: %v", err)
	}
	if !isMember {
		t.Error("expected IsTeamMember = true")
	}
}

// TestIntegration_CreateWorkflowSubmitApprovalChain exercises the workflow
// creation -> submit post -> multi-step approval chain.
func TestIntegration_CreateWorkflowSubmitApprovalChain(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	ownerID := uuid.New()
	contributorID := uuid.New()
	managerID := uuid.New()
	adminID := uuid.New()

	users := []struct {
		id   uuid.UUID
		name string
		role string
	}{
		{ownerID, "Chain Owner", "creator"},
		{contributorID, "Chain Contributor", "creator"},
		{managerID, "Chain Manager", "creator"},
		{adminID, "Chain Admin", "creator"},
	}
	for _, u := range users {
		_, err := pool.Exec(ctx,
			`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
			u.id, u.name, "chain-"+u.id.String()[:8]+"@test.local", u.role)
		if err != nil {
			t.Fatalf("insert %s: %v", u.name, err)
		}
	}

	// Set up team.
	teamMembers := []struct {
		memberID uuid.UUID
		role     string
	}{
		{contributorID, "contributor"},
		{managerID, "manager"},
		{adminID, "admin"},
	}
	for _, tm := range teamMembers {
		_, err := pool.Exec(ctx,
			`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
			 VALUES ($1, $2, $3, $1, 'active')`,
			ownerID, tm.memberID, tm.role)
		if err != nil {
			t.Fatalf("insert team member: %v", err)
		}
	}

	// Create a post.
	var postID uuid.UUID
	err := pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Chain Test Post', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "chain-post-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	approvalSvc := NewApprovalService(pool)

	// Create a 3-step workflow: contributor -> manager -> admin.
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name: "Three Step Review",
		Steps: []model.WorkflowStep{
			{ApproverRole: "contributor", Required: true},
			{ApproverRole: "manager", Required: true},
			{ApproverRole: "admin", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// Submit the post.
	ar, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Step 1: contributor approves.
	ar, err = approvalSvc.TakeAction(ctx, contributorID, ar.ID, &model.ApprovalActionRequest{
		Action: model.ActionApprove, Comment: "Content looks accurate",
	})
	if err != nil {
		t.Fatalf("contributor approve: %v", err)
	}
	if ar.CurrentStep != 2 {
		t.Errorf("expected step 2, got %d", ar.CurrentStep)
	}

	// Step 2: manager approves.
	ar, err = approvalSvc.TakeAction(ctx, managerID, ar.ID, &model.ApprovalActionRequest{
		Action: model.ActionApprove, Comment: "Meets quality standards",
	})
	if err != nil {
		t.Fatalf("manager approve: %v", err)
	}
	if ar.CurrentStep != 3 {
		t.Errorf("expected step 3, got %d", ar.CurrentStep)
	}

	// Step 3: admin approves -- should finalize.
	ar, err = approvalSvc.TakeAction(ctx, adminID, ar.ID, &model.ApprovalActionRequest{
		Action: model.ActionApprove, Comment: "Approved for publishing",
	})
	if err != nil {
		t.Fatalf("admin approve: %v", err)
	}
	if ar.Status != model.ApprovalApproved {
		t.Errorf("expected approved, got %s", ar.Status)
	}
	if len(ar.Actions) != 3 {
		t.Errorf("expected 3 action records, got %d", len(ar.Actions))
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	for _, u := range users {
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.id)
	}
}

// TestIntegration_RejectionAndResubmit exercises the reject -> resubmit flow.
func TestIntegration_RejectionAndResubmit(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	ownerID := uuid.New()
	managerID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Resub Owner", "resub-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		managerID, "Resub Manager", "resub-mgr-"+managerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert manager: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status) VALUES ($1, $2, 'manager', $1, 'active')`,
		ownerID, managerID)
	if err != nil {
		t.Fatalf("insert team member: %v", err)
	}

	var postID uuid.UUID
	err = pool.QueryRow(ctx,
		`INSERT INTO posts (author_id, title, slug, status, system_review_status, tone, target_platforms, tags)
		 VALUES ($1, 'Resub Test', $2, 'draft', 'pending', 'neutral', '{}', '{}')
		 RETURNING id`,
		ownerID, "resub-test-"+uuid.New().String()[:8],
	).Scan(&postID)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	approvalSvc := NewApprovalService(pool)
	wf, err := approvalSvc.CreateWorkflow(ctx, ownerID, &model.CreateWorkflowRequest{
		Name:  "Resub Workflow",
		Steps: []model.WorkflowStep{{ApproverRole: "manager", Required: true}},
	})
	if err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// First submission -- gets rejected.
	ar, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit 1: %v", err)
	}

	ar, err = approvalSvc.TakeAction(ctx, managerID, ar.ID, &model.ApprovalActionRequest{
		Action: model.ActionReject, Comment: "Needs more detail",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if ar.Status != model.ApprovalRejected {
		t.Errorf("expected rejected, got %s", ar.Status)
	}

	// Second submission -- gets approved.
	ar2, err := approvalSvc.SubmitForApproval(ctx, ownerID, postID, &model.SubmitApprovalRequest{
		WorkflowID: &wf.ID,
	})
	if err != nil {
		t.Fatalf("submit 2: %v", err)
	}
	if ar2.ID == ar.ID {
		t.Error("expected a new approval request on resubmit")
	}

	ar2, err = approvalSvc.TakeAction(ctx, managerID, ar2.ID, &model.ApprovalActionRequest{
		Action: model.ActionApprove, Comment: "Much better now",
	})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if ar2.Status != model.ApprovalApproved {
		t.Errorf("expected approved on resubmit, got %s", ar2.Status)
	}

	// GetApprovalStatus should return the latest (approved) request.
	latest, err := approvalSvc.GetApprovalStatus(ctx, postID)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if latest.Status != model.ApprovalApproved {
		t.Errorf("expected latest status=approved, got %s", latest.Status)
	}

	// Cleanup.
	pool.Exec(ctx, `DELETE FROM approval_actions WHERE request_id IN (SELECT id FROM approval_requests WHERE post_id = $1)`, postID)
	pool.Exec(ctx, `DELETE FROM approval_requests WHERE post_id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM approval_workflows WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
	pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, ownerID, managerID)
}
