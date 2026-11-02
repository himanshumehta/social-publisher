package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type PostService struct {
	pool      *pgxpool.Pool
	scanner   *PostScanner
	generator *PostGenerator
	reviewer  *PostReviewer
	logger    *slog.Logger
}

func NewPostService(
	pool *pgxpool.Pool,
	scanner *PostScanner,
	generator *PostGenerator,
	reviewer *PostReviewer,
	logger *slog.Logger,
) *PostService {
	return &PostService{
		pool:      pool,
		scanner:   scanner,
		generator: generator,
		reviewer:  reviewer,
		logger:    logger,
	}
}

// ListByProject returns all posts for a given project.
func (s *PostService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.Post, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, category_id, project_id, author_id, title, slug, content, excerpt,
			cover_image_url, subject_name, subject_type, source_url, source_metadata,
			ai_provider, ai_model, ai_prompt, ai_generated_at, is_ai_generated,
			platform_posts, target_platforms, linked_video_url, linked_video_id,
			tone, brand_voice, system_review_status, system_review_notes, system_review_at,
			status, error_message, retry_count, processing_started_at, auto_publish,
			publish_results, meta_title, meta_description, tags, published_at,
			created_at, updated_at
		FROM posts WHERE project_id = $1
		ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query posts by project: %w", err)
	}
	defer rows.Close()

	var posts []model.Post
	for rows.Next() {
		post := model.Post{}
		var sourceMetadataJSON, platformPostsJSON, publishResultsJSON []byte
		var (
			content, excerpt, coverImageURL     *string
			subjectName, subjectType, sourceURL *string
			aiProvider, aiModel, aiPrompt       *string
			linkedVideoURL                      *string
			brandVoice, systemReviewNotes       *string
			errorMessage                        *string
			metaTitle, metaDescription          *string
		)
		err := rows.Scan(
			&post.ID, &post.CategoryID, &post.ProjectID, &post.AuthorID,
			&post.Title, &post.Slug, &content, &excerpt,
			&coverImageURL, &subjectName, &subjectType,
			&sourceURL, &sourceMetadataJSON,
			&aiProvider, &aiModel, &aiPrompt, &post.AIGeneratedAt,
			&post.IsAIGenerated, &platformPostsJSON, &post.TargetPlatforms,
			&linkedVideoURL, &post.LinkedVideoID,
			&post.Tone, &brandVoice,
			&post.SystemReviewStatus, &systemReviewNotes, &post.SystemReviewAt,
			&post.Status, &errorMessage, &post.RetryCount,
			&post.ProcessingStartedAt, &post.AutoPublish,
			&publishResultsJSON,
			&metaTitle, &metaDescription, &post.Tags, &post.PublishedAt,
			&post.CreatedAt, &post.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		post.Content = derefStr(content)
		post.Excerpt = derefStr(excerpt)
		post.CoverImageURL = derefStr(coverImageURL)
		post.SubjectName = derefStr(subjectName)
		post.SubjectType = derefStr(subjectType)
		post.SourceURL = derefStr(sourceURL)
		post.AIProvider = derefStr(aiProvider)
		post.AIModel = derefStr(aiModel)
		post.AIPrompt = derefStr(aiPrompt)
		post.LinkedVideoURL = derefStr(linkedVideoURL)
		post.BrandVoice = derefStr(brandVoice)
		post.SystemReviewNotes = derefStr(systemReviewNotes)
		post.ErrorMessage = derefStr(errorMessage)
		post.MetaTitle = derefStr(metaTitle)
		post.MetaDescription = derefStr(metaDescription)
		if sourceMetadataJSON != nil {
			json.Unmarshal(sourceMetadataJSON, &post.SourceMetadata)
		}
		if platformPostsJSON != nil {
			json.Unmarshal(platformPostsJSON, &post.PlatformPosts)
		}
		if publishResultsJSON != nil {
			json.Unmarshal(publishResultsJSON, &post.PublishResults)
		}
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating posts: %w", err)
	}
	if posts == nil {
		posts = []model.Post{}
	}
	return posts, nil
}

