package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

const CreditCostPerClip = 3

type SmartClipService struct {
	pool *pgxpool.Pool
}

func NewSmartClipService(pool *pgxpool.Pool) *SmartClipService {
	return &SmartClipService{pool: pool}
}

func (s *SmartClipService) CreateJob(ctx context.Context, userID uuid.UUID, req *model.CreateSmartClipJobRequest) (*model.SmartClipJob, error) {
	clipCount := 5
	if req.ClipCount > 0 && req.ClipCount <= 20 {
		clipCount = req.ClipCount
	}
	minDur := 15
	if req.MinDurationSeconds > 0 {
		minDur = req.MinDurationSeconds
	}
	maxDur := 60
	if req.MaxDurationSeconds > 0 {
		maxDur = req.MaxDurationSeconds
	}
	if minDur > maxDur {
		return nil, fmt.Errorf("min_duration_seconds cannot exceed max_duration_seconds")
	}

	aspect := "9:16"
	validAspects := map[string]bool{"9:16": true, "1:1": true, "4:5": true, "16:9": true}
	if req.TargetAspectRatio != "" {
		if !validAspects[req.TargetAspectRatio] {
			return nil, fmt.Errorf("invalid aspect ratio: %s", req.TargetAspectRatio)
		}
		aspect = req.TargetAspectRatio
	}

	autoCaptions := true
	if req.AutoCaptions != nil {
		autoCaptions = *req.AutoCaptions
	}
	captionLang := "en"
	if req.CaptionLanguage != "" {
		captionLang = req.CaptionLanguage
	}
	smartCrop := true
	if req.SmartCrop != nil {
		smartCrop = *req.SmartCrop
	}
	detectHooks := true
	if req.DetectHooks != nil {
		detectHooks = *req.DetectHooks
	}

	job := &model.SmartClipJob{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO smart_clip_jobs (project_id, video_id, created_by, clip_count,
			min_duration_seconds, max_duration_seconds, target_aspect_ratio,
			auto_captions, caption_language, smart_crop, detect_hooks)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, project_id, video_id, created_by, status, provider,
			clip_count, min_duration_seconds, max_duration_seconds, target_aspect_ratio,
			auto_captions, caption_language, smart_crop, detect_hooks, config,
			credits_used, progress, error_message, processing_started_at, completed_at,
			created_at, updated_at`,
		req.ProjectID, req.VideoID, userID, clipCount, minDur, maxDur,
		aspect, autoCaptions, captionLang, smartCrop, detectHooks,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.Status, &job.Provider, &job.ClipCount, &job.MinDurationSeconds,
		&job.MaxDurationSeconds, &job.TargetAspectRatio, &job.AutoCaptions,
		&job.CaptionLanguage, &job.SmartCrop, &job.DetectHooks, &job.Config,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create smart clip job: %w", err)
	}
	return job, nil
}

func (s *SmartClipService) GetJob(ctx context.Context, jobID uuid.UUID) (*model.SmartClipJob, error) {
	job := &model.SmartClipJob{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, project_id, video_id, created_by, status, provider,
			clip_count, min_duration_seconds, max_duration_seconds, target_aspect_ratio,
			auto_captions, caption_language, smart_crop, detect_hooks, config,
			credits_used, progress, error_message, processing_started_at, completed_at,
			created_at, updated_at
		FROM smart_clip_jobs WHERE id = $1`, jobID,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.Status, &job.Provider, &job.ClipCount, &job.MinDurationSeconds,
		&job.MaxDurationSeconds, &job.TargetAspectRatio, &job.AutoCaptions,
		&job.CaptionLanguage, &job.SmartCrop, &job.DetectHooks, &job.Config,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get smart clip job: %w", err)
	}
	return job, nil
}

