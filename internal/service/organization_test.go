package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Slug generation tests ───────────────────────────────────────────────

func TestGenerateOrgSlug_Simple(t *testing.T) {
	slug := generateOrgSlug("My Cool Agency")

	if !strings.HasPrefix(slug, "my-cool-agency-") {
		t.Errorf("slug = %q, want prefix 'my-cool-agency-'", slug)
	}
	// slug should be: prefix + "-" + 8 hex chars
	parts := strings.Split(slug, "-")
	suffix := parts[len(parts)-1]
	if len(suffix) != 8 {
		t.Errorf("slug suffix length = %d, want 8 hex chars; slug = %q", len(suffix), slug)
	}
}

func TestGenerateOrgSlug_SpecialChars(t *testing.T) {
	slug := generateOrgSlug("Hello!!! @World# 123")

	if !strings.HasPrefix(slug, "hello-world-123-") {
		t.Errorf("slug = %q, want prefix 'hello-world-123-'", slug)
	}
}

func TestGenerateOrgSlug_EmptyName(t *testing.T) {
	slug := generateOrgSlug("")

	if !strings.HasPrefix(slug, "org-") {
		t.Errorf("slug = %q, want prefix 'org-'", slug)
	}
}

func TestGenerateOrgSlug_Uniqueness(t *testing.T) {
	slug1 := generateOrgSlug("Test")
	slug2 := generateOrgSlug("Test")

	if slug1 == slug2 {
		t.Errorf("two calls returned identical slugs: %q", slug1)
	}
}

// ── Plan limits tests ───────────────────────────────────────────────────

func TestGetPlanLimits_Starter(t *testing.T) {
	maxMembers, maxProjects := getPlanLimits(model.OrgPlanStarter)

	if maxMembers != 5 {
		t.Errorf("starter maxMembers = %d, want 5", maxMembers)
	}
	if maxProjects != 10 {
		t.Errorf("starter maxProjects = %d, want 10", maxProjects)
	}
}

func TestGetPlanLimits_Professional(t *testing.T) {
	maxMembers, maxProjects := getPlanLimits(model.OrgPlanProfessional)

	if maxMembers != 25 {
		t.Errorf("professional maxMembers = %d, want 25", maxMembers)
	}
	if maxProjects != 50 {
		t.Errorf("professional maxProjects = %d, want 50", maxProjects)
	}
}

func TestGetPlanLimits_Enterprise(t *testing.T) {
	maxMembers, maxProjects := getPlanLimits(model.OrgPlanEnterprise)

	if maxMembers != -1 {
		t.Errorf("enterprise maxMembers = %d, want -1 (unlimited)", maxMembers)
	}
	if maxProjects != -1 {
		t.Errorf("enterprise maxProjects = %d, want -1 (unlimited)", maxProjects)
	}
}

func TestGetPlanLimits_UnknownPlan(t *testing.T) {
	maxMembers, maxProjects := getPlanLimits("nonexistent")

	// Should fall back to starter limits
	if maxMembers != 5 {
		t.Errorf("unknown plan maxMembers = %d, want 5 (starter fallback)", maxMembers)
	}
	if maxProjects != 10 {
		t.Errorf("unknown plan maxProjects = %d, want 10 (starter fallback)", maxProjects)
	}
}

// ── Model validation tests ──────────────────────────────────────────────

func TestValidOrgPlans(t *testing.T) {
	valid := []model.OrgPlan{model.OrgPlanStarter, model.OrgPlanProfessional, model.OrgPlanEnterprise}
	for _, p := range valid {
		if !model.ValidOrgPlans[p] {
			t.Errorf("ValidOrgPlans[%q] = false, want true", p)
		}
	}

	invalid := []model.OrgPlan{"free", "premium", ""}
	for _, p := range invalid {
		if model.ValidOrgPlans[p] {
			t.Errorf("ValidOrgPlans[%q] = true, want false", p)
		}
	}
}

func TestValidOrgRoles(t *testing.T) {
	valid := []model.OrgRole{model.OrgRoleOwner, model.OrgRoleAdmin, model.OrgRoleManager, model.OrgRoleMember, model.OrgRoleViewer}
	for _, r := range valid {
		if !model.ValidOrgRoles[r] {
			t.Errorf("ValidOrgRoles[%q] = false, want true", r)
		}
	}

	invalid := []model.OrgRole{"superadmin", "guest", ""}
	for _, r := range invalid {
		if model.ValidOrgRoles[r] {
			t.Errorf("ValidOrgRoles[%q] = true, want false", r)
		}
	}
}

