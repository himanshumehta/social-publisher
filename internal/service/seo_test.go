package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestSEOService_AnalyzeSEO(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	projectID := createTestProjectForSEO(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	req := &model.AnalyzeSEORequest{
		ProjectID:   projectID,
		Title:       "Top 5 Best Bikes Under 3 Lakh [2025 Review]",
		Description: "In this video, we review the top 5 best bikes under 3 lakh rupees.\n\nTimestamps:\n0:00 Introduction\n1:30 Bike #1\n\nSubscribe for more bike reviews!\n\n#bikes #review #2025",
		Tags:        []string{"bikes", "review", "2025", "KTM", "Yamaha"},
		Platform:    "youtube",
	}

	analysis, err := svc.AnalyzeSEO(ctx, userID, req)
	if err != nil {
		t.Fatalf("AnalyzeSEO() error = %v", err)
	}

	if analysis.ID == uuid.Nil {
		t.Error("expected non-nil analysis ID")
	}
	if analysis.TitleScore == 0 {
		t.Error("expected non-zero title score for a good title")
	}
	if analysis.DescriptionScore == 0 {
		t.Error("expected non-zero description score for a good description")
	}
	if analysis.OverallScore == 0 {
		t.Error("expected non-zero overall score")
	}
	if analysis.Status != "completed" {
		t.Errorf("expected status completed, got %s", analysis.Status)
	}
}

func TestSEOService_GetAnalysis(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	projectID := createTestProjectForSEO(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	created, _ := svc.AnalyzeSEO(ctx, userID, &model.AnalyzeSEORequest{
		ProjectID: projectID,
		Title:     "Test Title",
		Tags:      []string{},
	})

	got, err := svc.GetAnalysis(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetAnalysis() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestSEOService_ListAnalysesByProject(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	projectID := createTestProjectForSEO(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	for i := 0; i < 3; i++ {
		_, _ = svc.AnalyzeSEO(ctx, userID, &model.AnalyzeSEORequest{
			ProjectID: projectID,
			Title:     "Test",
			Tags:      []string{},
		})
	}

	analyses, err := svc.ListAnalysesByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListAnalysesByProject() error = %v", err)
	}
	if len(analyses) < 3 {
		t.Errorf("expected at least 3 analyses, got %d", len(analyses))
	}
}

// ── Keyword Tests ──

func TestSEOService_TrackKeyword(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	kw, err := svc.TrackKeyword(ctx, userID, &model.TrackKeywordRequest{
		Keyword:  "best bikes 2025",
		Platform: "youtube",
		Category: "automotive",
	})
	if err != nil {
		t.Fatalf("TrackKeyword() error = %v", err)
	}
	if kw.Keyword != "best bikes 2025" {
		t.Errorf("expected keyword 'best bikes 2025', got %s", kw.Keyword)
	}
}

func TestSEOService_ListKeywords(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	for _, kw := range []string{"ktm duke review", "yamaha r15 review"} {
		_, _ = svc.TrackKeyword(ctx, userID, &model.TrackKeywordRequest{Keyword: kw})
	}

	keywords, err := svc.ListKeywords(ctx, userID)
	if err != nil {
		t.Fatalf("ListKeywords() error = %v", err)
	}
	if len(keywords) < 2 {
		t.Errorf("expected at least 2 keywords, got %d", len(keywords))
	}
}

func TestSEOService_DeleteKeyword(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	kw, _ := svc.TrackKeyword(ctx, userID, &model.TrackKeywordRequest{Keyword: "delete-me-keyword"})

	if err := svc.DeleteKeyword(ctx, userID, kw.ID); err != nil {
		t.Fatalf("DeleteKeyword() error = %v", err)
	}

	// Verify deleted
	keywords, _ := svc.ListKeywords(ctx, userID)
	for _, k := range keywords {
		if k.ID == kw.ID {
			t.Error("keyword should have been deleted")
		}
	}
}

// ── Content Suggestion Tests ──

func TestSEOService_CreateSuggestion(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	suggestion, err := svc.CreateSuggestion(ctx, userID, &model.CreateContentSuggestionRequest{
		SuggestionType: "trending_topic",
		Title:          "KTM 390 Duke 2025 - First Ride Review",
		Description:    "Trending topic in automotive niche",
		Keywords:       []string{"ktm", "390 duke", "2025"},
		TrendingScore:  85,
	})
	if err != nil {
		t.Fatalf("CreateSuggestion() error = %v", err)
	}
	if suggestion.ID == uuid.Nil {
		t.Error("expected non-nil suggestion ID")
	}
	if suggestion.TrendingScore != 85 {
		t.Errorf("expected trending score 85, got %d", suggestion.TrendingScore)
	}
}

func TestSEOService_DismissSuggestion(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	suggestion, _ := svc.CreateSuggestion(ctx, userID, &model.CreateContentSuggestionRequest{
		SuggestionType: "trending_topic",
		Title:          "Dismiss This",
		Keywords:       []string{},
	})

	if err := svc.DismissSuggestion(ctx, userID, suggestion.ID); err != nil {
		t.Fatalf("DismissSuggestion() error = %v", err)
	}

	// List without dismissed
	visible, _ := svc.ListSuggestions(ctx, userID, false)
	for _, s := range visible {
		if s.ID == suggestion.ID {
			t.Error("dismissed suggestion should not appear")
		}
	}

	// List with dismissed
	all, _ := svc.ListSuggestions(ctx, userID, true)
	found := false
	for _, s := range all {
		if s.ID == suggestion.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("dismissed suggestion should appear when includeDismissed=true")
	}
}

// ── Competitor Tests ──

func TestSEOService_AddCompetitor(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	cc, err := svc.AddCompetitor(ctx, userID, &model.AddCompetitorRequest{
		ChannelName: "RevZilla",
		ChannelURL:  "https://youtube.com/@revzilla",
		Platform:    "youtube",
	})
	if err != nil {
		t.Fatalf("AddCompetitor() error = %v", err)
	}
	if cc.ChannelName != "RevZilla" {
		t.Errorf("expected channel name RevZilla, got %s", cc.ChannelName)
	}
}

func TestSEOService_RemoveCompetitor(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSEOService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	cc, _ := svc.AddCompetitor(ctx, userID, &model.AddCompetitorRequest{
		ChannelName: "Remove Me Channel",
	})

	if err := svc.RemoveCompetitor(ctx, userID, cc.ID); err != nil {
		t.Fatalf("RemoveCompetitor() error = %v", err)
	}
}

// ── Pure Scoring Tests (no DB) ──

func TestScoreSEO_GoodTitle(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Title:    "Top 5 Best Bikes Under 3 Lakh [2025 Review]",
		Platform: "youtube",
	})

	if result.TitleScore < 50 {
		t.Errorf("expected high title score for good title, got %d", result.TitleScore)
	}
}

func TestScoreSEO_EmptyTitle(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Title:    "",
		Platform: "youtube",
	})

	if result.TitleScore != 0 {
		t.Errorf("expected 0 title score for empty title, got %d", result.TitleScore)
	}
}