// Create creates a new post and initiates the pipeline.
func (s *PostService) Create(ctx context.Context, authorID uuid.UUID, req *model.CreatePostRequest) (*model.Post, error) {
	if req.SourceURL == "" && req.Title == "" {
		return nil, fmt.Errorf("either source_url or title is required")
	}
	if len(req.TargetPlatforms) == 0 {
		return nil, fmt.Errorf("at least one target platform is required")
	}
	for _, p := range req.TargetPlatforms {
		if _, ok := model.PlatformCharLimits[p]; !ok {
			return nil, fmt.Errorf("unsupported platform: %s", p)
		}
	}

	status := "draft"
	if req.SourceURL == "" {
		status = "creator_editing"
	}

	slugBase := req.Title
	if slugBase == "" {
		slugBase = req.SourceURL
	}
	slug := generatePostSlug(slugBase)

	slug, err := s.ensureUniqueSlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("generating slug: %w", err)
	}

	tone := req.Tone
	if tone == "" {
		tone = "professional"
	}

	var categoryID *uuid.UUID
	if req.CategoryID != "" {
		id, err := uuid.Parse(req.CategoryID)
		if err == nil {
			categoryID = &id
		}
	}

	var projectID *uuid.UUID
	if req.ProjectID != "" {
		id, err := uuid.Parse(req.ProjectID)
		if err == nil {
			projectID = &id
		}
	}

	// Build platform_posts if provided directly (e.g. from content agent)
	var platformPosts map[string]*model.PlatformPost
	if req.PlatformPosts != nil {
		platformPosts = make(map[string]*model.PlatformPost)
		for platform, edit := range req.PlatformPosts {
			charLimit := 0
			if limit, ok := model.PlatformCharLimits[platform]; ok {
				charLimit = limit
			}
			platformPosts[platform] = &model.PlatformPost{
				Platform:  platform,
				Text:      edit.Text,
				Hashtags:  edit.Hashtags,
				CharCount: len(edit.Text),
				CharLimit: charLimit,
				ImageURL:  edit.ImageURL,
			}
		}
	}

	post := &model.Post{
		ID:                 uuid.New(),
		CategoryID:         categoryID,
		ProjectID:          projectID,
		AuthorID:           authorID,
		Title:              req.Title,
		Slug:               slug,
		Content:            req.Content,
		Excerpt:            req.Excerpt,
		CoverImageURL:      req.CoverImageURL,
		SourceURL:          req.SourceURL,
		TargetPlatforms:    req.TargetPlatforms,
		PlatformPosts:      platformPosts,
		LinkedVideoURL:     req.LinkedVideoURL,
		Tone:               tone,
		BrandVoice:         req.BrandVoice,
		AutoPublish:        req.AutoPublish,
		Status:             status,
		Tags:               req.Tags,
		SystemReviewStatus: "pending",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	var platformPostsJSON []byte
	if platformPosts != nil {
		platformPostsJSON, _ = json.Marshal(platformPosts)
	}

	query := `
		INSERT INTO posts (
			id, category_id, project_id, author_id, title, slug, content, excerpt,
			cover_image_url, source_url, target_platforms, linked_video_url,
			tone, brand_voice, auto_publish, status, tags,
			system_review_status, is_ai_generated, platform_posts, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15, $16::post_status, $17,
			$18, $19, $20, $21, $22
		)`

	_, err = s.pool.Exec(ctx, query,
		post.ID, post.CategoryID, post.ProjectID, post.AuthorID, post.Title, post.Slug,
		post.Content, post.Excerpt, post.CoverImageURL, nilIfEmptyPtr(post.SourceURL),
		post.TargetPlatforms, nilIfEmptyPtr(post.LinkedVideoURL),
		post.Tone, nilIfEmptyPtr(post.BrandVoice), post.AutoPublish,
		post.Status, post.Tags, post.SystemReviewStatus, false,
		platformPostsJSON,
		post.CreatedAt, post.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting post: %w", err)
	}

	s.logger.Info("post created",
		"post_id", post.ID,
		"status", post.Status,
		"source_url", post.SourceURL,
		"platforms", post.TargetPlatforms,
	)

	return post, nil
}

