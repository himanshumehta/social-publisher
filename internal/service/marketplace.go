package service

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// platformFeePercent is the percentage charged by the platform on each assignment.
const platformFeePercent = 15

type MarketplaceService struct {
	pool *pgxpool.Pool
}

func NewMarketplaceService(pool *pgxpool.Pool) *MarketplaceService {
	return &MarketplaceService{pool: pool}
}

// ── Request DTOs ────────────────────────────────────────────────────────

type CreateEditingJobRequest struct {
	Title      string  `json:"title"`
	Brief      string  `json:"brief"`
	Complexity string  `json:"complexity"`
	BudgetMin  int     `json:"budget_min"`
	BudgetMax  int     `json:"budget_max"`
	Deadline   *string `json:"deadline,omitempty"` // RFC3339
}

type UpdateEditingJobRequest struct {
	Title      *string `json:"title,omitempty"`
	Brief      *string `json:"brief,omitempty"`
	Complexity *string `json:"complexity,omitempty"`
	BudgetMin  *int    `json:"budget_min,omitempty"`
	BudgetMax  *int    `json:"budget_max,omitempty"`
	Deadline   *string `json:"deadline,omitempty"`
}

type ApplyForJobRequest struct {
	ProposedPrice  int      `json:"proposed_price"`
	CoverNote      string   `json:"cover_note"`
	PortfolioLinks []string `json:"portfolio_links"`
}

type DeliverWorkRequest struct {
	Notes string `json:"notes"`
}

type RequestChangesRequest struct {
	Notes string `json:"notes"`
}

type RateUserRequest struct {
	RatedUser uuid.UUID `json:"rated_user"`
	Rating    int       `json:"rating"`
	Review    string    `json:"review"`
}

// ── Helpers ─────────────────────────────────────────────────────────────

var validComplexities = map[string]bool{
	"simple":   true,
	"standard": true,
	"complex":  true,
}

