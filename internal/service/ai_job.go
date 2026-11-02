package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type AIJobService struct {
	pool *pgxpool.Pool
}

func NewAIJobService(pool *pgxpool.Pool) *AIJobService {
	return &AIJobService{pool: pool}
}

type CreateAIJobRequest struct {
	VideoID  *uuid.UUID             `json:"video_id"`
	JobType  string                 `json:"job_type"`
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config"`
}

// validJobTypes lists the allowed job type values.
var validJobTypes = map[string]bool{
	"auto_clip":     true,
	"auto_subtitle": true,
	"ai_brief":      true,
}

func (s *AIJobService) Create(ctx context.Context, projectID, userID uuid.UUID, req CreateAIJobRequest) (*model.AIJob, error) {
	// Validate job type
	if !validJobTypes[req.JobType] {
		return nil, fmt.Errorf("invalid job type: %s", req.JobType)
	}

	// Validate project exists
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1)`, projectID,
	).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("project not found")
	}

	provider := req.Provider
	if provider == "" {
		provider = "internal"
	}

	configJSON := []byte("{}")
	if req.Config != nil {
		configJSON, err = json.Marshal(req.Config)
		if err != nil {
			return nil, fmt.Errorf("marshal config: %w", err)
		}
	}

	var job model.AIJob
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ai_jobs (project_id, video_id, created_by, job_type, provider, config)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, project_id, video_id, created_by, job_type, provider, config,
		    status, result, error_message, credits_used,
		    processing_started_at, completed_at, created_at, updated_at`,
		projectID, req.VideoID, userID, req.JobType, provider, configJSON,
	).Scan(&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.JobType, &job.Provider, &job.Config,
		&job.Status, &job.Result, &job.ErrorMessage, &job.CreditsUsed,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert ai job: %w", err)
	}
	return &job, nil
}

func (s *AIJobService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.AIJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, video_id, created_by, job_type, provider, config,
		        status, result, error_message, credits_used,
		        processing_started_at, completed_at, created_at, updated_at
		 FROM ai_jobs WHERE project_id = $1
		 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query ai jobs: %w", err)
	}
	defer rows.Close()

	jobs := []model.AIJob{}
	for rows.Next() {
		var j model.AIJob
		if err := rows.Scan(&j.ID, &j.ProjectID, &j.VideoID, &j.CreatedBy,
			&j.JobType, &j.Provider, &j.Config,
			&j.Status, &j.Result, &j.ErrorMessage, &j.CreditsUsed,
			&j.ProcessingStartedAt, &j.CompletedAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan ai job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (s *AIJobService) GetJob(ctx context.Context, jobID uuid.UUID) (*model.AIJob, error) {
	var job model.AIJob
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, video_id, created_by, job_type, provider, config,
		        status, result, error_message, credits_used,
		        processing_started_at, completed_at, created_at, updated_at
		 FROM ai_jobs WHERE id = $1`, jobID,
	).Scan(&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.JobType, &job.Provider, &job.Config,
		&job.Status, &job.Result, &job.ErrorMessage, &job.CreditsUsed,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("ai job not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get ai job: %w", err)
	}
	return &job, nil
}

func (s *AIJobService) PickPending(ctx context.Context) (*model.AIJob, error) {
	var job model.AIJob
	err := s.pool.QueryRow(ctx,
		`UPDATE ai_jobs SET status = 'processing', processing_started_at = NOW(), updated_at = NOW()
		 WHERE id = (
		     SELECT id FROM ai_jobs
		     WHERE status = 'pending'
		     ORDER BY created_at
		     LIMIT 1
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, project_id, video_id, created_by, job_type, provider, config,
		    status, result, error_message, credits_used,
		    processing_started_at, completed_at, created_at, updated_at`,
	).Scan(&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.JobType, &job.Provider, &job.Config,
		&job.Status, &job.Result, &job.ErrorMessage, &job.CreditsUsed,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // no pending jobs
	}
	if err != nil {
		return nil, fmt.Errorf("pick pending ai job: %w", err)
	}
	return &job, nil
}

func (s *AIJobService) Complete(ctx context.Context, jobID uuid.UUID, result []byte, creditsUsed int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ai_jobs
		 SET status = 'completed', result = $1, credits_used = $2,
		     completed_at = NOW(), updated_at = NOW()
		 WHERE id = $3`, result, creditsUsed, jobID)
	if err != nil {
		return fmt.Errorf("complete ai job: %w", err)
	}
	return nil
}

func (s *AIJobService) Fail(ctx context.Context, jobID uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ai_jobs
		 SET status = 'failed', error_message = $1,
		     completed_at = NOW(), updated_at = NOW()
		 WHERE id = $2`, errMsg, jobID)
	if err != nil {
		return fmt.Errorf("fail ai job: %w", err)
	}
	return nil
}

func (s *AIJobService) Cancel(ctx context.Context, jobID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE ai_jobs
		 SET status = 'failed', error_message = 'cancelled by user',
		     completed_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND status = 'pending'`, jobID)
	if err != nil {
		return fmt.Errorf("cancel ai job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("job not found or not in pending state")
	}
	return nil
}

func (s *AIJobService) ListAssets(ctx context.Context, jobID uuid.UUID) ([]model.AIAsset, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ai_job_id, project_id, asset_type, COALESCE(storage_key, ''),
		        content, COALESCE(metadata, '{}'), created_at
		 FROM ai_assets WHERE ai_job_id = $1
		 ORDER BY created_at`, jobID)
	if err != nil {
		return nil, fmt.Errorf("query ai assets: %w", err)
	}
	defer rows.Close()

	assets := []model.AIAsset{}
	for rows.Next() {
		var a model.AIAsset
		if err := rows.Scan(&a.ID, &a.AIJobID, &a.ProjectID, &a.AssetType,
			&a.StorageKey, &a.Content, &a.Metadata, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ai asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, nil
}
