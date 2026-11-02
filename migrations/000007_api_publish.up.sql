-- =============================================================================
-- 000007_api_publish.up.sql
-- V2: API-Based Platform Publishing
-- Adds OAuth tracking to channels, upload queue, generalized platform analytics,
-- and upload tracking on publish_records.
-- =============================================================================

-- 1. Channel OAuth tracking columns
--    connection_status: disconnected | connected | token_expiring | token_expired | error
--    oauth_provider:    google | facebook | twitter | linkedin | threads
ALTER TABLE channels
    ADD COLUMN IF NOT EXISTS token_encrypted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS oauth_provider TEXT,
    ADD COLUMN IF NOT EXISTS last_token_refresh TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS connection_status TEXT NOT NULL DEFAULT 'disconnected';

-- 2. V2 upload tracking on publish_records
--    status values: pending, scheduled, uploading, processing, published, failed, removed
ALTER TABLE publish_records
    ADD COLUMN IF NOT EXISTS upload_session_id TEXT,
    ADD COLUMN IF NOT EXISTS upload_progress_pct INT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS upload_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS upload_completed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS platform_metadata JSONB DEFAULT '{}';

-- 3. Generalized platform analytics (cross-platform, replaces YouTube-only for new publishes)
CREATE TABLE IF NOT EXISTS platform_analytics (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    publish_record_id       UUID NOT NULL REFERENCES publish_records(id) ON DELETE CASCADE,
    platform                TEXT NOT NULL,
    period_start            DATE NOT NULL,
    period_end              DATE NOT NULL,
    views                   BIGINT NOT NULL DEFAULT 0,
    likes                   INTEGER NOT NULL DEFAULT 0,
    comments_count          INTEGER NOT NULL DEFAULT 0,
    shares                  INTEGER NOT NULL DEFAULT 0,
    saves                   INTEGER NOT NULL DEFAULT 0,
    watch_time_minutes      BIGINT NOT NULL DEFAULT 0,
    subscribers_gained      INTEGER NOT NULL DEFAULT 0,
    reach                   BIGINT NOT NULL DEFAULT 0,
    impressions             BIGINT NOT NULL DEFAULT 0,
    estimated_revenue_paise BIGINT NOT NULL DEFAULT 0,
    raw_response            JSONB,
    fetched_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (publish_record_id, platform, period_start)
);

CREATE INDEX IF NOT EXISTS idx_platform_analytics_record ON platform_analytics(publish_record_id);
CREATE INDEX IF NOT EXISTS idx_platform_analytics_period ON platform_analytics(period_start, period_end);

-- 4. Upload queue (background worker processes these)
--    status: pending | downloading | uploading | processing | completed | failed | cancelled
CREATE TABLE IF NOT EXISTS upload_queue (
    id                     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    publish_record_id      UUID NOT NULL REFERENCES publish_records(id) ON DELETE CASCADE,
    channel_id             UUID NOT NULL REFERENCES channels(id),
    deliverable_version_id UUID NOT NULL REFERENCES deliverable_versions(id),
    platform               TEXT NOT NULL,
    status                 TEXT NOT NULL DEFAULT 'pending',
    attempts               INT NOT NULL DEFAULT 0,
    max_attempts           INT NOT NULL DEFAULT 3,
    last_error             TEXT,
    scheduled_at           TIMESTAMPTZ,
    started_at             TIMESTAMPTZ,
    completed_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_upload_queue_status ON upload_queue(status)
    WHERE status IN ('pending', 'downloading', 'uploading', 'processing');
CREATE INDEX IF NOT EXISTS idx_upload_queue_scheduled ON upload_queue(scheduled_at)
    WHERE status = 'pending';

-- Rate-limiting guard: at most 1 active upload per channel at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_upload_queue_one_active_per_channel
    ON upload_queue(channel_id)
    WHERE status IN ('downloading', 'uploading', 'processing');

-- 5. New video_format enum values for platform-specific deliverables
ALTER TYPE video_format ADD VALUE IF NOT EXISTS 'threads_video';
ALTER TYPE video_format ADD VALUE IF NOT EXISTS 'twitter_video';
ALTER TYPE video_format ADD VALUE IF NOT EXISTS 'linkedin_video';
