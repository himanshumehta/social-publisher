package service

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/himanshumehta/social-publisher/internal/model"
)

func TestValidComplexities_AllPresent(t *testing.T) {
	expected := []string{"simple", "standard", "complex"}

	for _, c := range expected {
		if !validComplexities[c] {
			t.Errorf("validComplexities[%q] = false, want true", c)
		}
	}

	if len(validComplexities) != len(expected) {
		t.Errorf("len(validComplexities) = %d, want %d", len(validComplexities), len(expected))
	}
}

func TestValidComplexities_RejectsInvalid(t *testing.T) {
	invalids := []string{"extreme", "easy", "", "SIMPLE"}

	for _, c := range invalids {
		if validComplexities[c] {
			t.Errorf("validComplexities[%q] = true, want false (should reject invalid complexity)", c)
		}
	}
}

func TestPlatformFeePercent(t *testing.T) {
	if platformFeePercent != 15 {
		t.Errorf("platformFeePercent = %d, want 15", platformFeePercent)
	}
}

// ── Integration test helpers ────────────────────────────────────────────

type cleanupState struct {
	ctx       context.Context
	creatorID uuid.UUID
	editorID  uuid.UUID
	projectID uuid.UUID
}

// setupMarketplaceTest creates a creator user, an editor user, and a project,
// returning their IDs plus a cleanup function.
func setupMarketplaceTest(t *testing.T) (pool poolCloser, svc *MarketplaceService, st cleanupState, cleanup func()) {
	t.Helper()

	rawPool := setupTestPool(t)
	svc = NewMarketplaceService(rawPool)

	ctx := context.Background()
	creatorID := uuid.New()
	editorID := uuid.New()

	// Insert creator user
	_, err := rawPool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'creator')`,
		creatorID, "Test Creator", "creator-"+creatorID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert creator user: %v", err)
	}

	// Insert editor user
	_, err = rawPool.Exec(ctx,
		`INSERT INTO users (id, name, email, role) VALUES ($1, $2, $3, 'editor')`,
		editorID, "Test Editor", "editor-"+editorID.String()[:8]+"@test.io")
	if err != nil {
		t.Fatalf("insert editor user: %v", err)
	}

	// Insert project owned by creator (using helper that includes all required columns)
	projectID := createTestProject(t, rawPool, creatorID, "active")

	st = cleanupState{
		ctx:       ctx,
		creatorID: creatorID,
		editorID:  editorID,
		projectID: projectID,
	}

	cleanup = func() {
		// Delete in FK order
		rawPool.Exec(ctx, `DELETE FROM marketplace_transactions WHERE creator_id = $1 OR editor_id = $1`, creatorID)
		rawPool.Exec(ctx, `DELETE FROM marketplace_transactions WHERE creator_id = $1 OR editor_id = $1`, editorID)
		rawPool.Exec(ctx, `DELETE FROM editor_ratings WHERE rated_by = $1 OR rated_user = $1`, creatorID)
		rawPool.Exec(ctx, `DELETE FROM editor_ratings WHERE rated_by = $1 OR rated_user = $1`, editorID)
		rawPool.Exec(ctx, `DELETE FROM editing_assignments WHERE creator_id = $1`, creatorID)
		rawPool.Exec(ctx, `DELETE FROM job_applications WHERE editor_id = $1`, editorID)
		rawPool.Exec(ctx, `DELETE FROM editing_jobs WHERE creator_id = $1`, creatorID)
		rawPool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		rawPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, creatorID)
		rawPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, editorID)
		rawPool.Close()
	}

	return rawPool, svc, st, cleanup
}

// poolCloser is satisfied by *pgxpool.Pool.
type poolCloser interface {
	Close()
}

// ── Integration Tests ───────────────────────────────────────────────────

func TestMarketplaceCreateJob(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	job, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Color Grade My Short",
		Brief:      "Cinematic look, warm tones",
		Complexity: "standard",
		BudgetMin:  100,
		BudgetMax:  500,
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	if job.ID == uuid.Nil {
		t.Error("expected job ID to be set")
	}
	if job.Title != "Color Grade My Short" {
		t.Errorf("title = %q, want %q", job.Title, "Color Grade My Short")
	}
	if job.Brief != "Cinematic look, warm tones" {
		t.Errorf("brief = %q, want %q", job.Brief, "Cinematic look, warm tones")
	}
	if job.Complexity != model.ComplexityStandard {
		t.Errorf("complexity = %q, want %q", job.Complexity, model.ComplexityStandard)
	}
	if job.BudgetMin != 100 {
		t.Errorf("budget_min = %d, want 100", job.BudgetMin)
	}
	if job.BudgetMax != 500 {
		t.Errorf("budget_max = %d, want 500", job.BudgetMax)
	}
	if job.Status != model.EditingJobOpen {
		t.Errorf("status = %q, want %q", job.Status, model.EditingJobOpen)
	}
	if job.CreatorID != st.creatorID {
		t.Errorf("creator_id = %s, want %s", job.CreatorID, st.creatorID)
	}
	if job.ProjectID != st.projectID {
		t.Errorf("project_id = %s, want %s", job.ProjectID, st.projectID)
	}

	// Verify GetJob returns the same record
	fetched, err := svc.GetJob(st.ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if fetched.ID != job.ID {
		t.Errorf("GetJob returned id = %s, want %s", fetched.ID, job.ID)
	}
}

func TestMarketplaceCreateJob_InvalidComplexity(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	_, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Bad Complexity Job",
		Complexity: "extreme",
		BudgetMin:  50,
		BudgetMax:  200,
	})
	if err == nil {
		t.Fatal("expected error for invalid complexity 'extreme', got nil")
	}
	if got := err.Error(); got != "invalid complexity: extreme" {
		t.Errorf("error = %q, want %q", got, "invalid complexity: extreme")
	}
}

func TestMarketplaceApplyForJob(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	// Create a job first
	job, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Thumbnail Design",
		Complexity: "simple",
		BudgetMin:  20,
		BudgetMax:  80,
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Editor applies
	app, err := svc.Apply(st.ctx, job.ID, st.editorID, ApplyForJobRequest{
		ProposedPrice:  50,
		CoverNote:      "I have 5 years of thumbnail design experience.",
		PortfolioLinks: []string{"https://example.com/portfolio"},
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if app.ID == uuid.Nil {
		t.Error("expected application ID to be set")
	}
	if app.JobID != job.ID {
		t.Errorf("job_id = %s, want %s", app.JobID, job.ID)
	}
	if app.EditorID != st.editorID {
		t.Errorf("editor_id = %s, want %s", app.EditorID, st.editorID)
	}
	if app.ProposedPrice != 50 {
		t.Errorf("proposed_price = %d, want 50", app.ProposedPrice)
	}
	if app.CoverNote != "I have 5 years of thumbnail design experience." {
		t.Errorf("cover_note = %q, want %q", app.CoverNote, "I have 5 years of thumbnail design experience.")
	}
	if len(app.PortfolioLinks) != 1 || app.PortfolioLinks[0] != "https://example.com/portfolio" {
		t.Errorf("portfolio_links = %v, want [https://example.com/portfolio]", app.PortfolioLinks)
	}
	if app.Status != model.ApplicationPending {
		t.Errorf("status = %q, want %q", app.Status, model.ApplicationPending)
	}
}

func TestMarketplaceAcceptApplication(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	// Create job
	job, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Full Edit Package",
		Complexity: "complex",
		BudgetMin:  200,
		BudgetMax:  1000,
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Editor applies
	app, err := svc.Apply(st.ctx, job.ID, st.editorID, ApplyForJobRequest{
		ProposedPrice:  400,
		CoverNote:      "Experienced editor, fast turnaround.",
		PortfolioLinks: []string{},
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Creator accepts
	assignment, err := svc.AcceptApplication(st.ctx, app.ID, st.creatorID)
	if err != nil {
		t.Fatalf("AcceptApplication failed: %v", err)
	}

	if assignment.ID == uuid.Nil {
		t.Error("expected assignment ID to be set")
	}
	if assignment.JobID != job.ID {
		t.Errorf("job_id = %s, want %s", assignment.JobID, job.ID)
	}
	if assignment.ApplicationID != app.ID {
		t.Errorf("application_id = %s, want %s", assignment.ApplicationID, app.ID)
	}
	if assignment.EditorID != st.editorID {
		t.Errorf("editor_id = %s, want %s", assignment.EditorID, st.editorID)
	}
	if assignment.CreatorID != st.creatorID {
		t.Errorf("creator_id = %s, want %s", assignment.CreatorID, st.creatorID)
	}

	// Verify fee calculation: 15% of 400 = 60
	expectedFee := int(math.Round(float64(400) * float64(platformFeePercent) / 100.0))
	expectedTotal := 400 + expectedFee

	if assignment.AgreedPrice != 400 {
		t.Errorf("agreed_price = %d, want 400", assignment.AgreedPrice)
	}
	if assignment.PlatformFee != expectedFee {
		t.Errorf("platform_fee = %d, want %d", assignment.PlatformFee, expectedFee)
	}
	if assignment.TotalCharged != expectedTotal {
		t.Errorf("total_charged = %d, want %d", assignment.TotalCharged, expectedTotal)
	}
	if assignment.Status != model.AssignmentAssigned {
		t.Errorf("status = %q, want %q", assignment.Status, model.AssignmentAssigned)
	}
}

func TestMarketplaceDeliverWork(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	// Create job
	job, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Podcast Edit",
		Complexity: "standard",
		BudgetMin:  80,
		BudgetMax:  300,
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Editor applies
	app, err := svc.Apply(st.ctx, job.ID, st.editorID, ApplyForJobRequest{
		ProposedPrice:  150,
		CoverNote:      "Audio specialist.",
		PortfolioLinks: []string{},
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Creator accepts
	assignment, err := svc.AcceptApplication(st.ctx, app.ID, st.creatorID)
	if err != nil {
		t.Fatalf("AcceptApplication failed: %v", err)
	}

	// Editor delivers work
	err = svc.DeliverWork(st.ctx, assignment.ID, st.editorID, "Final mix attached. Please review.")
	if err != nil {
		t.Fatalf("DeliverWork failed: %v", err)
	}

	// Re-fetch assignment from the database to verify status
	rawPool := setupTestPool(t)
	defer rawPool.Close()

	var status model.AssignmentStatus
	var deliveryNotes string
	err = rawPool.QueryRow(st.ctx,
		`SELECT status, delivery_notes FROM editing_assignments WHERE id = $1`,
		assignment.ID,
	).Scan(&status, &deliveryNotes)
	if err != nil {
		t.Fatalf("query assignment after delivery: %v", err)
	}
	if status != model.AssignmentInReview {
		t.Errorf("status = %q, want %q", status, model.AssignmentInReview)
	}
	if deliveryNotes != "Final mix attached. Please review." {
		t.Errorf("delivery_notes = %q, want %q", deliveryNotes, "Final mix attached. Please review.")
	}
}

func TestMarketplaceAcceptDelivery(t *testing.T) {
	_, svc, st, cleanup := setupMarketplaceTest(t)
	defer cleanup()

	// Create job
	job, err := svc.CreateJob(st.ctx, st.creatorID, st.projectID, CreateEditingJobRequest{
		Title:      "Vlog Edit",
		Complexity: "simple",
		BudgetMin:  30,
		BudgetMax:  100,
	})
	if err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Editor applies
	app, err := svc.Apply(st.ctx, job.ID, st.editorID, ApplyForJobRequest{
		ProposedPrice:  75,
		CoverNote:      "Quick turnaround guaranteed.",
		PortfolioLinks: []string{"https://example.com/work"},
	})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Creator accepts application
	assignment, err := svc.AcceptApplication(st.ctx, app.ID, st.creatorID)
	if err != nil {
		t.Fatalf("AcceptApplication failed: %v", err)
	}

	// Editor delivers
	err = svc.DeliverWork(st.ctx, assignment.ID, st.editorID, "All done, exported in 4K.")
	if err != nil {
		t.Fatalf("DeliverWork failed: %v", err)
	}

	// Creator accepts delivery
	err = svc.AcceptDelivery(st.ctx, assignment.ID, st.creatorID)
	if err != nil {
		t.Fatalf("AcceptDelivery failed: %v", err)
	}

	// Verify assignment is completed
	rawPool := setupTestPool(t)
	defer rawPool.Close()

	var assignmentStatus model.AssignmentStatus
	err = rawPool.QueryRow(st.ctx,
		`SELECT status FROM editing_assignments WHERE id = $1`, assignment.ID,
	).Scan(&assignmentStatus)
	if err != nil {
		t.Fatalf("query assignment after accept delivery: %v", err)
	}
	if assignmentStatus != model.AssignmentCompleted {
		t.Errorf("assignment status = %q, want %q", assignmentStatus, model.AssignmentCompleted)
	}

	// Verify job is completed
	var jobStatus model.EditingJobStatus
	err = rawPool.QueryRow(st.ctx,
		`SELECT status FROM editing_jobs WHERE id = $1`, job.ID,
	).Scan(&jobStatus)
	if err != nil {
		t.Fatalf("query job after accept delivery: %v", err)
	}
	if jobStatus != model.EditingJobCompleted {
		t.Errorf("job status = %q, want %q", jobStatus, model.EditingJobCompleted)
	}

	// Verify marketplace transaction was released
	var txnStatus model.MarketplaceTransactionStatus
	err = rawPool.QueryRow(st.ctx,
		`SELECT status FROM marketplace_transactions WHERE assignment_id = $1`, assignment.ID,
	).Scan(&txnStatus)
	if err != nil {
		t.Fatalf("query transaction after accept delivery: %v", err)
	}
	if txnStatus != model.MktTxnReleased {
		t.Errorf("transaction status = %q, want %q", txnStatus, model.MktTxnReleased)
	}
}
