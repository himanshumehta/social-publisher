package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ProjectPostService handles auto-generation of posts from project content.
type ProjectPostService struct {
	pool   *pgxpool.Pool
	post   *PostService
	logger *slog.Logger
}

func NewProjectPostService(pool *pgxpool.Pool, post *PostService, logger *slog.Logger) *ProjectPostService {
	return &ProjectPostService{pool: pool, post: post, logger: logger}
}

// AutoGeneratePostsForProject creates draft posts from project content.
// Called async after a project transitions to in_review.
func (s *ProjectPostService) AutoGeneratePostsForProject(ctx context.Context, projectID uuid.UUID) {
	s.logger.Info("auto-generating posts for project", "project_id", projectID)

	// Get project details
	var title, description string
	var creatorID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT title, description, COALESCE(creator_id, '00000000-0000-0000-0000-000000000000')
		 FROM projects WHERE id = $1`, projectID,
	).Scan(&title, &description, &creatorID)
	if err != nil {
		s.logger.Error("auto-generate: failed to get project", "error", err)
		return
	}
	if creatorID == uuid.Nil {
		s.logger.Warn("auto-generate: no creator on project", "project_id", projectID)
		return
	}

	// Check if posts already exist for this project
	var existingCount int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM posts WHERE project_id = $1`, projectID,
	).Scan(&existingCount)
	if err != nil {
		s.logger.Error("auto-generate: failed to count posts", "error", err)
		return
	}
	if existingCount > 0 {
		s.logger.Info("auto-generate: posts already exist, skipping", "project_id", projectID, "count", existingCount)
		return
	}

	// Determine target platforms from channel targets (exclude youtube — it gets video, not posts)
	allPlatforms, err := s.getTargetPlatforms(ctx, projectID)
	if err != nil {
		s.logger.Error("auto-generate: failed to get platforms", "error", err)
		return
	}
	var platforms []string
	for _, p := range allPlatforms {
		if p != "youtube" {
			platforms = append(platforms, p)
		}
	}
	if len(platforms) == 0 {
		platforms = []string{"twitter", "instagram", "linkedin"}
	}

	// Create a post with project title/description as content
	content := description
	if content == "" {
		content = title
	}

	req := &model.CreatePostRequest{
		Title:           title,
		Content:         content,
		TargetPlatforms: platforms,
		AutoPublish:     false,
	}

	post, err := s.post.Create(ctx, creatorID, req)
	if err != nil {
		s.logger.Error("auto-generate: failed to create post", "error", err)
		return
	}

	// Link post to project
	_, err = s.pool.Exec(ctx,
		`UPDATE posts SET project_id = $1 WHERE id = $2`,
		projectID, post.ID)
	if err != nil {
		s.logger.Error("auto-generate: failed to link post to project", "error", err)
		return
	}

	s.logger.Info("auto-generated post for project",
		"project_id", projectID,
		"post_id", post.ID,
		"platforms", platforms,
	)
}

func (s *ProjectPostService) getTargetPlatforms(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ch.platform
		FROM channel_targets ct
		JOIN channels ch ON ch.id = ct.channel_id
		WHERE ct.project_id = $1`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query target platforms: %w", err)
	}
	defer rows.Close()

	var platforms []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}
