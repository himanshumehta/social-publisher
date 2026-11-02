package service

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestValidJobTypes_AllPresent(t *testing.T) {
	expected := []string{"auto_clip", "auto_subtitle", "ai_brief"}

	for _, jt := range expected {
		if !validJobTypes[jt] {
			t.Errorf("validJobTypes[%q] = false, want true", jt)
		}
	}
}

func TestValidJobTypes_RejectsUnknown(t *testing.T) {
	unknowns := []string{"unknown_type", "manual_clip", "", "AUTO_CLIP"}

	for _, jt := range unknowns {
		if validJobTypes[jt] {
			t.Errorf("validJobTypes[%q] = true, want false (should reject unknown type)", jt)
		}
	}
}

func TestValidJobTypes_Count(t *testing.T) {
	if got := len(validJobTypes); got != 3 {
		t.Errorf("len(validJobTypes) = %d, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require a live Postgres database)
// ---------------------------------------------------------------------------

func TestAIJobCreate(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-create-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	job, err := svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType:  "auto_clip",
		Provider: "openai",
		Config:   map[string]interface{}{"max_clips": 5},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if job.ID == uuid.Nil {
		t.Error("expected non-nil job ID")
	}
	if job.ProjectID != projectID {
		t.Errorf("project_id = %s, want %s", job.ProjectID, projectID)
	}
	if job.CreatedBy != userID {
		t.Errorf("created_by = %s, want %s", job.CreatedBy, userID)
	}
	if job.JobType != model.AIJobAutoClip {
		t.Errorf("job_type = %q, want %q", job.JobType, model.AIJobAutoClip)
	}
	if job.Provider != "openai" {
		t.Errorf("provider = %q, want %q", job.Provider, "openai")
	}
	if job.Status != model.AIJobPending {
		t.Errorf("status = %q, want %q", job.Status, model.AIJobPending)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(job.Config, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if v, ok := cfg["max_clips"]; !ok || v != float64(5) {
		t.Errorf("config[max_clips] = %v, want 5", v)
	}
}

func TestAIJobGetByID(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-get-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	created, err := svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType:  "ai_brief",
		Provider: "anthropic",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("ID = %s, want %s", got.ID, created.ID)
	}
	if got.JobType != model.AIJobAIBrief {
		t.Errorf("job_type = %q, want %q", got.JobType, model.AIJobAIBrief)
	}
	if got.Provider != "anthropic" {
		t.Errorf("provider = %q, want %q", got.Provider, "anthropic")
	}
}

func TestAIJobListByProject(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-list-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)

	_, err = svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "auto_clip", Provider: "openai",
	})
	if err != nil {
		t.Fatalf("Create job 1: %v", err)
	}

	_, err = svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "auto_subtitle", Provider: "openai",
	})
	if err != nil {
		t.Fatalf("Create job 2: %v", err)
	}

	jobs, err := svc.ListByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	for _, j := range jobs {
		if j.ProjectID != projectID {
			t.Errorf("job %s: project_id = %s, want %s", j.ID, j.ProjectID, projectID)
		}
	}
}

func TestAIJobPickPending(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-pick-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	_, err = svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "auto_clip", Provider: "internal",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	picked, err := svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending: %v", err)
	}
	if picked == nil {
		t.Fatal("PickPending returned nil, expected a job")
	}
	// PickPending picks the oldest pending job — which may not be ours if other
	// pending jobs exist in the database. Just verify it returned a valid job.
	if picked.Status != model.AIJobProcessing {
		t.Errorf("status = %q, want %q", picked.Status, model.AIJobProcessing)
	}
	if picked.ProcessingStartedAt == nil {
		t.Error("processing_started_at should be set")
	}
}

func TestAIJobComplete(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-complete-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	created, err := svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "ai_brief", Provider: "anthropic",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Move to processing first
	_, err = svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending: %v", err)
	}

	resultPayload := []byte(`{"summary":"test brief result"}`)
	err = svc.Complete(ctx, created.ID, resultPayload, 42)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	job, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob after Complete: %v", err)
	}
	if job.Status != model.AIJobCompleted {
		t.Errorf("status = %q, want %q", job.Status, model.AIJobCompleted)
	}
	if job.CreditsUsed != 42 {
		t.Errorf("credits_used = %d, want 42", job.CreditsUsed)
	}
	if job.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
	// PostgreSQL jsonb normalizes whitespace, so compare compacted JSON.
	var compactResult, compactExpected bytes.Buffer
	json.Compact(&compactResult, job.Result)
	json.Compact(&compactExpected, resultPayload)
	if compactResult.String() != compactExpected.String() {
		t.Errorf("result = %q, want %q", compactResult.String(), compactExpected.String())
	}
}

func TestAIJobFail(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-fail-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	created, err := svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "auto_subtitle", Provider: "openai",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Move to processing first
	_, err = svc.PickPending(ctx)
	if err != nil {
		t.Fatalf("PickPending: %v", err)
	}

	errMsg := "provider timeout after 30s"
	err = svc.Fail(ctx, created.ID, errMsg)
	if err != nil {
		t.Fatalf("Fail: %v", err)
	}

	job, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob after Fail: %v", err)
	}
	if job.Status != model.AIJobFailed {
		t.Errorf("status = %q, want %q", job.Status, model.AIJobFailed)
	}
	if job.ErrorMessage == nil || *job.ErrorMessage != errMsg {
		t.Errorf("error_message = %v, want %q", job.ErrorMessage, errMsg)
	}
	if job.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
}

func TestAIJobCancel(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()

	ctx := context.Background()
	userID := uuid.New()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, $4)`,
		userID, "aijob-test-user", "aijob-cancel-"+userID.String()+"@test.local", "creator")
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	projectID := createTestProject(t, pool, userID, "active")

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM ai_jobs WHERE project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	svc := NewAIJobService(pool)
	created, err := svc.Create(ctx, projectID, userID, CreateAIJobRequest{
		JobType: "auto_clip", Provider: "internal",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.Cancel(ctx, created.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	job, err := svc.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetJob after Cancel: %v", err)
	}
	if job.Status != model.AIJobFailed {
		t.Errorf("status = %q, want %q", job.Status, model.AIJobFailed)
	}
	if job.ErrorMessage == nil || *job.ErrorMessage != "cancelled by user" {
		t.Errorf("error_message = %v, want %q", job.ErrorMessage, "cancelled by user")
	}
	if job.CompletedAt == nil {
		t.Error("completed_at should be set")
	}
}
