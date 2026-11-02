-- =============================================================================
-- 000007_api_publish.down.sql
-- Reverse V2 API Publishing changes
-- Note: ALTER TYPE ... DROP VALUE is not supported in PostgreSQL.
-- The new video_format enum values (threads_video, twitter_video, linkedin_video)
-- cannot be removed without recreating the type. They are harmless to leave.
-- =============================================================================

DROP INDEX IF EXISTS idx_upload_queue_one_active_per_channel;
DROP INDEX IF EXISTS idx_upload_queue_scheduled;
DROP INDEX IF EXISTS idx_upload_queue_status;
DROP TABLE IF EXISTS upload_queue;

DROP INDEX IF EXISTS idx_platform_analytics_period;
DROP INDEX IF EXISTS idx_platform_analytics_record;
DROP TABLE IF EXISTS platform_analytics;

ALTER TABLE publish_records
    DROP COLUMN IF EXISTS platform_metadata,
    DROP COLUMN IF EXISTS processing_started_at,
    DROP COLUMN IF EXISTS upload_completed_at,
    DROP COLUMN IF EXISTS upload_started_at,
    DROP COLUMN IF EXISTS upload_progress_pct,
    DROP COLUMN IF EXISTS upload_session_id;

ALTER TABLE channels
    DROP COLUMN IF EXISTS connection_status,
    DROP COLUMN IF EXISTS last_token_refresh,
    DROP COLUMN IF EXISTS oauth_provider,
    DROP COLUMN IF EXISTS token_encrypted;
