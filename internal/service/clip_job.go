package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ClipJobService struct {
	pool *pgxpool.Pool
}

func NewClipJobService(pool *pgxpool.Pool) *ClipJobService {
	return &ClipJobService{pool: pool}
}

type CreateClipJobRequest struct {
	SourceVideoID uuid.UUID `json:"source_video_id"`
	Provider      string    `json:"provider"`
	Config        []byte    `json:"config"`
}

func (s *ClipJobService) Create(ctx context.Context, projectID, userID uuid.UUID, req CreateClipJobRequest) (*model.ClipJob, error) {
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
		provider = "default"
	}
	config := req.Config
	if config == nil {
		config = []byte("{}")
	}

	var job model.ClipJob
	err = s.pool.QueryRow(ctx,
		`INSERT INTO clip_jobs (project_id, source_video_id, provider, config, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, project_id, source_video_id, status, clip_count, provider, config,
		    error_message, created_at, updated_at, started_at, completed_at`,
		projectID, req.SourceVideoID, provider, config, userID,
	).Scan(&job.ID, &job.ProjectID, &job.SourceVideoID, &job.Status, &job.ClipCount,
		&job.Provider, &job.Config, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
		&job.StartedAt, &job.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("insert clip job: %w", err)
	}
	return &job, nil
}

func (s *ClipJobService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]model.ClipJob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, source_video_id, status, clip_count, provider, config,
		        error_message, created_at, updated_at, started_at, completed_at
		 FROM clip_jobs WHERE project_id = $1
		 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query clip jobs: %w", err)
	}
	defer rows.Close()

	var jobs []model.ClipJob
	for rows.Next() {
		var j model.ClipJob
		if err := rows.Scan(&j.ID, &j.ProjectID, &j.SourceVideoID, &j.Status, &j.ClipCount,
			&j.Provider, &j.Config, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt,
			&j.StartedAt, &j.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan clip job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if jobs == nil {
		jobs = []model.ClipJob{}
	}
	return jobs, nil
}

func (s *ClipJobService) ListClipsByJob(ctx context.Context, jobID uuid.UUID) ([]model.Video, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, project_id, uploaded_by, format, version, storage_key, storage_type,
		        file_name, file_size_bytes, mime_type, duration_seconds, width, height,
		        status, source_video_id, clip_job_id, virality_score, is_current, created_at, updated_at
		 FROM videos WHERE clip_job_id = $1 AND is_deleted = FALSE
		 ORDER BY virality_score DESC NULLS LAST`, jobID)
	if err != nil {
		return nil, fmt.Errorf("query clips: %w", err)
	}
	defer rows.Close()

	var clips []model.Video
	for rows.Next() {
		var v model.Video
		if err := rows.Scan(&v.ID, &v.ProjectID, &v.UploadedBy, &v.Format, &v.Version,
			&v.StorageKey, &v.StorageType, &v.FileName, &v.FileSizeBytes, &v.MimeType,
			&v.DurationSeconds, &v.Width, &v.Height, &v.Status,
			&v.SourceVideoID, &v.ClipJobID, &v.ViralityScore,
			&v.IsCurrent, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan clip: %w", err)
		}
		clips = append(clips, v)
	}
	if clips == nil {
		clips = []model.Video{}
	}
	return clips, nil
}

func (s *ClipJobService) PickPending(ctx context.Context) (*model.ClipJob, error) {
	var job model.ClipJob
	err := s.pool.QueryRow(ctx,
		`UPDATE clip_jobs SET status = 'processing', started_at = NOW(), updated_at = NOW()
		 WHERE id = (SELECT id FROM clip_jobs WHERE status = 'pending' ORDER BY created_at LIMIT 1)
		 RETURNING id, project_id, source_video_id, status, clip_count, provider, config,
		    error_message, created_at, updated_at, started_at, completed_at`,
	).Scan(&job.ID, &job.ProjectID, &job.SourceVideoID, &job.Status, &job.ClipCount,
		&job.Provider, &job.Config, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt,
		&job.StartedAt, &job.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // no pending jobs
	}
	if err != nil {
		return nil, fmt.Errorf("pick pending clip job: %w", err)
	}
	return &job, nil
}

func (s *ClipJobService) Complete(ctx context.Context, jobID uuid.UUID, clipCount int) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE clip_jobs SET status = 'completed', clip_count = $1, completed_at = NOW(), updated_at = NOW()
		 WHERE id = $2`, clipCount, jobID)
	if err != nil {
		return fmt.Errorf("complete clip job: %w", err)
	}
	return nil
}

func (s *ClipJobService) Fail(ctx context.Context, jobID uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE clip_jobs SET status = 'failed', error_message = $1, completed_at = NOW(), updated_at = NOW()
		 WHERE id = $2`, errMsg, jobID)
	if err != nil {
		return fmt.Errorf("fail clip job: %w", err)
	}
	return nil
}
