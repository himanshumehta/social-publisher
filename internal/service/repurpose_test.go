package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ---------------------------------------------------------------------------
// Unit tests (no database required)
// ---------------------------------------------------------------------------

func TestAdaptContentForPlatform_Twitter(t *testing.T) {
	// Content longer than 280 chars should be truncated
	longContent := "This is a very long piece of content that goes well beyond the Twitter character limit. " +
		"It contains multiple sentences and lots of detail that needs to be condensed down into a " +
		"much shorter format suitable for tweeting. Adding more words to ensure we exceed the limit " +
		"by a comfortable margin so the truncation logic is properly exercised in this test case."

	text, _ := adaptContentForPlatform(longContent, "twitter")

	if len(text) > 280 {
		t.Errorf("twitter text length = %d, want <= 280", len(text))
	}
	if text[len(text)-3:] != "..." {
		t.Errorf("truncated text should end with '...', got %q", text[len(text)-3:])
	}
}

func TestAdaptContentForPlatform_LinkedIn(t *testing.T) {
	// Short content should not be truncated on LinkedIn (limit: 3000)
	content := "A short LinkedIn post about professional development."

	text, _ := adaptContentForPlatform(content, "linkedin")

	if text != content {
		t.Errorf("linkedin text = %q, want %q (should not truncate short content)", text, content)
	}
}

func TestAdaptContentForPlatform_Instagram(t *testing.T) {
	// Instagram limit is 2200 chars. Content under limit should pass through.
	// Instagram should always have hashtags.
	content := "Check out this amazing photo from my latest trip."

	text, hashtags := adaptContentForPlatform(content, "instagram")

	if len(text) > 2200 {
		t.Errorf("instagram text length = %d, want <= 2200", len(text))
	}
	if text != content {
		t.Errorf("instagram text = %q, want %q", text, content)
	}
	if len(hashtags) == 0 {
		t.Error("instagram should have default hashtags when none found in content")
	}
}

func TestAdaptContentForPlatform_InstagramWithHashtags(t *testing.T) {
	content := "Great day at the beach #summer #vacation #travel"

	text, hashtags := adaptContentForPlatform(content, "instagram")

	// Hashtags should be extracted
	if len(hashtags) != 3 {
		t.Errorf("expected 3 hashtags, got %d: %v", len(hashtags), hashtags)
	}

	// Text should not contain the hashtag tokens
	if text == content {
		t.Error("text should have hashtags removed from inline content")
	}
}

func TestAdaptContentForPlatform_UnknownPlatform(t *testing.T) {
	content := "Some content for an unknown platform."

	text, _ := adaptContentForPlatform(content, "tiktok")

	// Should use default limit (500) and not panic
	if text != content {
		t.Errorf("unknown platform text = %q, want %q", text, content)
	}
}

func TestGetStrategyPrompt_LongToShort(t *testing.T) {
	prompt := getStrategyPrompt(model.StrategyLongToShort, "Long content here", "")

	if prompt == "" {
		t.Error("expected non-empty prompt for long_to_short strategy")
	}
	if !repurposeContains(prompt, "Condense") {
		t.Errorf("long_to_short prompt should contain 'Condense', got: %s", prompt)
	}
	if !repurposeContains(prompt, "Long content here") {
		t.Error("prompt should include the source content")
	}
}

func TestGetStrategyPrompt_BlogToThread(t *testing.T) {
	prompt := getStrategyPrompt(model.StrategyBlogToThread, "Blog post content", "")

	if prompt == "" {
		t.Error("expected non-empty prompt for blog_to_thread strategy")
	}
	if !repurposeContains(prompt, "thread") {
		t.Errorf("blog_to_thread prompt should mention 'thread', got: %s", prompt)
	}
}

func TestGetStrategyPrompt_CrossPlatform(t *testing.T) {
	prompt := getStrategyPrompt(model.StrategyCrossPlatform, "Some content", "twitter")

	if !repurposeContains(prompt, "twitter") {
		t.Errorf("cross_platform prompt should include source platform 'twitter', got: %s", prompt)
	}
}

