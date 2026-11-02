package service

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// calculateImprovement tests
// ---------------------------------------------------------------------------

func TestCalculateImprovement_Positive(t *testing.T) {
	// Variant beats control by 50%.
	result := calculateImprovement(4.0, 6.0)
	if result != 50.0 {
		t.Errorf("expected 50.0, got %f", result)
	}
}

func TestCalculateImprovement_Negative(t *testing.T) {
	// Control is better: variant is 25% worse.
	result := calculateImprovement(8.0, 6.0)
	if result != -25.0 {
		t.Errorf("expected -25.0, got %f", result)
	}
}

func TestCalculateImprovement_ZeroControl(t *testing.T) {
	// Control is zero, variant is positive -> capped at 100%.
	result := calculateImprovement(0.0, 5.0)
	if result != 100.0 {
		t.Errorf("expected 100.0 (capped), got %f", result)
	}

	// Both zero -> 0%.
	result2 := calculateImprovement(0.0, 0.0)
	if result2 != 0.0 {
		t.Errorf("expected 0.0, got %f", result2)
	}
}

// ---------------------------------------------------------------------------
// calculateConfidence tests
// ---------------------------------------------------------------------------

func TestCalculateConfidence_HighSample(t *testing.T) {
	// Large sample sizes with meaningful difference should yield high confidence.
	conf := calculateConfidence(10000, 10000, 0.05, 0.07)
	if conf < 90 {
		t.Errorf("expected high confidence (>90%%), got %f", conf)
	}
}

func TestCalculateConfidence_LowSample(t *testing.T) {
	// Very small samples should yield low confidence.
	conf := calculateConfidence(10, 10, 0.05, 0.06)
	if conf > 50 {
		t.Errorf("expected low confidence (<50%%), got %f", conf)
	}
}

func TestCalculateConfidence_ZeroSample(t *testing.T) {
	conf := calculateConfidence(0, 100, 0.05, 0.07)
	if conf != 0 {
		t.Errorf("expected 0 for zero sample, got %f", conf)
	}
}

func TestCalculateConfidence_EqualRates(t *testing.T) {
	conf := calculateConfidence(1000, 1000, 0.05, 0.05)
	if conf != 0 {
		t.Errorf("expected 0 for equal rates, got %f", conf)
	}
}

// ---------------------------------------------------------------------------
// Service-level tests (require DB via setupTestPool)
// ---------------------------------------------------------------------------

func TestCreateTest_MinimumTwoVariants(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	postID := uuid.New()

	// Create test user + post for FK constraints.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-user", "abtest-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
		postID, userID, "Test Post", "test-post-"+uuid.New().String()[:8],
	)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	_, err = svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postID,
		Name:           "Single Variant Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postID},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err == nil {
		t.Fatal("expected error for single variant, got nil")
	}
}

func TestCreateTest_MaxFourVariants(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-max-user", "abtest-max-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// Create 5 posts.
	var postIDs []uuid.UUID
	for i := 0; i < 5; i++ {
		pid := uuid.New()
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			pid, userID, "Post "+pid.String()[:8], "post-"+pid.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post %d: %v", i, err)
		}
		postIDs = append(postIDs, pid)
	}

	svc := NewABTestService(pool)

	_, err = svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postIDs[0],
		Name:           "Five Variant Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postIDs[0]},
			{VariantLabel: "B", PostID: postIDs[1]},
			{VariantLabel: "C", PostID: postIDs[2]},
			{VariantLabel: "D", PostID: postIDs[3]},
			{VariantLabel: "E", PostID: postIDs[4]},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err == nil {
		t.Fatal("expected error for 5 variants, got nil")
	}
}

func TestStartTest_RequiresDraft(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-start-user", "abtest-start-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Draft Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Start it once (should succeed).
	if err := svc.StartTest(ctx, userID, test.ID); err != nil {
		t.Fatalf("first StartTest: %v", err)
	}

	// Try to start again (already running, not draft).
	err = svc.StartTest(ctx, userID, test.ID)
	if err == nil {
		t.Fatal("expected error starting non-draft test, got nil")
	}
}

