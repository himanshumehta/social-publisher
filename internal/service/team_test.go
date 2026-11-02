package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ── Unit tests (no DB required) ─────────────────────────────────────────

func TestValidTeamRoles_AllPresent(t *testing.T) {
	expected := []model.TeamRole{
		model.TeamRoleViewer,
		model.TeamRoleContributor,
		model.TeamRoleManager,
		model.TeamRoleAdmin,
	}

	for _, r := range expected {
		if !model.ValidTeamRoles[r] {
			t.Errorf("ValidTeamRoles[%q] = false, want true", r)
		}
	}

	if len(model.ValidTeamRoles) != len(expected) {
		t.Errorf("len(ValidTeamRoles) = %d, want %d", len(model.ValidTeamRoles), len(expected))
	}
}

func TestValidTeamRoles_RejectsInvalid(t *testing.T) {
	invalids := []model.TeamRole{"owner", "superadmin", "", "VIEWER"}

	for _, r := range invalids {
		if model.ValidTeamRoles[r] {
			t.Errorf("ValidTeamRoles[%q] = true, want false", r)
		}
	}
}

func TestUpdateRole_CannotDemoteSelf(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	svc := NewTeamService(pool)
	ctx := context.Background()

	ownerID := uuid.New()
	// Owner tries to change their own role -- should fail.
	err := svc.UpdateRole(ctx, ownerID, ownerID, model.TeamRoleViewer)
	if err == nil {
		t.Fatal("expected error when demoting self, got nil")
	}
}

func TestUpdateRole_ValidTransition(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	ownerID := uuid.New()
	memberID := uuid.New()

	// Create test users.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Team Owner", "team-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		memberID, "Team Member", "team-member-"+memberID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, memberID)

	// Insert team member as active with viewer role.
	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, status)
		 VALUES ($1, $2, 'viewer', $1, 'active')`,
		ownerID, memberID)
	if err != nil {
		t.Fatalf("insert team member: %v", err)
	}

	svc := NewTeamService(pool)

	// Promote from viewer to manager.
	if err := svc.UpdateRole(ctx, ownerID, memberID, model.TeamRoleManager); err != nil {
		t.Fatalf("UpdateRole failed: %v", err)
	}

	// Verify the role changed.
	role, err := svc.GetMemberRole(ctx, ownerID, memberID)
	if err != nil {
		t.Fatalf("GetMemberRole failed: %v", err)
	}
	if role != model.TeamRoleManager {
		t.Errorf("expected role %q, got %q", model.TeamRoleManager, role)
	}
}

func TestInviteMember_DuplicateEmail(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	ownerID := uuid.New()
	memberID := uuid.New()
	email := "dup-" + uuid.New().String()[:8] + "@test.local"

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		ownerID, "Dup Owner", "dup-owner-"+ownerID.String()[:8]+"@test.local")
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		memberID, "Dup Member", email)
	if err != nil {
		t.Fatalf("insert member: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM team_members WHERE owner_id = $1`, ownerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, memberID)

	svc := NewTeamService(pool)

	// First invite should succeed.
	_, err = svc.InviteMember(ctx, ownerID, &model.InviteMemberRequest{
		Email: email,
		Role:  model.TeamRoleContributor,
	})
	if err != nil {
		t.Fatalf("first invite failed: %v", err)
	}

	// Second invite of same email should fail (conflict).
	_, err = svc.InviteMember(ctx, ownerID, &model.InviteMemberRequest{
		Email: email,
		Role:  model.TeamRoleContributor,
	})
	if err == nil {
		t.Fatal("expected conflict error on duplicate invite, got nil")
	}
}