func TestOrgRoleRank(t *testing.T) {
	cases := []struct {
		role model.OrgRole
		want int
	}{
		{model.OrgRoleOwner, 50},
		{model.OrgRoleAdmin, 40},
		{model.OrgRoleManager, 30},
		{model.OrgRoleMember, 20},
		{model.OrgRoleViewer, 10},
		{"unknown", 0},
	}

	for _, tc := range cases {
		got := model.OrgRoleRank(tc.role)
		if got != tc.want {
			t.Errorf("OrgRoleRank(%q) = %d, want %d", tc.role, got, tc.want)
		}
	}
}

// ── Integration test helpers ────────────────────────────────────────────

type orgTestState struct {
	ctx      context.Context
	ownerID  uuid.UUID
	adminID  uuid.UUID
	memberID uuid.UUID
}

// setupOrgTest creates test users and returns them with a cleanup function.
func setupOrgTest(t *testing.T) (pool *pgxpool.Pool, svc *OrgService, st orgTestState, cleanup func()) {
	t.Helper()

	rawPool := setupTestPool(t)
	svc = NewOrgService(rawPool)

	ctx := context.Background()
	ownerID := uuid.New()
	adminID := uuid.New()
	memberID := uuid.New()

	// Insert owner user
	_, err := rawPool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Org Owner", "owner-"+ownerID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert owner user: %v", err)
	}

	// Insert admin user
	_, err = rawPool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		adminID, "Org Admin", "admin-"+adminID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert admin user: %v", err)
	}

	// Insert member user
	_, err = rawPool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		memberID, "Org Member", "member-"+memberID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert member user: %v", err)
	}

	st = orgTestState{
		ctx:      ctx,
		ownerID:  ownerID,
		adminID:  adminID,
		memberID: memberID,
	}

	cleanup = func() {
		rawPool.Exec(ctx, `DELETE FROM org_invoices WHERE org_id IN (SELECT id FROM organizations WHERE owner_id = $1)`, ownerID)
		rawPool.Exec(ctx, `DELETE FROM org_subscriptions WHERE org_id IN (SELECT id FROM organizations WHERE owner_id = $1)`, ownerID)
		rawPool.Exec(ctx, `DELETE FROM org_members WHERE org_id IN (SELECT id FROM organizations WHERE owner_id = $1)`, ownerID)
		rawPool.Exec(ctx, `DELETE FROM organizations WHERE owner_id = $1`, ownerID)
		rawPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
		rawPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, adminID)
		rawPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, memberID)
		rawPool.Close()
	}

	return rawPool, svc, st, cleanup
}

// ── Integration Tests ───────────────────────────────────────────────────

func TestCreateOrg_AddsOwnerAsMember(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{
		Name: "Test Agency",
	})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	if org.ID == uuid.Nil {
		t.Error("expected org ID to be set")
	}
	if org.Name != "Test Agency" {
		t.Errorf("name = %q, want %q", org.Name, "Test Agency")
	}
	if org.OwnerID != st.ownerID {
		t.Errorf("owner_id = %s, want %s", org.OwnerID, st.ownerID)
	}
	if org.Plan != model.OrgPlanStarter {
		t.Errorf("plan = %q, want %q", org.Plan, model.OrgPlanStarter)
	}
	if !org.IsActive {
		t.Error("expected is_active = true")
	}

	// Verify owner was added as member with owner role
	role, err := svc.GetMemberRole(st.ctx, org.ID, st.ownerID)
	if err != nil {
		t.Fatalf("GetMemberRole failed: %v", err)
	}
	if role != model.OrgRoleOwner {
		t.Errorf("owner role = %q, want %q", role, model.OrgRoleOwner)
	}
}

func TestCreateOrg_EmptyName(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	_, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{
		Name: "",
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateOrg_InvalidPlan(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	_, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{
		Name: "Bad Plan Org",
		Plan: "ultra",
	})
	if err == nil {
		t.Fatal("expected error for invalid plan, got nil")
	}
}

func TestInviteMember_ValidatesRole(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Role Test Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Invite with invalid role
	_, err = svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "admin-" + st.adminID.String()[:8] + "@test.io",
		Role:  "superadmin",
	})
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}

	// Cannot invite as owner
	_, err = svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "admin-" + st.adminID.String()[:8] + "@test.io",
		Role:  model.OrgRoleOwner,
	})
	if err == nil {
		t.Fatal("expected error for owner role invitation, got nil")
	}
}

func TestInviteMember_RequiresAdminRole(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Invite Perm Test"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// A non-member should not be able to invite
	_, err = svc.InviteMember(st.ctx, st.adminID, org.ID, &model.InviteOrgMemberRequest{
		Email: "member-" + st.memberID.String()[:8] + "@test.io",
		Role:  model.OrgRoleMember,
	})
	if err == nil {
		t.Fatal("expected error when non-member tries to invite, got nil")
	}
}