// GetByID returns a post with all its data.
func (s *PostService) GetByID(ctx context.Context, id uuid.UUID, requesterID uuid.UUID, isAdmin bool) (*model.Post, error) {
	post, err := s.scanPost(ctx, s.pool, `
		SELECT id, category_id, project_id, author_id, title, slug, content, excerpt,
			cover_image_url, subject_name, subject_type, source_url, source_metadata,
			ai_provider, ai_model, ai_prompt, ai_generated_at, is_ai_generated,
			platform_posts, target_platforms, linked_video_url, linked_video_id,
			tone, brand_voice, system_review_status, system_review_notes, system_review_at,
			status, error_message, retry_count, processing_started_at, auto_publish,
			publish_results, meta_title, meta_description, tags, published_at,
			created_at, updated_at,
			scheduled_at, timezone, brand_kit_id
		FROM posts WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}

	if !isAdmin && post.AuthorID != requesterID {
		return nil, fmt.Errorf("post not found")
	}

	return post, nil
}

// List returns posts filtered and paginated.
func (s *PostService) List(ctx context.Context, filter *model.PostListFilter, requesterID uuid.UUID, isAdmin bool) ([]model.PostListItem, int, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if !isAdmin {
		conditions = append(conditions, fmt.Sprintf("p.author_id = $%d", argIdx))
		args = append(args, requesterID)
		argIdx++
	}

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("p.status = $%d::post_status", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}

	if filter.Platform != "" {
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(p.target_platforms)", argIdx))
		args = append(args, filter.Platform)
		argIdx++
	}

	if filter.AuthorID != "" {
		conditions = append(conditions, fmt.Sprintf("p.author_id = $%d", argIdx))
		args = append(args, filter.AuthorID)
		argIdx++
	}

	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(p.title ILIKE '%%' || $%d || '%%' OR p.content ILIKE '%%' || $%d || '%%')", argIdx, argIdx))
		args = append(args, filter.Search)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM posts p %s", whereClause)
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting posts: %w", err)
	}

	pageSize := filter.PageSize
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	sortBy := "created_at"
	if filter.SortBy == "published_at" {
		sortBy = "published_at"
	}
	sortOrder := "DESC"
	if filter.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT p.id, p.title, p.source_url, p.cover_image_url, p.target_platforms,
			p.status, p.system_review_status, u.name, p.published_at, p.created_at,
			p.publish_results
		FROM posts p
		JOIN users u ON u.id = p.author_id
		%s
		ORDER BY p.%s %s
		LIMIT %d OFFSET %d`,
		whereClause, sortBy, sortOrder, pageSize, offset,
	)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing posts: %w", err)
	}
	defer rows.Close()

	var items []model.PostListItem
	for rows.Next() {
		var item model.PostListItem
		var sourceURL, coverImageURL *string
		var publishResultsJSON []byte
		err := rows.Scan(
			&item.ID, &item.Title, &sourceURL, &coverImageURL,
			&item.TargetPlatforms, &item.Status, &item.SystemReviewStatus,
			&item.AuthorName, &item.PublishedAt, &item.CreatedAt,
			&publishResultsJSON,
		)
		item.SourceURL = derefStr(sourceURL)
		item.CoverImageURL = derefStr(coverImageURL)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning post row: %w", err)
		}
		if publishResultsJSON != nil {
			json.Unmarshal(publishResultsJSON, &item.PublishResults)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating post list: %w", err)
	}

	if items == nil {
		items = []model.PostListItem{}
	}
	return items, total, nil
}

