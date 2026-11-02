package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ChannelService struct {
	pool *pgxpool.Pool
}

func NewChannelService(pool *pgxpool.Pool) *ChannelService {
	return &ChannelService{pool: pool}
}

// channelColumns is the standard SELECT column list for channels.
const channelColumns = `id, category_id, user_id, channel_type, platform, channel_name, COALESCE(channel_id, ''), COALESCE(channel_url, ''), COALESCE(oauth_provider, ''), connection_status, is_active, token_expires_at, created_at, updated_at`

func scanChannel(row interface{ Scan(dest ...any) error }, c *model.Channel) error {
	return row.Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
}

type CreateChannelRequest struct {
	CategoryID  *uuid.UUID `json:"category_id,omitempty"`
	Platform    string     `json:"platform"`
	ChannelName string     `json:"channel_name"`
	ChannelID   string     `json:"channel_id"`
	ChannelURL  string     `json:"channel_url"`
}

type UpdateChannelRequest struct {
	ChannelName *string `json:"channel_name"`
	ChannelURL  *string `json:"channel_url"`
	ChannelID   *string `json:"channel_id"`
	IsActive    *bool   `json:"is_active"`
}

func (s *ChannelService) List(ctx context.Context, categoryID *uuid.UUID, platform string, limit, offset int) ([]model.Channel, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var args []interface{}
	var conditions []string
	argIdx := 1

	if categoryID != nil {
		conditions = append(conditions, fmt.Sprintf("category_id = $%d", argIdx))
		args = append(args, *categoryID)
		argIdx++
	}
	if platform != "" {
		conditions = append(conditions, fmt.Sprintf("platform = $%d", argIdx))
		args = append(args, platform)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM channels"+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count channels: %w", err)
	}

	query := fmt.Sprintf(
		`SELECT `+channelColumns+`
		 FROM channels%s ORDER BY created_at ASC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query channels: %w", err)
	}
	defer rows.Close()

	var channels []model.Channel
	for rows.Next() {
		var c model.Channel
		if err := scanChannel(rows, &c); err != nil {
			return nil, 0, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate channels: %w", err)
	}
	if channels == nil {
		channels = []model.Channel{}
	}
	return channels, total, nil
}

func (s *ChannelService) GetByID(ctx context.Context, id uuid.UUID) (*model.Channel, error) {
	var c model.Channel
	err := s.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE id = $1`, id,
	).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return &c, nil
}

