package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

const CreditCostPerSEOAnalysis = 2

type SEOService struct {
	pool *pgxpool.Pool
}

func NewSEOService(pool *pgxpool.Pool) *SEOService {
	return &SEOService{pool: pool}
}

func (s *SEOService) AnalyzeSEO(ctx context.Context, userID uuid.UUID, req *model.AnalyzeSEORequest) (*model.SEOAnalysis, error) {
	platform := "youtube"
	if req.Platform != "" {
		platform = req.Platform
	}

	// Score the content
	scoreResult := ScoreSEO(&model.SEOScoreInput{
		Title:       req.Title,
		Description: req.Description,
		Tags:        req.Tags,
		Platform:    platform,
	})

	analysis := &model.SEOAnalysis{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO seo_analyses (project_id, post_id, created_by, platform, status,
			original_title, original_description, original_tags,
			title_score, description_score, overall_score, credits_used, completed_at)
		VALUES ($1, $2, $3, $4, 'completed', $5, $6, $7, $8, $9, $10, $11, NOW())
		RETURNING id, project_id, post_id, created_by, platform, status,
			original_title, optimized_title, original_description, optimized_description,
			original_tags, optimized_tags, title_score, description_score, overall_score,
			suggestions, keyword_data, competitor_data, credits_used, error_message,
			completed_at, created_at, updated_at`,
		req.ProjectID, req.PostID, userID, platform,
		req.Title, req.Description, req.Tags,
		scoreResult.TitleScore, scoreResult.DescriptionScore, scoreResult.OverallScore,
		CreditCostPerSEOAnalysis,
	).Scan(
		&analysis.ID, &analysis.ProjectID, &analysis.PostID, &analysis.CreatedBy,
		&analysis.Platform, &analysis.Status, &analysis.OriginalTitle,
		&analysis.OptimizedTitle, &analysis.OriginalDescription,
		&analysis.OptimizedDescription, &analysis.OriginalTags, &analysis.OptimizedTags,
		&analysis.TitleScore, &analysis.DescriptionScore, &analysis.OverallScore,
		&analysis.Suggestions, &analysis.KeywordData, &analysis.CompetitorData,
		&analysis.CreditsUsed, &analysis.ErrorMessage, &analysis.CompletedAt,
		&analysis.CreatedAt, &analysis.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("analyze seo: %w", err)
	}
	return analysis, nil
}

func (s *SEOService) GetAnalysis(ctx context.Context, analysisID uuid.UUID) (*model.SEOAnalysis, error) {
	a := &model.SEOAnalysis{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, post_id, created_by, platform, status,
			original_title, optimized_title, original_description, optimized_description,
			original_tags, optimized_tags, title_score, description_score, overall_score,
			suggestions, keyword_data, competitor_data, credits_used, error_message,
			completed_at, created_at, updated_at
		FROM seo_analyses WHERE id = $1`, analysisID,
	).Scan(
		&a.ID, &a.ProjectID, &a.PostID, &a.CreatedBy,
		&a.Platform, &a.Status, &a.OriginalTitle,
		&a.OptimizedTitle, &a.OriginalDescription,
		&a.OptimizedDescription, &a.OriginalTags, &a.OptimizedTags,
		&a.TitleScore, &a.DescriptionScore, &a.OverallScore,
		&a.Suggestions, &a.KeywordData, &a.CompetitorData,
		&a.CreditsUsed, &a.ErrorMessage, &a.CompletedAt,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get seo analysis: %w", err)
	}
	return a, nil
}

func (s *SEOService) ListAnalysesByProject(ctx context.Context, projectID uuid.UUID) ([]*model.SEOAnalysis, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, post_id, created_by, platform, status,
			original_title, optimized_title, original_description, optimized_description,
			original_tags, optimized_tags, title_score, description_score, overall_score,
			suggestions, keyword_data, competitor_data, credits_used, error_message,
			completed_at, created_at, updated_at
		FROM seo_analyses WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list seo analyses: %w", err)
	}
	defer rows.Close()

	var analyses []*model.SEOAnalysis
	for rows.Next() {
		a := &model.SEOAnalysis{}
		if err := rows.Scan(
			&a.ID, &a.ProjectID, &a.PostID, &a.CreatedBy,
			&a.Platform, &a.Status, &a.OriginalTitle,
			&a.OptimizedTitle, &a.OriginalDescription,
			&a.OptimizedDescription, &a.OriginalTags, &a.OptimizedTags,
			&a.TitleScore, &a.DescriptionScore, &a.OverallScore,
			&a.Suggestions, &a.KeywordData, &a.CompetitorData,
			&a.CreditsUsed, &a.ErrorMessage, &a.CompletedAt,
			&a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan seo analysis: %w", err)
		}
		analyses = append(analyses, a)
	}
	return analyses, nil
}