func TestScoreSEO_AllCapsTitle(t *testing.T) {
	good := ScoreSEO(&model.SEOScoreInput{
		Title: "Top 5 Best Bikes [2025]",
	})
	allCaps := ScoreSEO(&model.SEOScoreInput{
		Title: "TOP 5 BEST BIKES [2025]",
	})

	if allCaps.TitleScore >= good.TitleScore {
		t.Errorf("all caps (%d) should score lower than mixed case (%d)", allCaps.TitleScore, good.TitleScore)
	}
}

func TestScoreSEO_GoodDescription(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Description: "In this video, we review the top 5 best bikes.\n\nTimestamps:\n0:00 Introduction\n1:30 Bike #1\n\nSubscribe for more!\nhttps://youtube.com/@example\n\n#bikes #review",
		Platform:    "youtube",
	})

	if result.DescriptionScore < 50 {
		t.Errorf("expected high desc score for good description, got %d", result.DescriptionScore)
	}
}

func TestScoreSEO_EmptyDescription(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Description: "",
		Platform:    "youtube",
	})

	if result.DescriptionScore != 0 {
		t.Errorf("expected 0 desc score for empty description, got %d", result.DescriptionScore)
	}
}

func TestScoreSEO_TitleSuggestions(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Title:    "bike review",
		Platform: "youtube",
	})

	if len(result.TitleSuggestions) == 0 {
		t.Error("expected suggestions for a basic title")
	}
}

func TestScoreSEO_DescriptionSuggestions(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Description: "Short desc.",
		Platform:    "youtube",
	})

	if len(result.DescSuggestions) == 0 {
		t.Error("expected suggestions for a short description")
	}
}

func TestScoreSEO_TagScore(t *testing.T) {
	good := ScoreSEO(&model.SEOScoreInput{
		Title: "Test",
		Tags:  []string{"a", "b", "c", "d", "e", "f", "g"},
	})
	poor := ScoreSEO(&model.SEOScoreInput{
		Title: "Test",
		Tags:  []string{},
	})

	if good.OverallScore <= poor.OverallScore {
		t.Errorf("more tags (%d) should score higher than no tags (%d)", good.OverallScore, poor.OverallScore)
	}
}

func TestScoreSEO_OverallScore(t *testing.T) {
	result := ScoreSEO(&model.SEOScoreInput{
		Title:       "Top 5 Best Bikes Under 3 Lakh [2025 Review]",
		Description: "Full review with timestamps.\n\n0:00 Intro\n\nSubscribe!\nhttps://youtube.com\n\n#bikes",
		Tags:        []string{"bikes", "review", "2025", "ktm", "yamaha"},
		Platform:    "youtube",
	})

	if result.OverallScore == 0 {
		t.Error("expected non-zero overall score for complete content")
	}
	if result.OverallScore > 100 {
		t.Errorf("overall score should not exceed 100, got %d", result.OverallScore)
	}
}

// Helper
func createTestProjectForSEO(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	// Find an existing user to satisfy FK constraints
	userID := findUserByRole(t, pool, "creator")

	slug := "seo-test-" + uuid.New().String()[:8]
	var projectID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO projects (title, slug, description, requirements, budget_amount, budget_currency,
			status, creator_id, created_by, priority, tags)
		VALUES ('SEO Test Project', $1, 'Test', '{}', 0, 'INR', 'active', $2, $2, 0, '{}')
		RETURNING id`, slug, userID,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("create test project: %v", err)
	}
	return projectID
}