func (s *MarketplaceService) scanJob(row pgx.Row) (*model.EditingJob, error) {
	var j model.EditingJob
	err := row.Scan(
		&j.ID, &j.ProjectID, &j.CreatorID, &j.Title, &j.Brief,
		&j.Complexity, &j.BudgetMin, &j.BudgetMax, &j.SuggestedPrice,
		&j.Deadline, &j.Status, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

const jobColumns = `id, project_id, creator_id, title, brief, complexity,
	budget_min, budget_max, suggested_price, deadline, status, created_at, updated_at`

func (s *MarketplaceService) scanApplication(row pgx.Row) (*model.JobApplication, error) {
	var a model.JobApplication
	err := row.Scan(
		&a.ID, &a.JobID, &a.EditorID, &a.ProposedPrice,
		&a.CoverNote, &a.PortfolioLinks, &a.Status,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const appColumns = `id, job_id, editor_id, proposed_price, cover_note, portfolio_links,
	status, created_at, updated_at`

func (s *MarketplaceService) scanAssignment(row pgx.Row) (*model.EditingAssignment, error) {
	var a model.EditingAssignment
	err := row.Scan(
		&a.ID, &a.JobID, &a.ApplicationID, &a.EditorID, &a.CreatorID,
		&a.AgreedPrice, &a.PlatformFee, &a.TotalCharged, &a.Status,
		&a.DeliveryNotes, &a.RevisionNotes, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

const assignmentColumns = `id, job_id, application_id, editor_id, creator_id,
	agreed_price, platform_fee, total_charged, status,
	delivery_notes, revision_notes, created_at, updated_at`

// ── Creator Methods ─────────────────────────────────────────────────────

func (s *MarketplaceService) CreateJob(ctx context.Context, creatorID, projectID uuid.UUID, req CreateEditingJobRequest) (*model.EditingJob, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	complexity := req.Complexity
	if complexity == "" {
		complexity = "standard"
	}
	if !validComplexities[complexity] {
		return nil, fmt.Errorf("invalid complexity: %s", complexity)
	}

	// Verify project exists and belongs to creator
	var ownerID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT creator_id FROM projects WHERE id = $1`, projectID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("check project: %w", err)
	}
	if ownerID != creatorID {
		return nil, fmt.Errorf("not authorised to post jobs for this project")
	}

	// Look up suggested price from config
	var suggestedPrice int
	_ = s.pool.QueryRow(ctx,
		`SELECT suggested_price FROM editing_price_config
		 WHERE complexity = $1 ORDER BY suggested_price LIMIT 1`, complexity,
	).Scan(&suggestedPrice)

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO editing_jobs
			(project_id, creator_id, title, brief, complexity, budget_min, budget_max, suggested_price, deadline)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING %s`, jobColumns),
		projectID, creatorID, req.Title, req.Brief, complexity,
		req.BudgetMin, req.BudgetMax, suggestedPrice, parseOptionalTime(req.Deadline),
	)
	return s.scanJob(row)
}

func (s *MarketplaceService) GetJob(ctx context.Context, jobID uuid.UUID) (*model.EditingJob, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM editing_jobs WHERE id = $1`, jobColumns), jobID,
	)
	j, err := s.scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("editing job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get editing job: %w", err)
	}
	return j, nil
}

func (s *MarketplaceService) GetJobByProject(ctx context.Context, projectID uuid.UUID) (*model.EditingJob, error) {
	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM editing_jobs
			WHERE project_id = $1 AND status != 'cancelled'
			ORDER BY created_at DESC LIMIT 1`, jobColumns), projectID,
	)
	j, err := s.scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("no editing job found for this project")
	}
	if err != nil {
		return nil, fmt.Errorf("get editing job by project: %w", err)
	}
	return j, nil
}

func (s *MarketplaceService) UpdateJob(ctx context.Context, jobID, creatorID uuid.UUID, req UpdateEditingJobRequest) (*model.EditingJob, error) {
	// Verify ownership and open status
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.CreatorID != creatorID {
		return nil, fmt.Errorf("not authorised to update this job")
	}
	if job.Status != model.EditingJobOpen {
		return nil, fmt.Errorf("can only update open jobs")
	}

	if req.Title != nil {
		job.Title = *req.Title
	}
	if req.Brief != nil {
		job.Brief = *req.Brief
	}
	if req.Complexity != nil {
		if !validComplexities[*req.Complexity] {
			return nil, fmt.Errorf("invalid complexity: %s", *req.Complexity)
		}
		job.Complexity = model.EditingJobComplexity(*req.Complexity)
	}
	if req.BudgetMin != nil {
		job.BudgetMin = *req.BudgetMin
	}
	if req.BudgetMax != nil {
		job.BudgetMax = *req.BudgetMax
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`UPDATE editing_jobs SET
			title = $1, brief = $2, complexity = $3, budget_min = $4, budget_max = $5,
			deadline = $6, updated_at = NOW()
			WHERE id = $7
			RETURNING %s`, jobColumns),
		job.Title, job.Brief, job.Complexity, job.BudgetMin, job.BudgetMax,
		parseOptionalTime(req.Deadline), jobID,
	)
	return s.scanJob(row)
}

