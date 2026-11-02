package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/model"
)

type AuthService struct {
	pool        *pgxpool.Pool
	rdb         *redis.Client
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	oauthCfgs   map[string]*oauth2.Config
	email       EmailSenderFunc
	frontendURL string
}

// EmailSenderFunc is a function signature for sending emails.
// It decouples the auth service from the infra email package.
type EmailSenderFunc func(to, subject, htmlBody string) error

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type GoogleUserInfo struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func NewAuthService(cfg *config.Config, pool *pgxpool.Pool, rdb *redis.Client, emailFn ...EmailSenderFunc) *AuthService {
	oauthCfgs := map[string]*oauth2.Config{
		"google": {
			ClientID:     cfg.OAuth.Google.ClientID,
			ClientSecret: cfg.OAuth.Google.ClientSecret,
			RedirectURL:  cfg.OAuth.Google.RedirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
	}

	var fn EmailSenderFunc
	if len(emailFn) > 0 && emailFn[0] != nil {
		fn = emailFn[0]
	}

	return &AuthService{
		pool:        pool,
		rdb:         rdb,
		jwtSecret:   []byte(cfg.JWT.Secret),
		accessTTL:   cfg.JWT.AccessTokenTTL,
		refreshTTL:  cfg.JWT.RefreshTokenTTL,
		oauthCfgs:   oauthCfgs,
		email:       fn,
		frontendURL: cfg.App.FrontendURL,
	}
}

func (s *AuthService) GetOAuthURL(provider string) (string, error) {
	cfg, ok := s.oauthCfgs[provider]
	if !ok {
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	state, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	ctx := context.Background()
	if err := s.rdb.Set(ctx, "oauth_state:"+state, provider, 10*time.Minute).Err(); err != nil {
		return "", fmt.Errorf("store state: %w", err)
	}

	return cfg.AuthCodeURL(state, oauth2.AccessTypeOffline), nil
}

func (s *AuthService) HandleCallback(ctx context.Context, provider, code, state string) (*TokenPair, *model.User, error) {
	// Validate state
	storedProvider, err := s.rdb.GetDel(ctx, "oauth_state:"+state).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid or expired state")
	}
	if storedProvider != provider {
		return nil, nil, fmt.Errorf("state provider mismatch")
	}

	cfg, ok := s.oauthCfgs[provider]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported provider: %s", provider)
	}

	// Exchange code for token
	oauthToken, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("exchange code: %w", err)
	}

	// Get user info from provider
	userInfo, err := s.getGoogleUserInfo(ctx, oauthToken)
	if err != nil {
		return nil, nil, fmt.Errorf("get user info: %w", err)
	}

	// Upsert user in transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if oauth account exists
	var userID uuid.UUID
	var user model.User
	var displayName, avatarURL, bio, phone *string
	err = tx.QueryRow(ctx,
		`SELECT u.id, u.email, u.name, u.display_name, u.avatar_url, u.bio, u.phone,
		        u.role, u.is_active, u.is_onboarded, u.last_login_at, u.created_at, u.updated_at
		 FROM oauth_accounts oa JOIN users u ON u.id = oa.user_id
		 WHERE oa.provider = $1 AND oa.provider_id = $2`,
		provider, userInfo.ID,
	).Scan(&user.ID, &user.Email, &user.Name, &displayName, &avatarURL,
		&bio, &phone, &user.Role, &user.IsActive, &user.IsOnboarded,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err == nil {
		if displayName != nil {
			user.DisplayName = *displayName
		}
		if avatarURL != nil {
			user.AvatarURL = *avatarURL
		}
		if bio != nil {
			user.Bio = *bio
		}
		if phone != nil {
			user.Phone = *phone
		}
	}

	if err == pgx.ErrNoRows {
		// New user - create user + oauth_account + creator_profile
		err = tx.QueryRow(ctx,
			`INSERT INTO users (email, name, display_name, avatar_url, role, is_active, is_onboarded)
			 VALUES ($1, $2, $2, $3, 'creator', TRUE, FALSE)
			 RETURNING id, email, name, display_name, avatar_url, bio, phone, role, is_active, is_onboarded, last_login_at, created_at, updated_at`,
			userInfo.Email, userInfo.Name, userInfo.Picture,
		).Scan(&user.ID, &user.Email, &user.Name, &displayName, &avatarURL,
			&bio, &phone, &user.Role, &user.IsActive, &user.IsOnboarded,
			&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
		if err == nil {
			if displayName != nil {
				user.DisplayName = *displayName
			}
			if avatarURL != nil {
				user.AvatarURL = *avatarURL
			}
			if bio != nil {
				user.Bio = *bio
			}
			if phone != nil {
				user.Phone = *phone
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("create user: %w", err)
		}
		userID = user.ID

		// Create oauth account
		rawProfile, _ := json.Marshal(userInfo)
		_, err = tx.Exec(ctx,
			`INSERT INTO oauth_accounts (user_id, provider, provider_id, access_token, refresh_token, token_expires_at, raw_profile)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			userID, provider, userInfo.ID, oauthToken.AccessToken, oauthToken.RefreshToken, oauthToken.Expiry, rawProfile,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create oauth account: %w", err)
		}

		// Create creator profile with slug from email
		slug := generateSlug(userInfo.Email)
		_, err = tx.Exec(ctx,
			`INSERT INTO creator_profiles (user_id, slug) VALUES ($1, $2)`,
			userID, slug,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create creator profile: %w", err)
		}

		slog.Info("new user created via OAuth", "user_id", userID, "provider", provider)
	} else if err != nil {
		return nil, nil, fmt.Errorf("lookup user: %w", err)
	} else {
		userID = user.ID
		// Update oauth tokens and last login
		_, err = tx.Exec(ctx,
			`UPDATE oauth_accounts SET access_token = $1, refresh_token = $2, token_expires_at = $3, updated_at = NOW()
			 WHERE user_id = $4 AND provider = $5`,
			oauthToken.AccessToken, oauthToken.RefreshToken, oauthToken.Expiry, userID, provider,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("update oauth tokens: %w", err)
		}
	}

	// Update last login
	_, err = tx.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("update last login: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}

	// Issue tokens
	accessToken, expiresAt, err := s.issueAccessToken(&user)
	if err != nil {
		return nil, nil, fmt.Errorf("issue access token: %w", err)
	}

	familyID := uuid.New()
	refreshToken, err := s.issueRefreshToken(ctx, userID, familyID)
	if err != nil {
		return nil, nil, fmt.Errorf("issue refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, &user, nil
}

func (s *AuthService) issueAccessToken(user *model.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.accessTTL)
	claims := jwt.RegisteredClaims{
		Subject:   user.ID.String(),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	// Use MapClaims to add custom fields
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   claims.Subject,
		"exp":   claims.ExpiresAt.Unix(),
		"iat":   claims.IssuedAt.Unix(),
		"email": user.Email,
		"role":  string(user.Role),
	})

	signed, err := token.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func (s *AuthService) issueRefreshToken(ctx context.Context, userID, familyID uuid.UUID) (string, error) {
	raw, err := generateRandomString(64)
	if err != nil {
		return "", err
	}

	hash := sha256Hash(raw)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		userID, hash, familyID, time.Now().Add(s.refreshTTL),
	)
	if err != nil {
		return "", err
	}

	return raw, nil
}

func (s *AuthService) RefreshAccessToken(ctx context.Context, rawToken string) (*TokenPair, error) {
	hash := sha256Hash(rawToken)

	var rt model.RefreshToken
	var user model.User
	err := s.pool.QueryRow(ctx,
		`SELECT rt.id, rt.user_id, rt.token_hash, rt.family_id, rt.is_revoked, rt.expires_at,
		        u.id, u.email, u.name, COALESCE(u.display_name,''), u.role, u.is_active, u.is_onboarded
		 FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
		 WHERE rt.token_hash = $1`, hash,
	).Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.FamilyID, &rt.IsRevoked, &rt.ExpiresAt,
		&user.ID, &user.Email, &user.Name, &user.DisplayName, &user.Role, &user.IsActive, &user.IsOnboarded)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token")
	}

	// Replay detection: if revoked, kill the whole family
	if rt.IsRevoked {
		s.revokeFamily(ctx, rt.FamilyID)
		return nil, fmt.Errorf("refresh token reuse detected, family revoked")
	}

	if time.Now().After(rt.ExpiresAt) {
		return nil, fmt.Errorf("refresh token expired")
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account deactivated")
	}

	// Revoke the old token
	_, _ = s.pool.Exec(ctx, `UPDATE refresh_tokens SET is_revoked = TRUE WHERE id = $1`, rt.ID)

	// Issue new pair
	accessToken, expiresAt, err := s.issueAccessToken(&user)
	if err != nil {
		return nil, err
	}

	newRefresh, err := s.issueRefreshToken(ctx, user.ID, rt.FamilyID)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    expiresAt,
	}, nil
}

func (s *AuthService) revokeFamily(ctx context.Context, familyID uuid.UUID) {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET is_revoked = TRUE WHERE family_id = $1`, familyID)
	if err != nil {
		slog.Error("failed to revoke token family", "family_id", familyID, "error", err)
	}
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_tokens SET is_revoked = TRUE WHERE user_id = $1 AND is_revoked = FALSE`, userID)
	return err
}

func (s *AuthService) getGoogleUserInfo(ctx context.Context, token *oauth2.Token) (*GoogleUserInfo, error) {
	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google API error: %s", string(body))
	}

	var info GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

type SignupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*TokenPair, *model.User, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" || req.Name == "" {
		return nil, nil, fmt.Errorf("email, password, and name are required")
	}
	if len(req.Password) < 8 {
		return nil, nil, fmt.Errorf("password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check if email already exists
	var exists bool
	_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists)
	if exists {
		return nil, nil, fmt.Errorf("email already registered")
	}

	var user model.User
	var displayName, avatarURL, bio, phone *string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (email, name, display_name, password_hash, role, is_active, is_onboarded)
		 VALUES ($1, $2, $2, $3, 'creator', TRUE, FALSE)
		 RETURNING id, email, name, display_name, avatar_url, bio, phone, role, is_active, is_onboarded, last_login_at, created_at, updated_at`,
		email, req.Name, string(hash),
	).Scan(&user.ID, &user.Email, &user.Name, &displayName, &avatarURL,
		&bio, &phone, &user.Role, &user.IsActive, &user.IsOnboarded,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create user: %w", err)
	}
	if displayName != nil {
		user.DisplayName = *displayName
	}
	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}
	if phone != nil {
		user.Phone = *phone
	}

	// Create creator profile
	slug := generateSlug(email)
	_, err = tx.Exec(ctx, `INSERT INTO creator_profiles (user_id, slug) VALUES ($1, $2)`, user.ID, slug)
	if err != nil {
		return nil, nil, fmt.Errorf("create creator profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit: %w", err)
	}

	accessToken, expiresAt, err := s.issueAccessToken(&user)
	if err != nil {
		return nil, nil, fmt.Errorf("issue access token: %w", err)
	}
	familyID := uuid.New()
	refreshToken, err := s.issueRefreshToken(ctx, user.ID, familyID)
	if err != nil {
		return nil, nil, fmt.Errorf("issue refresh token: %w", err)
	}

	slog.Info("new user signed up", "user_id", user.ID, "email", email)

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, &user, nil
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*TokenPair, *model.User, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		return nil, nil, fmt.Errorf("email and password are required")
	}

	var user model.User
	var passwordHash *string
	var displayName, avatarURL, bio, phone *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, name, display_name, avatar_url, bio, phone, password_hash,
		        role, is_active, is_onboarded, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&user.ID, &user.Email, &user.Name, &displayName, &avatarURL,
		&bio, &phone, &passwordHash,
		&user.Role, &user.IsActive, &user.IsOnboarded,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil, fmt.Errorf("invalid email or password")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lookup user: %w", err)
	}
	if displayName != nil {
		user.DisplayName = *displayName
	}
	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}
	if phone != nil {
		user.Phone = *phone
	}

	if passwordHash == nil || *passwordHash == "" {
		return nil, nil, fmt.Errorf("this account uses Google sign-in")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*passwordHash), []byte(req.Password)); err != nil {
		return nil, nil, fmt.Errorf("invalid email or password")
	}

	if !user.IsActive {
		return nil, nil, fmt.Errorf("account is deactivated")
	}

	// Update last login
	_, _ = s.pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, user.ID)

	accessToken, expiresAt, err := s.issueAccessToken(&user)
	if err != nil {
		return nil, nil, fmt.Errorf("issue access token: %w", err)
	}
	familyID := uuid.New()
	refreshToken, err := s.issueRefreshToken(ctx, user.ID, familyID)
	if err != nil {
		return nil, nil, fmt.Errorf("issue refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, &user, nil
}

func generateSlug(email string) string {
	// Take part before @ and replace special chars
	parts := []byte(email)
	slug := make([]byte, 0, len(parts))
	for _, b := range parts {
		if b == '@' {
			break
		}
		if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			slug = append(slug, b)
		} else if b >= 'A' && b <= 'Z' {
			slug = append(slug, b+32) // lowercase
		} else if b == '.' || b == '-' || b == '_' {
			slug = append(slug, '-')
		}
	}
	// Append random suffix to ensure uniqueness
	suffix, _ := generateRandomString(4)
	return string(slug) + "-" + suffix[:8]
}

