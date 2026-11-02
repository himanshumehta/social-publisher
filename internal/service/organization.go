package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// OrgService manages organizations, memberships, and subscriptions.
type OrgService struct {
	pool *pgxpool.Pool
}

// NewOrgService creates a new OrgService.
func NewOrgService(pool *pgxpool.Pool) *OrgService {
	return &OrgService{pool: pool}
}

// ── Slug generation ─────────────────────────────────────────────────────

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// generateOrgSlug converts a name into a URL-safe slug with a random suffix.
func generateOrgSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "org"
	}
	// Truncate to keep room for the suffix
	if len(s) > 80 {
		s = s[:80]
	}
	// Append a 4-byte random hex suffix for uniqueness
	b := make([]byte, 4)
	rand.Read(b)
	return s + "-" + hex.EncodeToString(b)
}

// getPlanLimits returns the member and project limits for a given plan.
func getPlanLimits(plan model.OrgPlan) (maxMembers, maxProjects int) {
	limits, ok := model.PlanLimitsMap[plan]
	if !ok {
		limits = model.PlanLimitsMap[model.OrgPlanStarter]
	}
	return limits.MaxMembers, limits.MaxProjects
}

// ── Create ──────────────────────────────────────────────────────────────

// CreateOrg creates a new organization, adds the owner as a member with
// the owner role, and creates a starter subscription.
func (s *OrgService) CreateOrg(ctx context.Context, ownerID uuid.UUID, req *model.CreateOrgRequest) (*model.Organization, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("organization name is required")
	}

	plan := req.Plan
	if plan == "" {
		plan = model.OrgPlanStarter
	}
	if !model.ValidOrgPlans[plan] {
		return nil, fmt.Errorf("invalid plan: %s", plan)
	}

	slug := req.Slug
	if slug == "" {
		slug = generateOrgSlug(req.Name)
	}

	maxMembers, maxProjects := getPlanLimits(plan)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var org model.Organization
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug, logo_url, domain, owner_id, plan, max_members, max_projects, settings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}')
		RETURNING id, name, slug, logo_url, domain, owner_id, plan, max_members, max_projects,
		          settings, is_active, created_at, updated_at`,
		req.Name, slug, req.LogoURL, req.Domain, ownerID, plan, maxMembers, maxProjects,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.LogoURL, &org.Domain,
		&org.OwnerID, &org.Plan, &org.MaxMembers, &org.MaxProjects,
		&org.Settings, &org.IsActive, &org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert organization: %w", err)
	}

	// Add owner as a member
	_, err = tx.Exec(ctx, `
		INSERT INTO org_members (org_id, user_id, role, status)
		VALUES ($1, $2, 'owner', 'active')`,
		org.ID, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert owner membership: %w", err)
	}

	// Create starter subscription
	now := time.Now()
	periodEnd := now.AddDate(0, 1, 0) // 1 month from now
	_, err = tx.Exec(ctx, `
		INSERT INTO org_subscriptions (org_id, plan, amount_paise, billing_cycle, status,
		                               current_period_start, current_period_end)
		VALUES ($1, $2, 0, 'monthly', 'active', $3, $4)`,
		org.ID, plan, now, periodEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("insert subscription: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &org, nil
}

// ── Read ────────────────────────────────────────────────────────────────

// GetOrg returns an organization by ID.
func (s *OrgService) GetOrg(ctx context.Context, orgID uuid.UUID) (*model.Organization, error) {
	var org model.Organization
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, slug, logo_url, domain, owner_id, plan, max_members, max_projects,
		       settings, is_active, created_at, updated_at
		FROM organizations
		WHERE id = $1`, orgID,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.LogoURL, &org.Domain,
		&org.OwnerID, &org.Plan, &org.MaxMembers, &org.MaxProjects,
		&org.Settings, &org.IsActive, &org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("query organization: %w", err)
	}
	return &org, nil
}

// ListUserOrgs returns all organizations a user belongs to.
func (s *OrgService) ListUserOrgs(ctx context.Context, userID uuid.UUID) ([]model.Organization, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT o.id, o.name, o.slug, o.logo_url, o.domain, o.owner_id, o.plan,
		       o.max_members, o.max_projects, o.settings, o.is_active,
		       o.created_at, o.updated_at
		FROM organizations o
		INNER JOIN org_members m ON m.org_id = o.id
		WHERE m.user_id = $1 AND m.status = 'active' AND o.is_active = TRUE
		ORDER BY o.created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query user orgs: %w", err)
	}
	defer rows.Close()

	var orgs []model.Organization
	for rows.Next() {
		var org model.Organization
		err := rows.Scan(
			&org.ID, &org.Name, &org.Slug, &org.LogoURL, &org.Domain,
			&org.OwnerID, &org.Plan, &org.MaxMembers, &org.MaxProjects,
			&org.Settings, &org.IsActive, &org.CreatedAt, &org.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating organizations: %w", err)
	}

	return orgs, nil
}

