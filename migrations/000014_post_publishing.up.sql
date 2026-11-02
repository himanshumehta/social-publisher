-- =============================================================================
-- Migration 000014: Post Publishing Pipeline
-- Extends posts table for URL scanning, AI generation, multi-platform publishing
-- =============================================================================

-- 1. Extend post_status enum with pipeline stages
-- IMPORTANT: ALTER TYPE ... ADD VALUE cannot run inside a transaction block.
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'draft';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'scanning';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'scan_failed';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'scanned';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'generating';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'generation_failed';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'generated';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'pending_review';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'flagged';
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'publishing';

BEGIN;

-- 2. Add pipeline columns to posts table
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS source_url           TEXT,
    ADD COLUMN IF NOT EXISTS source_metadata      JSONB,
    ADD COLUMN IF NOT EXISTS platform_posts       JSONB,
    ADD COLUMN IF NOT EXISTS target_platforms      TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS linked_video_url      TEXT,
    ADD COLUMN IF NOT EXISTS linked_video_id       UUID REFERENCES videos(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS tone                  TEXT NOT NULL DEFAULT 'professional',
    ADD COLUMN IF NOT EXISTS brand_voice           TEXT,
    ADD COLUMN IF NOT EXISTS system_review_status  TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS system_review_notes   TEXT,
    ADD COLUMN IF NOT EXISTS system_review_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS publish_results       JSONB,
    ADD COLUMN IF NOT EXISTS error_message         TEXT,
    ADD COLUMN IF NOT EXISTS retry_count           INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS auto_publish          BOOLEAN NOT NULL DEFAULT FALSE;

-- 3. Pipeline queue index (workers poll this)
CREATE INDEX IF NOT EXISTS idx_posts_pipeline_queue
    ON posts(status, created_at ASC)
    WHERE status IN ('draft', 'scanning', 'scanned', 'generating', 'approved');

-- 4. Admin review queue index
CREATE INDEX IF NOT EXISTS idx_posts_review_queue
    ON posts(created_at DESC)
    WHERE status IN ('pending_review', 'flagged');

-- 5. Source URL uniqueness (prevent duplicate posts for same URL)
CREATE UNIQUE INDEX IF NOT EXISTS idx_posts_source_url_unique
    ON posts(source_url)
    WHERE source_url IS NOT NULL AND status != 'rejected';

-- 6. Post → Publish Record link table
CREATE TABLE IF NOT EXISTS post_publish_links (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    post_id           UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    publish_record_id UUID REFERENCES publish_records(id) ON DELETE SET NULL,
    platform          TEXT NOT NULL,
    channel_id        UUID REFERENCES channels(id) ON DELETE SET NULL,
    platform_post_id  TEXT,
    platform_url      TEXT,
    status            TEXT NOT NULL DEFAULT 'pending',
    error_message     TEXT,
    published_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(post_id, platform)
);

CREATE INDEX IF NOT EXISTS idx_ppl_post ON post_publish_links(post_id);
CREATE INDEX IF NOT EXISTS idx_ppl_status ON post_publish_links(status)
    WHERE status IN ('pending', 'publishing');

-- 7. API keys for external service access
CREATE TABLE IF NOT EXISTS api_keys (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name          TEXT NOT NULL,
    key_hash      TEXT NOT NULL UNIQUE,
    key_prefix    TEXT NOT NULL,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes        TEXT[] NOT NULL DEFAULT '{}',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    rate_limit    INT NOT NULL DEFAULT 100,
    last_used_at  TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_lookup
    ON api_keys(key_hash)
    WHERE is_active = TRUE;

-- 8. Trigger: auto-update updated_at on post_publish_links
CREATE OR REPLACE FUNCTION update_post_publish_links_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_post_publish_links_updated_at
    BEFORE UPDATE ON post_publish_links
    FOR EACH ROW
    EXECUTE FUNCTION update_post_publish_links_updated_at();

COMMIT;

-- 9. Add audit actions for post publishing (must be outside transaction)
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'post_scan';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'post_generate';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'post_system_review';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'post_platform_publish';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'api_key_create';
ALTER TYPE audit_action ADD VALUE IF NOT EXISTS 'api_key_revoke';