func (s *ChannelService) Create(ctx context.Context, req CreateChannelRequest) (*model.Channel, error) {
	if req.ChannelName == "" {
		return nil, fmt.Errorf("channel_name is required")
	}
	if req.Platform == "" {
		return nil, fmt.Errorf("platform is required")
	}

	var c model.Channel
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channels (category_id, platform, channel_name, channel_id, channel_url, channel_type)
		 VALUES ($1, $2, $3, $4, $5, 'system')
		 RETURNING `+channelColumns,
		req.CategoryID, req.Platform, req.ChannelName, nilIfEmptyStr(req.ChannelID), nilIfEmptyStr(req.ChannelURL),
	).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return &c, nil
}

// CreateCreatorChannel creates a channel owned by a creator (via OAuth).
func (s *ChannelService) CreateCreatorChannel(ctx context.Context, userID uuid.UUID, platform, channelName, channelIDStr, channelURL string) (*model.Channel, error) {
	var c model.Channel
	err := s.pool.QueryRow(ctx,
		`INSERT INTO channels (user_id, channel_type, platform, channel_name, channel_id, channel_url, connection_status)
		 VALUES ($1, 'creator', $2, $3, $4, $5, 'connected')
		 RETURNING `+channelColumns,
		userID, platform, channelName, nilIfEmptyStr(channelIDStr), nilIfEmptyStr(channelURL),
	).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create creator channel: %w", err)
	}
	return &c, nil
}

// ListByUser returns channels owned by a specific user.
func (s *ChannelService) ListByUser(ctx context.Context, userID uuid.UUID) ([]model.Channel, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE user_id = $1 AND is_active = TRUE ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query user channels: %w", err)
	}
	defer rows.Close()

	var channels []model.Channel
	for rows.Next() {
		var c model.Channel
		if err := scanChannel(rows, &c); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
	}
	if channels == nil {
		channels = []model.Channel{}
	}
	return channels, nil
}

// ListAvailableForProject returns creator's channels + system channels for the project's category.
func (s *ChannelService) ListAvailableForProject(ctx context.Context, projectID, userID uuid.UUID) ([]model.Channel, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+channelColumns+`
		 FROM channels
		 WHERE is_active = TRUE AND connection_status = 'connected' AND (
		     user_id = $1
		     OR (channel_type = 'system' AND category_id = (SELECT category_id FROM projects WHERE id = $2))
		 )
		 ORDER BY channel_type DESC, created_at ASC`, userID, projectID)
	if err != nil {
		return nil, fmt.Errorf("query available channels: %w", err)
	}
	defer rows.Close()

	var channels []model.Channel
	for rows.Next() {
		var c model.Channel
		if err := scanChannel(rows, &c); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
	}
	if channels == nil {
		channels = []model.Channel{}
	}
	return channels, nil
}

func (s *ChannelService) Update(ctx context.Context, id uuid.UUID, req UpdateChannelRequest) (*model.Channel, error) {
	var sets []string
	var args []interface{}
	argIdx := 1

	if req.ChannelName != nil {
		sets = append(sets, fmt.Sprintf("channel_name = $%d", argIdx))
		args = append(args, *req.ChannelName)
		argIdx++
	}
	if req.ChannelURL != nil {
		sets = append(sets, fmt.Sprintf("channel_url = $%d", argIdx))
		args = append(args, *req.ChannelURL)
		argIdx++
	}
	if req.ChannelID != nil {
		sets = append(sets, fmt.Sprintf("channel_id = $%d", argIdx))
		args = append(args, *req.ChannelID)
		argIdx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetByID(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE channels SET %s WHERE id = $%d
		 RETURNING `+channelColumns,
		strings.Join(sets, ", "), argIdx,
	)

	var c model.Channel
	err := s.pool.QueryRow(ctx, query, args...).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, fmt.Errorf("update channel: %w", err)
	}
	return &c, nil
}

func (s *ChannelService) Delete(ctx context.Context, id uuid.UUID) error {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM publish_records WHERE channel_id = $1 AND status NOT IN ('removed', 'failed')`, id,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check publish records: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("cannot delete channel with %d active publish records", count)
	}

	result, err := s.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("channel not found")
	}
	return nil
}

// GetByIDAndUser returns a channel only if it belongs to the given user.
func (s *ChannelService) GetByIDAndUser(ctx context.Context, id, userID uuid.UUID) (*model.Channel, error) {
	var c model.Channel
	err := s.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID, &c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get channel by user: %w", err)
	}
	return &c, nil
}

// HasPlatformChannel returns true if the user already has a creator channel for the given platform.
func (s *ChannelService) HasPlatformChannel(ctx context.Context, userID uuid.UUID, platform string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM channels WHERE user_id = $1 AND platform = $2 AND channel_type = 'creator')`,
		userID, platform,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check platform channel: %w", err)
	}
	return exists, nil
}

// DeleteByUser deletes a creator channel owned by the given user.
func (s *ChannelService) DeleteByUser(ctx context.Context, id, userID uuid.UUID) error {
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM publish_records WHERE channel_id = $1 AND status NOT IN ('removed', 'failed')`, id,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("check publish records: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("cannot delete channel with %d active publish records", count)
	}

	result, err := s.pool.Exec(ctx, `DELETE FROM channels WHERE id = $1 AND user_id = $2 AND channel_type = 'creator'`, id, userID)
	if err != nil {
		return fmt.Errorf("delete creator channel: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("channel not found")
	}
	return nil
}

func nilIfEmptyStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