func TestExtractHashtags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"no hashtags", "just plain text", 0},
		{"single hashtag", "hello #world", 1},
		{"multiple hashtags", "#go #programming #code", 3},
		{"duplicate hashtags", "#go #Go #GO", 1}, // case-insensitive dedup
		{"hashtag with punctuation", "#hello, #world!", 2},
		{"just hash sign", "# nothing", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := extractHashtags(tt.content)
			if len(tags) != tt.want {
				t.Errorf("extractHashtags(%q) returned %d tags, want %d: %v", tt.content, len(tags), tt.want, tags)
			}
		})
	}
}

func TestValidStrategies(t *testing.T) {
	validOnes := []string{"long_to_short", "blog_to_thread", "video_to_caption", "podcast_to_blog", "cross_platform"}
	for _, s := range validOnes {
		if !model.ValidStrategies[model.RepurposeStrategy(s)] {
			t.Errorf("expected strategy %q to be valid", s)
		}
	}

	invalidOnes := []string{"invalid", "", "summarize", "LONG_TO_SHORT"}
	for _, s := range invalidOnes {
		if model.ValidStrategies[model.RepurposeStrategy(s)] {
			t.Errorf("expected strategy %q to be invalid", s)
		}
	}
}

// repurposeContains is a helper for readable assertions.
func repurposeContains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// ---------------------------------------------------------------------------
// Integration tests (require a live Postgres database)
// ---------------------------------------------------------------------------

// createRepurposeTestPost inserts a minimal post for repurpose testing.
// projectID is optional -- when non-nil it sets the post's project_id so
// that the downstream ai_job FK constraint is satisfied.
func createRepurposeTestPost(t *testing.T, pool *pgxpool.Pool, authorID uuid.UUID, content string, projectID *uuid.UUID) uuid.UUID {
	t.Helper()
	slug := "test-post-" + uuid.New().String()[:8]
	var postID uuid.UUID
	err := pool.QueryRow(context.Background(),
		`INSERT INTO posts
		    (author_id, project_id, title, slug, content, target_platforms, status,
		     system_review_status, tone, tags, is_ai_generated)
		 VALUES ($1, $2, $3, $4, $5, '{twitter}', 'draft', 'pending', '', '{}', FALSE)
		 RETURNING id`,
		authorID, projectID, "Test Post "+slug, slug, content,
	).Scan(&postID)
	if err != nil {
		t.Fatalf("failed to create test post: %v", err)
	}
	return postID
}

func TestCreateRepurposeJob_ValidatesSourcePost(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-test-user", "repurpose-validate-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewRepurposeService(pool)

	// Non-existent source post
	_, err = svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    uuid.New(),
		TargetPlatforms: []string{"twitter"},
		Strategy:        "long_to_short",
	})
	if err == nil {
		t.Fatal("expected error for non-existent source post")
	}
	if !repurposeContains(err.Error(), "source post not found") {
		t.Errorf("expected 'source post not found' error, got: %v", err)
	}
}

func TestCreateRepurposeJob_InvalidStrategy(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-test-user", "repurpose-strategy-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewRepurposeService(pool)

	_, err = svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    uuid.New(),
		TargetPlatforms: []string{"twitter"},
		Strategy:        "invalid_strategy",
	})
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
	if !repurposeContains(err.Error(), "invalid repurpose strategy") {
		t.Errorf("expected 'invalid repurpose strategy' error, got: %v", err)
	}
}

func TestCreateRepurposeJob_InvalidPlatform(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-test-user", "repurpose-platform-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewRepurposeService(pool)

	_, err = svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    uuid.New(),
		TargetPlatforms: []string{"myspace"},
		Strategy:        "long_to_short",
	})
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !repurposeContains(err.Error(), "unsupported target platform") {
		t.Errorf("expected 'unsupported target platform' error, got: %v", err)
	}
}

