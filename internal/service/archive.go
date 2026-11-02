package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/infra"
	"github.com/himanshumehta/social-publisher/internal/model"
)

const multipartCopyThreshold int64 = 5 * 1024 * 1024 * 1024 // 5GB

type ArchiveService struct {
	pool   *pgxpool.Pool
	hotS3  *infra.S3Client
	coldS3 *infra.S3Client
}

func NewArchiveService(pool *pgxpool.Pool, hotS3, coldS3 *infra.S3Client) *ArchiveService {
	return &ArchiveService{pool: pool, hotS3: hotS3, coldS3: coldS3}
}

// InitiateArchive creates a new archive task for a published project.
func (s *ArchiveService) InitiateArchive(ctx context.Context, userID, projectID uuid.UUID) (*model.ArchiveTask, error) {
	// Verify project is published and at least 10 days old
	var status string
	var updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT status, updated_at FROM projects WHERE id = $1`, projectID,
	).Scan(&status, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("project not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	if status != "published" {
		return nil, fmt.Errorf("project must be published to archive")
	}
	if time.Since(updatedAt) < 10*24*time.Hour {
		return nil, fmt.Errorf("project must be published for at least 10 days before archiving")
	}

	// Check no pending/in_progress archive task exists
	var existing int
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM archive_tasks
		 WHERE project_id = $1 AND status IN ('pending', 'in_progress')`,
		projectID,
	).Scan(&existing)
	if err != nil {
		return nil, fmt.Errorf("check existing archive: %w", err)
	}
	if existing > 0 {
		return nil, fmt.Errorf("an archive task is already pending or in progress")
	}

	// Count raw videos in hot storage
	var totalFiles int
	var totalBytes int64
	err = s.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(file_size_bytes), 0)
		 FROM videos
		 WHERE project_id = $1 AND format = 'raw' AND storage_type = 'spaces' AND is_current = true`,
		projectID,
	).Scan(&totalFiles, &totalBytes)
	if err != nil {
		return nil, fmt.Errorf("count raw videos: %w", err)
	}
	if totalFiles == 0 {
		return nil, fmt.Errorf("no raw videos in hot storage to archive")
	}

	// Create archive task
	var task model.ArchiveTask
	err = s.pool.QueryRow(ctx,
		`INSERT INTO archive_tasks (project_id, initiated_by, total_files, total_bytes)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, project_id, initiated_by, status, total_files, files_processed,
		    total_bytes, bytes_processed, error_message, started_at, completed_at, created_at, updated_at`,
		projectID, userID, totalFiles, totalBytes,
	).Scan(&task.ID, &task.ProjectID, &task.InitiatedBy, &task.Status,
		&task.TotalFiles, &task.FilesProcessed, &task.TotalBytes, &task.BytesProcessed,
		&task.ErrorMessage, &task.StartedAt, &task.CompletedAt, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create archive task: %w", err)
	}

	return &task, nil
}

// GetArchiveStatus returns the latest archive task for an project.
func (s *ArchiveService) GetArchiveStatus(ctx context.Context, projectID uuid.UUID) (*model.ArchiveTask, error) {
	var task model.ArchiveTask
	err := s.pool.QueryRow(ctx,
		`SELECT id, project_id, initiated_by, status, total_files, files_processed,
		    total_bytes, bytes_processed, error_message, started_at, completed_at, created_at, updated_at
		 FROM archive_tasks
		 WHERE project_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		projectID,
	).Scan(&task.ID, &task.ProjectID, &task.InitiatedBy, &task.Status,
		&task.TotalFiles, &task.FilesProcessed, &task.TotalBytes, &task.BytesProcessed,
		&task.ErrorMessage, &task.StartedAt, &task.CompletedAt, &task.CreatedAt, &task.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get archive status: %w", err)
	}
	return &task, nil
}