func (s *AuthService) ForgotPassword(ctx context.Context, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil // silently return
	}

	// Check if user exists and has a password (not OAuth-only)
	var userID uuid.UUID
	var userName string
	var passwordHash *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, password_hash FROM users WHERE email = $1 AND is_active = TRUE`, email,
	).Scan(&userID, &userName, &passwordHash)
	if err != nil {
		return nil // silently return - prevents email enumeration
	}

	// If user has no password_hash, they signed up via OAuth only
	if passwordHash == nil || *passwordHash == "" {
		slog.Info("forgot password for OAuth-only user, skipping", "email", email)
		return nil
	}

	// Rate limit: check if a reset was already requested recently
	existingKey := "pwd_reset_rate:" + email
	exists, _ := s.rdb.Exists(ctx, existingKey).Result()
	if exists > 0 {
		slog.Info("forgot password rate limited", "email", email)
		return nil // silently return - prevents abuse
	}

	// Generate reset token
	token, err := generateRandomString(32)
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	// Store in Redis with 1 hour TTL
	hash := sha256Hash(token)
	if err := s.rdb.Set(ctx, "pwd_reset:"+hash, userID.String(), time.Hour).Err(); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	// Rate limit: prevent another request for 2 minutes
	_ = s.rdb.Set(ctx, existingKey, "1", 2*time.Minute).Err()

	// Build reset link
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.frontendURL, token)

	// Log token in dev for testing
	slog.Info("password reset token generated", "email", email, "token", token, "user_id", userID, "link", resetLink)

	// Send email if email sender is configured
	if s.email != nil {
		subject := "Reset your Social Publisher password"
		body := buildPasswordResetEmail(userName, resetLink)
		if err := s.email(email, subject, body); err != nil {
			slog.Error("failed to send password reset email", "email", email, "error", err)
			return fmt.Errorf("send email: %w", err)
		}
	}

	return nil
}

func buildPasswordResetEmail(name, resetLink string) string {
	if name == "" {
		name = "there"
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
  <div style="text-align: center; margin-bottom: 32px;">
    <h1 style="color: #2563eb; margin: 0;">Social Publisher</h1>
  </div>
  <h2 style="margin-top: 0;">Hi %s,</h2>
  <p>We received a request to reset your password. Click the button below to create a new password:</p>
  <div style="text-align: center; margin: 32px 0;">
    <a href="%s" style="display: inline-block; background: #2563eb; color: #fff; text-decoration: none; padding: 12px 32px; border-radius: 6px; font-weight: 600; font-size: 16px;">
      Reset Password
    </a>
  </div>
  <p style="color: #666; font-size: 14px;">This link expires in 1 hour. If you did not request a password reset, you can safely ignore this email.</p>
  <hr style="border: none; border-top: 1px solid #e5e7eb; margin: 32px 0;">
  <p style="color: #9ca3af; font-size: 12px; text-align: center;">Social Publisher</p>
</body>
</html>`, name, resetLink)
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	hash := sha256Hash(token)
	userIDStr, err := s.rdb.GetDel(ctx, "pwd_reset:"+hash).Result()
	if err != nil {
		return fmt.Errorf("invalid or expired reset token")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user ID in token")
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.pool.Exec(ctx, `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, string(passwordHash), userID)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	// Revoke all refresh tokens
	_, _ = s.pool.Exec(ctx, `UPDATE refresh_tokens SET is_revoked = TRUE WHERE user_id = $1 AND is_revoked = FALSE`, userID)

	slog.Info("password reset completed", "user_id", userID)

	return nil
}
