-- =============================================================================
-- 000006_platform_publishing.down.sql
-- Reverse deliverable tracking additions
-- =============================================================================

ALTER TABLE youtube_analytics DROP COLUMN IF EXISTS deliverable_id;

DROP INDEX IF EXISTS idx_publish_records_deliverable;

ALTER TABLE publish_records
    DROP COLUMN IF EXISTS scheduled_at,
    DROP COLUMN IF EXISTS visibility,
    DROP COLUMN IF EXISTS thumbnail_url,
    DROP COLUMN IF EXISTS tags,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS title,
    DROP COLUMN IF EXISTS deliverable_version_id,
    DROP COLUMN IF EXISTS deliverable_id;
