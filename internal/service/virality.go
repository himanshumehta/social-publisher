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

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ═══════════════════════════════════════════════════════════════════════════
// Scoring Engine -- pure-logic scorer with no database dependencies
// ═══════════════════════════════════════════════════════════════════════════

// ScoringEngine is a stateless scorer. It has no dependencies and is
// safe to share across goroutines.
type ScoringEngine struct{}

// NewScoringEngine returns a new ScoringEngine.
func NewScoringEngine() *ScoringEngine {
	return &ScoringEngine{}
}

// Score evaluates a post and returns a 0-100 score with factor breakdown.
func (e *ScoringEngine) Score(input model.PostScoringInput, trendingTags []model.TrendingHashtag) model.ScoreResult {
	factors := model.ScoringFactors{
		HashtagScore:           e.scoreHashtags(input, trendingTags),
		TimingScore:            e.scoreTiming(input),
		ContentLengthScore:     e.scoreContentLength(input),
		EngagementHistoryScore: e.scoreEngagementHistory(input),
		MediaRichnessScore:     e.scoreMediaRichness(input),
		PlatformMatchScore:     e.scorePlatformMatch(input),
	}

	total := factors.HashtagScore +
		factors.TimingScore +
		factors.ContentLengthScore +
		factors.EngagementHistoryScore +
		factors.MediaRichnessScore +
		factors.PlatformMatchScore

	if total > 100 {
		total = 100
	}

	suggestions := e.GenerateSuggestions(input, factors)

	return model.ScoreResult{
		Score:       total,
		Factors:     factors,
		Suggestions: suggestions,
	}
}

// ── Hashtag trending (0-20) ─────────────────────────────────────────────

func (e *ScoringEngine) scoreHashtags(input model.PostScoringInput, trendingTags []model.TrendingHashtag) int {
	if len(input.Hashtags) == 0 {
		return 0
	}

	// Build a lookup from trending tags (lowercase).
	trendMap := make(map[string]float64, len(trendingTags))
	for _, t := range trendingTags {
		trendMap[strings.ToLower(t.Hashtag)] = t.TrendScore
	}

	var totalTrendScore float64
	for _, h := range input.Hashtags {
		if score, ok := trendMap[strings.ToLower(h)]; ok {
			totalTrendScore += score
		}
	}

	result := int(totalTrendScore / 5)
	if result > 20 {
		result = 20
	}
	return result
}

// ── Posting time optimality (0-20) ──────────────────────────────────────

func (e *ScoringEngine) scoreTiming(input model.PostScoringInput) int {
	hour := input.PostingHour
	weekday := input.PostingWeekday

	var score int

	// Peak hours: 9-11 AM, 7-9 PM (19-21)
	if (hour >= 9 && hour <= 11) || (hour >= 19 && hour <= 21) {
		score = 20
	} else if (hour >= 12 && hour <= 14) || (hour >= 17 && hour < 19) {
		// Good hours: 12-2 PM, 5-7 PM
		score = 15
	} else if (hour >= 7 && hour < 9) || (hour >= 15 && hour < 17) {
		// OK hours: 7-9 AM, 2-5 PM (note: 3 PM = 15, 4 PM = 16)
		score = 10
	} else {
		// Off-peak
		score = 5
	}

	// Weekend bonus for lifestyle/entertainment content.
	// Weekday 0 = Sunday, 6 = Saturday.
	isWeekend := weekday == 0 || weekday == 6
	if isWeekend {
		score += 3
		if score > 20 {
			score = 20
		}
	}

	return score
}

// ── Content length (0-15) ───────────────────────────────────────────────

func (e *ScoringEngine) scoreContentLength(input model.PostScoringInput) int {
	length := input.ContentLength
	if length == 0 {
		return 0
	}

	// Platform-aware scoring. If multiple platforms are targeted, use the
	// first one for the primary scoring heuristic.
	platform := ""
	if len(input.TargetPlatforms) > 0 {
		platform = strings.ToLower(input.TargetPlatforms[0])
	}

	switch platform {
	case "twitter":
		if length >= 100 && length <= 250 {
			return 15
		} else if length >= 50 && length < 100 {
			return 10
		} else if length > 250 {
			return 5
		}
		return 5
	case "linkedin":
		if length >= 500 && length <= 1500 {
			return 15
		} else if length >= 200 && length < 500 {
			return 10
		}
		return 5
	case "instagram":
		if length >= 100 && length <= 500 {
			return 15
		} else if length >= 50 && length < 100 {
			return 10
		}
		return 5
	default:
		// Generic / facebook / threads
		if length >= 200 && length <= 1000 {
			return 15
		} else if length >= 100 && length < 200 {
			return 10
		}
		return 5
	}
}

