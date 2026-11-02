package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type PublishService struct {
	pool *pgxpool.Pool
}

func NewPublishService(pool *pgxpool.Pool) *PublishService {
	return &PublishService{pool: pool}
}

type CreatePublishRecordRequest struct {
	ProjectID         uuid.UUID  `json:"-"`
	ChannelID         uuid.UUID  `json:"channel_id"`
	PlatformURL       string     `json:"platform_url"`
	PlatformContentID string     `json:"platform_content_id"`
	Title             string     `json:"title"`
	Description       string     `json:"description"`
	Visibility        string     `json:"visibility"`
	PublishedAt       *time.Time `json:"published_at"`
}

type UpdatePublishRecordRequest struct {
	PlatformURL       *string    `json:"platform_url"`
	PlatformContentID *string    `json:"platform_content_id"`
	Title             *string    `json:"title"`
	Description       *string    `json:"description"`
	Visibility        *string    `json:"visibility"`
	Status            *string    `json:"status"`
	PublishedAt       *time.Time `json:"published_at"`
}

type ChannelSuggestion struct {
	Channel         model.Channel `json:"channel"`
	MatchingFormats []string      `json:"matching_formats"`
	Reason          string        `json:"reason"`
}

// Format-to-platform mapping
var formatPlatform = map[string]string{
	"youtube_16_9":   "youtube",
	"youtube_short":  "youtube",
	"instagram_reel": "instagram",
	"facebook_feed":  "facebook",
	"facebook_reel":  "facebook",
}

func (s *PublishService) CreateRecord(ctx context.Context, req CreatePublishRecordRequest) (*model.PublishRecordWithChannel, error) {
	if req.PlatformURL == "" {
		return nil, fmt.Errorf("platform_url is required")
	}

	// Validate project exists and is published
	var projectStatus string
	var creatorID *uuid.UUID
	var projectTitle string
	err := s.pool.QueryRow(ctx,
		`SELECT status::text, creator_id, title FROM projects WHERE id = $1`, req.ProjectID,
	).Scan(&projectStatus, &creatorID, &projectTitle)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if projectStatus != "published" {
		return nil, fmt.Errorf("project must be in 'published' status to create publish records")
	}

	visibility := req.Visibility
	if visibility == "" {
		visibility = "public"
	}

	status := "published"
	publishedAt := req.PublishedAt
	if publishedAt == nil {
		now := time.Now()
		publishedAt = &now
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var pr model.PublishRecordWithChannel
	err = tx.QueryRow(ctx,
		`INSERT INTO publish_records (project_id, channel_id, platform_content_id, platform_url, title, description, visibility, status, published_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, project_id, channel_id, COALESCE(platform_content_id, ''), COALESCE(platform_url, ''), COALESCE(title, ''), COALESCE(description, ''), visibility, status, COALESCE(error_message, ''), published_at, created_at, updated_at`,
		req.ProjectID, req.ChannelID,
		nilIfEmptyStr(req.PlatformContentID), req.PlatformURL, nilIfEmptyStr(req.Title), nilIfEmptyStr(req.Description),
		visibility, status, publishedAt,
	).Scan(&pr.ID, &pr.ProjectID, &pr.ChannelID,
		&pr.PlatformContentID, &pr.PlatformURL, &pr.Title, &pr.Description, &pr.Visibility,
		&pr.Status, &pr.ErrorMessage, &pr.PublishedAt, &pr.CreatedAt, &pr.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert publish record: %w", err)
	}

	// Fetch channel info
	err = tx.QueryRow(ctx,
		`SELECT channel_name, platform FROM channels WHERE id = $1`, pr.ChannelID,
	).Scan(&pr.ChannelName, &pr.Platform)
	if err != nil {
		return nil, fmt.Errorf("get channel info: %w", err)
	}

	// Create notification for creator
	if creatorID != nil {
		notifTitle := fmt.Sprintf("Content Published: %s", projectTitle)
		notifBody := fmt.Sprintf("Your content has been published on %s (%s)", pr.ChannelName, pr.Platform)
		notifLink := fmt.Sprintf("/projects/%s", req.ProjectID)
		_, err = tx.Exec(ctx,
			`INSERT INTO notifications (user_id, title, body, link, type, is_read)
			 VALUES ($1, $2, $3, $4, 'platform_publish', FALSE)`,
			*creatorID, notifTitle, notifBody, notifLink)
		if err != nil {
			return nil, fmt.Errorf("insert notification: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &pr, nil
}

func (s *PublishService) ListByProject(ctx context.Context, projectID uuid.UUID, limit, offset int) ([]model.PublishRecordWithChannel, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM publish_records WHERE project_id = $1`, projectID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count publish records: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT pr.id, pr.project_id, pr.channel_id,
		        c.channel_name, c.platform,
		        COALESCE(pr.platform_content_id, ''), COALESCE(pr.platform_url, ''),
		        COALESCE(pr.title, ''), COALESCE(pr.description, ''), pr.visibility,
		        pr.status, COALESCE(pr.error_message, ''), pr.published_at, pr.created_at, pr.updated_at
		 FROM publish_records pr
		 JOIN channels c ON c.id = pr.channel_id
		 WHERE pr.project_id = $1
		 ORDER BY pr.created_at DESC
		 LIMIT $2 OFFSET $3`,
		projectID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query publish records: %w", err)
	}
	defer rows.Close()

	records, err := scanPublishRecords(rows)
	return records, total, err
}

func (s *PublishService) UpdateRecord(ctx context.Context, id uuid.UUID, req UpdatePublishRecordRequest) (*model.PublishRecordWithChannel, error) {
	var sets []string
	var args []interface{}
	argIdx := 1

	if req.PlatformURL != nil {
		sets = append(sets, fmt.Sprintf("platform_url = $%d", argIdx))
		args = append(args, *req.PlatformURL)
		argIdx++
	}
	if req.PlatformContentID != nil {
		sets = append(sets, fmt.Sprintf("platform_content_id = $%d", argIdx))
		args = append(args, *req.PlatformContentID)
		argIdx++
	}
	if req.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if req.Visibility != nil {
		sets = append(sets, fmt.Sprintf("visibility = $%d", argIdx))
		args = append(args, *req.Visibility)
		argIdx++
	}
	if req.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}
	if req.PublishedAt != nil {
		sets = append(sets, fmt.Sprintf("published_at = $%d", argIdx))
		args = append(args, *req.PublishedAt)
		argIdx++
	}

	if len(sets) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(
		`UPDATE publish_records SET %s WHERE id = $%d
		 RETURNING id, project_id, channel_id,
		           COALESCE(platform_content_id, ''), COALESCE(platform_url, ''),
		           COALESCE(title, ''), COALESCE(description, ''), visibility,
		           status, COALESCE(error_message, ''), published_at, created_at, updated_at`,
		strings.Join(sets, ", "), argIdx,
	)

	var pr model.PublishRecordWithChannel
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&pr.ID, &pr.ProjectID, &pr.ChannelID,
		&pr.PlatformContentID, &pr.PlatformURL, &pr.Title, &pr.Description, &pr.Visibility,
		&pr.Status, &pr.ErrorMessage, &pr.PublishedAt, &pr.CreatedAt, &pr.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("publish record not found")
	}
	if err != nil {
		return nil, fmt.Errorf("update publish record: %w", err)
	}

	// Fetch channel info
	err = s.pool.QueryRow(ctx,
		`SELECT channel_name, platform FROM channels WHERE id = $1`, pr.ChannelID,
	).Scan(&pr.ChannelName, &pr.Platform)
	if err != nil {
		return nil, fmt.Errorf("get channel info: %w", err)
	}

	return &pr, nil
}

func (s *PublishService) DeleteRecord(ctx context.Context, id uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM publish_records WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete publish record: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("publish record not found")
	}
	return nil
}