func TestCreateRepurposeJob_EmptyPlatforms(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-test-user", "repurpose-empty-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewRepurposeService(pool)

	_, err = svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    uuid.New(),
		TargetPlatforms: []string{},
		Strategy:        "long_to_short",
	})
	if err == nil {
		t.Fatal("expected error for empty target platforms")
	}
	if !repurposeContains(err.Error(), "at least one target platform") {
		t.Errorf("expected 'at least one target platform' error, got: %v", err)
	}
}

func TestRepurposeIntegration_FullLifecycle(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	// Setup: create user
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-int-user", "repurpose-int-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	// Setup: create a project so ai_jobs FK constraint is satisfied
	projectID := createTestProject(t, pool, userID, "active")

	// Setup: create a source post with enough content
	sourceContent := "This is a comprehensive blog post about software engineering best practices. " +
		"It covers topics like code reviews, testing strategies, continuous integration, and deployment " +
		"pipelines. The post dives deep into each topic with practical examples and actionable advice " +
		"that engineering teams can apply immediately to improve their development workflow."
	postID := createRepurposeTestPost(t, pool, userID, sourceContent, &projectID)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM repurpose_outputs WHERE repurpose_job_id IN (SELECT id FROM repurpose_jobs WHERE user_id = $1)`, userID)
		pool.Exec(ctx, `DELETE FROM repurpose_jobs WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE created_by = $1`, userID)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewRepurposeService(pool)

	// Step 1: Create a repurpose job
	job, err := svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"twitter", "linkedin"},
		Strategy:        "long_to_short",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if job.ID == uuid.Nil {
		t.Error("expected non-nil job ID")
	}
	if job.Status != model.RepurposeStatusPending {
		t.Errorf("status = %q, want %q", job.Status, model.RepurposeStatusPending)
	}
	if job.AIJobID == nil {
		t.Error("expected ai_job_id to be set")
	}
	if len(job.TargetPlatforms) != 2 {
		t.Errorf("target_platforms count = %d, want 2", len(job.TargetPlatforms))
	}

	// Step 2: Verify we can retrieve the job
	gotJob, err := svc.GetJob(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if gotJob.ID != job.ID {
		t.Errorf("GetJob ID = %s, want %s", gotJob.ID, job.ID)
	}

	// Step 3: List jobs
	jobs, err := svc.ListJobs(ctx, userID)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	found := false
	for _, j := range jobs {
		if j.ID == job.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("created job not found in ListJobs results")
	}

	// Step 4: Process the repurpose job (simulating worker)
	err = svc.ProcessRepurpose(ctx, *job.AIJobID)
	if err != nil {
		t.Fatalf("ProcessRepurpose: %v", err)
	}

	// Step 5: Verify job status is completed
	completedJob, err := svc.GetJob(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetJob after process: %v", err)
	}
	if completedJob.Status != model.RepurposeStatusCompleted {
		t.Errorf("status after processing = %q, want %q", completedJob.Status, model.RepurposeStatusCompleted)
	}

	// Step 6: Get outputs
	outputs, err := svc.GetOutputs(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("outputs count = %d, want 2", len(outputs))
	}

	// Verify each output has content for the correct platform
	platformsSeen := map[string]bool{}
	for _, o := range outputs {
		platformsSeen[o.TargetPlatform] = true
		if o.GeneratedText == "" {
			t.Errorf("output for %s has empty text", o.TargetPlatform)
		}
		if o.Accepted {
			t.Errorf("output for %s should not be accepted yet", o.TargetPlatform)
		}
	}
	if !platformsSeen["twitter"] {
		t.Error("missing twitter output")
	}
	if !platformsSeen["linkedin"] {
		t.Error("missing linkedin output")
	}

	// Verify twitter output respects character limit
	for _, o := range outputs {
		if o.TargetPlatform == "twitter" && len(o.GeneratedText) > 280 {
			t.Errorf("twitter output text length = %d, want <= 280", len(o.GeneratedText))
		}
	}

	// Step 7: Accept an output -- creates a new post
	twitterOutput := outputs[0]
	if outputs[1].TargetPlatform == "twitter" {
		twitterOutput = outputs[1]
	}

	newPostID, err := svc.AcceptOutput(ctx, userID, twitterOutput.ID)
	if err != nil {
		t.Fatalf("AcceptOutput: %v", err)
	}
	if newPostID == nil || *newPostID == uuid.Nil {
		t.Fatal("expected non-nil new post ID from AcceptOutput")
	}

	// Verify the new post exists
	var newPostTitle, newPostStatus string
	err = pool.QueryRow(ctx,
		`SELECT title, status FROM posts WHERE id = $1`, *newPostID,
	).Scan(&newPostTitle, &newPostStatus)
	if err != nil {
		t.Fatalf("query new post: %v", err)
	}
	if newPostStatus != "draft" {
		t.Errorf("new post status = %q, want 'draft'", newPostStatus)
	}
	if newPostTitle == "" {
		t.Error("new post should have a title")
	}

	// Step 8: Verify output is marked as accepted
	updatedOutputs, err := svc.GetOutputs(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetOutputs after accept: %v", err)
	}
	for _, o := range updatedOutputs {
		if o.ID == twitterOutput.ID {
			if !o.Accepted {
				t.Error("accepted output should have accepted=true")
			}
			if o.AcceptedPostID == nil || *o.AcceptedPostID != *newPostID {
				t.Errorf("accepted_post_id = %v, want %s", o.AcceptedPostID, *newPostID)
			}
		}
	}

	// Step 9: Try to accept the same output again -- should fail
	_, err = svc.AcceptOutput(ctx, userID, twitterOutput.ID)
	if err == nil {
		t.Fatal("expected error when accepting already accepted output")
	}
	if !repurposeContains(err.Error(), "already accepted") {
		t.Errorf("expected 'already accepted' error, got: %v", err)
	}
}

func TestRepurposeIntegration_SourcePostOwnership(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Create two users
	userA := uuid.New()
	userB := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userA, "user-a", "repurpose-owner-a-"+userA.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert user A: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userB, "user-b", "repurpose-owner-b-"+userB.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert user B: %v", err)
	}

	// User A creates a project and a post
	projectID := createTestProject(t, pool, userA, "active")
	postContent := "This is user A's post with enough content for repurposing into shorter format for various platforms."
	postID := createRepurposeTestPost(t, pool, userA, postContent, &projectID)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM repurpose_jobs WHERE user_id IN ($1, $2)`, userA, userB)
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE created_by IN ($1, $2)`, userA, userB)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id IN ($1, $2)`, userA, userB)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, userA, userB)
	})

	svc := NewRepurposeService(pool)

	// User B tries to repurpose User A's post -- should fail
	_, err = svc.CreateJob(ctx, userB, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"twitter"},
		Strategy:        "long_to_short",
	})
	if err == nil {
		t.Fatal("expected error when repurposing another user's post")
	}
	if !repurposeContains(err.Error(), "does not belong to user") {
		t.Errorf("expected ownership error, got: %v", err)
	}

	// User A can repurpose their own post
	job, err := svc.CreateJob(ctx, userA, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"twitter"},
		Strategy:        "cross_platform",
	})
	if err != nil {
		t.Fatalf("CreateJob for owner: %v", err)
	}
	if job.ID == uuid.Nil {
		t.Error("expected non-nil job ID for post owner")
	}
}