// ── Update ──────────────────────────────────────────────────────────────

// UpdateOrg updates an organization. Only the owner or an admin can update.
func (s *OrgService) UpdateOrg(ctx context.Context, userID, orgID uuid.UUID, req *model.CreateOrgRequest) (*model.Organization, error) {
	role, err := s.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}
	if model.OrgRoleRank(role) < model.OrgRoleRank(model.OrgRoleAdmin) {
		return nil, fmt.Errorf("only owner or admin can update organization")
	}

	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("organization name is required")
	}

	var org model.Organization
	err = s.pool.QueryRow(ctx, `
		UPDATE organizations
		SET name = $1, logo_url = $2, domain = $3, updated_at = NOW()
		WHERE id = $4
		RETURNING id, name, slug, logo_url, domain, owner_id, plan, max_members, max_projects,
		          settings, is_active, created_at, updated_at`,
		req.Name, req.LogoURL, req.Domain, orgID,
	).Scan(
		&org.ID, &org.Name, &org.Slug, &org.LogoURL, &org.Domain,
		&org.OwnerID, &org.Plan, &org.MaxMembers, &org.MaxProjects,
		&org.Settings, &org.IsActive, &org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("update organization: %w", err)
	}

	return &org, nil
}

// ── Members ─────────────────────────────────────────────────────────────

// InviteMember invites a user to the organization. The inviter must have
// admin+ role. Enforces the plan member limit.
func (s *OrgService) InviteMember(ctx context.Context, inviterID, orgID uuid.UUID, req *model.InviteOrgMemberRequest) (*model.OrgMember, error) {
	if req.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if !model.ValidOrgRoles[req.Role] {
		return nil, fmt.Errorf("invalid role: %s", req.Role)
	}
	if req.Role == model.OrgRoleOwner {
		return nil, fmt.Errorf("cannot invite with owner role")
	}

	// Validate inviter has admin+ role
	inviterRole, err := s.GetMemberRole(ctx, orgID, inviterID)
	if err != nil {
		return nil, fmt.Errorf("inviter is not a member of this organization")
	}
	if model.OrgRoleRank(inviterRole) < model.OrgRoleRank(model.OrgRoleAdmin) {
		return nil, fmt.Errorf("only owner or admin can invite members")
	}

	// Check plan limit
	if err := s.CheckPlanLimit(ctx, orgID, "members"); err != nil {
		return nil, err
	}

	// Look up the user by email
	var userID uuid.UUID
	err = s.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&userID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user with email %s not found", req.Email)
		}
		return nil, fmt.Errorf("query user: %w", err)
	}

	var member model.OrgMember
	err = s.pool.QueryRow(ctx, `
		INSERT INTO org_members (org_id, user_id, role, invited_by, status)
		VALUES ($1, $2, $3, $4, 'pending')
		ON CONFLICT (org_id, user_id) DO NOTHING
		RETURNING id, org_id, user_id, role, invited_by, status, created_at, updated_at`,
		orgID, userID, req.Role, inviterID,
	).Scan(
		&member.ID, &member.OrgID, &member.UserID, &member.Role,
		&member.InvitedBy, &member.Status, &member.CreatedAt, &member.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user is already a member of this organization")
		}
		return nil, fmt.Errorf("insert member: %w", err)
	}

	return &member, nil
}

// AcceptOrgInvite accepts a pending membership invite.
func (s *OrgService) AcceptOrgInvite(ctx context.Context, userID, membershipID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE org_members
		SET status = 'active', updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND status = 'pending'`,
		membershipID, userID,
	)
	if err != nil {
		return fmt.Errorf("accept invite: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("invite not found or already accepted")
	}
	return nil
}

// RemoveMember removes a member from the organization.
// The actor must have admin+ role and cannot remove the owner.
func (s *OrgService) RemoveMember(ctx context.Context, actorID, orgID, memberID uuid.UUID) error {
	// Validate actor has admin+ role
	actorRole, err := s.GetMemberRole(ctx, orgID, actorID)
	if err != nil {
		return fmt.Errorf("actor is not a member of this organization")
	}
	if model.OrgRoleRank(actorRole) < model.OrgRoleRank(model.OrgRoleAdmin) {
		return fmt.Errorf("only owner or admin can remove members")
	}

	// Check the target member's role - cannot remove owner
	var targetRole model.OrgRole
	var targetUserID uuid.UUID
	err = s.pool.QueryRow(ctx,
		`SELECT user_id, role FROM org_members WHERE id = $1 AND org_id = $2`,
		memberID, orgID,
	).Scan(&targetUserID, &targetRole)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("member not found")
		}
		return fmt.Errorf("query member: %w", err)
	}

	if targetRole == model.OrgRoleOwner {
		return fmt.Errorf("cannot remove the organization owner")
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE org_members
		SET status = 'removed', updated_at = NOW()
		WHERE id = $1 AND org_id = $2`,
		memberID, orgID,
	)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// UpdateMemberRole updates a member's role. The actor must have admin+ role.