// ── Engagement history (0-25) ───────────────────────────────────────────

func (e *ScoringEngine) scoreEngagementHistory(input model.PostScoringInput) int {
	avg := input.AuthorAvgEngagement
	switch {
	case avg > 5.0:
		return 25
	case avg > 3.0:
		return 20
	case avg > 1.0:
		return 15
	case avg > 0.5:
		return 10
	default:
		return 5
	}
}

// ── Media richness (0-10) ───────────────────────────────────────────────

func (e *ScoringEngine) scoreMediaRichness(input model.PostScoringInput) int {
	if input.HasVideo {
		return 10
	}
	if input.HasImage {
		return 7
	}
	if input.HasLink {
		return 4
	}
	return 1
}

// ── Platform match (0-10) ───────────────────────────────────────────────

func (e *ScoringEngine) scorePlatformMatch(input model.PostScoringInput) int {
	if len(input.TargetPlatforms) == 0 {
		return 5
	}

	contentLen := input.ContentLength
	matchCount := 0
	overCount := 0

	for _, p := range input.TargetPlatforms {
		limit, ok := model.PlatformCharLimits[strings.ToLower(p)]
		if !ok {
			continue
		}
		if contentLen <= limit {
			matchCount++
		} else {
			overCount++
		}
	}

	total := matchCount + overCount
	if total == 0 {
		return 5
	}

	if overCount > 0 {
		return 2
	}
	return 10
}

// ── Suggestions ─────────────────────────────────────────────────────────

// GenerateSuggestions returns actionable recommendations for low-scoring factors.
func (e *ScoringEngine) GenerateSuggestions(input model.PostScoringInput, factors model.ScoringFactors) []string {
	var suggestions []string

	// Hashtag suggestions (max 20, threshold: 10)
	if factors.HashtagScore < 10 {
		if len(input.Hashtags) == 0 {
			suggestions = append(suggestions, "Add trending hashtags to increase discoverability. Posts with 3-5 relevant hashtags perform significantly better.")
		} else {
			suggestions = append(suggestions, "Your hashtags are not currently trending. Replace some with trending tags for your target platform to boost reach.")
		}
	}

	// Timing suggestions (max 20, threshold: 10)
	if factors.TimingScore < 10 {
		suggestions = append(suggestions, "Schedule your post during peak engagement hours: 9-11 AM or 7-9 PM. Weekday lunch hours (12-2 PM) also perform well.")
	}

	// Content length suggestions (max 15, threshold: 10)
	if factors.ContentLengthScore < 10 {
		if len(input.TargetPlatforms) > 0 {
			platform := input.TargetPlatforms[0]
			switch strings.ToLower(platform) {
			case "twitter":
				suggestions = append(suggestions, "Aim for 100-250 characters on Twitter. Short, punchy posts with room for engagement perform best.")
			case "linkedin":
				suggestions = append(suggestions, "LinkedIn posts perform best at 500-1500 characters. Add professional insights or storytelling to hit the sweet spot.")
			case "instagram":
				suggestions = append(suggestions, "Keep Instagram captions between 100-500 characters. Front-load the key message before the 'more' fold.")
			default:
				suggestions = append(suggestions, fmt.Sprintf("Adjust your content length for %s. The optimal range is 200-1000 characters.", platform))
			}
		} else {
			suggestions = append(suggestions, "Optimize your content length to 200-1000 characters for maximum engagement.")
		}
	}

	// Engagement history suggestions (max 25, threshold: 10)
	if factors.EngagementHistoryScore < 10 {
		suggestions = append(suggestions, "Your average engagement rate is below 0.5%. Focus on building audience interaction through questions, polls, and calls to action.")
	}

	// Media richness suggestions (max 10, threshold: 5)
	if factors.MediaRichnessScore < 5 {
		suggestions = append(suggestions, "Add an image or video to your post. Visual content receives 2-3x more engagement than text-only posts.")
	}

	// Platform match suggestions (max 10, threshold: 5)
	if factors.PlatformMatchScore < 5 {
		var overPlatforms []string
		for _, p := range input.TargetPlatforms {
			limit, ok := model.PlatformCharLimits[strings.ToLower(p)]
			if ok && input.ContentLength > limit {
				overPlatforms = append(overPlatforms, fmt.Sprintf("%s (limit: %d)", p, limit))
			}
		}
		if len(overPlatforms) > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Your content exceeds character limits for: %s. Shorten the content or create platform-specific versions.", strings.Join(overPlatforms, ", ")))
		}
	}

	return suggestions
}