func TestRepurposeIntegration_ProcessGeneratesOutputs(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-process-user", "repurpose-process-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	sourceContent := "A detailed article about machine learning, covering neural networks, " +
		"training data, model evaluation, and deployment strategies. #machinelearning #AI #tech"
	postID := createRepurposeTestPost(t, pool, userID, sourceContent, &projectID)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM repurpose_outputs WHERE repurpose_job_id IN (SELECT id FROM repurpose_jobs WHERE user_id = $1)`, userID)
		pool.Exec(ctx, `DELETE FROM repurpose_jobs WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE created_by = $1`, userID)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewRepurposeService(pool)

	// Create and process
	job, err := svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"twitter", "instagram", "linkedin"},
		Strategy:        "long_to_short",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	err = svc.ProcessRepurpose(ctx, *job.AIJobID)
	if err != nil {
		t.Fatalf("ProcessRepurpose: %v", err)
	}

	outputs, err := svc.GetOutputs(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}

	if len(outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(outputs))
	}

	// Verify platform-specific rules
	for _, o := range outputs {
		switch o.TargetPlatform {
		case "twitter":
			if len(o.GeneratedText) > 280 {
				t.Errorf("twitter output exceeds 280 chars: %d", len(o.GeneratedText))
			}
		case "instagram":
			if len(o.GeneratedText) > 2200 {
				t.Errorf("instagram output exceeds 2200 chars: %d", len(o.GeneratedText))
			}
			if len(o.GeneratedHashtags) == 0 {
				t.Error("instagram output should have hashtags")
			}
		case "linkedin":
			if len(o.GeneratedText) > 3000 {
				t.Errorf("linkedin output exceeds 3000 chars: %d", len(o.GeneratedText))
			}
		}
	}
}

func TestAcceptOutput_CreatesNewPost(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-accept-user", "repurpose-accept-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	sourceContent := "Original content that is long enough to be repurposed into a shorter format for social media platforms."
	postID := createRepurposeTestPost(t, pool, userID, sourceContent, &projectID)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM repurpose_outputs WHERE repurpose_job_id IN (SELECT id FROM repurpose_jobs WHERE user_id = $1)`, userID)
		pool.Exec(ctx, `DELETE FROM repurpose_jobs WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE created_by = $1`, userID)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewRepurposeService(pool)

	job, err := svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"linkedin"},
		Strategy:        "cross_platform",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	err = svc.ProcessRepurpose(ctx, *job.AIJobID)
	if err != nil {
		t.Fatalf("ProcessRepurpose: %v", err)
	}

	outputs, err := svc.GetOutputs(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	newPostID, err := svc.AcceptOutput(ctx, userID, outputs[0].ID)
	if err != nil {
		t.Fatalf("AcceptOutput: %v", err)
	}

	// Verify the new post
	var isAI bool
	var targetPlatforms []string
	err = pool.QueryRow(ctx,
		`SELECT is_ai_generated, target_platforms FROM posts WHERE id = $1`, *newPostID,
	).Scan(&isAI, &targetPlatforms)
	if err != nil {
		t.Fatalf("query new post: %v", err)
	}
	if !isAI {
		t.Error("new post should be marked as AI generated")
	}
	if len(targetPlatforms) != 1 || targetPlatforms[0] != "linkedin" {
		t.Errorf("target_platforms = %v, want [linkedin]", targetPlatforms)
	}
}

