package service

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://socialpub:socialpub@localhost:5433/socialpub?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping: cannot connect to test database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Skipf("skipping: cannot ping test database: %v", err)
	}
	return pool
}

func TestCreateSelfProject_NullSubjectRefID(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	// Need a real user to satisfy FK constraint on created_by/creator_id.
	var userID uuid.UUID
	err := pool.QueryRow(context.Background(),
		"SELECT id FROM users LIMIT 1").Scan(&userID)
	if err != nil {
		t.Skipf("skipping: no users in test database: %v", err)
	}

	svc := NewProjectService(pool)
	project, err := svc.CreateSelfProject(context.Background(),
		userID,
		CreateSelfProjectRequest{Title: "Test Self Project NULL Fix"},
	)
	if err != nil {
		t.Fatalf("CreateSelfProject failed: %v", err)
	}

	if project.ID.String() == "" {
		t.Fatal("expected project to have an ID")
	}
	if project.Title != "Test Self Project NULL Fix" {
		t.Errorf("expected title 'Test Self Project NULL Fix', got %q", project.Title)
	}
	if project.SubjectRefID != "" {
		t.Errorf("expected empty subject_ref_id, got %q", project.SubjectRefID)
	}
	if project.Status != model.ProjectActive {
		t.Errorf("expected status 'active', got %q", project.Status)
	}

	// Verify GetByID also handles the NULL subject_ref_id
	fetched, err := svc.GetByID(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.SubjectRefID != "" {
		t.Errorf("GetByID: expected empty subject_ref_id, got %q", fetched.SubjectRefID)
	}

	// Cleanup
	_, _ = pool.Exec(context.Background(),
		"DELETE FROM projects WHERE id = $1", project.ID)
}
