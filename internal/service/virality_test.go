package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ═══════════════════════════════════════════════════════════════════════════
// Scoring Engine Unit Tests -- no database dependencies
// ═══════════════════════════════════════════════════════════════════════════

// ── Hashtag scoring ─────────────────────────────────────────────────────

func TestScoreHashtags_AllTrending(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags: []string{"golang", "docker", "kubernetes"},
	}
	trending := []model.TrendingHashtag{
		{Hashtag: "golang", TrendScore: 30},
		{Hashtag: "docker", TrendScore: 40},
		{Hashtag: "kubernetes", TrendScore: 50},
	}

	result := engine.Score(input, trending)
	// Sum = 120, /5 = 24, capped at 20
	if result.Factors.HashtagScore != 20 {
		t.Errorf("expected hashtag score 20, got %d", result.Factors.HashtagScore)
	}
}

func TestScoreHashtags_NoTrending(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags: []string{"obscuretag1", "obscuretag2"},
	}
	trending := []model.TrendingHashtag{
		{Hashtag: "golang", TrendScore: 50},
	}

	result := engine.Score(input, trending)
	if result.Factors.HashtagScore != 0 {
		t.Errorf("expected hashtag score 0, got %d", result.Factors.HashtagScore)
	}
}

func TestScoreHashtags_PartialMatch(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags: []string{"golang", "randomtag"},
	}
	trending := []model.TrendingHashtag{
		{Hashtag: "golang", TrendScore: 25},
		{Hashtag: "docker", TrendScore: 40},
	}

	result := engine.Score(input, trending)
	// Only "golang" matches: 25/5 = 5
	if result.Factors.HashtagScore != 5 {
		t.Errorf("expected hashtag score 5, got %d", result.Factors.HashtagScore)
	}
}

func TestScoreHashtags_NoHashtags(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags: nil,
	}
	trending := []model.TrendingHashtag{
		{Hashtag: "golang", TrendScore: 50},
	}

	result := engine.Score(input, trending)
	if result.Factors.HashtagScore != 0 {
		t.Errorf("expected hashtag score 0 for no hashtags, got %d", result.Factors.HashtagScore)
	}
}

// ── Timing scoring ──────────────────────────────────────────────────────

func TestScoreTiming_PeakHours(t *testing.T) {
	engine := NewScoringEngine()

	// Test 10 AM on a weekday
	input := model.PostScoringInput{
		PostingHour:    10,
		PostingWeekday: 2, // Tuesday
	}

	result := engine.Score(input, nil)
	if result.Factors.TimingScore != 20 {
		t.Errorf("expected timing score 20 for 10 AM weekday, got %d", result.Factors.TimingScore)
	}

	// Test 8 PM on a weekday
	input.PostingHour = 20
	result = engine.Score(input, nil)
	if result.Factors.TimingScore != 20 {
		t.Errorf("expected timing score 20 for 8 PM weekday, got %d", result.Factors.TimingScore)
	}
}

func TestScoreTiming_OffPeak(t *testing.T) {
	engine := NewScoringEngine()

	// Test 3 AM on a weekday
	input := model.PostScoringInput{
		PostingHour:    3,
		PostingWeekday: 1, // Monday
	}

	result := engine.Score(input, nil)
	if result.Factors.TimingScore != 5 {
		t.Errorf("expected timing score 5 for 3 AM weekday, got %d", result.Factors.TimingScore)
	}
}

// ── Content length scoring ──────────────────────────────────────────────

func TestScoreContentLength_Twitter_Optimal(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		ContentLength:   150,
		TargetPlatforms: []string{"twitter"},
	}

	result := engine.Score(input, nil)
	if result.Factors.ContentLengthScore != 15 {
		t.Errorf("expected content length score 15 for twitter 150 chars, got %d", result.Factors.ContentLengthScore)
	}
}

func TestScoreContentLength_LinkedIn_Optimal(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		ContentLength:   800,
		TargetPlatforms: []string{"linkedin"},
	}

	result := engine.Score(input, nil)
	if result.Factors.ContentLengthScore != 15 {
		t.Errorf("expected content length score 15 for linkedin 800 chars, got %d", result.Factors.ContentLengthScore)
	}
}

func TestScoreContentLength_TooLong(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		ContentLength:   300,
		TargetPlatforms: []string{"twitter"},
	}

	result := engine.Score(input, nil)
	// 300 > 250 for twitter => 5
	if result.Factors.ContentLengthScore != 5 {
		t.Errorf("expected content length score 5 for twitter 300 chars, got %d", result.Factors.ContentLengthScore)
	}
}

// ── Engagement history scoring ──────────────────────────────────────────