func TestInviteMember_ExceedsLimit(t *testing.T) {
	rawPool, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Limit Test Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Set max_members to 1 (just the owner) to test limit
	_, err = rawPool.Exec(st.ctx,
		`UPDATE organizations SET max_members = 1 WHERE id = $1`, org.ID)
	if err != nil {
		t.Fatalf("update max_members: %v", err)
	}

	// Try to invite - should fail because limit is reached (owner counts as 1)
	_, err = svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "admin-" + st.adminID.String()[:8] + "@test.io",
		Role:  model.OrgRoleMember,
	})
	if err == nil {
		t.Fatal("expected error for exceeding member limit, got nil")
	}
	if !strings.Contains(err.Error(), "member limit reached") {
		t.Errorf("error = %q, want to contain 'member limit reached'", err.Error())
	}
}

func TestRemoveMember_CannotRemoveOwner(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Remove Owner Test"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Get the owner's membership ID
	members, err := svc.ListMembers(st.ctx, org.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) == 0 {
		t.Fatal("expected at least one member")
	}

	ownerMemberID := members[0].ID

	// Try to remove owner - should fail
	err = svc.RemoveMember(st.ctx, st.ownerID, org.ID, ownerMemberID)
	if err == nil {
		t.Fatal("expected error when removing owner, got nil")
	}
	if !strings.Contains(err.Error(), "cannot remove the organization owner") {
		t.Errorf("error = %q, want to contain 'cannot remove the organization owner'", err.Error())
	}
}

func TestUpdateMemberRole_RequiresAdmin(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Role Update Test"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Invite and accept admin
	member, err := svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "admin-" + st.adminID.String()[:8] + "@test.io",
		Role:  model.OrgRoleMember,
	})
	if err != nil {
		t.Fatalf("InviteMember failed: %v", err)
	}
	if err := svc.AcceptOrgInvite(st.ctx, st.adminID, member.ID); err != nil {
		t.Fatalf("AcceptOrgInvite failed: %v", err)
	}

	// The newly added member (role=member) should not be able to update roles
	err = svc.UpdateMemberRole(st.ctx, st.adminID, org.ID, member.ID, model.OrgRoleAdmin)
	if err == nil {
		t.Fatal("expected error when member tries to update role, got nil")
	}
	if !strings.Contains(err.Error(), "only owner or admin") {
		t.Errorf("error = %q, want to contain 'only owner or admin'", err.Error())
	}
}

func TestGetMemberRole_NotMember(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Not Member Test"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// adminID is not a member
	_, err = svc.GetMemberRole(st.ctx, org.ID, st.adminID)
	if err == nil {
		t.Fatal("expected error for non-member, got nil")
	}
	if !strings.Contains(err.Error(), "not a member") {
		t.Errorf("error = %q, want to contain 'not a member'", err.Error())
	}
}

func TestGetDashboard_AggregatesCounts(t *testing.T) {
	rawPool, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Dashboard Test Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Create an org-scoped project
	slug := "test-" + uuid.New().String()[:8]
	_, err = rawPool.Exec(st.ctx,
		`INSERT INTO projects (title, slug, description, requirements, budget_amount, budget_currency,
		    status, creator_id, created_by, priority, tags, org_id)
		 VALUES ($1, $2, '', '{}', 0, 'INR', 'active', $3, $3, 0, '{}', $4)`,
		"Org Project "+slug, slug, st.ownerID, org.ID,
	)
	if err != nil {
		t.Fatalf("insert org project: %v", err)
	}

	dash, err := svc.GetDashboard(st.ctx, st.ownerID, org.ID)
	if err != nil {
		t.Fatalf("GetDashboard failed: %v", err)
	}

	if dash.MemberCount != 1 {
		t.Errorf("member_count = %d, want 1", dash.MemberCount)
	}
	if dash.ProjectCount != 1 {
		t.Errorf("project_count = %d, want 1", dash.ProjectCount)
	}
	if dash.PostCount != 0 {
		t.Errorf("post_count = %d, want 0", dash.PostCount)
	}
	if dash.ActiveSubscription == nil {
		t.Error("expected active subscription, got nil")
	}
	if dash.RecentActivity == nil {
		t.Error("expected recent_activity to be non-nil slice")
	}

	// Cleanup
	rawPool.Exec(st.ctx, `DELETE FROM projects WHERE org_id = $1`, org.ID)
}

func TestCheckPlanLimit_UnderLimit(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{
		Name: "Under Limit Org",
	})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Starter plan has max_members=5, we only have 1 (owner)
	err = svc.CheckPlanLimit(st.ctx, org.ID, "members")
	if err != nil {
		t.Errorf("expected no error for under-limit members, got: %v", err)
	}

	err = svc.CheckPlanLimit(st.ctx, org.ID, "projects")
	if err != nil {
		t.Errorf("expected no error for under-limit projects, got: %v", err)
	}
}

