package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestSmartClipService_CreateJob(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	req := &model.CreateSmartClipJobRequest{
		ProjectID:          projectID,
		VideoID:            videoID,
		ClipCount:          10,
		MinDurationSeconds: 15,
		MaxDurationSeconds: 45,
		TargetAspectRatio:  "9:16",
	}

	job, err := svc.CreateJob(ctx, userID, req)
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	if job.ID == uuid.Nil {
		t.Error("expected non-nil job ID")
	}
	if job.Status != model.SmartClipJobPending {
		t.Errorf("expected status pending, got %s", job.Status)
	}
	if job.ClipCount != 10 {
		t.Errorf("expected clip count 10, got %d", job.ClipCount)
	}
	if job.TargetAspectRatio != "9:16" {
		t.Errorf("expected aspect ratio 9:16, got %s", job.TargetAspectRatio)
	}
	if !job.AutoCaptions {
		t.Error("expected auto_captions to be true by default")
	}
	if !job.DetectHooks {
		t.Error("expected detect_hooks to be true by default")
	}
}

func TestSmartClipService_InvalidAspectRatio(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	_, err := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID:         uuid.New(),
		VideoID:           uuid.New(),
		TargetAspectRatio: "3:2",
	})
	if err == nil {
		t.Fatal("expected error for invalid aspect ratio")
	}
}

func TestSmartClipService_MinExceedsMax(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	_, err := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID:          uuid.New(),
		VideoID:            uuid.New(),
		MinDurationSeconds: 60,
		MaxDurationSeconds: 15,
	})
	if err == nil {
		t.Fatal("expected error when min > max")
	}
}

func TestSmartClipService_GetJob(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	created, _ := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID: projectID, VideoID: videoID,
	})

	got, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, got.ID)
	}
}

func TestSmartClipService_ListByProject(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	for i := 0; i < 3; i++ {
		_, _ = svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
			ProjectID: projectID, VideoID: videoID,
		})
	}

	jobs, err := svc.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(jobs) < 3 {
		t.Errorf("expected at least 3 jobs, got %d", len(jobs))
	}
}

func TestSmartClipService_Cancel(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, _ := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID: projectID, VideoID: videoID,
	})

	if err := svc.Cancel(ctx, userID, job.ID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	got, _ := svc.GetJob(ctx, job.ID)
	if got.Status != model.SmartClipJobCancelled {
		t.Errorf("expected cancelled, got %s", got.Status)
	}
}

func TestSmartClipService_PickPendingAndComplete(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	_, _ = svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID: projectID, VideoID: videoID,
	})

	picked, err := svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending() error = %v", err)
	}
	if picked.Status != model.SmartClipJobProcessing {
		t.Errorf("expected processing, got %s", picked.Status)
	}

	if err := svc.Complete(ctx, picked.ID, 15); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	got, _ := svc.GetJob(ctx, picked.ID)
	if got.Status != model.SmartClipJobCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
	if got.CreditsUsed != 15 {
		t.Errorf("expected 15 credits, got %d", got.CreditsUsed)
	}
}

func TestSmartClipService_CreateClipAndMoment(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, _ := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID: projectID, VideoID: videoID,
	})

	// Create a clip
	title := "Best Hook Moment"
	hookText := "You won't believe what happens next"
	clip, err := svc.CreateClip(ctx, &model.SmartClip{
		JobID:                job.ID,
		Title:                &title,
		StartTimeMs:          5000,
		EndTimeMs:            35000,
		DurationMs:           30000,
		HookScore:            85,
		ViralityScore:        72,
		EngagementPrediction: 0.85,
		AspectRatio:          "9:16",
		HasCaptions:          true,
		HookText:             &hookText,
		Tags:                 []string{"automotive", "review"},
		Status:               "completed",
		Rank:                 1,
	})
	if err != nil {
		t.Fatalf("CreateClip() error = %v", err)
	}
	if clip.ID == uuid.Nil {
		t.Error("expected non-nil clip ID")
	}
	if clip.HookScore != 85 {
		t.Errorf("expected hook score 85, got %d", clip.HookScore)
	}

	// Create a moment
	desc := "Strong opening statement"
	moment, err := svc.CreateMoment(ctx, &model.ClipMoment{
		JobID:       job.ID,
		MomentType:  model.MomentHook,
		StartTimeMs: 5000,
		EndTimeMs:   8000,
		Confidence:  0.92,
		Description: &desc,
		HookScore:   88,
	})
	if err != nil {
		t.Fatalf("CreateMoment() error = %v", err)
	}
	if moment.ID == uuid.Nil {
		t.Error("expected non-nil moment ID")
	}

	// List clips
	clips, err := svc.ListClips(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListClips() error = %v", err)
	}
	if len(clips) != 1 {
		t.Errorf("expected 1 clip, got %d", len(clips))
	}

	// List moments
	moments, err := svc.ListMoments(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListMoments() error = %v", err)
	}
	if len(moments) != 1 {
		t.Errorf("expected 1 moment, got %d", len(moments))
	}
}

