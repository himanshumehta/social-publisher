-- =============================================================================
-- 000006_platform_publishing.up.sql
-- Add deliverable tracking to publish_records and youtube_analytics
-- =============================================================================

-- Add deliverable tracking columns to publish_records
ALTER TABLE publish_records
    ADD COLUMN deliverable_id UUID REFERENCES deliverables(id) ON DELETE SET NULL,
    ADD COLUMN deliverable_version_id UUID REFERENCES deliverable_versions(id) ON DELETE SET NULL,
    ADD COLUMN title TEXT,
    ADD COLUMN description TEXT,
    ADD COLUMN tags TEXT[],
    ADD COLUMN thumbnail_url TEXT,
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public',
    ADD COLUMN scheduled_at TIMESTAMPTZ;

CREATE INDEX idx_publish_records_deliverable ON publish_records(deliverable_id) WHERE deliverable_id IS NOT NULL;

-- Add deliverable_id to youtube_analytics for direct lookups
ALTER TABLE youtube_analytics
    ADD COLUMN deliverable_id UUID REFERENCES deliverables(id) ON DELETE SET NULL;
