-- Rollback migration 000009: Direct Publish + Auto-Clip workflows

BEGIN;

-- ============================================================================
-- Rollback in reverse order
-- ============================================================================

-- 4. Remove video clip columns (drop constraint and indexes first, then columns)
ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_virality_score_check;
DROP INDEX IF EXISTS idx_videos_virality_score;
DROP INDEX IF EXISTS idx_videos_clip_job_id;
DROP INDEX IF EXISTS idx_videos_source_video_id;
ALTER TABLE videos DROP COLUMN IF EXISTS virality_score;
ALTER TABLE videos DROP COLUMN IF EXISTS clip_job_id;
ALTER TABLE videos DROP COLUMN IF EXISTS source_video_id;

-- 3. Drop clip_jobs table (indexes are dropped automatically with the table)
DROP TABLE IF EXISTS clip_jobs;

-- 2. Drop channel_targets table (indexes are dropped automatically with the table)
DROP TABLE IF EXISTS channel_targets;

-- 1. Remove workflow_type from projects
DROP INDEX IF EXISTS idx_projects_workflow_type;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_workflow_type_check;
ALTER TABLE projects DROP COLUMN IF EXISTS workflow_type;

COMMIT;
