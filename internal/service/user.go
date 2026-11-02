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
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/model"
)

type UserService struct {
	pool  *pgxpool.Pool
	audit *AuditService
	s3    *infra.S3Client
	rdb   *redis.Client
}

func NewUserService(pool *pgxpool.Pool, audit *AuditService, s3 *infra.S3Client, rdb *redis.Client) *UserService {
	return &UserService{pool: pool, audit: audit, s3: s3, rdb: rdb}
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, name, COALESCE(display_name,''), COALESCE(avatar_url,''), COALESCE(bio,''), COALESCE(phone,''),
		        role, is_active, is_onboarded, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.Phone,
		&u.Role, &u.IsActive, &u.IsOnboarded, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	return &u, err
}

func (s *UserService) GetCreatorProfile(ctx context.Context, userID uuid.UUID) (*model.CreatorProfile, error) {
	var p model.CreatorProfile
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, slug, city, state, specializations, youtube_channel, instagram_handle,
		        facebook_page_url, twitter_handle, linkedin_profile_url, threads_handle,
		        total_videos, total_views, total_earnings, is_public, created_at, updated_at
		 FROM creator_profiles WHERE user_id = $1`, userID,
	).Scan(&p.ID, &p.UserID, &p.Slug, &p.City, &p.State, &p.Specializations,
		&p.YouTubeChannel, &p.InstagramHandle, &p.FacebookPageURL, &p.TwitterHandle,
		&p.LinkedInProfileURL, &p.ThreadsHandle, &p.TotalVideos, &p.TotalViews,
		&p.TotalEarnings, &p.IsPublic, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("creator profile not found")
	}
	return &p, err
}

type OnboardRequest struct {
	DisplayName     string   `json:"display_name"`
	Bio             string   `json:"bio"`
	City            string   `json:"city"`
	State           string   `json:"state"`
	Specializations []string `json:"specializations"`
	CategoryIDs     []string `json:"category_ids"`
}

type BankingRequest struct {
	AccountHolder string `json:"account_holder"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
	IFSCCode      string `json:"ifsc_code"`
	UPIID         string `json:"upi_id"`
	PANNumber     string `json:"pan_number"`
}

type OnboardingStatus struct {
	HasProfile    bool   `json:"has_profile"`
	HasBanking    bool   `json:"has_banking"`
	HasAvatar     bool   `json:"has_avatar"`
	BankingStatus string `json:"banking_status"`
	Projects      int    `json:"projects"`
}