// ProcessNextTask claims the next pending archive task and processes it.
// Returns true if a task was processed.
func (s *ArchiveService) ProcessNextTask(ctx context.Context) (bool, error) {
	// Claim next pending task atomically
	var task model.ArchiveTask
	err := s.pool.QueryRow(ctx,
		`UPDATE archive_tasks
		 SET status = 'in_progress', started_at = NOW(), updated_at = NOW()
		 WHERE id = (
		     SELECT id FROM archive_tasks
		     WHERE status = 'pending'
		     ORDER BY created_at ASC
		     LIMIT 1
		     FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, project_id, initiated_by, status, total_files, files_processed,
		    total_bytes, bytes_processed, error_message, started_at, completed_at, created_at, updated_at`,
	).Scan(&task.ID, &task.ProjectID, &task.InitiatedBy, &task.Status,
		&task.TotalFiles, &task.FilesProcessed, &task.TotalBytes, &task.BytesProcessed,
		&task.ErrorMessage, &task.StartedAt, &task.CompletedAt, &task.CreatedAt, &task.UpdatedAt)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim archive task: %w", err)
	}

	slog.Info("processing archive task", "task_id", task.ID, "project_id", task.ProjectID)

	// Get raw videos for this project in hot storage
	rows, err := s.pool.Query(ctx,
		`SELECT id, storage_key, file_size_bytes
		 FROM videos
		 WHERE project_id = $1 AND format = 'raw' AND storage_type = 'spaces' AND is_current = true`,
		task.ProjectID,
	)
	if err != nil {
		s.failTask(ctx, task.ID, fmt.Sprintf("query videos: %v", err))
		return true, nil
	}
	defer rows.Close()

	type videoInfo struct {
		ID         uuid.UUID
		StorageKey string
		SizeBytes  int64
	}
	var videos []videoInfo
	for rows.Next() {
		var v videoInfo
		if err := rows.Scan(&v.ID, &v.StorageKey, &v.SizeBytes); err != nil {
			s.failTask(ctx, task.ID, fmt.Sprintf("scan video: %v", err))
			return true, nil
		}
		videos = append(videos, v)
	}

	hotBucket := s.hotS3.Bucket()
	coldBucket := s.coldS3.Bucket()

	for i, v := range videos {
		// Copy from hot to cold (server-side)
		var copyErr error
		if v.SizeBytes > multipartCopyThreshold {
			copyErr = s.coldS3.CopyObjectMultipart(ctx, hotBucket, v.StorageKey, coldBucket, v.StorageKey, v.SizeBytes)
		} else {
			copyErr = s.coldS3.CopyObject(ctx, hotBucket, v.StorageKey, coldBucket, v.StorageKey)
		}
		if copyErr != nil {
			s.failTask(ctx, task.ID, fmt.Sprintf("copy video %s: %v", v.ID, copyErr))
			return true, nil
		}

		// Delete from hot storage
		if err := s.hotS3.DeleteObject(ctx, v.StorageKey); err != nil {
			s.failTask(ctx, task.ID, fmt.Sprintf("delete video %s from hot: %v", v.ID, err))
			return true, nil
		}

		// Update video record
		_, err := s.pool.Exec(ctx,
			`UPDATE videos SET storage_type = 'spaces_cold', updated_at = NOW() WHERE id = $1`,
			v.ID,
		)
		if err != nil {
			s.failTask(ctx, task.ID, fmt.Sprintf("update video %s storage_type: %v", v.ID, err))
			return true, nil
		}

		// Update progress
		filesProcessed := i + 1
		bytesProcessed := task.BytesProcessed + v.SizeBytes
		task.BytesProcessed = bytesProcessed
		_, _ = s.pool.Exec(ctx,
			`UPDATE archive_tasks SET files_processed = $1, bytes_processed = $2, updated_at = NOW() WHERE id = $3`,
			filesProcessed, bytesProcessed, task.ID,
		)

		slog.Info("archived video", "task_id", task.ID, "video_id", v.ID,
			"progress", fmt.Sprintf("%d/%d", filesProcessed, task.TotalFiles))
	}

	// Mark completed
	_, err = s.pool.Exec(ctx,
		`UPDATE archive_tasks SET status = 'completed', completed_at = NOW(), updated_at = NOW() WHERE id = $1`,
		task.ID,
	)
	if err != nil {
		return true, fmt.Errorf("mark task completed: %w", err)
	}

	slog.Info("archive task completed", "task_id", task.ID, "project_id", task.ProjectID)
	return true, nil
}

// AutoArchiveStale finds published projects with raw videos in hot storage older than 10 days
// and creates pending archive tasks.
func (s *ArchiveService) AutoArchiveStale(ctx context.Context, systemUserID uuid.UUID) error {
	cutoff := time.Now().Add(-10 * 24 * time.Hour)

	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT a.id
		 FROM projects a
		 JOIN videos v ON v.project_id = a.id
		 WHERE a.status = 'published'
		   AND v.format = 'raw'
		   AND v.storage_type = 'spaces'
		   AND v.is_current = true
		   AND v.created_at < $1
		   AND NOT EXISTS (
		       SELECT 1 FROM archive_tasks at
		       WHERE at.project_id = a.id
		         AND at.status IN ('pending', 'in_progress', 'completed')
		   )`,
		cutoff,
	)
	if err != nil {
		return fmt.Errorf("query stale projects: %w", err)
	}
	defer rows.Close()

	var projectIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan project id: %w", err)
		}
		projectIDs = append(projectIDs, id)
	}

	for _, aID := range projectIDs {
		_, err := s.InitiateArchive(ctx, systemUserID, aID)
		if err != nil {
			slog.Warn("auto-archive failed for project", "project_id", aID, "error", err)
			continue
		}
		slog.Info("auto-archive task created", "project_id", aID)
	}

	return nil
}

func (s *ArchiveService) failTask(ctx context.Context, taskID uuid.UUID, errMsg string) {
	slog.Error("archive task failed", "task_id", taskID, "error", errMsg)
	_, _ = s.pool.Exec(ctx,
		`UPDATE archive_tasks SET status = 'failed', error_message = $1, completed_at = NOW(), updated_at = NOW() WHERE id = $2`,
		errMsg, taskID,
	)
}