func TestSmartClipService_GetTopClips(t *testing.T) {
	pool := setupTestPool(t)
	svc := NewSmartClipService(pool)
	ctx := context.Background()

	projectID, videoID := createTestProjectAndVideoForClips(t, pool)
	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")

	job, _ := svc.CreateJob(ctx, userID, &model.CreateSmartClipJobRequest{
		ProjectID: projectID, VideoID: videoID,
	})

	// Create clips with different scores
	for i, score := range []int{90, 70, 50, 85, 60} {
		_, err := svc.CreateClip(ctx, &model.SmartClip{
			JobID:         job.ID,
			StartTimeMs:   i * 30000,
			EndTimeMs:     (i + 1) * 30000,
			DurationMs:    30000,
			ViralityScore: score,
			HookScore:     score - 10,
			AspectRatio:   "9:16",
			Tags:          []string{},
			Status:        "completed",
			Rank:          i + 1,
		})
		if err != nil {
			t.Fatalf("CreateClip() error = %v", err)
		}
	}

	top, err := svc.GetTopClips(ctx, job.ID, 3)
	if err != nil {
		t.Fatalf("GetTopClips() error = %v", err)
	}
	if len(top) != 3 {
		t.Errorf("expected 3 top clips, got %d", len(top))
	}
	// Should be sorted by virality_score DESC
	if len(top) >= 2 && top[0].ViralityScore < top[1].ViralityScore {
		t.Error("top clips should be sorted by virality score descending")
	}
}

// -- Scoring Engine Tests (Pure Logic, No DB) --

func TestScoreClip_PerfectClip(t *testing.T) {
	input := &model.ClipScoringInput{
		DurationMs:     30000, // 30 seconds -- sweet spot
		HasHook:        true,
		HookScore:      90,
		HasCTA:         true,
		WordCount:      90, // ~3 words/sec
		TopicRelevance: 0.9,
		PacingScore:    0.8,
		Tags:           []string{"automotive"},
	}

	score := ScoreClip(input)
	if score < 70 {
		t.Errorf("expected high score for perfect clip, got %d", score)
	}
}

func TestScoreClip_NoHookNoCTA(t *testing.T) {
	input := &model.ClipScoringInput{
		DurationMs:     30000,
		HasHook:        false,
		HookScore:      0,
		HasCTA:         false,
		WordCount:      90,
		TopicRelevance: 0.5,
		PacingScore:    0.5,
	}

	score := ScoreClip(input)
	if score > 50 {
		t.Errorf("expected lower score without hook/CTA, got %d", score)
	}
}

func TestScoreClip_DurationSweetSpot(t *testing.T) {
	// 30s (sweet spot) vs 120s (too long)
	short := ScoreClip(&model.ClipScoringInput{
		DurationMs: 30000,
		HasHook:    true, HookScore: 50,
		WordCount: 90, PacingScore: 0.5, TopicRelevance: 0.5,
	})
	long := ScoreClip(&model.ClipScoringInput{
		DurationMs: 120000,
		HasHook:    true, HookScore: 50,
		WordCount: 360, PacingScore: 0.5, TopicRelevance: 0.5,
	})

	if short <= long {
		t.Errorf("30s clip (%d) should score higher than 120s clip (%d)", short, long)
	}
}

func TestScoreClip_ExceptionalHookBonus(t *testing.T) {
	normal := ScoreClip(&model.ClipScoringInput{
		DurationMs: 30000, HasHook: true, HookScore: 70,
		WordCount: 90, PacingScore: 0.5, TopicRelevance: 0.5,
	})
	exceptional := ScoreClip(&model.ClipScoringInput{
		DurationMs: 30000, HasHook: true, HookScore: 90,
		WordCount: 90, PacingScore: 0.5, TopicRelevance: 0.5,
	})

	if exceptional <= normal {
		t.Errorf("exceptional hook (%d) should score higher than normal (%d)", exceptional, normal)
	}
}

func TestScoreClip_WordDensity(t *testing.T) {
	// Optimal: 2-4 words/sec
	optimal := ScoreClip(&model.ClipScoringInput{
		DurationMs: 30000, WordCount: 90, // 3 words/sec
		PacingScore: 0.5, TopicRelevance: 0.5,
	})
	sparse := ScoreClip(&model.ClipScoringInput{
		DurationMs: 30000, WordCount: 15, // 0.5 words/sec
		PacingScore: 0.5, TopicRelevance: 0.5,
	})

	if optimal <= sparse {
		t.Errorf("optimal word density (%d) should score higher than sparse (%d)", optimal, sparse)
	}
}

func TestSmartClipService_EstimateCredits(t *testing.T) {
	svc := &SmartClipService{}

	tests := []struct {
		clipCount int
		expected  int
	}{
		{5, 15},
		{10, 30},
		{0, 15}, // defaults to 5
		{1, 3},
	}

	for _, tt := range tests {
		got := svc.EstimateCredits(tt.clipCount)
		if got != tt.expected {
			t.Errorf("EstimateCredits(%d) = %d, want %d", tt.clipCount, got, tt.expected)
		}
	}
}

// Helper
func createTestProjectAndVideoForClips(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	slug := "clip-test-" + uuid.New().String()[:8]
	var projectID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO projects (category_id, creator_id, created_by, title, slug, description, status)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000004',
			'b0000000-0000-0000-0000-000000000004', 'Clip Test Project', $1, 'Test', 'active')
		RETURNING id`, slug,
	).Scan(&projectID)
	if err != nil {
		t.Fatalf("create test project: %v", err)
	}

	userID := uuid.MustParse("b0000000-0000-0000-0000-000000000004")
	videoID := createTestVideo(t, pool, projectID, userID, "youtube_16_9")

	return projectID, videoID
}
