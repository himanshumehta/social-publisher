package service

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestPostCreate_WithSourceURL(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-src-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post Source Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		SourceURL:       "https://example.com/article",
		Title:           "Test Source URL Post",
		TargetPlatforms: []string{"twitter", "linkedin"},
	}

	post, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create with source URL failed: %v", err)
	}

	if post.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", post.Status)
	}
	if post.SourceURL != "https://example.com/article" {
		t.Errorf("expected source_url 'https://example.com/article', got %q", post.SourceURL)
	}
	if post.AuthorID != userID {
		t.Errorf("expected author_id %s, got %s", userID, post.AuthorID)
	}
	if post.Title != "Test Source URL Post" {
		t.Errorf("expected title 'Test Source URL Post', got %q", post.Title)
	}
	if len(post.TargetPlatforms) != 2 {
		t.Errorf("expected 2 target platforms, got %d", len(post.TargetPlatforms))
	}
	if post.Tone != "professional" {
		t.Errorf("expected default tone 'professional', got %q", post.Tone)
	}
}

func TestPostCreate_WithoutSourceURL(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-nosrc-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post No-Source Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		Title:           "Manual Post Without Source",
		Content:         "Some hand-written content",
		TargetPlatforms: []string{"threads"},
	}

	post, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create without source URL failed: %v", err)
	}

	if post.Status != "creator_editing" {
		t.Errorf("expected status 'creator_editing', got %q", post.Status)
	}
	if post.SourceURL != "" {
		t.Errorf("expected empty source_url, got %q", post.SourceURL)
	}
	if post.Title != "Manual Post Without Source" {
		t.Errorf("expected title 'Manual Post Without Source', got %q", post.Title)
	}
}

func TestPostCreate_MissingPlatform(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-noplat-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post No-Platform Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		Title:           "Post With No Platforms",
		TargetPlatforms: []string{},
	}

	_, err = svc.Create(ctx, userID, req)
	if err == nil {
		t.Fatal("expected error when creating post without target platforms, got nil")
	}
}

func TestPostGetByID(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-get-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post GetByID Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		SourceURL:       "https://example.com/get-test",
		Title:           "GetByID Test Post",
		TargetPlatforms: []string{"facebook"},
		Tone:            "casual",
	}

	created, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Fetch as the author (non-admin)
	fetched, err := svc.GetByID(ctx, created.ID, userID, false)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fetched.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, fetched.ID)
	}
	if fetched.Title != "GetByID Test Post" {
		t.Errorf("expected title 'GetByID Test Post', got %q", fetched.Title)
	}
	if fetched.SourceURL != "https://example.com/get-test" {
		t.Errorf("expected source_url 'https://example.com/get-test', got %q", fetched.SourceURL)
	}
	if fetched.Tone != "casual" {
		t.Errorf("expected tone 'casual', got %q", fetched.Tone)
	}
	if fetched.AuthorID != userID {
		t.Errorf("expected author_id %s, got %s", userID, fetched.AuthorID)
	}
	if fetched.Status != "draft" {
		t.Errorf("expected status 'draft', got %q", fetched.Status)
	}

	// Fetch as a different user (non-admin) should fail
	otherUser := uuid.New()
	_, err = svc.GetByID(ctx, created.ID, otherUser, false)
	if err == nil {
		t.Error("expected error when non-author non-admin fetches post, got nil")
	}

	// Fetch as admin should succeed
	fetched2, err := svc.GetByID(ctx, created.ID, otherUser, true)
	if err != nil {
		t.Fatalf("GetByID as admin failed: %v", err)
	}
	if fetched2.ID != created.ID {
		t.Errorf("admin fetch: expected ID %s, got %s", created.ID, fetched2.ID)
	}
}

