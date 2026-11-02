package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type TeamService struct {
	pool *pgxpool.Pool
}

func NewTeamService(pool *pgxpool.Pool) *TeamService {
	return &TeamService{pool: pool}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (s *TeamService) scanMember(row pgx.Row) (*model.TeamMember, error) {
	var m model.TeamMember
	err := row.Scan(
		&m.ID, &m.OwnerID, &m.MemberID, &m.Role, &m.InvitedBy,
		&m.InvitedEmail, &m.Status, &m.CreatedAt, &m.UpdatedAt,
		&m.MemberName, &m.MemberEmail,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

const memberSelectCols = `
	tm.id, tm.owner_id, tm.member_id, tm.role, tm.invited_by,
	COALESCE(tm.invited_email, ''), tm.status, tm.created_at, tm.updated_at,
	COALESCE(u.name, ''), COALESCE(u.email, '')
`

const memberJoin = `
	FROM team_members tm
	LEFT JOIN users u ON u.id = tm.member_id
`

// ── InviteMember ────────────────────────────────────────────────────────

// InviteMember looks up a user by email, creates a team_member row with
// status=pending, and returns the new member record. Returns ErrConflict
// if the member is already on the team.
func (s *TeamService) InviteMember(ctx context.Context, ownerID uuid.UUID, req *model.InviteMemberRequest) (*model.TeamMember, error) {
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrInvalidInput)
	}
	if !model.ValidTeamRoles[req.Role] {
		return nil, fmt.Errorf("%w: invalid role %q", ErrInvalidInput, req.Role)
	}

	// Look up the user by email.
	var memberID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&memberID)
	if err != nil {
		return nil, fmt.Errorf("%w: user with email %q not found", ErrNotFound, req.Email)
	}

	// Cannot invite yourself.
	if memberID == ownerID {
		return nil, fmt.Errorf("%w: cannot invite yourself", ErrInvalidInput)
	}

	// Insert then select to get the full member record.
	var tmID uuid.UUID
	err = s.pool.QueryRow(ctx,
		`INSERT INTO team_members (owner_id, member_id, role, invited_by, invited_email, status)
		 VALUES ($1, $2, $3, $1, $4, 'pending')
		 ON CONFLICT (owner_id, member_id) DO NOTHING
		 RETURNING id`,
		ownerID, memberID, req.Role, req.Email,
	).Scan(&tmID)
	if err != nil {
		return nil, fmt.Errorf("%w: member already on team", ErrConflict)
	}

	return s.getMemberByID(ctx, tmID)
}

func (s *TeamService) getMemberByID(ctx context.Context, id uuid.UUID) (*model.TeamMember, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+memberSelectCols+memberJoin+`WHERE tm.id = $1`, id,
	)
	return s.scanMember(row)
}

// ── AcceptInvite ────────────────────────────────────────────────────────

// AcceptInvite sets the team member status to active. The caller must be
// the member_id on the record.
func (s *TeamService) AcceptInvite(ctx context.Context, memberID, teamMemberID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE team_members
		 SET status = 'active', updated_at = NOW()
		 WHERE id = $1 AND member_id = $2 AND status = 'pending'`,
		teamMemberID, memberID,
	)
	if err != nil {
		return fmt.Errorf("accept invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: invite not found or already accepted", ErrNotFound)
	}
	return nil
}

// ── RemoveMember ────────────────────────────────────────────────────────

// RemoveMember sets the member status to removed. Only the team owner can
// perform this action.
func (s *TeamService) RemoveMember(ctx context.Context, ownerID, memberID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE team_members
		 SET status = 'removed', updated_at = NOW()
		 WHERE owner_id = $1 AND member_id = $2 AND status != 'removed'`,
		ownerID, memberID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: member not found", ErrNotFound)
	}
	return nil
}

// ── ListMembers ─────────────────────────────────────────────────────────

// ListMembers returns all active and pending team members for an owner.
func (s *TeamService) ListMembers(ctx context.Context, ownerID uuid.UUID) ([]model.TeamMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+memberSelectCols+memberJoin+
			`WHERE tm.owner_id = $1 AND tm.status IN ('pending', 'active')
			 ORDER BY tm.created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []model.TeamMember
	for rows.Next() {
		var m model.TeamMember
		if err := rows.Scan(
			&m.ID, &m.OwnerID, &m.MemberID, &m.Role, &m.InvitedBy,
			&m.InvitedEmail, &m.Status, &m.CreatedAt, &m.UpdatedAt,
			&m.MemberName, &m.MemberEmail,
		); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

// ── UpdateRole ──────────────────────────────────────────────────────────

// UpdateRole changes a team member's role. Only the team owner can change roles.
// The owner cannot demote themselves.
func (s *TeamService) UpdateRole(ctx context.Context, ownerID, memberID uuid.UUID, role model.TeamRole) error {
	if !model.ValidTeamRoles[role] {
		return fmt.Errorf("%w: invalid role %q", ErrInvalidInput, role)
	}

	// Prevent owner from demoting themselves (they are not in team_members,
	// but guard against edge cases).
	if ownerID == memberID {
		return fmt.Errorf("%w: cannot change your own role", ErrForbidden)
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE team_members
		 SET role = $3, updated_at = NOW()
		 WHERE owner_id = $1 AND member_id = $2 AND status = 'active'`,
		ownerID, memberID, role,
	)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: member not found or not active", ErrNotFound)
	}
	return nil
}

// ── GetMemberRole ───────────────────────────────────────────────────────

// GetMemberRole returns the role of a team member. Returns ErrNotFound if
// the user is not an active member of the team.
func (s *TeamService) GetMemberRole(ctx context.Context, ownerID, memberID uuid.UUID) (model.TeamRole, error) {
	var role model.TeamRole
	err := s.pool.QueryRow(ctx,
		`SELECT role FROM team_members
		 WHERE owner_id = $1 AND member_id = $2 AND status = 'active'`,
		ownerID, memberID,
	).Scan(&role)
	if err != nil {
		return "", fmt.Errorf("%w: not a team member", ErrNotFound)
	}
	return role, nil
}

// ── IsTeamMember ────────────────────────────────────────────────────────

// IsTeamMember performs a quick existence check: is userID an active member
// of ownerID's team (or the owner themselves)?
func (s *TeamService) IsTeamMember(ctx context.Context, ownerID, userID uuid.UUID) (bool, error) {
	if ownerID == userID {
		return true, nil
	}

	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM team_members
			WHERE owner_id = $1 AND member_id = $2 AND status = 'active'
		)`,
		ownerID, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check membership: %w", err)
	}
	return exists, nil
}