func TestCheckPlanLimit_AtLimit(t *testing.T) {
	rawPool, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "At Limit Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Set max_members to 1 to simulate being at limit
	_, err = rawPool.Exec(st.ctx,
		`UPDATE organizations SET max_members = 1 WHERE id = $1`, org.ID)
	if err != nil {
		t.Fatalf("update max_members: %v", err)
	}

	err = svc.CheckPlanLimit(st.ctx, org.ID, "members")
	if err == nil {
		t.Fatal("expected error at member limit, got nil")
	}
	if !strings.Contains(err.Error(), "member limit reached") {
		t.Errorf("error = %q, want to contain 'member limit reached'", err.Error())
	}
}

func TestInviteAndAccept_Flow(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Invite Flow Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Invite the admin user
	member, err := svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "admin-" + st.adminID.String()[:8] + "@test.io",
		Role:  model.OrgRoleAdmin,
	})
	if err != nil {
		t.Fatalf("InviteMember failed: %v", err)
	}
	if member.Status != model.OrgMemberPending {
		t.Errorf("status = %q, want %q", member.Status, model.OrgMemberPending)
	}

	// Accept the invite
	err = svc.AcceptOrgInvite(st.ctx, st.adminID, member.ID)
	if err != nil {
		t.Fatalf("AcceptOrgInvite failed: %v", err)
	}

	// Verify role
	role, err := svc.GetMemberRole(st.ctx, org.ID, st.adminID)
	if err != nil {
		t.Fatalf("GetMemberRole failed: %v", err)
	}
	if role != model.OrgRoleAdmin {
		t.Errorf("role = %q, want %q", role, model.OrgRoleAdmin)
	}

	// Verify listed in members
	members, err := svc.ListMembers(st.ctx, org.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("member count = %d, want 2", len(members))
	}
}

func TestListUserOrgs(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	// Create two orgs
	_, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Org Alpha"})
	if err != nil {
		t.Fatalf("CreateOrg Alpha failed: %v", err)
	}
	_, err = svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Org Beta"})
	if err != nil {
		t.Fatalf("CreateOrg Beta failed: %v", err)
	}

	orgs, err := svc.ListUserOrgs(st.ctx, st.ownerID)
	if err != nil {
		t.Fatalf("ListUserOrgs failed: %v", err)
	}
	if len(orgs) < 2 {
		t.Errorf("org count = %d, want >= 2", len(orgs))
	}
}

func TestSwitchOrg_ValidatesMembership(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Switch Test Org"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Owner can switch
	err = svc.SwitchOrg(st.ctx, st.ownerID, org.ID)
	if err != nil {
		t.Errorf("expected owner to switch, got: %v", err)
	}

	// Non-member cannot switch
	err = svc.SwitchOrg(st.ctx, st.adminID, org.ID)
	if err == nil {
		t.Error("expected error for non-member switch, got nil")
	}
}

func TestUpdateOrg_RequiresAdminRole(t *testing.T) {
	_, svc, st, cleanup := setupOrgTest(t)
	defer cleanup()

	org, err := svc.CreateOrg(st.ctx, st.ownerID, &model.CreateOrgRequest{Name: "Update Perm Test"})
	if err != nil {
		t.Fatalf("CreateOrg failed: %v", err)
	}

	// Invite member with viewer role
	member, err := svc.InviteMember(st.ctx, st.ownerID, org.ID, &model.InviteOrgMemberRequest{
		Email: "member-" + st.memberID.String()[:8] + "@test.io",
		Role:  model.OrgRoleViewer,
	})
	if err != nil {
		t.Fatalf("InviteMember failed: %v", err)
	}
	svc.AcceptOrgInvite(st.ctx, st.memberID, member.ID)

	// Viewer should not be able to update org
	_, err = svc.UpdateOrg(st.ctx, st.memberID, org.ID, &model.CreateOrgRequest{
		Name: "Hacked Name",
	})
	if err == nil {
		t.Fatal("expected error when viewer tries to update org, got nil")
	}

	// Owner should be able to update
	updated, err := svc.UpdateOrg(st.ctx, st.ownerID, org.ID, &model.CreateOrgRequest{
		Name:    "Updated Agency Name",
		LogoURL: "https://example.com/logo.png",
	})
	if err != nil {
		t.Fatalf("UpdateOrg by owner failed: %v", err)
	}
	if updated.Name != "Updated Agency Name" {
		t.Errorf("name = %q, want %q", updated.Name, "Updated Agency Name")
	}
}