func TestPostList(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-list-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post List Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	// Create two posts
	req1 := &model.CreatePostRequest{
		SourceURL:       "https://example.com/list-1",
		Title:           "List Test Post One",
		TargetPlatforms: []string{"twitter"},
	}
	req2 := &model.CreatePostRequest{
		SourceURL:       "https://example.com/list-2",
		Title:           "List Test Post Two",
		TargetPlatforms: []string{"linkedin"},
	}

	_, err = svc.Create(ctx, userID, req1)
	if err != nil {
		t.Fatalf("Create post 1 failed: %v", err)
	}
	_, err = svc.Create(ctx, userID, req2)
	if err != nil {
		t.Fatalf("Create post 2 failed: %v", err)
	}

	// List as admin with author filter to isolate our test posts
	filter := &model.PostListFilter{
		AuthorID: userID.String(),
		Page:     1,
		PageSize: 10,
	}

	items, total, err := svc.List(ctx, filter, userID, true)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if total < 2 {
		t.Errorf("expected total >= 2, got %d", total)
	}
	if len(items) < 2 {
		t.Errorf("expected at least 2 items, got %d", len(items))
	}
}

func TestPostDelete_Draft(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-del-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post Delete Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		SourceURL:       "https://example.com/delete-test",
		Title:           "Delete Test Post",
		TargetPlatforms: []string{"twitter"},
	}

	post, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if post.Status != "draft" {
		t.Fatalf("expected draft status before delete, got %q", post.Status)
	}

	// Delete as the author
	err = svc.Delete(ctx, post.ID, userID, false)
	if err != nil {
		t.Fatalf("Delete of draft post failed: %v", err)
	}

	// Verify the post is gone
	_, err = svc.GetByID(ctx, post.ID, userID, true)
	if err == nil {
		t.Error("expected error when getting deleted post, got nil")
	}
}

func TestPostDelete_NonDeletableState(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-nodel-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post NonDel Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		SourceURL:       "https://example.com/nodel-test",
		Title:           "Non-Deletable State Post",
		TargetPlatforms: []string{"instagram"},
	}

	post, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Manually set status to "approved" which is not in the deletable set
	_, err = pool.Exec(ctx,
		`UPDATE posts SET status = 'approved'::post_status WHERE id = $1`, post.ID)
	if err != nil {
		t.Fatalf("failed to update status to approved: %v", err)
	}

	// Attempt to delete -- should fail
	err = svc.Delete(ctx, post.ID, userID, false)
	if err == nil {
		t.Fatal("expected error when deleting post in 'approved' state, got nil")
	}
}

func TestPostStatusTransitions(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()
	email := fmt.Sprintf("test-post-trans-%s@example.com", uuid.New().String()[:8])

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "Post Transition Test User", email, "creator",
	)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, userID)
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)

	svc := NewPostService(pool, nil, nil, nil, slog.Default())

	req := &model.CreatePostRequest{
		SourceURL:       "https://example.com/transition-test",
		Title:           "Transition Test Post",
		TargetPlatforms: []string{"linkedin"},
	}

	post, err := svc.Create(ctx, userID, req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if post.Status != "draft" {
		t.Fatalf("expected initial status 'draft', got %q", post.Status)
	}

	// Manually set to pending_review (simulating the pipeline completing)
	_, err = pool.Exec(ctx,
		`UPDATE posts SET status = 'pending_review'::post_status WHERE id = $1`, post.ID)
	if err != nil {
		t.Fatalf("failed to set status to pending_review: %v", err)
	}

	// AdminApprove should transition to approved
	err = svc.AdminApprove(ctx, post.ID)
	if err != nil {
		t.Fatalf("AdminApprove failed: %v", err)
	}

	approved, err := svc.GetByID(ctx, post.ID, userID, true)
	if err != nil {
		t.Fatalf("GetByID after approve failed: %v", err)
	}
	if approved.Status != "approved" {
		t.Errorf("expected status 'approved' after AdminApprove, got %q", approved.Status)
	}

	// AdminReject from approved state should work
	err = svc.AdminReject(ctx, post.ID, "not suitable for publication")
	if err != nil {
		t.Fatalf("AdminReject failed: %v", err)
	}

	rejected, err := svc.GetByID(ctx, post.ID, userID, true)
	if err != nil {
		t.Fatalf("GetByID after reject failed: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Errorf("expected status 'rejected' after AdminReject, got %q", rejected.Status)
	}
	if rejected.ErrorMessage != "not suitable for publication" {
		t.Errorf("expected error_message 'not suitable for publication', got %q", rejected.ErrorMessage)
	}
}