// ═══════════════════════════════════════════════════════════════════════════
// Virality Service -- persistence and orchestration layer
// ═══════════════════════════════════════════════════════════════════════════

// ViralityService manages virality score persistence and orchestrates scoring.
type ViralityService struct {
	pool   *pgxpool.Pool
	engine *ScoringEngine
}

// NewViralityService creates a new ViralityService.
func NewViralityService(pool *pgxpool.Pool, engine *ScoringEngine) *ViralityService {
	return &ViralityService{pool: pool, engine: engine}
}

// ── Read operations ─────────────────────────────────────────────────────

// GetScore returns the existing virality score for a post, or nil if not scored.
func (s *ViralityService) GetScore(ctx context.Context, userID, postID uuid.UUID) (*model.ViralityScore, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, post_id, user_id, score, factors, suggestions,
		       scored_at, created_at, updated_at
		FROM virality_scores
		WHERE post_id = $1 AND user_id = $2
	`, postID, userID)

	var vs model.ViralityScore
	var factorsJSON []byte
	err := row.Scan(
		&vs.ID, &vs.PostID, &vs.UserID, &vs.Score,
		&factorsJSON, &vs.Suggestions,
		&vs.ScoredAt, &vs.CreatedAt, &vs.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get virality score: %w", err)
	}
	if err := json.Unmarshal(factorsJSON, &vs.Factors); err != nil {
		return nil, fmt.Errorf("unmarshal factors: %w", err)
	}
	return &vs, nil
}

// GetSuggestions returns the improvement suggestions from an existing score.
func (s *ViralityService) GetSuggestions(ctx context.Context, userID, postID uuid.UUID) ([]string, error) {
	var suggestions []string
	err := s.pool.QueryRow(ctx, `
		SELECT suggestions FROM virality_scores
		WHERE post_id = $1 AND user_id = $2
	`, postID, userID).Scan(&suggestions)
	if err == pgx.ErrNoRows {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get suggestions: %w", err)
	}
	if suggestions == nil {
		return []string{}, nil
	}
	return suggestions, nil
}

// ── Scoring operations ──────────────────────────────────────────────────

// ScorePost loads a post, assembles scoring input, runs the engine, and
// upserts the result. Returns the persisted ViralityScore.
func (s *ViralityService) ScorePost(ctx context.Context, userID, postID uuid.UUID) (*model.ViralityScore, error) {
	// 1. Load the post.
	var post model.Post
	var platformPostsJSON []byte
	var coverImageURL, linkedVideoURL *string
	err := s.pool.QueryRow(ctx, `
		SELECT id, author_id, content, tags, target_platforms,
		       cover_image_url, linked_video_url, linked_video_id,
		       COALESCE(platform_posts, '{}'::jsonb)
		FROM posts
		WHERE id = $1
	`, postID).Scan(
		&post.ID, &post.AuthorID, &post.Content, &post.Tags,
		&post.TargetPlatforms, &coverImageURL,
		&linkedVideoURL, &post.LinkedVideoID,
		&platformPostsJSON,
	)
	if err == nil {
		if coverImageURL != nil {
			post.CoverImageURL = *coverImageURL
		}
		if linkedVideoURL != nil {
			post.LinkedVideoURL = *linkedVideoURL
		}
	}
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("post not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load post: %w", err)
	}

	// Verify ownership.
	if post.AuthorID != userID {
		return nil, fmt.Errorf("post not found")
	}

	// 2. Collect hashtags from platform posts.
	var hashtags []string
	if len(platformPostsJSON) > 0 {
		pp, _ := model.UnmarshalPlatformPosts(platformPostsJSON)
		for _, p := range pp {
			hashtags = append(hashtags, p.Hashtags...)
		}
	}
	// Also include post tags as potential hashtags.
	hashtags = append(hashtags, post.Tags...)
	hashtags = deduplicateStrings(hashtags)

	// 3. Get author average engagement (placeholder: query publish results).
	authorAvg := s.getAuthorAvgEngagement(ctx, post.AuthorID)

	// 4. Load trending hashtags for the target platforms.
	trendingTags, err := s.loadTrendingForPlatforms(ctx, post.TargetPlatforms)
	if err != nil {
		return nil, fmt.Errorf("load trending: %w", err)
	}

	// 5. Determine content length.
	contentLength := len(post.Content)
	if contentLength == 0 && len(platformPostsJSON) > 0 {
		pp, _ := model.UnmarshalPlatformPosts(platformPostsJSON)
		for _, p := range pp {
			if len(p.Text) > contentLength {
				contentLength = len(p.Text)
			}
		}
	}

	// 6. Assemble scoring input.
	now := time.Now()
	input := model.PostScoringInput{
		Content:             post.Content,
		Hashtags:            hashtags,
		TargetPlatforms:     post.TargetPlatforms,
		HasImage:            post.CoverImageURL != "",
		HasVideo:            post.LinkedVideoURL != "" || post.LinkedVideoID != nil,
		HasLink:             false, // Could be enhanced to detect URLs in content
		PostingHour:         now.Hour(),
		PostingWeekday:      int(now.Weekday()),
		AuthorAvgEngagement: authorAvg,
		ContentLength:       contentLength,
	}

	// 7. Run the scoring engine.
	result := s.engine.Score(input, trendingTags)

	// 8. Persist (upsert).
	vs, err := s.upsertScore(ctx, postID, userID, result)
	if err != nil {
		return nil, fmt.Errorf("upsert score: %w", err)
	}

	return vs, nil
}

// ScoreBatch scores multiple posts. Used by the background worker.
func (s *ViralityService) ScoreBatch(ctx context.Context, postIDs []uuid.UUID) error {
	for _, postID := range postIDs {
		// Load the post to get the author.
		var authorID uuid.UUID
		err := s.pool.QueryRow(ctx,
			`SELECT author_id FROM posts WHERE id = $1`, postID,
		).Scan(&authorID)
		if err != nil {
			continue // Skip posts that can't be loaded.
		}
		if _, err := s.ScorePost(ctx, authorID, postID); err != nil {
			// Log but don't fail the entire batch.
			continue
		}
	}
	return nil
}

// PickUnscoredPosts finds draft/scheduled posts that don't have a virality_score yet.
func (s *ViralityService) PickUnscoredPosts(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id
		FROM posts p
		LEFT JOIN virality_scores vs ON vs.post_id = p.id
		WHERE p.status IN ('draft', 'scheduled')
		  AND vs.id IS NULL
		ORDER BY p.created_at ASC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("pick unscored posts: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ── Trending hashtags ───────────────────────────────────────────────────

// UpdateTrendingHashtags upserts a batch of trending hashtags.
func (s *ViralityService) UpdateTrendingHashtags(ctx context.Context, hashtags []model.TrendingHashtag) error {
	if len(hashtags) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, h := range hashtags {
		_, err := tx.Exec(ctx, `
			INSERT INTO trending_hashtags (hashtag, platform, trend_score, sample_count, last_seen)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (hashtag, platform)
			DO UPDATE SET
				trend_score  = EXCLUDED.trend_score,
				sample_count = EXCLUDED.sample_count,
				last_seen    = NOW(),
				id           = trending_hashtags.id
		`, h.Hashtag, h.Platform, h.TrendScore, h.SampleCount)
		if err != nil {
			return fmt.Errorf("upsert hashtag %q: %w", h.Hashtag, err)
		}
	}

	return tx.Commit(ctx)
}

// GetTrendingHashtags returns the top trending hashtags for a platform.
func (s *ViralityService) GetTrendingHashtags(ctx context.Context, platform string, limit int) ([]model.TrendingHashtag, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, hashtag, platform, trend_score, sample_count, first_seen, last_seen
		FROM trending_hashtags
		WHERE platform = $1
		ORDER BY trend_score DESC
		LIMIT $2
	`, platform, limit)
	if err != nil {
		return nil, fmt.Errorf("get trending hashtags: %w", err)
	}
	defer rows.Close()

	var tags []model.TrendingHashtag
	for rows.Next() {
		var t model.TrendingHashtag
		if err := rows.Scan(
			&t.ID, &t.Hashtag, &t.Platform, &t.TrendScore,
			&t.SampleCount, &t.FirstSeen, &t.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("scan trending hashtag: %w", err)
		}
		tags = append(tags, t)
	}
	return safeSliceVirality(tags), rows.Err()
}