func (s *MarketplaceService) CancelJob(ctx context.Context, jobID, creatorID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE editing_jobs SET status = 'cancelled', updated_at = NOW()
		 WHERE id = $1 AND creator_id = $2 AND status = 'open'`, jobID, creatorID)
	if err != nil {
		return fmt.Errorf("cancel job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("job not found, not owned by you, or not open")
	}
	return nil
}

func (s *MarketplaceService) ListApplications(ctx context.Context, jobID, creatorID uuid.UUID) ([]model.JobApplication, error) {
	// Verify creator owns the job
	var ownerID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT creator_id FROM editing_jobs WHERE id = $1`, jobID,
	).Scan(&ownerID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("check job: %w", err)
	}
	if ownerID != creatorID {
		return nil, fmt.Errorf("not authorised to view applications for this job")
	}

	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM job_applications WHERE job_id = $1
			ORDER BY created_at DESC`, appColumns), jobID,
	)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	results := []model.JobApplication{}
	for rows.Next() {
		a, err := s.scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		results = append(results, *a)
	}
	return results, nil
}

func (s *MarketplaceService) AcceptApplication(ctx context.Context, applicationID, creatorID uuid.UUID) (*model.EditingAssignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Fetch application
	var app model.JobApplication
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM job_applications WHERE id = $1`, appColumns),
		applicationID,
	).Scan(
		&app.ID, &app.JobID, &app.EditorID, &app.ProposedPrice,
		&app.CoverNote, &app.PortfolioLinks, &app.Status,
		&app.CreatedAt, &app.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("application not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	if app.Status != model.ApplicationPending && app.Status != model.ApplicationShortlisted {
		return nil, fmt.Errorf("application is not in a pending or shortlisted state")
	}

	// Verify creator owns the job
	var jobCreatorID uuid.UUID
	var jobStatus model.EditingJobStatus
	err = tx.QueryRow(ctx,
		`SELECT creator_id, status FROM editing_jobs WHERE id = $1`, app.JobID,
	).Scan(&jobCreatorID, &jobStatus)
	if err != nil {
		return nil, fmt.Errorf("get job: %w", err)
	}
	if jobCreatorID != creatorID {
		return nil, fmt.Errorf("not authorised to accept applications for this job")
	}
	if jobStatus != model.EditingJobOpen {
		return nil, fmt.Errorf("job is not open")
	}

	// Calculate fees
	agreedPrice := app.ProposedPrice
	platformFee := int(math.Round(float64(agreedPrice) * float64(platformFeePercent) / 100.0))
	totalCharged := agreedPrice + platformFee

	// Mark application accepted
	_, err = tx.Exec(ctx,
		`UPDATE job_applications SET status = 'accepted', updated_at = NOW() WHERE id = $1`,
		applicationID)
	if err != nil {
		return nil, fmt.Errorf("accept application: %w", err)
	}

	// Reject all other pending applications
	_, err = tx.Exec(ctx,
		`UPDATE job_applications SET status = 'rejected', updated_at = NOW()
		 WHERE job_id = $1 AND id != $2 AND status IN ('pending', 'shortlisted')`,
		app.JobID, applicationID)
	if err != nil {
		return nil, fmt.Errorf("reject other applications: %w", err)
	}

	// Mark job as in_progress
	_, err = tx.Exec(ctx,
		`UPDATE editing_jobs SET status = 'in_progress', updated_at = NOW() WHERE id = $1`,
		app.JobID)
	if err != nil {
		return nil, fmt.Errorf("update job status: %w", err)
	}

	// Create assignment
	var assignment model.EditingAssignment
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO editing_assignments
			(job_id, application_id, editor_id, creator_id, agreed_price, platform_fee, total_charged)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING %s`, assignmentColumns),
		app.JobID, applicationID, app.EditorID, creatorID,
		agreedPrice, platformFee, totalCharged,
	).Scan(
		&assignment.ID, &assignment.JobID, &assignment.ApplicationID,
		&assignment.EditorID, &assignment.CreatorID,
		&assignment.AgreedPrice, &assignment.PlatformFee, &assignment.TotalCharged,
		&assignment.Status, &assignment.DeliveryNotes, &assignment.RevisionNotes,
		&assignment.CreatedAt, &assignment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create assignment: %w", err)
	}

	// Create pending marketplace transaction (escrow hold)
	_, err = tx.Exec(ctx,
		`INSERT INTO marketplace_transactions
			(assignment_id, creator_id, editor_id, amount, editor_payout, platform_fee, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'held')`,
		assignment.ID, creatorID, app.EditorID,
		totalCharged, agreedPrice, platformFee,
	)
	if err != nil {
		return nil, fmt.Errorf("create transaction: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return &assignment, nil
}

func (s *MarketplaceService) RejectApplication(ctx context.Context, applicationID, creatorID uuid.UUID) error {
	// Fetch application to verify job ownership
	var jobID uuid.UUID
	var appStatus model.ApplicationStatus
	err := s.pool.QueryRow(ctx,
		`SELECT job_id, status FROM job_applications WHERE id = $1`, applicationID,
	).Scan(&jobID, &appStatus)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("application not found")
	}
	if err != nil {
		return fmt.Errorf("get application: %w", err)
	}
	if appStatus != model.ApplicationPending && appStatus != model.ApplicationShortlisted {
		return fmt.Errorf("application cannot be rejected in current state")
	}

	var jobCreatorID uuid.UUID
	err = s.pool.QueryRow(ctx,
		`SELECT creator_id FROM editing_jobs WHERE id = $1`, jobID,
	).Scan(&jobCreatorID)
	if err != nil {
		return fmt.Errorf("get job: %w", err)
	}
	if jobCreatorID != creatorID {
		return fmt.Errorf("not authorised to reject this application")
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE job_applications SET status = 'rejected', updated_at = NOW() WHERE id = $1`,
		applicationID)
	if err != nil {
		return fmt.Errorf("reject application: %w", err)
	}
	return nil
}