func (s *SmartClipService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*model.SmartClipJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, project_id, video_id, created_by, status, provider,
			clip_count, min_duration_seconds, max_duration_seconds, target_aspect_ratio,
			auto_captions, caption_language, smart_crop, detect_hooks, config,
			credits_used, progress, error_message, processing_started_at, completed_at,
			created_at, updated_at
		FROM smart_clip_jobs WHERE project_id = $1 ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list smart clip jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*model.SmartClipJob
	for rows.Next() {
		job := &model.SmartClipJob{}
		if err := rows.Scan(
			&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
			&job.Status, &job.Provider, &job.ClipCount, &job.MinDurationSeconds,
			&job.MaxDurationSeconds, &job.TargetAspectRatio, &job.AutoCaptions,
			&job.CaptionLanguage, &job.SmartCrop, &job.DetectHooks, &job.Config,
			&job.CreditsUsed, &job.Progress, &job.ErrorMessage,
			&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan smart clip job: %w", err)
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *SmartClipService) ListClips(ctx context.Context, jobID uuid.UUID) ([]*model.SmartClip, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, job_id, title, start_time_ms, end_time_ms, duration_ms,
			hook_score, virality_score, engagement_prediction, aspect_ratio,
			has_captions, caption_data, video_url, thumbnail_url, transcript,
			hook_text, tags, detected_moments, status, rank, file_size_bytes, created_at
		FROM smart_clips WHERE job_id = $1 ORDER BY rank ASC`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list smart clips: %w", err)
	}
	defer rows.Close()

	var clips []*model.SmartClip
	for rows.Next() {
		c := &model.SmartClip{}
		if err := rows.Scan(
			&c.ID, &c.JobID, &c.Title, &c.StartTimeMs, &c.EndTimeMs,
			&c.DurationMs, &c.HookScore, &c.ViralityScore, &c.EngagementPrediction,
			&c.AspectRatio, &c.HasCaptions, &c.CaptionData, &c.VideoURL,
			&c.ThumbnailURL, &c.Transcript, &c.HookText, &c.Tags,
			&c.DetectedMoments, &c.Status, &c.Rank, &c.FileSizeBytes, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan smart clip: %w", err)
		}
		clips = append(clips, c)
	}
	return clips, nil
}

func (s *SmartClipService) ListMoments(ctx context.Context, jobID uuid.UUID) ([]*model.ClipMoment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, job_id, moment_type, start_time_ms, end_time_ms,
			confidence, description, hook_score, metadata, created_at
		FROM clip_moments WHERE job_id = $1 ORDER BY start_time_ms`, jobID)
	if err != nil {
		return nil, fmt.Errorf("list clip moments: %w", err)
	}
	defer rows.Close()

	var moments []*model.ClipMoment
	for rows.Next() {
		m := &model.ClipMoment{}
		if err := rows.Scan(
			&m.ID, &m.JobID, &m.MomentType, &m.StartTimeMs, &m.EndTimeMs,
			&m.Confidence, &m.Description, &m.HookScore, &m.Metadata, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan clip moment: %w", err)
		}
		moments = append(moments, m)
	}
	return moments, nil
}

func (s *SmartClipService) Cancel(ctx context.Context, userID, jobID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE smart_clip_jobs SET status = 'cancelled', updated_at = NOW()
		WHERE id = $1 AND created_by = $2 AND status = 'pending'`, jobID, userID)
	if err != nil {
		return fmt.Errorf("cancel smart clip job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("job not found or cannot be cancelled")
	}
	return nil
}

func (s *SmartClipService) PickPending(ctx context.Context) (*model.SmartClipJob, error) {
	job := &model.SmartClipJob{}
	err := s.pool.QueryRow(ctx, `
		UPDATE smart_clip_jobs SET status = 'processing', processing_started_at = NOW(), updated_at = NOW()
		WHERE id = (
			SELECT id FROM smart_clip_jobs WHERE status = 'pending'
			ORDER BY created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING id, project_id, video_id, created_by, status, provider,
			clip_count, min_duration_seconds, max_duration_seconds, target_aspect_ratio,
			auto_captions, caption_language, smart_crop, detect_hooks, config,
			credits_used, progress, error_message, processing_started_at, completed_at,
			created_at, updated_at`,
	).Scan(
		&job.ID, &job.ProjectID, &job.VideoID, &job.CreatedBy,
		&job.Status, &job.Provider, &job.ClipCount, &job.MinDurationSeconds,
		&job.MaxDurationSeconds, &job.TargetAspectRatio, &job.AutoCaptions,
		&job.CaptionLanguage, &job.SmartCrop, &job.DetectHooks, &job.Config,
		&job.CreditsUsed, &job.Progress, &job.ErrorMessage,
		&job.ProcessingStartedAt, &job.CompletedAt, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return job, nil
}

func (s *SmartClipService) Complete(ctx context.Context, jobID uuid.UUID, creditsUsed int) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `
		UPDATE smart_clip_jobs SET status = 'completed', credits_used = $2,
			completed_at = $3, progress = 100, updated_at = NOW()
		WHERE id = $1`, jobID, creditsUsed, now)
	if err != nil {
		return fmt.Errorf("complete smart clip job: %w", err)
	}
	return nil
}

func (s *SmartClipService) Fail(ctx context.Context, jobID uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE smart_clip_jobs SET status = 'failed', error_message = $2, updated_at = NOW()
		WHERE id = $1`, jobID, errMsg)
	if err != nil {
		return fmt.Errorf("fail smart clip job: %w", err)
	}
	return nil
}

func (s *SmartClipService) CreateClip(ctx context.Context, clip *model.SmartClip) (*model.SmartClip, error) {
	c := &model.SmartClip{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO smart_clips (job_id, title, start_time_ms, end_time_ms, duration_ms,
			hook_score, virality_score, engagement_prediction, aspect_ratio,
			has_captions, caption_data, video_url, thumbnail_url, transcript,
			hook_text, tags, detected_moments, status, rank)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, job_id, title, start_time_ms, end_time_ms, duration_ms,
			hook_score, virality_score, engagement_prediction, aspect_ratio,
			has_captions, caption_data, video_url, thumbnail_url, transcript,
			hook_text, tags, detected_moments, status, rank, file_size_bytes, created_at`,
		clip.JobID, clip.Title, clip.StartTimeMs, clip.EndTimeMs, clip.DurationMs,
		clip.HookScore, clip.ViralityScore, clip.EngagementPrediction, clip.AspectRatio,
		clip.HasCaptions, clip.CaptionData, clip.VideoURL, clip.ThumbnailURL,
		clip.Transcript, clip.HookText, clip.Tags, clip.DetectedMoments, clip.Status, clip.Rank,
	).Scan(
		&c.ID, &c.JobID, &c.Title, &c.StartTimeMs, &c.EndTimeMs,
		&c.DurationMs, &c.HookScore, &c.ViralityScore, &c.EngagementPrediction,
		&c.AspectRatio, &c.HasCaptions, &c.CaptionData, &c.VideoURL,
		&c.ThumbnailURL, &c.Transcript, &c.HookText, &c.Tags,
		&c.DetectedMoments, &c.Status, &c.Rank, &c.FileSizeBytes, &c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create smart clip: %w", err)
	}
	return c, nil
}

func (s *SmartClipService) CreateMoment(ctx context.Context, m *model.ClipMoment) (*model.ClipMoment, error) {
	moment := &model.ClipMoment{}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO clip_moments (job_id, moment_type, start_time_ms, end_time_ms,
			confidence, description, hook_score, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, job_id, moment_type, start_time_ms, end_time_ms,
			confidence, description, hook_score, metadata, created_at`,
		m.JobID, m.MomentType, m.StartTimeMs, m.EndTimeMs,
		m.Confidence, m.Description, m.HookScore, m.Metadata,
	).Scan(
		&moment.ID, &moment.JobID, &moment.MomentType, &moment.StartTimeMs,
		&moment.EndTimeMs, &moment.Confidence, &moment.Description,
		&moment.HookScore, &moment.Metadata, &moment.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create clip moment: %w", err)
	}
	return moment, nil
}

func (s *SmartClipService) GetTopClips(ctx context.Context, jobID uuid.UUID, limit int) ([]*model.SmartClip, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, job_id, title, start_time_ms, end_time_ms, duration_ms,
			hook_score, virality_score, engagement_prediction, aspect_ratio,
			has_captions, caption_data, video_url, thumbnail_url, transcript,
			hook_text, tags, detected_moments, status, rank, file_size_bytes, created_at
		FROM smart_clips WHERE job_id = $1 AND status = 'completed'
		ORDER BY virality_score DESC, hook_score DESC
		LIMIT $2`, jobID, limit)
	if err != nil {
		return nil, fmt.Errorf("get top clips: %w", err)
	}
	defer rows.Close()

	var clips []*model.SmartClip
	for rows.Next() {
		c := &model.SmartClip{}
		if err := rows.Scan(
			&c.ID, &c.JobID, &c.Title, &c.StartTimeMs, &c.EndTimeMs,
			&c.DurationMs, &c.HookScore, &c.ViralityScore, &c.EngagementPrediction,
			&c.AspectRatio, &c.HasCaptions, &c.CaptionData, &c.VideoURL,
			&c.ThumbnailURL, &c.Transcript, &c.HookText, &c.Tags,
			&c.DetectedMoments, &c.Status, &c.Rank, &c.FileSizeBytes, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan top clip: %w", err)
		}
		clips = append(clips, c)
	}
	return clips, nil
}

func (s *SmartClipService) EstimateCredits(clipCount int) int {
	if clipCount <= 0 {
		clipCount = 5
	}
	return clipCount * CreditCostPerClip
}

// ScoreClip calculates a virality score for a clip based on multiple factors.
// This is our "Better than OpusClip" scoring -- multi-factor weighted algorithm.
func ScoreClip(input *model.ClipScoringInput) int {
	score := 0.0

	// 1. Hook factor (0-30): Strong hooks drive engagement
	if input.HasHook {
		score += float64(input.HookScore) * 0.3
		if input.HookScore > 80 {
			score += 5 // bonus for exceptional hooks
		}
	}

	// 2. Duration sweet spot (0-20): 15-45s is optimal for shorts
	durSec := float64(input.DurationMs) / 1000.0
	if durSec >= 15 && durSec <= 45 {
		score += 20
	} else if durSec >= 10 && durSec <= 60 {
		score += 15
	} else if durSec >= 5 && durSec <= 90 {
		score += 8
	}

	// 3. Pacing score (0-15): Fast-paced content performs better
	score += float64(input.PacingScore) * 15.0

	// 4. Topic relevance (0-15): How well the clip matches trending topics
	score += float64(input.TopicRelevance) * 15.0

	// 5. CTA presence (0-10): Clips with calls to action drive engagement
	if input.HasCTA {
		score += 10
	}

	// 6. Word density (0-10): Moderate word count (not too sparse, not too dense)
	wordsPerSecond := float64(input.WordCount) / math.Max(durSec, 1)
	if wordsPerSecond >= 2.0 && wordsPerSecond <= 4.0 {
		score += 10
	} else if wordsPerSecond >= 1.0 && wordsPerSecond <= 5.0 {
		score += 5
	}

	// Clamp to 0-100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return int(score)
}