// Update modifies a post's editable fields.
func (s *PostService) Update(ctx context.Context, postID, requesterID uuid.UUID, isAdmin bool, req *model.UpdatePostRequest) (*model.Post, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	var authorID uuid.UUID
	err = tx.QueryRow(ctx,
		"SELECT status, author_id FROM posts WHERE id = $1 FOR UPDATE",
		postID,
	).Scan(&currentStatus, &authorID)
	if err != nil {
		return nil, fmt.Errorf("post not found: %w", err)
	}

	if !isAdmin && authorID != requesterID {
		return nil, fmt.Errorf("post not found")
	}

	editableStates := map[string]bool{
		"draft": true, "creator_editing": true, "generated": true,
		"pending_review": true, "flagged": true,
	}
	if !editableStates[currentStatus] {
		return nil, fmt.Errorf("post in status '%s' cannot be edited", currentStatus)
	}

	sets := []string{"updated_at = NOW()"}
	args := []interface{}{postID}
	argIdx := 2

	if req.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", argIdx))
		args = append(args, *req.Title)
		argIdx++
	}
	if req.CoverImageURL != nil {
		sets = append(sets, fmt.Sprintf("cover_image_url = $%d", argIdx))
		args = append(args, *req.CoverImageURL)
		argIdx++
	}
	if req.Tags != nil {
		sets = append(sets, fmt.Sprintf("tags = $%d", argIdx))
		args = append(args, *req.Tags)
		argIdx++
	}
	if req.TargetPlatforms != nil {
		sets = append(sets, fmt.Sprintf("target_platforms = $%d", argIdx))
		args = append(args, *req.TargetPlatforms)
		argIdx++
	}
	if req.PlatformPosts != nil {
		var existingJSON []byte
		tx.QueryRow(ctx, "SELECT platform_posts FROM posts WHERE id = $1", postID).Scan(&existingJSON)

		existing := make(map[string]*model.PlatformPost)
		if existingJSON != nil {
			json.Unmarshal(existingJSON, &existing)
		}

		for platform, edit := range req.PlatformPosts {
			if pp, ok := existing[platform]; ok {
				pp.IsEdited = true
				pp.EditedText = edit.Text
				pp.Text = edit.Text
				if edit.Hashtags != nil {
					pp.Hashtags = edit.Hashtags
				}
				if edit.ImageURL != "" {
					pp.ImageURL = edit.ImageURL
				}
				pp.CharCount = len(edit.Text)
			}
		}

		mergedJSON, _ := json.Marshal(existing)
		sets = append(sets, fmt.Sprintf("platform_posts = $%d", argIdx))
		args = append(args, mergedJSON)
		argIdx++
	}

	query := fmt.Sprintf("UPDATE posts SET %s WHERE id = $1", strings.Join(sets, ", "))
	_, err = tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("updating post: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}

	return s.GetByID(ctx, postID, requesterID, isAdmin)
}

// Delete removes a post.
func (s *PostService) Delete(ctx context.Context, postID, requesterID uuid.UUID, isAdmin bool) error {
	var status string
	var authorID uuid.UUID
	err := s.pool.QueryRow(ctx,
		"SELECT status, author_id FROM posts WHERE id = $1", postID,
	).Scan(&status, &authorID)
	if err != nil {
		return fmt.Errorf("post not found")
	}

	if !isAdmin && authorID != requesterID {
		return fmt.Errorf("post not found")
	}

	deletableStates := map[string]bool{
		"draft": true, "creator_editing": true, "rejected": true,
		"scan_failed": true, "generation_failed": true, "publish_failed": true,
	}
	if !deletableStates[status] {
		return fmt.Errorf("post in status '%s' cannot be deleted", status)
	}

	_, err = s.pool.Exec(ctx, "DELETE FROM posts WHERE id = $1", postID)
	return err
}

// PickForScan picks the next post needing scanning.
func (s *PostService) PickForScan(ctx context.Context) (*model.Post, error) {
	return s.pickForStage(ctx, "draft", "scanning")
}