func (s *PublishService) GetSuggestedChannels(ctx context.Context, projectID uuid.UUID) ([]ChannelSuggestion, error) {
	// Get project's category_id
	var categoryID *uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT category_id FROM projects WHERE id = $1`, projectID,
	).Scan(&categoryID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	// Get video formats for this project (exclude raw)
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT format::text FROM videos WHERE project_id = $1 AND format != 'raw' AND is_deleted = FALSE`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query video formats: %w", err)
	}
	defer rows.Close()

	// Build platform → formats map
	platformFormats := make(map[string][]string)
	for rows.Next() {
		var format string
		if err := rows.Scan(&format); err != nil {
			return nil, fmt.Errorf("scan format: %w", err)
		}
		if platform, ok := formatPlatform[format]; ok {
			platformFormats[platform] = append(platformFormats[platform], format)
		}
	}

	if len(platformFormats) == 0 {
		return []ChannelSuggestion{}, nil
	}

	// Get channels matching the category
	var channelRows interface{ Close() }
	var channels []model.Channel

	if categoryID != nil {
		chRows, err := s.pool.Query(ctx,
			`SELECT `+channelColumns+`
			 FROM channels WHERE category_id = $1 AND is_active = TRUE ORDER BY channel_name`, *categoryID)
		if err != nil {
			return nil, fmt.Errorf("query channels: %w", err)
		}
		channelRows = chRows
		for chRows.Next() {
			var c model.Channel
			if err := scanChannel(chRows, &c); err != nil {
				return nil, fmt.Errorf("scan channel: %w", err)
			}
			channels = append(channels, c)
		}
	}
	if channelRows != nil {
		channelRows.Close()
	}

	// Build suggestions
	var suggestions []ChannelSuggestion
	for _, ch := range channels {
		if formats, ok := platformFormats[ch.Platform]; ok {
			suggestions = append(suggestions, ChannelSuggestion{
				Channel:         ch,
				MatchingFormats: formats,
				Reason:          fmt.Sprintf("Matches %s formats in this category", ch.Platform),
			})
		}
	}

	if suggestions == nil {
		suggestions = []ChannelSuggestion{}
	}
	return suggestions, nil
}

func (s *PublishService) GetCategoryChannels(ctx context.Context, projectID uuid.UUID) ([]model.Channel, error) {
	var categoryID *uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT category_id FROM projects WHERE id = $1`, projectID,
	).Scan(&categoryID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if categoryID == nil {
		return []model.Channel{}, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+channelColumns+`
		 FROM channels
		 WHERE category_id = $1 AND is_active = TRUE AND connection_status = 'connected'
		 ORDER BY platform, channel_name`, *categoryID)
	if err != nil {
		return nil, fmt.Errorf("query category channels: %w", err)
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

func scanPublishRecords(rows pgx.Rows) ([]model.PublishRecordWithChannel, error) {
	var records []model.PublishRecordWithChannel
	for rows.Next() {
		var pr model.PublishRecordWithChannel
		if err := rows.Scan(
			&pr.ID, &pr.ProjectID, &pr.ChannelID,
			&pr.ChannelName, &pr.Platform,
			&pr.PlatformContentID, &pr.PlatformURL, &pr.Title, &pr.Description, &pr.Visibility,
			&pr.Status, &pr.ErrorMessage, &pr.PublishedAt, &pr.CreatedAt, &pr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan publish record: %w", err)
		}
		records = append(records, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publish records: %w", err)
	}
	if records == nil {
		records = []model.PublishRecordWithChannel{}
	}
	return records, nil
}
