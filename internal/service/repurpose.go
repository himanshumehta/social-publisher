package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type RepurposeService struct {
	pool *pgxpool.Pool
}

func NewRepurposeService(pool *pgxpool.Pool) *RepurposeService {
	return &RepurposeService{pool: pool}
}

// CreateJob validates the request and creates a repurpose job + a backing ai_job.
func (s *RepurposeService) CreateJob(ctx context.Context, userID uuid.UUID, req *model.CreateRepurposeRequest) (*model.RepurposeJob, error) {
	// Validate strategy
	strategy := model.RepurposeStrategy(req.Strategy)
	if !model.ValidStrategies[strategy] {
		return nil, fmt.Errorf("invalid repurpose strategy: %s", req.Strategy)
	}

	// Validate target platforms
	if len(req.TargetPlatforms) == 0 {
		return nil, fmt.Errorf("at least one target platform is required")
	}
	for _, p := range req.TargetPlatforms {
		if _, ok := model.PlatformCharLimits[p]; !ok {
			return nil, fmt.Errorf("unsupported target platform: %s", p)
		}
	}

	// Validate source post exists and belongs to user
	var postAuthorID uuid.UUID
	var postContent, postTitle string
	err := s.pool.QueryRow(ctx,
		`SELECT author_id, COALESCE(content, ''), COALESCE(title, '')
		 FROM posts WHERE id = $1`, req.SourcePostID,
	).Scan(&postAuthorID, &postContent, &postTitle)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("source post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query source post: %w", err)
	}
	if postAuthorID != userID {
		return nil, fmt.Errorf("source post does not belong to user")
	}

	// Validate minimum content length per strategy
	rules, ok := model.RepurposeContentRules[strategy]
	if ok && len(postContent) < rules.MinSourceLength {
		return nil, fmt.Errorf("source content too short for strategy %s: need at least %d characters", req.Strategy, rules.MinSourceLength)
	}

	// Marshal options
	optionsJSON := []byte("{}")
	if req.Options != nil {
		optionsJSON, err = json.Marshal(req.Options)
		if err != nil {
			return nil, fmt.Errorf("marshal options: %w", err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Create the backing ai_job (project_id is nullable in our repurpose context,
	// but ai_jobs requires it; we use a self-referencing lookup via the post).
	var projectID *uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT project_id FROM posts WHERE id = $1`, req.SourcePostID,
	).Scan(&projectID)

	// ai_jobs.project_id is NOT NULL, so if the post has no project we need a
	// sentinel. Use a zero UUID as a fallback (the worker routes by job_type, not
	// project_id).
	aiProjectID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	if projectID != nil {
		aiProjectID = *projectID
	}

	configJSON, _ := json.Marshal(map[string]interface{}{
		"repurpose_strategy": req.Strategy,
		"source_post_id":     req.SourcePostID.String(),
		"target_platforms":   req.TargetPlatforms,
	})

	var aiJobID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO ai_jobs (project_id, created_by, job_type, provider, config)
		 VALUES ($1, $2, 'repurpose_text', 'internal', $3)
		 RETURNING id`,
		aiProjectID, userID, configJSON,
	).Scan(&aiJobID)
	if err != nil {
		return nil, fmt.Errorf("insert ai_job: %w", err)
	}

	// Create the repurpose_job
	var job model.RepurposeJob
	err = tx.QueryRow(ctx,
		`INSERT INTO repurpose_jobs
		    (source_post_id, user_id, ai_job_id, strategy, target_platforms, options)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, source_post_id, user_id, ai_job_id, strategy, target_platforms,
		           options, status, error_message, created_at, updated_at`,
		req.SourcePostID, userID, aiJobID, req.Strategy, req.TargetPlatforms, optionsJSON,
	).Scan(
		&job.ID, &job.SourcePostID, &job.UserID, &job.AIJobID,
		&job.Strategy, &job.TargetPlatforms, &job.Options, &job.Status,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert repurpose_job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &job, nil
}

// GetJob returns a repurpose job by ID, scoped to the user.
func (s *RepurposeService) GetJob(ctx context.Context, userID, jobID uuid.UUID) (*model.RepurposeJob, error) {
	var job model.RepurposeJob
	err := s.pool.QueryRow(ctx,
		`SELECT id, source_post_id, user_id, ai_job_id, strategy, target_platforms,
		        options, status, error_message, created_at, updated_at
		 FROM repurpose_jobs
		 WHERE id = $1 AND user_id = $2`, jobID, userID,
	).Scan(
		&job.ID, &job.SourcePostID, &job.UserID, &job.AIJobID,
		&job.Strategy, &job.TargetPlatforms, &job.Options, &job.Status,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("repurpose job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get repurpose job: %w", err)
	}
	return &job, nil
}

// ListJobs returns all repurpose jobs for a user, newest first.
func (s *RepurposeService) ListJobs(ctx context.Context, userID uuid.UUID) ([]model.RepurposeJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, source_post_id, user_id, ai_job_id, strategy, target_platforms,
		        options, status, error_message, created_at, updated_at
		 FROM repurpose_jobs
		 WHERE user_id = $1
		 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query repurpose jobs: %w", err)
	}
	defer rows.Close()

	jobs := []model.RepurposeJob{}
	for rows.Next() {
		var j model.RepurposeJob
		if err := rows.Scan(
			&j.ID, &j.SourcePostID, &j.UserID, &j.AIJobID,
			&j.Strategy, &j.TargetPlatforms, &j.Options, &j.Status,
			&j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repurpose job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// GetOutputs returns all outputs for a repurpose job, scoped to the user.
func (s *RepurposeService) GetOutputs(ctx context.Context, userID, jobID uuid.UUID) ([]model.RepurposeOutput, error) {
	// Verify job belongs to user
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM repurpose_jobs WHERE id = $1 AND user_id = $2)`,
		jobID, userID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check job ownership: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("repurpose job not found")
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, repurpose_job_id, target_platform, generated_text,
		        generated_hashtags, generated_title, media_suggestions,
		        accepted, accepted_post_id, created_at
		 FROM repurpose_outputs
		 WHERE repurpose_job_id = $1
		 ORDER BY created_at`, jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("query repurpose outputs: %w", err)
	}
	defer rows.Close()

	outputs := []model.RepurposeOutput{}
	for rows.Next() {
		var o model.RepurposeOutput
		if err := rows.Scan(
			&o.ID, &o.RepurposeJobID, &o.TargetPlatform, &o.GeneratedText,
			&o.GeneratedHashtags, &o.GeneratedTitle, &o.MediaSuggestions,
			&o.Accepted, &o.AcceptedPostID, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan repurpose output: %w", err)
		}
		outputs = append(outputs, o)
	}
	return outputs, nil
}

// AcceptOutput creates a new post from the output and marks it as accepted.
// Returns the new post ID.
func (s *RepurposeService) AcceptOutput(ctx context.Context, userID, outputID uuid.UUID) (*uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch the output and verify ownership via the parent job
	var output model.RepurposeOutput
	var jobUserID uuid.UUID
	var jobSourcePostID uuid.UUID
	err = tx.QueryRow(ctx,
		`SELECT o.id, o.repurpose_job_id, o.target_platform, o.generated_text,
		        o.generated_hashtags, o.generated_title, o.media_suggestions,
		        o.accepted, o.accepted_post_id, o.created_at,
		        j.user_id, j.source_post_id
		 FROM repurpose_outputs o
		 JOIN repurpose_jobs j ON j.id = o.repurpose_job_id
		 WHERE o.id = $1`, outputID,
	).Scan(
		&output.ID, &output.RepurposeJobID, &output.TargetPlatform,
		&output.GeneratedText, &output.GeneratedHashtags, &output.GeneratedTitle,
		&output.MediaSuggestions, &output.Accepted, &output.AcceptedPostID,
		&output.CreatedAt, &jobUserID, &jobSourcePostID,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("repurpose output not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get repurpose output: %w", err)
	}

	if jobUserID != userID {
		return nil, fmt.Errorf("repurpose output does not belong to user")
	}

	if output.Accepted {
		return nil, fmt.Errorf("output already accepted")
	}

	// Build title for the new post
	title := output.TargetPlatform + " repurposed content"
	if output.GeneratedTitle != nil && *output.GeneratedTitle != "" {
		title = *output.GeneratedTitle
	}

	// Create a slug
	slug := "repurposed-" + uuid.New().String()[:8]

	// Build platform_posts JSONB for the target platform
	charLimit := 0
	if limit, ok := model.PlatformCharLimits[output.TargetPlatform]; ok {
		charLimit = limit
	}
	platformPosts := map[string]*model.PlatformPost{
		output.TargetPlatform: {
			Platform:  output.TargetPlatform,
			Text:      output.GeneratedText,
			Hashtags:  output.GeneratedHashtags,
			CharCount: len(output.GeneratedText),
			CharLimit: charLimit,
		},
	}
	platformPostsJSON, err := json.Marshal(platformPosts)
	if err != nil {
		return nil, fmt.Errorf("marshal platform posts: %w", err)
	}

	// Get the source post's project_id and category_id for the new post
	var categoryID *uuid.UUID
	var projectID *uuid.UUID
	_ = tx.QueryRow(ctx,
		`SELECT category_id, project_id FROM posts WHERE id = $1`, jobSourcePostID,
	).Scan(&categoryID, &projectID)

	// Create the new post
	var newPostID uuid.UUID
	err = tx.QueryRow(ctx,
		`INSERT INTO posts
		    (author_id, category_id, project_id, title, slug, content,
		     target_platforms, platform_posts, is_ai_generated, status,
		     tone, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, 'draft', '', '{}')
		 RETURNING id`,
		userID, categoryID, projectID, title, slug, output.GeneratedText,
		[]string{output.TargetPlatform}, platformPostsJSON,
	).Scan(&newPostID)
	if err != nil {
		return nil, fmt.Errorf("insert new post: %w", err)
	}

	// Mark output as accepted
	_, err = tx.Exec(ctx,
		`UPDATE repurpose_outputs
		 SET accepted = TRUE, accepted_post_id = $1
		 WHERE id = $2`, newPostID, outputID,
	)
	if err != nil {
		return nil, fmt.Errorf("update output accepted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &newPostID, nil
}

// ProcessRepurpose is called by the AI job worker. It reads the source post,
// generates adapted content for each target platform, and stores the results
// as repurpose_outputs. Real AI integration is a TODO -- for now it uses
// simple content adaptation (truncation + hashtag extraction).
func (s *RepurposeService) ProcessRepurpose(ctx context.Context, aiJobID uuid.UUID) error {
	// Find the repurpose_job linked to this ai_job
	var job model.RepurposeJob
	err := s.pool.QueryRow(ctx,
		`SELECT id, source_post_id, user_id, ai_job_id, strategy, target_platforms,
		        options, status, error_message, created_at, updated_at
		 FROM repurpose_jobs
		 WHERE ai_job_id = $1`, aiJobID,
	).Scan(
		&job.ID, &job.SourcePostID, &job.UserID, &job.AIJobID,
		&job.Strategy, &job.TargetPlatforms, &job.Options, &job.Status,
		&job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("repurpose job not found for ai_job %s", aiJobID)
	}
	if err != nil {
		return fmt.Errorf("get repurpose job: %w", err)
	}

	// Update status to processing
	_, err = s.pool.Exec(ctx,
		`UPDATE repurpose_jobs SET status = 'processing', updated_at = NOW() WHERE id = $1`,
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("update repurpose job status: %w", err)
	}

	// Get source post content
	var sourceContent, sourceTitle string
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(content, ''), COALESCE(title, '') FROM posts WHERE id = $1`,
		job.SourcePostID,
	).Scan(&sourceContent, &sourceTitle)
	if err != nil {
		s.failJob(ctx, job.ID, fmt.Sprintf("source post not found: %v", err))
		return fmt.Errorf("get source post: %w", err)
	}

	// Generate outputs for each target platform
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		s.failJob(ctx, job.ID, fmt.Sprintf("begin tx: %v", err))
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, platform := range job.TargetPlatforms {
		text, hashtags := adaptContentForPlatform(sourceContent, platform)
		generatedTitle := fmt.Sprintf("%s - %s", sourceTitle, platform)

		_, err = tx.Exec(ctx,
			`INSERT INTO repurpose_outputs
			    (repurpose_job_id, target_platform, generated_text,
			     generated_hashtags, generated_title, media_suggestions)
			 VALUES ($1, $2, $3, $4, $5, '{}')`,
			job.ID, platform, text, hashtags, generatedTitle,
		)
		if err != nil {
			s.failJob(ctx, job.ID, fmt.Sprintf("insert output for %s: %v", platform, err))
			return fmt.Errorf("insert output for %s: %w", platform, err)
		}
	}

	// Mark repurpose job as completed
	_, err = tx.Exec(ctx,
		`UPDATE repurpose_jobs SET status = 'completed', updated_at = NOW() WHERE id = $1`,
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("update repurpose job completed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		s.failJob(ctx, job.ID, fmt.Sprintf("commit: %v", err))
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// failJob marks a repurpose job as failed with an error message.
func (s *RepurposeService) failJob(ctx context.Context, jobID uuid.UUID, errMsg string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE repurpose_jobs SET status = 'failed', error_message = $1, updated_at = NOW()
		 WHERE id = $2`, errMsg, jobID,
	)
}

// adaptContentForPlatform applies platform-specific character limits and
// extracts hashtags. This is the stub implementation -- real AI integration
// will replace the simple truncation with intelligent summarization.
func adaptContentForPlatform(content string, platform string) (text string, hashtags []string) {
	limit, ok := model.PlatformCharLimits[platform]
	if !ok {
		limit = 500 // safe default
	}

	// Extract existing hashtags from content
	hashtags = extractHashtags(content)

	// Remove hashtags from the main text to avoid duplication
	cleanContent := content
	for _, tag := range hashtags {
		cleanContent = strings.ReplaceAll(cleanContent, "#"+tag, "")
	}
	cleanContent = strings.TrimSpace(cleanContent)

	// Truncate to platform limit
	if len(cleanContent) > limit {
		// Leave room for ellipsis
		truncated := cleanContent[:limit-3]
		// Try to break at a word boundary
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > limit/2 {
			truncated = truncated[:lastSpace]
		}
		text = truncated + "..."
	} else {
		text = cleanContent
	}

	// For Instagram, ensure we have some hashtags
	if platform == "instagram" && len(hashtags) == 0 {
		hashtags = []string{"repurposed", "content"}
	}

	return text, hashtags
}

// extractHashtags pulls #hashtag tokens out of content.
func extractHashtags(content string) []string {
	var hashtags []string
	seen := map[string]bool{}

	words := strings.Fields(content)
	for _, w := range words {
		if strings.HasPrefix(w, "#") && len(w) > 1 {
			tag := strings.TrimPrefix(w, "#")
			tag = strings.Trim(tag, ".,!?;:")
			lower := strings.ToLower(tag)
			if !seen[lower] && len(tag) > 0 {
				hashtags = append(hashtags, tag)
				seen[lower] = true
			}
		}
	}

	return hashtags
}

// getStrategyPrompt generates an AI prompt for the given strategy.
// This will be used when real AI integration is implemented.
func getStrategyPrompt(strategy model.RepurposeStrategy, sourceContent, sourcePlatform string) string {
	switch strategy {
	case model.StrategyLongToShort:
		return fmt.Sprintf(
			"Condense the following long-form content into a short, engaging social media post. "+
				"Keep the key message and make it punchy.\n\nSource content:\n%s",
			sourceContent,
		)
	case model.StrategyBlogToThread:
		return fmt.Sprintf(
			"Convert the following blog post into a Twitter/X thread format. "+
				"Break it into numbered tweets (max 280 chars each). "+
				"Start with a hook tweet.\n\nBlog content:\n%s",
			sourceContent,
		)
	case model.StrategyVideoToCaption:
		return fmt.Sprintf(
			"Create an engaging social media caption from the following video description. "+
				"Include relevant emojis and a call to action.\n\nVideo description:\n%s",
			sourceContent,
		)
	case model.StrategyPodcastToBlog:
		return fmt.Sprintf(
			"Transform the following podcast notes into a well-structured blog post. "+
				"Add an introduction, key takeaways, and a conclusion.\n\nPodcast notes:\n%s",
			sourceContent,
		)
	case model.StrategyCrossPlatform:
		return fmt.Sprintf(
			"Adapt the following content (originally from %s) for a different social media platform. "+
				"Adjust tone, length, and formatting to match the target platform's best practices.\n\nOriginal content:\n%s",
			sourcePlatform, sourceContent,
		)
	default:
		return fmt.Sprintf("Repurpose the following content for social media:\n\n%s", sourceContent)
	}
}