// PickForGenerate picks the next post needing content generation.
func (s *PostService) PickForGenerate(ctx context.Context) (*model.Post, error) {
	return s.pickForStage(ctx, "scanned", "generating")
}

// PickForPublish picks the next post ready for publishing.
// Picks from "approved" (new), "publish_failed" (retry), and "publishing" (stuck/timed out) posts.
func (s *PostService) PickForPublish(ctx context.Context) (*model.Post, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		UPDATE posts
		SET status = 'publishing'::post_status,
			processing_started_at = NOW(),
			updated_at = NOW()
		WHERE id = (
			SELECT id FROM posts
			WHERE (
				status = 'approved'::post_status
				OR (status = 'publish_failed'::post_status AND retry_count < $1)
				OR (status = 'publishing'::post_status
					AND processing_started_at < NOW() - INTERVAL '5 minutes')
			)
			  AND (processing_started_at IS NULL
				   OR processing_started_at < NOW() - INTERVAL '5 minutes')
			ORDER BY
				CASE WHEN status = 'approved' THEN 0
					 WHEN status = 'publish_failed' THEN 1
					 ELSE 2 END,
				created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id`, maxPublishRetries)

	var postID uuid.UUID
	if err := row.Scan(&postID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.GetByID(ctx, postID, uuid.Nil, true)
}

func (s *PostService) pickForStage(ctx context.Context, fromStatus, toStatus string) (*model.Post, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		UPDATE posts
		SET status = $2::post_status,
			processing_started_at = NOW(),
			updated_at = NOW()
		WHERE id = (
			SELECT id FROM posts
			WHERE status = $1::post_status
			  AND (processing_started_at IS NULL
				   OR processing_started_at < NOW() - INTERVAL '5 minutes')
			ORDER BY created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id`, fromStatus, toStatus)

	var postID uuid.UUID
	if err := row.Scan(&postID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return s.GetByID(ctx, postID, uuid.Nil, true)
}

// CompleteScan updates the post after successful scanning.
func (s *PostService) CompleteScan(ctx context.Context, postID uuid.UUID, scanned *model.ScannedContent) error {
	metadata, err := json.Marshal(scanned)
	if err != nil {
		return fmt.Errorf("marshaling scan result: %w", err)
	}

	title := scanned.Title
	coverImage := scanned.CoverImageURL

	_, err = s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'scanned'::post_status,
			source_metadata = $2,
			title = COALESCE(NULLIF(title, ''), $3),
			cover_image_url = COALESCE(NULLIF(cover_image_url, ''), $4),
			processing_started_at = NULL,
			retry_count = 0,
			error_message = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, metadata, title, coverImage,
	)
	return err
}

// FailScan marks a scan as failed.
func (s *PostService) FailScan(ctx context.Context, postID uuid.UUID, errMsg string) error {
	return s.failStage(ctx, postID, "scan_failed", errMsg)
}

// CompleteGeneration updates the post after successful content generation.
func (s *PostService) CompleteGeneration(ctx context.Context, postID uuid.UUID, result *GenerateResult) error {
	platformJSON, err := json.Marshal(result.Platforms)
	if err != nil {
		return fmt.Errorf("marshaling generated content: %w", err)
	}

	now := time.Now().UTC()

	_, err = s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'generated'::post_status,
			platform_posts = $2,
			ai_provider = 'anthropic',
			ai_model = $3,
			ai_generated_at = $4,
			is_ai_generated = TRUE,
			processing_started_at = NULL,
			retry_count = 0,
			error_message = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, platformJSON, result.Model, now,
	)
	return err
}

// FailGeneration marks generation as failed.
func (s *PostService) FailGeneration(ctx context.Context, postID uuid.UUID, errMsg string) error {
	return s.failStage(ctx, postID, "generation_failed", errMsg)
}