func TestAcceptOutput_AlreadyAccepted(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "repurpose-dup-user", "repurpose-dup-"+userID.String()+"@test.local", "creator",
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	sourceContent := "Content for testing duplicate acceptance handling across the repurpose pipeline and output system."
	postID := createRepurposeTestPost(t, pool, userID, sourceContent, &projectID)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM repurpose_outputs WHERE repurpose_job_id IN (SELECT id FROM repurpose_jobs WHERE user_id = $1)`, userID)
		pool.Exec(ctx, `DELETE FROM repurpose_jobs WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE created_by = $1`, userID)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewRepurposeService(pool)

	job, err := svc.CreateJob(ctx, userID, &model.CreateRepurposeRequest{
		SourcePostID:    postID,
		TargetPlatforms: []string{"twitter"},
		Strategy:        "cross_platform",
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	err = svc.ProcessRepurpose(ctx, *job.AIJobID)
	if err != nil {
		t.Fatalf("ProcessRepurpose: %v", err)
	}

	outputs, err := svc.GetOutputs(ctx, userID, job.ID)
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}

	// Accept once
	_, err = svc.AcceptOutput(ctx, userID, outputs[0].ID)
	if err != nil {
		t.Fatalf("first AcceptOutput: %v", err)
	}

	// Accept again -- should fail
	_, err = svc.AcceptOutput(ctx, userID, outputs[0].ID)
	if err == nil {
		t.Fatal("expected error for duplicate accept")
	}
	if !repurposeContains(err.Error(), "already accepted") {
		t.Errorf("expected 'already accepted' error, got: %v", err)
	}
}