func TestStartTest_RequiresVariants(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	testID := uuid.New()
	postID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-novar-user", "abtest-novar-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
		postID, userID, "Post X", "post-x-"+uuid.New().String()[:8],
	)
	if err != nil {
		t.Fatalf("insert post: %v", err)
	}

	// Insert a test directly with 0 variants to simulate edge case.
	_, err = pool.Exec(ctx,
		`INSERT INTO ab_tests (id, user_id, original_post_id, name, status, metric, duration_hours)
		 VALUES ($1, $2, $3, 'Empty Test', 'draft', 'engagement', 24)`,
		testID, userID, postID,
	)
	if err != nil {
		t.Fatalf("insert ab_test: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id = $1`, testID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE id = $1`, testID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	err = svc.StartTest(ctx, userID, testID)
	if err == nil {
		t.Fatal("expected error for test with no variants, got nil")
	}
}

func TestSelectWinner_ValidVariant(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-winner-user", "abtest-winner-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Winner Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Select variant B as the winner.
	variantB := test.Variants[1]
	if err := svc.SelectWinner(ctx, userID, test.ID, variantB.ID); err != nil {
		t.Fatalf("SelectWinner: %v", err)
	}

	// Reload and verify.
	updated, err := svc.GetTest(ctx, userID, test.ID)
	if err != nil {
		t.Fatalf("GetTest after winner: %v", err)
	}
	if updated.Status != model.ABTestStatusCompleted {
		t.Errorf("expected status completed, got %s", updated.Status)
	}
	if updated.WinnerVariantID == nil || *updated.WinnerVariantID != variantB.ID {
		t.Errorf("expected winner_variant_id %s, got %v", variantB.ID, updated.WinnerVariantID)
	}

	// Check that variant B is marked as winner.
	for _, v := range updated.Variants {
		if v.ID == variantB.ID && !v.IsWinner {
			t.Error("expected variant B is_winner = true")
		}
		if v.ID != variantB.ID && v.IsWinner {
			t.Errorf("expected variant %s is_winner = false", v.VariantLabel)
		}
	}
}

func TestSelectWinner_InvalidVariant(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-inv-user", "abtest-inv-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Invalid Winner Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Try to select a random UUID that is not part of this test.
	fakeVariantID := uuid.New()
	err = svc.SelectWinner(ctx, userID, test.ID, fakeVariantID)
	if err == nil {
		t.Fatal("expected error for invalid variant, got nil")
	}
}

func TestEvaluateTest_ClearWinner(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-eval-user", "abtest-eval-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Evaluate Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Update metrics: A (control) has 5% engagement, B has 10%.
	variantA := test.Variants[0]
	variantB := test.Variants[1]

	if err := svc.UpdateVariantMetrics(ctx, variantA.ID, 1000, 30, 10, 10, 50, 1200); err != nil {
		t.Fatalf("UpdateVariantMetrics A: %v", err)
	}
	if err := svc.UpdateVariantMetrics(ctx, variantB.ID, 1000, 60, 20, 20, 100, 1500); err != nil {
		t.Fatalf("UpdateVariantMetrics B: %v", err)
	}

	result, err := svc.EvaluateTest(ctx, test.ID)
	if err != nil {
		t.Fatalf("EvaluateTest: %v", err)
	}

	if result.WinnerLabel != "B" {
		t.Errorf("expected winner B, got %s", result.WinnerLabel)
	}
	if result.Improvement <= 0 {
		t.Errorf("expected positive improvement, got %f", result.Improvement)
	}
}

func TestEvaluateTest_TiedVariants(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-tie-user", "abtest-tie-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Tied Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Same metrics for both variants.
	for _, v := range test.Variants {
		if err := svc.UpdateVariantMetrics(ctx, v.ID, 1000, 50, 10, 10, 100, 1200); err != nil {
			t.Fatalf("UpdateVariantMetrics %s: %v", v.VariantLabel, err)
		}
	}

	result, err := svc.EvaluateTest(ctx, test.ID)
	if err != nil {
		t.Fatalf("EvaluateTest: %v", err)
	}

	// When tied, the first variant encountered with the best score wins (A).
	if result.Improvement != 0 {
		t.Errorf("expected 0%% improvement for tied variants, got %f", result.Improvement)
	}
	// Confidence should be 0 for identical rates.
	if result.Confidence != 0 {
		t.Errorf("expected 0%% confidence for tied variants, got %f", result.Confidence)
	}
}

func TestAutoEvaluate_IgnoresNotExpired(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-autoeval-user", "abtest-autoeval-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Post "+p.String()[:8], "post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Long Running Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 720, // 30 days
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Start the test (started_at = now).
	if err := svc.StartTest(ctx, userID, test.ID); err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	// AutoEvaluate should not touch this test (720h not yet passed).
	if err := svc.AutoEvaluate(ctx); err != nil {
		t.Fatalf("AutoEvaluate: %v", err)
	}

	// Verify test is still running.
	updated, err := svc.GetTest(ctx, userID, test.ID)
	if err != nil {
		t.Fatalf("GetTest: %v", err)
	}
	if updated.Status != model.ABTestStatusRunning {
		t.Errorf("expected status running, got %s", updated.Status)
	}
}

// Ensure math import is used (compile guard).
var _ = math.Abs

// ---------------------------------------------------------------------------
// Integration Tests - Full lifecycle
// ---------------------------------------------------------------------------

func TestABTest_FullLifecycle(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	// Setup: user + 2 posts.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-lifecycle", "abtest-lifecycle-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Lifecycle Post "+p.String()[:8], "lc-post-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	// 1. Create test.
	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Lifecycle Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricEngagement,
		DurationHours: 24,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}
	if test.Status != model.ABTestStatusDraft {
		t.Errorf("expected draft, got %s", test.Status)
	}
	if len(test.Variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(test.Variants))
	}

	// 2. List tests.
	tests, err := svc.ListTests(ctx, userID)
	if err != nil {
		t.Fatalf("ListTests: %v", err)
	}
	if len(tests) == 0 {
		t.Fatal("expected at least 1 test in list")
	}

	// 3. Start test.
	if err := svc.StartTest(ctx, userID, test.ID); err != nil {
		t.Fatalf("StartTest: %v", err)
	}

	started, err := svc.GetTest(ctx, userID, test.ID)
	if err != nil {
		t.Fatalf("GetTest after start: %v", err)
	}
	if started.Status != model.ABTestStatusRunning {
		t.Errorf("expected running, got %s", started.Status)
	}
	if started.StartedAt == nil {
		t.Error("expected started_at to be set")
	}

	// 4. Update metrics.
	varA := test.Variants[0]
	varB := test.Variants[1]

	if err := svc.UpdateVariantMetrics(ctx, varA.ID, 5000, 100, 50, 25, 200, 6000); err != nil {
		t.Fatalf("UpdateVariantMetrics A: %v", err)
	}
	if err := svc.UpdateVariantMetrics(ctx, varB.ID, 5000, 250, 100, 50, 500, 7000); err != nil {
		t.Fatalf("UpdateVariantMetrics B: %v", err)
	}

	// 5. Evaluate.
	result, err := svc.EvaluateTest(ctx, test.ID)
	if err != nil {
		t.Fatalf("EvaluateTest: %v", err)
	}
	if result.WinnerLabel != "B" {
		t.Errorf("expected winner B, got %s", result.WinnerLabel)
	}
	if result.Improvement <= 0 {
		t.Errorf("expected positive improvement, got %f", result.Improvement)
	}
	if result.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", result.Confidence)
	}

	// 6. Select winner.
	if err := svc.SelectWinner(ctx, userID, test.ID, varB.ID); err != nil {
		t.Fatalf("SelectWinner: %v", err)
	}

	final, err := svc.GetTest(ctx, userID, test.ID)
	if err != nil {
		t.Fatalf("GetTest after winner: %v", err)
	}
	if final.Status != model.ABTestStatusCompleted {
		t.Errorf("expected completed, got %s", final.Status)
	}
	if final.WinnerVariantID == nil || *final.WinnerVariantID != varB.ID {
		t.Errorf("expected winner %s, got %v", varB.ID, final.WinnerVariantID)
	}
	if final.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestABTest_CancelLifecycle(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		userID, "abtest-cancel", "abtest-cancel-"+userID.String()+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	postA, postB := uuid.New(), uuid.New()
	for _, p := range []uuid.UUID{postA, postB} {
		_, err := pool.Exec(ctx,
			`INSERT INTO posts (id, author_id, title, slug, status) VALUES ($1, $2, $3, $4, 'draft')`,
			p, userID, "Cancel Post "+p.String()[:8], "cancel-"+p.String()[:8],
		)
		if err != nil {
			t.Fatalf("insert post: %v", err)
		}
	}
	defer pool.Exec(ctx, `DELETE FROM ab_test_variants WHERE ab_test_id IN (SELECT id FROM ab_tests WHERE user_id = $1)`, userID)
	defer pool.Exec(ctx, `DELETE FROM ab_tests WHERE user_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewABTestService(pool)

	test, err := svc.CreateTest(ctx, userID, &model.CreateABTestRequest{
		OriginalPostID: postA,
		Name:           "Cancel Test",
		Variants: []model.ABTestVariantInput{
			{VariantLabel: "A", PostID: postA},
			{VariantLabel: "B", PostID: postB},
		},
		Metric:        model.ABTestMetricClicks,
		DurationHours: 48,
	})
	if err != nil {
		t.Fatalf("CreateTest: %v", err)
	}

	// Cancel from draft.
	if err := svc.CancelTest(ctx, userID, test.ID); err != nil {
		t.Fatalf("CancelTest: %v", err)
	}

	cancelled, err := svc.GetTest(ctx, userID, test.ID)
	if err != nil {
		t.Fatalf("GetTest after cancel: %v", err)
	}
	if cancelled.Status != model.ABTestStatusCancelled {
		t.Errorf("expected cancelled, got %s", cancelled.Status)
	}

	// Cannot cancel again.
	err = svc.CancelTest(ctx, userID, test.ID)
	if err == nil {
		t.Fatal("expected error cancelling already-cancelled test, got nil")
	}
}