// ── Private helpers ─────────────────────────────────────────────────────

func (s *ViralityService) upsertScore(ctx context.Context, postID, userID uuid.UUID, result model.ScoreResult) (*model.ViralityScore, error) {
	factorsJSON, err := json.Marshal(result.Factors)
	if err != nil {
		return nil, fmt.Errorf("marshal factors: %w", err)
	}

	suggestions := result.Suggestions
	if suggestions == nil {
		suggestions = []string{}
	}

	var vs model.ViralityScore
	var returnedFactors []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO virality_scores (post_id, user_id, score, factors, suggestions, scored_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (post_id)
		DO UPDATE SET
			score       = EXCLUDED.score,
			factors     = EXCLUDED.factors,
			suggestions = EXCLUDED.suggestions,
			scored_at   = NOW(),
			updated_at  = NOW()
		RETURNING id, post_id, user_id, score, factors, suggestions,
		          scored_at, created_at, updated_at
	`, postID, userID, result.Score, factorsJSON, suggestions).Scan(
		&vs.ID, &vs.PostID, &vs.UserID, &vs.Score,
		&returnedFactors, &vs.Suggestions,
		&vs.ScoredAt, &vs.CreatedAt, &vs.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(returnedFactors, &vs.Factors); err != nil {
		return nil, fmt.Errorf("unmarshal returned factors: %w", err)
	}
	return &vs, nil
}

func (s *ViralityService) getAuthorAvgEngagement(ctx context.Context, authorID uuid.UUID) float64 {
	// Calculate from published post metrics. Returns 0 if no data.
	var avg float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(AVG(
			CASE WHEN (pp.value->>'char_count')::int > 0
			THEN 1.0 ELSE 0.0 END
		), 0)
		FROM posts p,
		     jsonb_each(COALESCE(p.platform_posts, '{}')) AS pp(key, value)
		WHERE p.author_id = $1 AND p.status = 'published'
	`, authorID).Scan(&avg)
	if err != nil {
		return 0
	}
	return avg
}

func (s *ViralityService) loadTrendingForPlatforms(ctx context.Context, platforms []string) ([]model.TrendingHashtag, error) {
	if len(platforms) == 0 {
		return nil, nil
	}

	lowerPlatforms := make([]string, len(platforms))
	for i, p := range platforms {
		lowerPlatforms[i] = strings.ToLower(p)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, hashtag, platform, trend_score, sample_count, first_seen, last_seen
		FROM trending_hashtags
		WHERE platform = ANY($1)
		ORDER BY trend_score DESC
		LIMIT 200
	`, lowerPlatforms)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []model.TrendingHashtag
	for rows.Next() {
		var t model.TrendingHashtag
		if err := rows.Scan(
			&t.ID, &t.Hashtag, &t.Platform, &t.TrendScore,
			&t.SampleCount, &t.FirstSeen, &t.LastSeen,
		); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// deduplicateStrings removes duplicate strings (case-insensitive).
func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, s)
		}
	}
	return result
}

// safeSliceVirality returns an empty slice instead of nil to ensure clean JSON encoding.
func safeSliceVirality[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
