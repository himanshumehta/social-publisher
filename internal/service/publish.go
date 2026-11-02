package service

import (
	"context"
	"fmt"

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

// Format-to-platform mapping
var formatPlatform = map[string]string{
	"youtube_16_9":   "youtube",
	"youtube_short":  "youtube",
	"instagram_reel": "instagram",
	"facebook_feed":  "facebook",
	"facebook_reel":  "facebook",
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