// ── Keywords ──

func (s *SEOService) TrackKeyword(ctx context.Context, userID uuid.UUID, req *model.TrackKeywordRequest) (*model.SEOKeyword, error) {
	platform := "youtube"
	if req.Platform != "" {
		platform = req.Platform
	}

	kw := &model.SEOKeyword{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO seo_keywords (user_id, keyword, platform, category)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, keyword, search_volume, competition, trend_direction,
			relevance_score, platform, category, last_updated_at, created_at`,
		userID, req.Keyword, platform, strPtrOrNil(req.Category),
	).Scan(
		&kw.ID, &kw.UserID, &kw.Keyword, &kw.SearchVolume,
		&kw.Competition, &kw.TrendDirection, &kw.RelevanceScore,
		&kw.Platform, &kw.Category, &kw.LastUpdatedAt, &kw.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("track keyword: %w", err)
	}
	return kw, nil
}

func (s *SEOService) ListKeywords(ctx context.Context, userID uuid.UUID) ([]*model.SEOKeyword, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, keyword, search_volume, competition, trend_direction,
			relevance_score, platform, category, last_updated_at, created_at
		FROM seo_keywords WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list keywords: %w", err)
	}
	defer rows.Close()

	var keywords []*model.SEOKeyword
	for rows.Next() {
		kw := &model.SEOKeyword{}
		if err := rows.Scan(
			&kw.ID, &kw.UserID, &kw.Keyword, &kw.SearchVolume,
			&kw.Competition, &kw.TrendDirection, &kw.RelevanceScore,
			&kw.Platform, &kw.Category, &kw.LastUpdatedAt, &kw.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan keyword: %w", err)
		}
		keywords = append(keywords, kw)
	}
	return keywords, nil
}

func (s *SEOService) DeleteKeyword(ctx context.Context, userID, keywordID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM seo_keywords WHERE id = $1 AND user_id = $2`, keywordID, userID)
	if err != nil {
		return fmt.Errorf("delete keyword: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("keyword not found")
	}
	return nil
}

// ── Content Suggestions ──

func (s *SEOService) CreateSuggestion(ctx context.Context, userID uuid.UUID, req *model.CreateContentSuggestionRequest) (*model.ContentSuggestion, error) {
	cs := &model.ContentSuggestion{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO content_suggestions (user_id, suggestion_type, title, description,
			keywords, estimated_views, trending_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, user_id, suggestion_type, title, description, keywords,
			estimated_views, confidence, trending_score, source, metadata,
			is_dismissed, created_at`,
		userID, req.SuggestionType, req.Title, strPtrOrNil(req.Description),
		req.Keywords, intPtrOrNil(req.EstimatedViews), req.TrendingScore,
	).Scan(
		&cs.ID, &cs.UserID, &cs.SuggestionType, &cs.Title, &cs.Description,
		&cs.Keywords, &cs.EstimatedViews, &cs.Confidence, &cs.TrendingScore,
		&cs.Source, &cs.Metadata, &cs.IsDismissed, &cs.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create content suggestion: %w", err)
	}
	return cs, nil
}

func (s *SEOService) ListSuggestions(ctx context.Context, userID uuid.UUID, includeDismissed bool) ([]*model.ContentSuggestion, error) {
	query := `
		SELECT id, user_id, suggestion_type, title, description, keywords,
			estimated_views, confidence, trending_score, source, metadata,
			is_dismissed, created_at
		FROM content_suggestions WHERE user_id = $1`
	if !includeDismissed {
		query += ` AND is_dismissed = FALSE`
	}
	query += ` ORDER BY trending_score DESC, created_at DESC`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list suggestions: %w", err)
	}
	defer rows.Close()

	var suggestions []*model.ContentSuggestion
	for rows.Next() {
		cs := &model.ContentSuggestion{}
		if err := rows.Scan(
			&cs.ID, &cs.UserID, &cs.SuggestionType, &cs.Title, &cs.Description,
			&cs.Keywords, &cs.EstimatedViews, &cs.Confidence, &cs.TrendingScore,
			&cs.Source, &cs.Metadata, &cs.IsDismissed, &cs.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		suggestions = append(suggestions, cs)
	}
	return suggestions, nil
}

func (s *SEOService) DismissSuggestion(ctx context.Context, userID, suggestionID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE content_suggestions SET is_dismissed = TRUE
		WHERE id = $1 AND user_id = $2`, suggestionID, userID)
	if err != nil {
		return fmt.Errorf("dismiss suggestion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("suggestion not found")
	}
	return nil
}

// ── Competitor Channels ──

func (s *SEOService) AddCompetitor(ctx context.Context, userID uuid.UUID, req *model.AddCompetitorRequest) (*model.CompetitorChannel, error) {
	platform := "youtube"
	if req.Platform != "" {
		platform = req.Platform
	}

	cc := &model.CompetitorChannel{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO competitor_channels (user_id, platform, channel_name, channel_url, platform_channel_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, platform, channel_name, channel_url, platform_channel_id,
			subscriber_count, video_count, avg_views, top_keywords, last_analyzed_at,
			created_at, updated_at`,
		userID, platform, req.ChannelName, strPtrOrNil(req.ChannelURL), strPtrOrNil(req.PlatformChannelID),
	).Scan(
		&cc.ID, &cc.UserID, &cc.Platform, &cc.ChannelName, &cc.ChannelURL,
		&cc.PlatformChannelID, &cc.SubscriberCount, &cc.VideoCount, &cc.AvgViews,
		&cc.TopKeywords, &cc.LastAnalyzedAt, &cc.CreatedAt, &cc.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("add competitor: %w", err)
	}
	return cc, nil
}

func (s *SEOService) ListCompetitors(ctx context.Context, userID uuid.UUID) ([]*model.CompetitorChannel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, platform, channel_name, channel_url, platform_channel_id,
			subscriber_count, video_count, avg_views, top_keywords, last_analyzed_at,
			created_at, updated_at
		FROM competitor_channels WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list competitors: %w", err)
	}
	defer rows.Close()

	var channels []*model.CompetitorChannel
	for rows.Next() {
		cc := &model.CompetitorChannel{}
		if err := rows.Scan(
			&cc.ID, &cc.UserID, &cc.Platform, &cc.ChannelName, &cc.ChannelURL,
			&cc.PlatformChannelID, &cc.SubscriberCount, &cc.VideoCount, &cc.AvgViews,
			&cc.TopKeywords, &cc.LastAnalyzedAt, &cc.CreatedAt, &cc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan competitor: %w", err)
		}
		channels = append(channels, cc)
	}
	return channels, nil
}

func (s *SEOService) RemoveCompetitor(ctx context.Context, userID, competitorID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM competitor_channels WHERE id = $1 AND user_id = $2`, competitorID, userID)
	if err != nil {
		return fmt.Errorf("remove competitor: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("competitor not found")
	}
	return nil
}

// ── Helpers ──

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtrOrNil(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// ── Pure Scoring Engine (no DB, great for unit tests) ──

// ScoreSEO scores title, description, and tags for SEO quality.
func ScoreSEO(input *model.SEOScoreInput) *model.SEOScoreResult {
	result := &model.SEOScoreResult{}

	// Title scoring (0-100)
	result.TitleScore = scoreSEOTitle(input.Title, input.Platform)
	result.TitleSuggestions = suggestTitleImprovements(input.Title, input.Platform)

	// Description scoring (0-100)
	result.DescriptionScore = scoreSEODescription(input.Description, input.Platform)
	result.DescSuggestions = suggestDescriptionImprovements(input.Description, input.Platform)

	// Tag suggestions
	result.TagSuggestions = suggestTags(input.Tags, input.TargetKeywords)

	// Overall = weighted average
	result.OverallScore = (result.TitleScore*40 + result.DescriptionScore*40 + tagScore(input.Tags)*20) / 100

	return result
}

func scoreSEOTitle(title, platform string) int {
	if title == "" {
		return 0
	}
	score := 0
	length := utf8.RuneCountInString(title)

	// Length check (YouTube optimal: 60-70 chars)
	switch {
	case length >= 40 && length <= 70:
		score += 30 // optimal
	case length >= 20 && length <= 100:
		score += 20
	case length > 0:
		score += 10
	}

	// Contains numbers (drives CTR)
	if strings.ContainsAny(title, "0123456789") {
		score += 15
	}

	// Contains power words
	powerWords := []string{"best", "top", "ultimate", "review", "vs", "how to", "guide", "new", "first"}
	titleLower := strings.ToLower(title)
	for _, pw := range powerWords {
		if strings.Contains(titleLower, pw) {
			score += 10
			break
		}
	}

	// Starts with keyword (first 3 words)
	words := strings.Fields(title)
	if len(words) >= 3 {
		score += 10
	}

	// Has emotional trigger
	emotionalWords := []string{"amazing", "shocking", "incredible", "honest", "truth", "real", "must"}
	for _, ew := range emotionalWords {
		if strings.Contains(titleLower, ew) {
			score += 10
			break
		}
	}

	// Contains brackets/parentheses (drives CTR by 33%)
	if strings.ContainsAny(title, "[]()") {
		score += 10
	}

	// Not all caps
	if title != strings.ToUpper(title) {
		score += 5
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}
	return score
}

func scoreSEODescription(desc, platform string) int {
	if desc == "" {
		return 0
	}
	score := 0
	length := utf8.RuneCountInString(desc)

	// Length check (YouTube: 200+ chars recommended, 5000 max)
	switch {
	case length >= 200 && length <= 5000:
		score += 30
	case length >= 100:
		score += 20
	case length > 0:
		score += 10
	}

	descLower := strings.ToLower(desc)

	// Contains timestamps
	if strings.Contains(desc, ":") && (strings.Contains(descLower, "0:") || strings.Contains(descLower, "1:") || strings.Contains(descLower, "2:")) {
		score += 15
	}

	// Contains links
	if strings.Contains(descLower, "http") || strings.Contains(descLower, "www.") {
		score += 10
	}

	// Contains CTA
	ctaWords := []string{"subscribe", "like", "comment", "share", "follow", "check out", "link"}
	for _, cta := range ctaWords {
		if strings.Contains(descLower, cta) {
			score += 10
			break
		}
	}

	// Has hashtags
	if strings.Contains(desc, "#") {
		score += 10
	}

	// Paragraph structure (multiple lines)
	if strings.Count(desc, "\n") >= 2 {
		score += 10
	}

	// Contains social media links
	socialPlatforms := []string{"instagram", "twitter", "facebook", "tiktok", "discord"}
	for _, sp := range socialPlatforms {
		if strings.Contains(descLower, sp) {
			score += 5
			break
		}
	}

	if score > 100 {
		score = 100
	}
	return score
}

func suggestTitleImprovements(title, platform string) []string {
	var suggestions []string
	if title == "" {
		return []string{"Add a title"}
	}
	length := utf8.RuneCountInString(title)

	if length < 40 {
		suggestions = append(suggestions, "Title is too short -- aim for 40-70 characters")
	} else if length > 70 {
		suggestions = append(suggestions, "Title may be truncated -- keep under 70 characters")
	}

	titleLower := strings.ToLower(title)
	if !strings.ContainsAny(title, "0123456789") {
		suggestions = append(suggestions, "Add a number to boost CTR (e.g., 'Top 5', '2025')")
	}
	if !strings.ContainsAny(title, "[]()") {
		suggestions = append(suggestions, "Add brackets like [Review] or (2025) to increase CTR by ~33%")
	}
	if title == strings.ToUpper(title) && length > 5 {
		suggestions = append(suggestions, "Avoid ALL CAPS -- use title case instead")
	}

	hasPowerWord := false
	for _, pw := range []string{"best", "top", "ultimate", "review", "how to", "guide", "vs"} {
		if strings.Contains(titleLower, pw) {
			hasPowerWord = true
			break
		}
	}
	if !hasPowerWord {
		suggestions = append(suggestions, "Consider adding a power word: Best, Top, Ultimate, Review, How To, Guide")
	}

	return suggestions
}

func suggestDescriptionImprovements(desc, platform string) []string {
	var suggestions []string
	if desc == "" {
		return []string{"Add a description -- YouTube uses it for search ranking"}
	}
	length := utf8.RuneCountInString(desc)

	if length < 200 {
		suggestions = append(suggestions, "Description is too short -- aim for 200+ characters")
	}

	descLower := strings.ToLower(desc)
	if !strings.Contains(descLower, "subscribe") {
		suggestions = append(suggestions, "Add a subscribe CTA")
	}
	if !strings.Contains(desc, "#") {
		suggestions = append(suggestions, "Add relevant hashtags (3-5 recommended)")
	}
	if strings.Count(desc, "\n") < 2 {
		suggestions = append(suggestions, "Break description into paragraphs for readability")
	}
	if !strings.Contains(descLower, "http") {
		suggestions = append(suggestions, "Add links to your social media profiles")
	}

	return suggestions
}

func suggestTags(existing, target []string) []string {
	existingMap := make(map[string]bool)
	for _, t := range existing {
		existingMap[strings.ToLower(t)] = true
	}

	var suggestions []string
	for _, t := range target {
		if !existingMap[strings.ToLower(t)] {
			suggestions = append(suggestions, t)
		}
	}
	return suggestions
}

func tagScore(tags []string) int {
	count := len(tags)
	switch {
	case count >= 5 && count <= 15:
		return 100
	case count >= 3:
		return 70
	case count >= 1:
		return 40
	default:
		return 0
	}
}