func (s *MarketplaceService) AcceptDelivery(ctx context.Context, assignmentID, creatorID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var a model.EditingAssignment
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM editing_assignments WHERE id = $1`, assignmentColumns),
		assignmentID,
	).Scan(
		&a.ID, &a.JobID, &a.ApplicationID, &a.EditorID, &a.CreatorID,
		&a.AgreedPrice, &a.PlatformFee, &a.TotalCharged, &a.Status,
		&a.DeliveryNotes, &a.RevisionNotes, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("assignment not found")
	}
	if err != nil {
		return fmt.Errorf("get assignment: %w", err)
	}
	if a.CreatorID != creatorID {
		return fmt.Errorf("not authorised")
	}
	if a.Status != model.AssignmentInReview {
		return fmt.Errorf("assignment is not in review state")
	}

	// Mark assignment completed
	_, err = tx.Exec(ctx,
		`UPDATE editing_assignments SET status = 'completed', updated_at = NOW() WHERE id = $1`,
		assignmentID)
	if err != nil {
		return fmt.Errorf("complete assignment: %w", err)
	}

	// Mark job completed
	_, err = tx.Exec(ctx,
		`UPDATE editing_jobs SET status = 'completed', updated_at = NOW() WHERE id = $1`,
		a.JobID)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}

	// Release payment
	_, err = tx.Exec(ctx,
		`UPDATE marketplace_transactions SET status = 'released', updated_at = NOW()
		 WHERE assignment_id = $1 AND status = 'held'`, assignmentID)
	if err != nil {
		return fmt.Errorf("release payment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *MarketplaceService) RequestChanges(ctx context.Context, assignmentID, creatorID uuid.UUID, notes string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE editing_assignments SET status = 'in_progress', revision_notes = $1, updated_at = NOW()
		 WHERE id = $2 AND creator_id = $3 AND status = 'in_review'`,
		notes, assignmentID, creatorID)
	if err != nil {
		return fmt.Errorf("request changes: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("assignment not found, not owned by you, or not in review state")
	}
	return nil
}

// ── Editor Methods ──────────────────────────────────────────────────────

func (s *MarketplaceService) ListOpenJobs(ctx context.Context, limit, offset int) ([]model.EditingJob, int, error) {
	limit, offset = ClampPagination(limit, offset)

	var total int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM editing_jobs WHERE status = 'open'`,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count open jobs: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM editing_jobs WHERE status = 'open'
			ORDER BY created_at DESC LIMIT $1 OFFSET $2`, jobColumns),
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list open jobs: %w", err)
	}
	defer rows.Close()

	results := []model.EditingJob{}
	for rows.Next() {
		j, err := s.scanJob(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan job: %w", err)
		}
		results = append(results, *j)
	}
	return results, total, nil
}

func (s *MarketplaceService) Apply(ctx context.Context, jobID, editorID uuid.UUID, req ApplyForJobRequest) (*model.JobApplication, error) {
	// Verify job is open
	var jobStatus model.EditingJobStatus
	var jobCreatorID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT status, creator_id FROM editing_jobs WHERE id = $1`, jobID,
	).Scan(&jobStatus, &jobCreatorID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("check job: %w", err)
	}
	if jobStatus != model.EditingJobOpen {
		return nil, fmt.Errorf("job is not open for applications")
	}
	if jobCreatorID == editorID {
		return nil, fmt.Errorf("cannot apply to your own job")
	}

	if req.ProposedPrice <= 0 {
		return nil, fmt.Errorf("proposed price must be positive")
	}

	links := req.PortfolioLinks
	if links == nil {
		links = []string{}
	}

	row := s.pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO job_applications
			(job_id, editor_id, proposed_price, cover_note, portfolio_links)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING %s`, appColumns),
		jobID, editorID, req.ProposedPrice, req.CoverNote, links,
	)
	a, err := s.scanApplication(row)
	if err != nil {
		return nil, fmt.Errorf("create application: %w", err)
	}
	return a, nil
}