// CompleteSystemReview runs the system reviewer and updates the post status.
func (s *PostService) CompleteSystemReview(ctx context.Context, postID uuid.UUID) error {
	post, err := s.GetByID(ctx, postID, uuid.Nil, true)
	if err != nil {
		return err
	}

	result := s.reviewer.Review(ctx, post, post.PlatformPosts)
	status, notes, reviewedAt := ReviewResultToDBFields(result)

	nextStatus := "pending_review"
	if status == "blocked" {
		nextStatus = "flagged"
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE posts
		SET status = $2::post_status,
			system_review_status = $3,
			system_review_notes = $4,
			system_review_at = $5,
			updated_at = NOW()
		WHERE id = $1`,
		postID, nextStatus, status, notes, reviewedAt,
	)
	return err
}

// AdminApprove approves a post for publishing.
func (s *PostService) AdminApprove(ctx context.Context, postID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'approved'::post_status,
			updated_at = NOW()
		WHERE id = $1 AND status IN ('pending_review', 'flagged')`,
		postID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("post not in reviewable state")
	}
	return nil
}

// AdminReject rejects a post.
func (s *PostService) AdminReject(ctx context.Context, postID uuid.UUID, reason string) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'rejected'::post_status,
			error_message = $2,
			updated_at = NOW()
		WHERE id = $1 AND status IN ('pending_review', 'flagged', 'approved')`,
		postID, reason,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("post not in reviewable state")
	}
	return nil
}

// Submit transitions a post from creator editing to pending review.
func (s *PostService) Submit(ctx context.Context, postID, authorID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'pending_review'::post_status,
			updated_at = NOW()
		WHERE id = $1 AND author_id = $2 AND status IN ('creator_editing', 'generated')`,
		postID, authorID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("post not in submittable state or not owned by you")
	}
	return nil
}

// Regenerate re-triggers AI content generation for a post.
func (s *PostService) Regenerate(ctx context.Context, postID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'scanned'::post_status,
			platform_posts = NULL,
			ai_generated_at = NULL,
			processing_started_at = NULL,
			updated_at = NOW()
		WHERE id = $1 AND status IN ('generated', 'pending_review', 'flagged', 'generation_failed')`,
		postID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("post not in regeneratable state")
	}
	return nil
}

// CompletePublish marks a post as published after platform publishing completes.
// If at least one platform succeeded, the post is marked as published.
// If ALL platforms failed, the post is marked as publish_failed for retry.
func (s *PostService) CompletePublish(ctx context.Context, postID uuid.UUID, results []model.PostPublishResult) error {
	resultsJSON, _ := json.Marshal(results)
	now := time.Now().UTC()

	published := 0
	for _, r := range results {
		if r.Status == "published" {
			published++
		}
	}

	if published == 0 && len(results) > 0 {
		// All platforms failed — mark as publish_failed for retry
		errMsgs := make([]string, 0, len(results))
		for _, r := range results {
			if r.Error != "" {
				errMsgs = append(errMsgs, r.Platform+": "+r.Error)
			}
		}
		errMsg := strings.Join(errMsgs, "; ")
		return s.FailPublish(ctx, postID, errMsg, resultsJSON)
	}

	_, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'published'::post_status,
			publish_results = $2,
			published_at = $3,
			processing_started_at = NULL,
			error_message = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, resultsJSON, now,
	)
	return err
}

const maxPublishRetries = 3

// FailPublish marks a post as publish_failed with retry tracking.
// After maxPublishRetries attempts, the post stays in publish_failed for manual intervention.
func (s *PostService) FailPublish(ctx context.Context, postID uuid.UUID, errMsg string, resultsJSON []byte) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = 'publish_failed'::post_status,
			error_message = $2,
			publish_results = COALESCE($3, publish_results),
			retry_count = retry_count + 1,
			processing_started_at = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, errMsg, resultsJSON,
	)
	return err
}

func (s *PostService) failStage(ctx context.Context, postID uuid.UUID, failStatus, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE posts
		SET status = $2::post_status,
			error_message = $3,
			retry_count = retry_count + 1,
			processing_started_at = NULL,
			updated_at = NOW()
		WHERE id = $1`,
		postID, failStatus, errMsg,
	)
	return err
}