func (s *UserService) Onboard(ctx context.Context, userID uuid.UUID, req OnboardRequest) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update user
	_, err = tx.Exec(ctx,
		`UPDATE users SET display_name = $1, bio = $2, is_onboarded = TRUE, updated_at = NOW() WHERE id = $3`,
		req.DisplayName, req.Bio, userID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	// Update creator profile
	_, err = tx.Exec(ctx,
		`UPDATE creator_profiles SET city = $1, state = $2, specializations = $3, updated_at = NOW() WHERE user_id = $4`,
		req.City, req.State, req.Specializations, userID)
	if err != nil {
		return fmt.Errorf("update creator profile: %w", err)
	}

	// Insert category associations
	var profileID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM creator_profiles WHERE user_id = $1`, userID).Scan(&profileID)
	if err != nil {
		return fmt.Errorf("get profile id: %w", err)
	}
	for _, catID := range req.CategoryIDs {
		cid, err := uuid.Parse(catID)
		if err != nil {
			continue
		}
		_, _ = tx.Exec(ctx,
			`INSERT INTO creator_categories (creator_profile_id, category_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			profileID, cid)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (s *UserService) SubmitBanking(ctx context.Context, userID uuid.UUID, req BankingRequest) error {
	// Use nil for empty optional fields to store NULL instead of empty strings
	var accountHolder, bankName, accountNumber, ifscCode, upiID, panNumber *string
	if req.AccountHolder != "" {
		accountHolder = &req.AccountHolder
	}
	if req.BankName != "" {
		bankName = &req.BankName
	}
	if req.AccountNumber != "" {
		accountNumber = &req.AccountNumber
	}
	if req.IFSCCode != "" {
		ifscCode = &req.IFSCCode
	}
	if req.UPIID != "" {
		upiID = &req.UPIID
	}
	if req.PANNumber != "" {
		panNumber = &req.PANNumber
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO banking_info (user_id, account_holder, bank_name, account_number, ifsc_code, upi_id, pan_number, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'submitted')
		 ON CONFLICT (user_id) DO UPDATE SET
		   account_holder = EXCLUDED.account_holder, bank_name = EXCLUDED.bank_name,
		   account_number = EXCLUDED.account_number, ifsc_code = EXCLUDED.ifsc_code,
		   upi_id = EXCLUDED.upi_id, pan_number = EXCLUDED.pan_number,
		   status = 'submitted', updated_at = NOW()`,
		userID, accountHolder, bankName, accountNumber, ifscCode, upiID, panNumber)
	if err != nil {
		return fmt.Errorf("upsert banking info: %w", err)
	}
	return nil
}

func (s *UserService) UpdateAvatarURL(ctx context.Context, userID uuid.UUID, storageKey string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET avatar_url = $1, updated_at = NOW() WHERE id = $2`,
		storageKey, userID)
	if err != nil {
		return fmt.Errorf("update avatar: %w", err)
	}
	return nil
}

func (s *UserService) GetOnboardingStatus(ctx context.Context, userID uuid.UUID) (*OnboardingStatus, error) {
	status := &OnboardingStatus{}

	// Check profile
	var city, avatarURL string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(u.avatar_url, ''), COALESCE(cp.city, '')
		 FROM users u LEFT JOIN creator_profiles cp ON cp.user_id = u.id
		 WHERE u.id = $1`, userID,
	).Scan(&avatarURL, &city)
	if err != nil {
		return nil, fmt.Errorf("check profile: %w", err)
	}
	status.HasProfile = city != ""
	status.HasAvatar = avatarURL != ""

	// Check banking
	var bankingStatus string
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(status::text, 'pending') FROM banking_info WHERE user_id = $1`, userID,
	).Scan(&bankingStatus)
	if err == pgx.ErrNoRows {
		status.HasBanking = false
		status.BankingStatus = "not_submitted"
	} else if err != nil {
		return nil, fmt.Errorf("check banking: %w", err)
	} else {
		status.HasBanking = true
		status.BankingStatus = bankingStatus
	}

	// Count projects
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM projects WHERE creator_id = $1`, userID,
	).Scan(&status.Projects)
	if err != nil {
		status.Projects = 0
	}

	return status, nil
}

func (s *UserService) ListByRole(ctx context.Context, role string) ([]model.User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, email, COALESCE(name,''), COALESCE(display_name,''), COALESCE(avatar_url,''), COALESCE(bio,''), COALESCE(phone,''), role, is_active, is_onboarded, last_login_at, created_at, updated_at
		 FROM users WHERE role = $1 AND is_active = TRUE ORDER BY name ASC`, role)
	if err != nil {
		return nil, fmt.Errorf("query users by role: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.Phone,
			&u.Role, &u.IsActive, &u.IsOnboarded, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if users == nil {
		users = []model.User{}
	}
	return users, nil
}

func (s *UserService) GetBankingInfo(ctx context.Context, userID uuid.UUID) (*model.BankingInfo, error) {
	var b model.BankingInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, account_holder, bank_name, account_number, ifsc_code, upi_id, pan_number,
		        status, verified_at, verified_by, rejection_reason, created_at, updated_at
		 FROM banking_info WHERE user_id = $1`, userID,
	).Scan(&b.ID, &b.UserID, &b.AccountHolder, &b.BankName, &b.AccountNumber, &b.IFSCCode,
		&b.UPIID, &b.PANNumber, &b.Status, &b.VerifiedAt, &b.VerifiedBy,
		&b.RejectionReason, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // no banking info yet
	}
	return &b, err
}

type UpdateProfileRequest struct {
	DisplayName        string   `json:"display_name"`
	Bio                string   `json:"bio"`
	Phone              string   `json:"phone"`
	City               string   `json:"city"`
	State              string   `json:"state"`
	Specializations    []string `json:"specializations"`
	YouTubeChannel     string   `json:"youtube_channel"`
	InstagramHandle    string   `json:"instagram_handle"`
	FacebookPageURL    string   `json:"facebook_page_url"`
	TwitterHandle      string   `json:"twitter_handle"`
	LinkedInProfileURL string   `json:"linkedin_profile_url"`
	ThreadsHandle      string   `json:"threads_handle"`
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) (*model.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update users table
	_, err = tx.Exec(ctx,
		`UPDATE users SET display_name = $1, bio = $2, phone = $3, updated_at = NOW() WHERE id = $4`,
		req.DisplayName, req.Bio, req.Phone, userID)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	// Update creator_profiles table
	_, err = tx.Exec(ctx,
		`UPDATE creator_profiles SET city = $1, state = $2, specializations = $3,
		 youtube_channel = $4, instagram_handle = $5,
		 facebook_page_url = $6, twitter_handle = $7, linkedin_profile_url = $8, threads_handle = $9,
		 updated_at = NOW() WHERE user_id = $10`,
		req.City, req.State, req.Specializations,
		req.YouTubeChannel, req.InstagramHandle,
		req.FacebookPageURL, req.TwitterHandle, req.LinkedInProfileURL, req.ThreadsHandle, userID)
	if err != nil {
		return nil, fmt.Errorf("update creator profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Fetch updated user
	return s.GetByID(ctx, userID)
}

// PresignAvatarURL generates a presigned GET URL for a stored avatar key.
func (s *UserService) PresignAvatarURL(ctx context.Context, key string) (string, error) {
	if s.s3 == nil || key == "" {
		return "", nil
	}
	return s.s3.PresignGetObject(ctx, key, 24*time.Hour)
}

// ListAll returns all users with optional search and role filters.
func (s *UserService) ListAll(ctx context.Context, search, role string, limit, offset int) ([]model.User, int, error) {
	if limit <= 0 {
		limit = 50
	}

	countQuery := `SELECT COUNT(*) FROM users WHERE 1=1`
	dataQuery := `SELECT id, email, COALESCE(name,''), COALESCE(display_name,''), COALESCE(avatar_url,''), COALESCE(bio,''), COALESCE(phone,''), role, is_active, is_onboarded, last_login_at, created_at, updated_at FROM users WHERE 1=1`

	var args []interface{}
	argIdx := 1

	if search != "" {
		filter := fmt.Sprintf(` AND (LOWER(name) LIKE LOWER($%d) OR LOWER(email) LIKE LOWER($%d) OR LOWER(display_name) LIKE LOWER($%d))`, argIdx, argIdx, argIdx)
		countQuery += filter
		dataQuery += filter
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if role != "" {
		filter := fmt.Sprintf(` AND role = $%d`, argIdx)
		countQuery += filter
		dataQuery += filter
		args = append(args, role)
		argIdx++
	}

	var total int
	err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	dataQuery += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []model.User
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.Phone,
			&u.Role, &u.IsActive, &u.IsOnboarded, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if users == nil {
		users = []model.User{}
	}
	return users, total, nil
}

// ChangeRole updates a user's role and logs the action.
func (s *UserService) ChangeRole(ctx context.Context, userID uuid.UUID, newRole string, actorID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get current role for audit
	var currentRole string
	err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&currentRole)
	if err != nil {
		return fmt.Errorf("get current role: %w", err)
	}

	if currentRole == "super_admin" {
		return fmt.Errorf("cannot change super_admin role")
	}

	if currentRole == newRole {
		return nil // no change needed
	}

	_, err = tx.Exec(ctx, `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`, newRole, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Log to audit after successful commit
	if s.audit != nil {
		s.audit.Log(ctx, AuditEntry{
			UserID:     &actorID,
			Action:     model.AuditUserRoleChange,
			EntityType: "user",
			EntityID:   &userID,
			OldValues:  map[string]string{"role": currentRole},
			NewValues:  map[string]string{"role": newRole},
		})
	}

	return nil
}

func (s *UserService) SaveOnboardDraft(ctx context.Context, userID uuid.UUID, draft json.RawMessage) error {
	if s.rdb == nil {
		return fmt.Errorf("redis not available")
	}
	key := fmt.Sprintf("onboard_draft:%s", userID.String())
	return s.rdb.Set(ctx, key, string(draft), 24*time.Hour).Err()
}

func (s *UserService) GetCreatorCountByCity(ctx context.Context, city string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM creator_profiles WHERE LOWER(city) = LOWER($1)`,
		strings.TrimSpace(city),
	).Scan(&count)
	return count, err
}