func (s *MarketplaceService) WithdrawApplication(ctx context.Context, applicationID, editorID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE job_applications SET status = 'withdrawn', updated_at = NOW()
		 WHERE id = $1 AND editor_id = $2 AND status IN ('pending', 'shortlisted')`,
		applicationID, editorID)
	if err != nil {
		return fmt.Errorf("withdraw application: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("application not found, not owned by you, or not withdrawable")
	}
	return nil
}

func (s *MarketplaceService) ListMyAssignments(ctx context.Context, editorID uuid.UUID) ([]model.EditingAssignment, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM editing_assignments WHERE editor_id = $1
			ORDER BY created_at DESC`, assignmentColumns), editorID,
	)
	if err != nil {
		return nil, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()

	results := []model.EditingAssignment{}
	for rows.Next() {
		a, err := s.scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		results = append(results, *a)
	}
	return results, nil
}

func (s *MarketplaceService) DeliverWork(ctx context.Context, assignmentID, editorID uuid.UUID, notes string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE editing_assignments
		 SET status = 'in_review', delivery_notes = $1, updated_at = NOW()
		 WHERE id = $2 AND editor_id = $3 AND status IN ('assigned', 'in_progress')`,
		notes, assignmentID, editorID)
	if err != nil {
		return fmt.Errorf("deliver work: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("assignment not found, not owned by you, or not in a deliverable state")
	}
	return nil
}

func (s *MarketplaceService) GetEditorStats(ctx context.Context, editorID uuid.UUID) (*model.EditorStats, error) {
	stats := &model.EditorStats{}

	// Completed assignments count
	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM editing_assignments
		 WHERE editor_id = $1 AND status = 'completed'`, editorID,
	).Scan(&stats.Completed)

	// Average rating
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating)::float8, 0) FROM editor_ratings
		 WHERE rated_user = $1`, editorID,
	).Scan(&stats.AvgRating)

	// Total earned
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(editor_payout), 0) FROM marketplace_transactions
		 WHERE editor_id = $1 AND status = 'released'`, editorID,
	).Scan(&stats.TotalEarned)

	return stats, nil
}

// ── Shared Methods ──────────────────────────────────────────────────────

func (s *MarketplaceService) RateUser(ctx context.Context, assignmentID, ratedBy, ratedUser uuid.UUID, rating int, review string) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	// Verify assignment exists and is completed
	var aStatus model.AssignmentStatus
	var aEditorID, aCreatorID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT status, editor_id, creator_id FROM editing_assignments WHERE id = $1`,
		assignmentID,
	).Scan(&aStatus, &aEditorID, &aCreatorID)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("assignment not found")
	}
	if err != nil {
		return fmt.Errorf("get assignment: %w", err)
	}
	if aStatus != model.AssignmentCompleted {
		return fmt.Errorf("can only rate completed assignments")
	}

	// Verify rater is part of the assignment
	if ratedBy != aEditorID && ratedBy != aCreatorID {
		return fmt.Errorf("you are not part of this assignment")
	}
	// Verify rated user is the other party
	if ratedUser != aEditorID && ratedUser != aCreatorID {
		return fmt.Errorf("rated user is not part of this assignment")
	}
	if ratedBy == ratedUser {
		return fmt.Errorf("cannot rate yourself")
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO editor_ratings (assignment_id, rated_by, rated_user, rating, review_text)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (assignment_id, rated_by) DO UPDATE
		 SET rating = EXCLUDED.rating, review_text = EXCLUDED.review_text`,
		assignmentID, ratedBy, ratedUser, rating, review,
	)
	if err != nil {
		return fmt.Errorf("save rating: %w", err)
	}
	return nil
}

func (s *MarketplaceService) GetPriceConfig(ctx context.Context) ([]model.EditingPriceConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, duration_bracket, complexity, suggested_price
		 FROM editing_price_config ORDER BY duration_bracket, complexity`)
	if err != nil {
		return nil, fmt.Errorf("list price config: %w", err)
	}
	defer rows.Close()

	results := []model.EditingPriceConfig{}
	for rows.Next() {
		var c model.EditingPriceConfig
		if err := rows.Scan(&c.ID, &c.DurationBracket, &c.Complexity, &c.SuggestedPrice); err != nil {
			return nil, fmt.Errorf("scan price config: %w", err)
		}
		results = append(results, c)
	}
	return results, nil
}

// ── Internal helpers ────────────────────────────────────────────────────

func parseOptionalTime(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s // let postgres parse the RFC3339 string
}