func (s *PostService) scanPost(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}, query string, args ...interface{}) (*model.Post, error) {
	post := &model.Post{}
	var sourceMetadataJSON, platformPostsJSON, publishResultsJSON []byte

	// Nullable text columns need *string intermediaries since pgx v5
	// cannot scan SQL NULL into a plain Go string.
	var (
		content, excerpt, coverImageURL     *string
		subjectName, subjectType, sourceURL *string
		aiProvider, aiModel, aiPrompt       *string
		linkedVideoURL                      *string
		brandVoice, systemReviewNotes       *string
		errorMessage                        *string
		metaTitle, metaDescription          *string
	)

	err := q.QueryRow(ctx, query, args...).Scan(
		&post.ID, &post.CategoryID, &post.ProjectID, &post.AuthorID,
		&post.Title, &post.Slug, &content, &excerpt,
		&coverImageURL, &subjectName, &subjectType,
		&sourceURL, &sourceMetadataJSON,
		&aiProvider, &aiModel, &aiPrompt, &post.AIGeneratedAt,
		&post.IsAIGenerated, &platformPostsJSON, &post.TargetPlatforms,
		&linkedVideoURL, &post.LinkedVideoID,
		&post.Tone, &brandVoice,
		&post.SystemReviewStatus, &systemReviewNotes, &post.SystemReviewAt,
		&post.Status, &errorMessage, &post.RetryCount,
		&post.ProcessingStartedAt, &post.AutoPublish,
		&publishResultsJSON,
		&metaTitle, &metaDescription, &post.Tags, &post.PublishedAt,
		&post.CreatedAt, &post.UpdatedAt,
		&post.ScheduledAt, &post.Timezone, &post.BrandKitID,
	)
	if err != nil {
		return nil, err
	}

	// Copy nullable strings into the model (NULL → empty string).
	post.Content = derefStr(content)
	post.Excerpt = derefStr(excerpt)
	post.CoverImageURL = derefStr(coverImageURL)
	post.SubjectName = derefStr(subjectName)
	post.SubjectType = derefStr(subjectType)
	post.SourceURL = derefStr(sourceURL)
	post.AIProvider = derefStr(aiProvider)
	post.AIModel = derefStr(aiModel)
	post.AIPrompt = derefStr(aiPrompt)
	post.LinkedVideoURL = derefStr(linkedVideoURL)
	post.BrandVoice = derefStr(brandVoice)
	post.SystemReviewNotes = derefStr(systemReviewNotes)
	post.ErrorMessage = derefStr(errorMessage)
	post.MetaTitle = derefStr(metaTitle)
	post.MetaDescription = derefStr(metaDescription)

	if sourceMetadataJSON != nil {
		json.Unmarshal(sourceMetadataJSON, &post.SourceMetadata)
	}
	if platformPostsJSON != nil {
		json.Unmarshal(platformPostsJSON, &post.PlatformPosts)
	}
	if publishResultsJSON != nil {
		json.Unmarshal(publishResultsJSON, &post.PublishResults)
	}

	return post, nil
}

func (s *PostService) ensureUniqueSlug(ctx context.Context, base string) (string, error) {
	slug := base
	for i := 0; i < 10; i++ {
		var exists bool
		err := s.pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM posts WHERE slug = $1)", slug,
		).Scan(&exists)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, time.Now().UnixNano()%10000)
	}
	return slug + "-" + uuid.New().String()[:8], nil
}

var postSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generatePostSlug(title string) string {
	lower := strings.ToLower(title)
	slug := postSlugRegex.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	runes := []rune(slug)
	if len(runes) > 80 {
		slug = string(runes[:80])
		slug = strings.TrimRight(slug, "-")
	}
	if slug == "" {
		slug = "post-" + uuid.New().String()[:8]
	}
	return slug
}