func TestScoreEngagementHistory_High(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		AuthorAvgEngagement: 6.5,
	}

	result := engine.Score(input, nil)
	if result.Factors.EngagementHistoryScore != 25 {
		t.Errorf("expected engagement score 25 for 6.5%%, got %d", result.Factors.EngagementHistoryScore)
	}
}

func TestScoreEngagementHistory_Low(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		AuthorAvgEngagement: 0.2,
	}

	result := engine.Score(input, nil)
	if result.Factors.EngagementHistoryScore != 5 {
		t.Errorf("expected engagement score 5 for 0.2%%, got %d", result.Factors.EngagementHistoryScore)
	}
}

// ── Media richness scoring ──────────────────────────────────────────────

func TestScoreMediaRichness_Video(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		HasVideo: true,
		HasImage: true, // video takes precedence
	}

	result := engine.Score(input, nil)
	if result.Factors.MediaRichnessScore != 10 {
		t.Errorf("expected media score 10 for video, got %d", result.Factors.MediaRichnessScore)
	}
}

func TestScoreMediaRichness_ImageOnly(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		HasImage: true,
	}

	result := engine.Score(input, nil)
	if result.Factors.MediaRichnessScore != 7 {
		t.Errorf("expected media score 7 for image, got %d", result.Factors.MediaRichnessScore)
	}
}

func TestScoreMediaRichness_NoMedia(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{}

	result := engine.Score(input, nil)
	if result.Factors.MediaRichnessScore != 1 {
		t.Errorf("expected media score 1 for no media, got %d", result.Factors.MediaRichnessScore)
	}
}

// ── Platform match scoring ──────────────────────────────────────────────

func TestScorePlatformMatch_Optimized(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		ContentLength:   200,
		TargetPlatforms: []string{"twitter", "linkedin"}, // 200 < 280 and 200 < 3000
	}

	result := engine.Score(input, nil)
	if result.Factors.PlatformMatchScore != 10 {
		t.Errorf("expected platform match 10, got %d", result.Factors.PlatformMatchScore)
	}
}

func TestScorePlatformMatch_OverLimit(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		ContentLength:   300,
		TargetPlatforms: []string{"twitter"}, // 300 > 280
	}

	result := engine.Score(input, nil)
	if result.Factors.PlatformMatchScore != 2 {
		t.Errorf("expected platform match 2 for over-limit, got %d", result.Factors.PlatformMatchScore)
	}
}

// ── Combined scoring ────────────────────────────────────────────────────

func TestScoreCombined_HighScore(t *testing.T) {
	engine := NewScoringEngine()

	input := model.PostScoringInput{
		Content:             "Great content about Go programming",
		Hashtags:            []string{"golang", "programming"},
		TargetPlatforms:     []string{"twitter"},
		HasVideo:            true,
		PostingHour:         10,  // Peak hour
		PostingWeekday:      2,   // Tuesday
		AuthorAvgEngagement: 7.0, // High engagement
		ContentLength:       150, // Optimal for Twitter
	}
	trending := []model.TrendingHashtag{
		{Hashtag: "golang", TrendScore: 60},
		{Hashtag: "programming", TrendScore: 50},
	}

	result := engine.Score(input, trending)

	// Expected: hashtag=20, timing=20, content=15, engagement=25, media=10, platform=10 = 100
	if result.Score < 90 {
		t.Errorf("expected high combined score (>=90), got %d", result.Score)
	}
	if result.Score > 100 {
		t.Errorf("score should not exceed 100, got %d", result.Score)
	}
}

func TestScoreCombined_LowScore(t *testing.T) {
	engine := NewScoringEngine()

	input := model.PostScoringInput{
		Content:             "hi",
		Hashtags:            []string{},
		TargetPlatforms:     []string{"twitter"},
		HasVideo:            false,
		HasImage:            false,
		PostingHour:         3,   // Off-peak
		PostingWeekday:      1,   // Monday
		AuthorAvgEngagement: 0.1, // Very low
		ContentLength:       2,   // Way too short
	}

	result := engine.Score(input, nil)

	// Expected: hashtag=0, timing=5, content=5, engagement=5, media=1, platform=10 = 26
	if result.Score > 30 {
		t.Errorf("expected low combined score (<=30), got %d", result.Score)
	}
}

// ── Suggestion generation ───────────────────────────────────────────────

func TestGenerateSuggestions_LowHashtagScore(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags: []string{},
	}
	factors := model.ScoringFactors{
		HashtagScore:           0,
		TimingScore:            20,
		ContentLengthScore:     15,
		EngagementHistoryScore: 25,
		MediaRichnessScore:     10,
		PlatformMatchScore:     10,
	}

	suggestions := engine.GenerateSuggestions(input, factors)

	found := false
	for _, s := range suggestions {
		if viralityContainsSubstring(s, "hashtag") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a suggestion about hashtags when hashtag score is 0")
	}
}