// Cannot change the owner's role.
func (s *OrgService) UpdateMemberRole(ctx context.Context, actorID, orgID, memberID uuid.UUID, role model.OrgRole) error {
	if !model.ValidOrgRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}
	if role == model.OrgRoleOwner {
		return fmt.Errorf("cannot assign owner role")
	}

	actorRole, err := s.GetMemberRole(ctx, orgID, actorID)
	if err != nil {
		return fmt.Errorf("actor is not a member of this organization")
	}
	if model.OrgRoleRank(actorRole) < model.OrgRoleRank(model.OrgRoleAdmin) {
		return fmt.Errorf("only owner or admin can update member roles")
	}

	// Cannot change owner's role
	var targetRole model.OrgRole
	err = s.pool.QueryRow(ctx,
		`SELECT role FROM org_members WHERE id = $1 AND org_id = $2`,
		memberID, orgID,
	).Scan(&targetRole)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("member not found")
		}
		return fmt.Errorf("query member: %w", err)
	}
	if targetRole == model.OrgRoleOwner {
		return fmt.Errorf("cannot change the owner's role")
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE org_members
		SET role = $1, updated_at = NOW()
		WHERE id = $2 AND org_id = $3`,
		role, memberID, orgID,
	)
	if err != nil {
		return fmt.Errorf("update member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("member not found")
	}
	return nil
}

// ListMembers returns all active/pending members of an organization with user details.
func (s *OrgService) ListMembers(ctx context.Context, orgID uuid.UUID) ([]model.OrgMember, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.org_id, m.user_id, m.role, m.invited_by, m.status,
		       m.created_at, m.updated_at,
		       u.name, u.email
		FROM org_members m
		INNER JOIN users u ON u.id = m.user_id
		WHERE m.org_id = $1 AND m.status IN ('active', 'pending')
		ORDER BY m.created_at ASC`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	var members []model.OrgMember
	for rows.Next() {
		var m model.OrgMember
		err := rows.Scan(
			&m.ID, &m.OrgID, &m.UserID, &m.Role, &m.InvitedBy, &m.Status,
			&m.CreatedAt, &m.UpdatedAt,
			&m.UserName, &m.UserEmail,
		)
		if err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating members: %w", err)
	}

	return members, nil
}

// GetMemberRole returns the role of a user within an organization.
// Returns an error if the user is not an active member.
func (s *OrgService) GetMemberRole(ctx context.Context, orgID, userID uuid.UUID) (model.OrgRole, error) {
	var role model.OrgRole
	err := s.pool.QueryRow(ctx, `
		SELECT role FROM org_members
		WHERE org_id = $1 AND user_id = $2 AND status = 'active'`,
		orgID, userID,
	).Scan(&role)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("user is not a member of this organization")
		}
		return "", fmt.Errorf("query member role: %w", err)
	}
	return role, nil
}

// ── Dashboard ───────────────────────────────────────────────────────────

