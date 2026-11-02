package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ChannelTargetService struct {
	pool *pgxpool.Pool
}

func NewChannelTargetService(pool *pgxpool.Pool) *ChannelTargetService {
	return &ChannelTargetService{pool: pool}
}

type CreateChannelTargetRequest struct {
	VideoID               *uuid.UUID `json:"video_id,omitempty"`
	ChannelID             uuid.UUID  `json:"channel_id"`
	ScheduledAt           *time.Time `json:"scheduled_at,omitempty"`
	PerChannelTitle       string     `json:"per_channel_title,omitempty"`
	PerChannelDescription string     `json:"per_channel_description,omitempty"`
}

func (s *ChannelTargetService) CreateBulk(ctx context.Context, projectID, userID uuid.UUID, isAdmin bool, targets []CreateChannelTargetRequest) ([]model.ChannelTargetWithChannel, error) {
	var creatorID *uuid.UUID
	var projectTitle string
	err := s.pool.QueryRow(ctx,
		`SELECT creator_id, title FROM projects WHERE id = $1`, projectID,
	).Scan(&creatorID, &projectTitle)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if !isAdmin && (creatorID == nil || *creatorID != userID) {
		return nil, fmt.Errorf("forbidden: not project creator")
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var results []model.ChannelTargetWithChannel
	for _, t := range targets {
		var ct model.ChannelTarget
		err := tx.QueryRow(ctx,
			`INSERT INTO channel_targets (project_id, video_id, channel_id, scheduled_at, created_by, per_channel_title, per_channel_description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id, project_id, video_id, channel_id, scheduled_at, created_by, created_at, COALESCE(per_channel_title, ''), COALESCE(per_channel_description, '')`,
			projectID, t.VideoID, t.ChannelID, t.ScheduledAt, userID,
			nilIfEmptyStr(t.PerChannelTitle), nilIfEmptyStr(t.PerChannelDescription),
		).Scan(&ct.ID, &ct.ProjectID, &ct.VideoID,
			&ct.ChannelID, &ct.ScheduledAt, &ct.CreatedBy, &ct.CreatedAt,
			&ct.PerChannelTitle, &ct.PerChannelDescription)
		if err != nil {
			return nil, fmt.Errorf("insert channel target: %w", err)
		}

		var channelName, platform string
		err = tx.QueryRow(ctx,
			`SELECT channel_name, platform FROM channels WHERE id = $1`, ct.ChannelID,
		).Scan(&channelName, &platform)
		if err != nil {
			return nil, fmt.Errorf("get channel info: %w", err)
		}

		var createdByName string
		_ = tx.QueryRow(ctx,
			`SELECT COALESCE(display_name, name) FROM users WHERE id = $1`, ct.CreatedBy,
		).Scan(&createdByName)

		results = append(results, model.ChannelTargetWithChannel{
			ChannelTarget: ct,
			ChannelName:   channelName,
			Platform:      platform,
			CreatedByName: createdByName,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return results, nil
}

func (s *ChannelTargetService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.ChannelTargetWithChannel, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT ct.id, ct.project_id, ct.video_id,
		        ct.channel_id, ct.scheduled_at, ct.created_by, ct.created_at,
		        COALESCE(ct.per_channel_title, ''), COALESCE(ct.per_channel_description, ''),
		        c.channel_name, c.platform,
		        COALESCE(u.display_name, u.name) AS created_by_name
		 FROM channel_targets ct
		 JOIN channels c ON c.id = ct.channel_id
		 JOIN users u ON u.id = ct.created_by
		 WHERE ct.project_id = $1
		 ORDER BY ct.created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query channel targets: %w", err)
	}
	defer rows.Close()

	var targets []model.ChannelTargetWithChannel
	for rows.Next() {
		var ct model.ChannelTargetWithChannel
		if err := rows.Scan(&ct.ID, &ct.ProjectID, &ct.VideoID,
			&ct.ChannelID, &ct.ScheduledAt, &ct.CreatedBy, &ct.CreatedAt,
			&ct.PerChannelTitle, &ct.PerChannelDescription,
			&ct.ChannelName, &ct.Platform,
			&ct.CreatedByName); err != nil {
			return nil, fmt.Errorf("scan channel target: %w", err)
		}
		targets = append(targets, ct)
	}
	if targets == nil {
		targets = []model.ChannelTargetWithChannel{}
	}
	return targets, nil
}

func (s *ChannelTargetService) Delete(ctx context.Context, targetID, userID uuid.UUID, isAdmin bool) error {
	var createdBy uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT created_by FROM channel_targets WHERE id = $1`, targetID,
	).Scan(&createdBy)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("channel target not found")
	}
	if err != nil {
		return fmt.Errorf("get channel target: %w", err)
	}
	if !isAdmin && createdBy != userID {
		return fmt.Errorf("forbidden: not target creator")
	}

	_, err = s.pool.Exec(ctx, `DELETE FROM channel_targets WHERE id = $1`, targetID)
	if err != nil {
		return fmt.Errorf("delete channel target: %w", err)
	}
	return nil
}