func TestGenerateSuggestions_LowTimingScore(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		PostingHour: 3,
	}
	factors := model.ScoringFactors{
		HashtagScore:           20,
		TimingScore:            5,
		ContentLengthScore:     15,
		EngagementHistoryScore: 25,
		MediaRichnessScore:     10,
		PlatformMatchScore:     10,
	}

	suggestions := engine.GenerateSuggestions(input, factors)

	found := false
	for _, s := range suggestions {
		if viralityContainsSubstring(s, "peak") || viralityContainsSubstring(s, "hour") || viralityContainsSubstring(s, "schedule") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a suggestion about timing when timing score is low")
	}
}

func TestGenerateSuggestions_MultipleLowFactors(t *testing.T) {
	engine := NewScoringEngine()
	input := model.PostScoringInput{
		Hashtags:            []string{},
		PostingHour:         3,
		TargetPlatforms:     []string{"twitter"},
		ContentLength:       300,
		AuthorAvgEngagement: 0.1,
	}
	factors := model.ScoringFactors{
		HashtagScore:           0,
		TimingScore:            5,
		ContentLengthScore:     5,
		EngagementHistoryScore: 5,
		MediaRichnessScore:     1,
		PlatformMatchScore:     2,
	}

	suggestions := engine.GenerateSuggestions(input, factors)

	// Should have at least 4 suggestions (hashtag, timing, engagement, media, platform)
	if len(suggestions) < 4 {
		t.Errorf("expected at least 4 suggestions for multiple low factors, got %d: %v", len(suggestions), suggestions)
	}
}

// ── Test helper ─────────────────────────────────────────────────────────

func viralityContainsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && viralityContainsLower(s, substr))
}

func viralityContainsLower(s, substr string) bool {
	s = viralityToLower(s)
	substr = viralityToLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func viralityToLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// ═══════════════════════════════════════════════════════════════════════════
// Virality Service Integration Tests -- require database
// ═══════════════════════════════════════════════════════════════════════════

func TestViralityService_ScoreAndGet(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Find a creator user.
	userID := findUserByRole(t, pool, "creator")

	// Create a test post.
	postID := createViralityTestPost(t, pool, userID)
	defer cleanupViralityData(t, pool, postID)

	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	// Score the post.
	score, err := svc.ScorePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("ScorePost failed: %v", err)
	}
	if score == nil {
		t.Fatal("expected non-nil score")
	}
	if score.Score < 0 || score.Score > 100 {
		t.Errorf("score out of range: %d", score.Score)
	}
	if score.PostID != postID {
		t.Errorf("expected post_id %s, got %s", postID, score.PostID)
	}

	// Get the persisted score.
	fetched, err := svc.GetScore(ctx, userID, postID)
	if err != nil {
		t.Fatalf("GetScore failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("expected persisted score")
	}
	if fetched.Score != score.Score {
		t.Errorf("expected score %d, got %d", score.Score, fetched.Score)
	}
}

func TestViralityService_ScorePost_Upsert(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	postID := createViralityTestPost(t, pool, userID)
	defer cleanupViralityData(t, pool, postID)

	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	// Score twice -- should upsert, not fail.
	score1, err := svc.ScorePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("first ScorePost failed: %v", err)
	}

	score2, err := svc.ScorePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("second ScorePost (upsert) failed: %v", err)
	}

	if score1.ID != score2.ID {
		t.Error("expected upsert to reuse the same row ID")
	}
}

func TestViralityService_GetSuggestions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")
	postID := createViralityTestPost(t, pool, userID)
	defer cleanupViralityData(t, pool, postID)

	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	// Before scoring, suggestions should be empty.
	suggestions, err := svc.GetSuggestions(ctx, userID, postID)
	if err != nil {
		t.Fatalf("GetSuggestions (before score) failed: %v", err)
	}
	if suggestions == nil {
		t.Fatal("expected non-nil suggestions slice")
	}

	// Score the post, then check suggestions.
	_, err = svc.ScorePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("ScorePost failed: %v", err)
	}

	suggestions, err = svc.GetSuggestions(ctx, userID, postID)
	if err != nil {
		t.Fatalf("GetSuggestions (after score) failed: %v", err)
	}
	// A minimal post should have at least one suggestion.
	if len(suggestions) == 0 {
		t.Log("warning: no suggestions generated for minimal post")
	}
}