// GetDashboard returns aggregated stats for the organization.
func (s *OrgService) GetDashboard(ctx context.Context, userID, orgID uuid.UUID) (*model.OrgDashboard, error) {
	// Validate user is a member
	_, err := s.GetMemberRole(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	dash := &model.OrgDashboard{}

	// Member count
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM org_members WHERE org_id = $1 AND status = 'active'`,
		orgID,
	).Scan(&dash.MemberCount)
	if err != nil {
		return nil, fmt.Errorf("count members: %w", err)
	}

	// Project count
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE org_id = $1`,
		orgID,
	).Scan(&dash.ProjectCount)
	if err != nil {
		return nil, fmt.Errorf("count projects: %w", err)
	}

	// Post count
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM posts WHERE org_id = $1`,
		orgID,
	).Scan(&dash.PostCount)
	if err != nil {
		return nil, fmt.Errorf("count posts: %w", err)
	}

	// Active subscription
	var sub model.OrgSubscription
	err = s.pool.QueryRow(ctx, `
		SELECT id, org_id, plan, amount_paise, billing_cycle, status,
		       current_period_start, current_period_end,
		       gateway_subscription_id, created_at, updated_at
		FROM org_subscriptions
		WHERE org_id = $1 AND status = 'active'
		ORDER BY created_at DESC LIMIT 1`, orgID,
	).Scan(
		&sub.ID, &sub.OrgID, &sub.Plan, &sub.AmountPaise, &sub.BillingCycle,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.GatewaySubscriptionID, &sub.CreatedAt, &sub.UpdatedAt,
	)
	if err == nil {
		dash.ActiveSubscription = &sub
		dash.MonthlySpend = sub.AmountPaise
	}
	// If no subscription found, leave as nil (not an error)

	// Recent activity (latest org member additions and project creations)
	rows, err := s.pool.Query(ctx, `
		(
			SELECT 'member_joined' AS type, u.name AS actor_name,
			       'joined the organization' AS description, m.created_at
			FROM org_members m
			INNER JOIN users u ON u.id = m.user_id
			WHERE m.org_id = $1 AND m.status = 'active'
			ORDER BY m.created_at DESC LIMIT 5
		)
		UNION ALL
		(
			SELECT 'project_created' AS type, u.name AS actor_name,
			       'created project ' || p.title AS description, p.created_at
			FROM projects p
			INNER JOIN users u ON u.id = p.created_by
			WHERE p.org_id = $1
			ORDER BY p.created_at DESC LIMIT 5
		)
		ORDER BY created_at DESC LIMIT 10`, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent activity: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var a model.OrgActivity
		if err := rows.Scan(&a.Type, &a.ActorName, &a.Description, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		dash.RecentActivity = append(dash.RecentActivity, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating activity: %w", err)
	}

	if dash.RecentActivity == nil {
		dash.RecentActivity = []model.OrgActivity{}
	}

	return dash, nil
}

// ── Org-scoped queries ──────────────────────────────────────────────────

// ListOrgProjects returns paginated projects scoped to an organization.
func (s *OrgService) ListOrgProjects(ctx context.Context, orgID uuid.UUID, page, pageSize int) ([]model.Project, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE org_id = $1`, orgID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count org projects: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, category_id, title, slug, description, subject_name, subject_type,
		       subject_ref_id, requirements, budget_amount, budget_currency,
		       deadline_at, publish_target, status, creator_id, editor_id,
		       created_by, priority, tags, editing_requested, editing_notes,
		       editing_completed_at, project_type, publish_mode, claimed_at,
		       agreement_accepted, created_at, updated_at
		FROM projects
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pageSize, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query org projects: %w", err)
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		err := rows.Scan(
			&p.ID, &p.CategoryID, &p.Title, &p.Slug, &p.Description,
			&p.SubjectName, &p.SubjectType, &p.SubjectRefID,
			&p.Requirements, &p.BudgetAmount, &p.BudgetCurrency,
			&p.DeadlineAt, &p.PublishTarget, &p.Status, &p.CreatorID, &p.EditorID,
			&p.CreatedBy, &p.Priority, &p.Tags, &p.EditingRequested, &p.EditingNotes,
			&p.EditingCompletedAt, &p.ProjectType, &p.PublishMode, &p.ClaimedAt,
			&p.AgreementAccepted, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating projects: %w", err)
	}

	return projects, total, nil
}

// ── Context switching ───────────────────────────────────────────────────

// SwitchOrg validates that the user is a member of the organization.
// This is used by the middleware to validate context switching.
func (s *OrgService) SwitchOrg(ctx context.Context, userID, orgID uuid.UUID) error {
	_, err := s.GetMemberRole(ctx, orgID, userID)
	return err
}

// ── Plan limit enforcement ──────────────────────────────────────────────

// CheckPlanLimit checks if the organization has capacity for more of the
// given resource (members or projects). Returns an error if the limit is reached.
func (s *OrgService) CheckPlanLimit(ctx context.Context, orgID uuid.UUID, resource string) error {
	var plan model.OrgPlan
	var maxMembers, maxProjects int
	err := s.pool.QueryRow(ctx,
		`SELECT plan, max_members, max_projects FROM organizations WHERE id = $1`,
		orgID,
	).Scan(&plan, &maxMembers, &maxProjects)
	if err != nil {
		return fmt.Errorf("query organization: %w", err)
	}

	switch resource {
	case "members":
		if maxMembers == -1 {
			return nil // unlimited
		}
		var count int
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM org_members WHERE org_id = $1 AND status IN ('active', 'pending')`,
			orgID,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("count members: %w", err)
		}
		if count >= maxMembers {
			return fmt.Errorf("member limit reached (%d/%d); upgrade your plan", count, maxMembers)
		}

	case "projects":
		if maxProjects == -1 {
			return nil // unlimited
		}
		var count int
		err = s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM projects WHERE org_id = $1`,
			orgID,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("count projects: %w", err)
		}
		if count >= maxProjects {
			return fmt.Errorf("project limit reached (%d/%d); upgrade your plan", count, maxProjects)
		}

	default:
		return fmt.Errorf("unknown resource: %s", resource)
	}

	return nil
}
