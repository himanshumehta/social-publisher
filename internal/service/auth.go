package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/model"
)

type AuthService struct {
	pool       *pgxpool.Pool
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func NewAuthService(cfg *config.Config, pool *pgxpool.Pool) *AuthService {
	return &AuthService{
		pool:       pool,
		jwtSecret:  []byte(cfg.JWT.Secret),
		accessTTL:  cfg.JWT.AccessTokenTTL,
		refreshTTL: cfg.JWT.RefreshTokenTTL,
	}
}

func (s *AuthService) issueAccessToken(user *model.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.accessTTL)
	claims := jwt.RegisteredClaims{
		Subject:   user.ID.String(),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}

	// Use MapClaims to add custom fields
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, // must match middleware.SigningMethod
		jwt.MapClaims{
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

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*TokenPair, *model.User, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		return nil, nil, ErrInvalidCredentials
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
	if errors.Is(err, pgx.ErrNoRows) {
		checkPassword(nil, req.Password) // same cost as a real comparison
		return nil, nil, ErrInvalidCredentials
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

	if !checkPassword(passwordHash, req.Password) || !user.IsActive {
		return nil, nil, ErrInvalidCredentials
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