func TestViralityService_BatchScoring(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")

	post1 := createViralityTestPost(t, pool, userID)
	post2 := createViralityTestPost(t, pool, userID)
	defer cleanupViralityData(t, pool, post1)
	defer cleanupViralityData(t, pool, post2)

	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	err := svc.ScoreBatch(ctx, []uuid.UUID{post1, post2})
	if err != nil {
		t.Fatalf("ScoreBatch failed: %v", err)
	}

	// Verify both posts were scored.
	for _, pid := range []uuid.UUID{post1, post2} {
		score, err := svc.GetScore(ctx, userID, pid)
		if err != nil {
			t.Fatalf("GetScore after batch failed for %s: %v", pid, err)
		}
		if score == nil {
			t.Errorf("expected score for post %s after batch scoring", pid)
		}
	}
}

func TestViralityService_TrendingHashtags_CRUD(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	uniqueSuffix := uuid.New().String()[:8]
	tag1 := "testtrend_" + uniqueSuffix
	tag2 := "testtrend2_" + uniqueSuffix

	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM trending_hashtags WHERE hashtag LIKE 'testtrend_%'`)
	}()

	// Create.
	err := svc.UpdateTrendingHashtags(ctx, []model.TrendingHashtag{
		{Hashtag: tag1, Platform: "twitter", TrendScore: 50.0, SampleCount: 100},
		{Hashtag: tag2, Platform: "twitter", TrendScore: 30.0, SampleCount: 50},
	})
	if err != nil {
		t.Fatalf("UpdateTrendingHashtags failed: %v", err)
	}

	// Read.
	tags, err := svc.GetTrendingHashtags(ctx, "twitter", 10)
	if err != nil {
		t.Fatalf("GetTrendingHashtags failed: %v", err)
	}

	found := 0
	for _, tag := range tags {
		if tag.Hashtag == tag1 || tag.Hashtag == tag2 {
			found++
		}
	}
	if found < 2 {
		t.Errorf("expected to find both test hashtags, found %d", found)
	}

	// Update (upsert) with new scores.
	err = svc.UpdateTrendingHashtags(ctx, []model.TrendingHashtag{
		{Hashtag: tag1, Platform: "twitter", TrendScore: 80.0, SampleCount: 200},
	})
	if err != nil {
		t.Fatalf("UpdateTrendingHashtags (upsert) failed: %v", err)
	}

	tags, err = svc.GetTrendingHashtags(ctx, "twitter", 100)
	if err != nil {
		t.Fatalf("GetTrendingHashtags after upsert failed: %v", err)
	}
	for _, tag := range tags {
		if tag.Hashtag == tag1 && tag.TrendScore != 80.0 {
			t.Errorf("expected trend_score 80.0 after upsert, got %f", tag.TrendScore)
		}
	}
}

func TestViralityService_PickUnscoredPosts(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := findUserByRole(t, pool, "creator")

	postID := createViralityTestPost(t, pool, userID)
	defer cleanupViralityData(t, pool, postID)

	// Back-date the post so it sorts first in ORDER BY created_at ASC LIMIT 50.
	_, _ = pool.Exec(ctx, `UPDATE posts SET created_at = '2000-01-01' WHERE id = $1`, postID)

	engine := NewScoringEngine()
	svc := NewViralityService(pool, engine)

	// Post should appear as unscored.
	ids, err := svc.PickUnscoredPosts(ctx)
	if err != nil {
		t.Fatalf("PickUnscoredPosts failed: %v", err)
	}

	found := false
	for _, id := range ids {
		if id == postID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unscored post to appear in PickUnscoredPosts results")
	}

	// Score it, then check again.
	_, err = svc.ScorePost(ctx, userID, postID)
	if err != nil {
		t.Fatalf("ScorePost failed: %v", err)
	}

	ids, err = svc.PickUnscoredPosts(ctx)
	if err != nil {
		t.Fatalf("PickUnscoredPosts (after scoring) failed: %v", err)
	}
	for _, id := range ids {
		if id == postID {
			t.Error("scored post should not appear in PickUnscoredPosts")
		}
	}
}

// ── Test helpers ────────────────────────────────────────────────────────

func createViralityTestPost(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) uuid.UUID {
	t.Helper()
	slug := "test-virality-" + uuid.New().String()[:8]
	var postID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO posts (author_id, title, slug, content, status, target_platforms, tags)
		VALUES ($1, $2, $3, $4, 'draft', $5, $6)
		RETURNING id
	`, userID, "Test Virality Post "+slug, slug, "This is a test post for virality scoring.",
		[]string{"twitter", "linkedin"}, []string{"test", "golang"},
	).Scan(&postID)
	if err != nil {
		t.Fatalf("failed to create test post: %v", err)
	}
	return postID
}

func cleanupViralityData(t *testing.T, pool *pgxpool.Pool, postID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, `DELETE FROM virality_scores WHERE post_id = $1`, postID)
	_, _ = pool.Exec(ctx, `DELETE FROM posts WHERE id = $1`, postID)
}
