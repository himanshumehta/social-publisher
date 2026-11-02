package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// CreditCostPerMinuteDub is the credit cost per minute of video for dubbing
const CreditCostPerMinuteDub = 5

type DubService struct {
	pool *pgxpool.Pool
}

func NewDubService(pool *pgxpool.Pool) *DubService {
	return &DubService{pool: pool}
}

func (s *DubService) CreateJob(ctx context.Context, userID uuid.UUID, req *model.CreateDubJobRequest) (*model.DubJob, error) {
	if _, ok := model.SupportedDubLanguages[req.TargetLanguage]; !ok {
		return nil, fmt.Errorf("unsupported target language: %s", req.TargetLanguage)
	}

	srcLang := "en"
	if req.SourceLanguage != "" {
		srcLang = req.SourceLanguage
	}

	preserveBg := true
	if req.PreserveBackgroundAudio != nil {
		preserveBg = *req.PreserveBackgroundAudio
	}

	lipSync := false
	if req.LipSyncEnabled != nil {
		lipSync = *req.LipSyncEnabled
	}

	job := &model.DubJob{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dub_jobs (project_id, video_id, created_by, source_language, target_language,
			voice_profile, preserve_background_audio, lip_sync_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, project_id, video_id, created_by, source_language, target_language,
			status, provider, config, voice_profile, preserve_background_audio, lip_sync_enabled,
			credits_used, progress, error_message, result, output_video_url, output_audio_url,
			duration_seconds, processing_started_at, completed_at, created_at, updated_at`,
		req.ProjectID, req.VideoID, userID, srcLang, req.TargetLanguage,
		req.VoiceProfile, preserveBg, lipSync,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.SourceLanguage, &job.TargetLanguage, &job.Status, &job.Provider,
		&job.Config, &job.VoiceProfile, &job.PreserveBackgroundAudio, &job.LipSyncEnabled,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage, &job.Result,
		&job.OutputVideoURL, &job.OutputAudioURL, &job.DurationSeconds,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create dub job: %w", err)
	}

	return job, nil
}

func (s *DubService) BatchCreate(ctx context.Context, userID uuid.UUID, req *model.BatchDubRequest) ([]*model.DubJob, error) {
	var jobs []*model.DubJob
	for _, lang := range req.TargetLanguages {
		job, err := s.CreateJob(ctx, userID, &model.CreateDubJobRequest{
			ProjectID:      req.ProjectID,
			VideoID:        req.VideoID,
			TargetLanguage: lang,
			VoiceProfile:   req.VoiceProfile,
		})
		if err != nil {
			return nil, fmt.Errorf("batch create dub job for %s: %w", lang, err)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *DubService) GetJob(ctx context.Context, jobID uuid.UUID) (*model.DubJob, error) {
	job := &model.DubJob{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, video_id, created_by, source_language, target_language,
			status, provider, config, voice_profile, preserve_background_audio, lip_sync_enabled,
			credits_used, progress, error_message, result, output_video_url, output_audio_url,
			duration_seconds, processing_started_at, completed_at, created_at, updated_at
		FROM dub_jobs WHERE id = $1`, jobID,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.SourceLanguage, &job.TargetLanguage, &job.Status, &job.Provider,
		&job.Config, &job.VoiceProfile, &job.PreserveBackgroundAudio, &job.LipSyncEnabled,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage, &job.Result,
		&job.OutputVideoURL, &job.OutputAudioURL, &job.DurationSeconds,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get dub job: %w", err)
	}
	return job, nil
}

func (s *DubService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*model.DubJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, video_id, created_by, source_language, target_language,
			status, provider, config, voice_profile, preserve_background_audio, lip_sync_enabled,
			credits_used, progress, error_message, result, output_video_url, output_audio_url,
			duration_seconds, processing_started_at, completed_at, created_at, updated_at
		FROM dub_jobs WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list dub jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*model.DubJob
	for rows.Next() {
		job := &model.DubJob{}
		if err := rows.Scan(
			&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
			&job.SourceLanguage, &job.TargetLanguage, &job.Status, &job.Provider,
			&job.Config, &job.VoiceProfile, &job.PreserveBackgroundAudio, &job.LipSyncEnabled,
			&job.CreditsUsed, &job.Progress, &job.ErrorMessage, &job.Result,
			&job.OutputVideoURL, &job.OutputAudioURL, &job.DurationSeconds,
			&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dub job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *DubService) ListOutputs(ctx context.Context, jobID uuid.UUID) ([]*model.DubOutput, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, dub_job_id, language, audio_url, video_url, subtitle_url,
			transcript, quality_score, duration_seconds, file_size_bytes, status,
			error_message, created_at
		FROM dub_outputs WHERE dub_job_id = $1 ORDER BY created_at`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list dub outputs: %w", err)
	}
	defer rows.Close()

	var outputs []*model.DubOutput
	for rows.Next() {
		o := &model.DubOutput{}
		if err := rows.Scan(
			&o.ID, &o.DubJobID, &o.Language, &o.AudioURL, &o.VideoURL,
			&o.SubtitleURL, &o.Transcript, &o.QualityScore, &o.DurationSeconds,
			&o.FileSizeBytes, &o.Status, &o.ErrorMessage, &o.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dub output: %w", err)
		}
		outputs = append(outputs, o)
	}
	return outputs, nil
}

func (s *DubService) Cancel(ctx context.Context, userID, jobID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE dub_jobs SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND created_by = $2 AND status = 'pending'`, jobID, userID)
	if err != nil {
		return fmt.Errorf("cancel dub job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("job not found or cannot be cancelled")
	}
	return nil
}

func (s *DubService) PickPending(ctx context.Context) (*model.DubJob, error) {
	job := &model.DubJob{}
	err := s.pool.QueryRow(ctx, `
		UPDATE dub_jobs SET status = 'processing', processing_started_at = NOW(), updated_at = NOW()
		WHERE id = (
			SELECT id FROM dub_jobs WHERE status = 'pending'
			ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING id, project_id, video_id, created_by, source_language, target_language,
			status, provider, config, voice_profile, preserve_background_audio, lip_sync_enabled,
			credits_used, progress, error_message, result, output_video_url, output_audio_url,
			duration_seconds, processing_started_at, completed_at, created_at, updated_at`,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.SourceLanguage, &job.TargetLanguage, &job.Status, &job.Provider,
		&job.Config, &job.VoiceProfile, &job.PreserveBackgroundAudio, &job.LipSyncEnabled,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage, &job.Result,
		&job.OutputVideoURL, &job.OutputAudioURL, &job.DurationSeconds,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err // pgx.ErrNoRows when no pending jobs
	}
	return job, nil
}

func (s *DubService) Complete(ctx context.Context, jobID uuid.UUID, outputVideoURL, outputAudioURL string, creditsUsed int) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE dub_jobs SET status = 'completed', output_video_url = $2, output_audio_url = $3,
			credits_used = $4, completed_at = $5, progress = 100, updated_at = NOW()
		WHERE id = $1`, jobID, outputVideoURL, outputAudioURL, creditsUsed, now)
	if err != nil {
		return fmt.Errorf("complete dub job: %w", err)
	}
	return nil
}

func (s *DubService) Fail(ctx context.Context, jobID uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dub_jobs SET status = 'failed', error_message = $2, updated_at = NOW()
		WHERE id = $1`, jobID, errMsg)
	if err != nil {
		return fmt.Errorf("fail dub job: %w", err)
	}
	return nil
}

func (s *DubService) UpdateProgress(ctx context.Context, jobID uuid.UUID, progress int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE dub_jobs SET progress = $2, updated_at = NOW()
		WHERE id = $1`, jobID, progress)
	if err != nil {
		return fmt.Errorf("update dub progress: %w", err)
	}
	return nil
}

func (s *DubService) CreateOutput(ctx context.Context, output *model.DubOutput) (*model.DubOutput, error) {
	o := &model.DubOutput{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dub_outputs (dub_job_id, language, audio_url, video_url, subtitle_url,
			transcript, quality_score, duration_seconds, file_size_bytes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, dub_job_id, language, audio_url, video_url, subtitle_url,
			transcript, quality_score, duration_seconds, file_size_bytes, status,
			error_message, created_at`,
		output.DubJobID, output.Language, output.AudioURL, output.VideoURL,
		output.SubtitleURL, output.Transcript, output.QualityScore,
		output.DurationSeconds, output.FileSizeBytes, output.Status,
	).Scan(
		&o.ID, &o.DubJobID, &o.Language, &o.AudioURL, &o.VideoURL,
		&o.SubtitleURL, &o.Transcript, &o.QualityScore, &o.DurationSeconds,
		&o.FileSizeBytes, &o.Status, &o.ErrorMessage, &o.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create dub output: %w", err)
	}
	return o, nil
}

func (s *DubService) GetSupportedLanguages() []model.DubLanguageInfo {
	var langs []model.DubLanguageInfo
	for code, name := range model.SupportedDubLanguages {
		langs = append(langs, model.DubLanguageInfo{Code: code, Name: name})
	}
	return langs
}

func (s *DubService) EstimateCredits(durationSeconds int) int {
	minutes := durationSeconds / 60
	if durationSeconds%60 > 0 {
		minutes++
	}
	if minutes < 1 {
		minutes = 1
	}
	return minutes * CreditCostPerMinuteDub
}
