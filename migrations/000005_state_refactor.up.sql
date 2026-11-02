-- =============================================================================
-- 000005_state_refactor.up.sql
-- Three-layer state model: Assignment (9 states) → Raw Assets → Deliverables
-- =============================================================================

-- =============================================================================
-- 1. New enum types for raw assets and deliverables
-- =============================================================================
CREATE TYPE raw_asset_status AS ENUM ('uploading', 'processing', 'ready', 'in_use', 'rejected', 'superseded');
CREATE TYPE deliverable_status AS ENUM ('queued', 'editing', 'in_review', 'revision', 'approved', 'published');
CREATE TYPE deliverable_format AS ENUM ('youtube_16_9', 'instagram_reel', 'youtube_short', 'facebook_feed', 'facebook_reel');

-- =============================================================================
-- 2. Create new assignment_status enum (9 states replacing old 13)
-- =============================================================================
CREATE TYPE assignment_status_new AS ENUM (
    'draft', 'open', 'claimed', 'in_production', 'in_editing', 'in_review',
    'published', 'cancelled', 'on_hold'
);

-- =============================================================================
-- 3. Drop indexes that depend on assignment_status type
-- =============================================================================
DROP INDEX IF EXISTS idx_assignments_open;

-- =============================================================================
-- 4. Migrate assignments.status from old enum → new enum
-- =============================================================================
-- Drop default first so the type cast doesn't conflict
ALTER TABLE assignments ALTER COLUMN status DROP DEFAULT;
ALTER TABLE assignments
    ALTER COLUMN status TYPE assignment_status_new
    USING (
        CASE status::text
            WHEN 'raw_uploaded'      THEN 'in_production'
            WHEN 'editing'           THEN 'in_editing'
            WHEN 'edited'            THEN 'in_editing'
            WHEN 'creator_review'    THEN 'in_review'
            WHEN 'creator_approved'  THEN 'in_review'
            WHEN 'admin_review'      THEN 'in_review'
            WHEN 'approved'          THEN 'in_review'
            WHEN 'rejected'          THEN 'cancelled'
            ELSE status::text
        END::assignment_status_new
    );
ALTER TABLE assignments ALTER COLUMN status SET DEFAULT 'draft'::assignment_status_new;

-- =============================================================================
-- 5. Migrate reviews.from_status and to_status
-- =============================================================================
ALTER TABLE reviews
    ALTER COLUMN from_status TYPE text USING from_status::text;
ALTER TABLE reviews
    ALTER COLUMN to_status TYPE text USING to_status::text;

UPDATE reviews SET from_status = CASE from_status
    WHEN 'raw_uploaded'      THEN 'in_production'
    WHEN 'editing'           THEN 'in_editing'
    WHEN 'edited'            THEN 'in_editing'
    WHEN 'creator_review'    THEN 'in_review'
    WHEN 'creator_approved'  THEN 'in_review'
    WHEN 'admin_review'      THEN 'in_review'
    WHEN 'approved'          THEN 'in_review'
    WHEN 'rejected'          THEN 'cancelled'
    ELSE from_status
END WHERE from_status IS NOT NULL;

UPDATE reviews SET to_status = CASE to_status
    WHEN 'raw_uploaded'      THEN 'in_production'
    WHEN 'editing'           THEN 'in_editing'
    WHEN 'edited'            THEN 'in_editing'
    WHEN 'creator_review'    THEN 'in_review'
    WHEN 'creator_approved'  THEN 'in_review'
    WHEN 'admin_review'      THEN 'in_review'
    WHEN 'approved'          THEN 'in_review'
    WHEN 'rejected'          THEN 'cancelled'
    ELSE to_status
END WHERE to_status IS NOT NULL;

ALTER TABLE reviews
    ALTER COLUMN from_status TYPE assignment_status_new USING from_status::assignment_status_new;
ALTER TABLE reviews
    ALTER COLUMN to_status TYPE assignment_status_new USING to_status::assignment_status_new;

-- =============================================================================
-- 6. Swap enum types
-- =============================================================================
DROP TYPE assignment_status;
ALTER TYPE assignment_status_new RENAME TO assignment_status;

-- =============================================================================
-- 7. Recreate indexes
-- =============================================================================
CREATE INDEX idx_assignments_open ON assignments(status, priority DESC, created_at DESC)
    WHERE status = 'open';

-- =============================================================================
-- 8. Create raw_assets table
-- =============================================================================
CREATE TABLE raw_assets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id   UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    uploaded_by     UUID NOT NULL REFERENCES users(id),
    storage_key     TEXT NOT NULL,
    storage_type    TEXT NOT NULL DEFAULT 'spaces',
    file_name       TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type       TEXT NOT NULL,
    duration_seconds INTEGER,
    width           INTEGER,
    height          INTEGER,
    status          raw_asset_status NOT NULL DEFAULT 'processing',
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raw_assets_assignment ON raw_assets(assignment_id);
CREATE INDEX idx_raw_assets_status ON raw_assets(assignment_id, status);

-- =============================================================================
-- 9. Create deliverables table
-- =============================================================================
CREATE TABLE deliverables (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id       UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    format              deliverable_format NOT NULL,
    status              deliverable_status NOT NULL DEFAULT 'queued',
    editor_id           UUID REFERENCES users(id),
    current_version     INTEGER NOT NULL DEFAULT 0,
    creator_approved    BOOLEAN NOT NULL DEFAULT FALSE,
    admin_approved      BOOLEAN NOT NULL DEFAULT FALSE,
    total_revenue_paise BIGINT NOT NULL DEFAULT 0,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (assignment_id, format)
);

CREATE INDEX idx_deliverables_assignment ON deliverables(assignment_id);
CREATE INDEX idx_deliverables_status ON deliverables(status);
CREATE INDEX idx_deliverables_editor ON deliverables(editor_id);

-- =============================================================================
-- 10. Create deliverable_versions table
-- =============================================================================
CREATE TABLE deliverable_versions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    deliverable_id  UUID NOT NULL REFERENCES deliverables(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    storage_key     TEXT NOT NULL,
    storage_type    TEXT NOT NULL DEFAULT 'spaces',
    file_name       TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type       TEXT NOT NULL,
    duration_seconds INTEGER,
    width           INTEGER,
    height          INTEGER,
    uploaded_by     UUID NOT NULL REFERENCES users(id),
    is_current      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_deliverable_versions_deliverable ON deliverable_versions(deliverable_id);
CREATE INDEX idx_deliverable_versions_current ON deliverable_versions(deliverable_id) WHERE is_current = TRUE;

-- =============================================================================
-- 11. Add deliverable_id column to reviews
-- =============================================================================
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS deliverable_id UUID REFERENCES deliverables(id);
CREATE INDEX IF NOT EXISTS idx_reviews_deliverable ON reviews(deliverable_id) WHERE deliverable_id IS NOT NULL;

-- =============================================================================
-- 12. Migrate existing video data into new tables
-- =============================================================================

-- 12a: Raw videos → raw_assets
INSERT INTO raw_assets (id, assignment_id, uploaded_by, storage_key, storage_type, file_name,
    file_size_bytes, mime_type, duration_seconds, width, height, status, created_at, updated_at)
SELECT id, assignment_id, uploaded_by, storage_key, storage_type, file_name,
    file_size_bytes, mime_type, duration_seconds, width, height,
    CASE
        WHEN status::text = 'approved' THEN 'in_use'::raw_asset_status
        WHEN status::text = 'rejected' THEN 'rejected'::raw_asset_status
        ELSE 'ready'::raw_asset_status
    END,
    created_at, updated_at
FROM videos
WHERE format = 'raw' AND is_deleted = FALSE;

-- 12b: Non-raw videos → deliverables (one per assignment+format)
INSERT INTO deliverables (assignment_id, format, status, editor_id, current_version,
    creator_approved, admin_approved, created_at, updated_at)
SELECT
    sub.assignment_id,
    sub.format::deliverable_format,
    sub.del_status::deliverable_status,
    sub.editor_id,
    sub.max_version,
    sub.creator_app,
    sub.admin_app,
    sub.min_created,
    sub.max_updated
FROM (
    SELECT
        v.assignment_id,
        v.format::text AS format,
        CASE
            WHEN a.status::text = 'published'  THEN 'published'
            WHEN a.status::text = 'in_review'   THEN 'in_review'
            WHEN a.status::text = 'in_editing'  THEN 'editing'
            ELSE 'queued'
        END AS del_status,
        a.editor_id,
        MAX(v.version) AS max_version,
        a.status::text IN ('in_review', 'published') AS creator_app,
        a.status::text = 'published' AS admin_app,
        MIN(v.created_at) AS min_created,
        MAX(v.updated_at) AS max_updated
    FROM videos v
    JOIN assignments a ON a.id = v.assignment_id
    WHERE v.format != 'raw' AND v.is_deleted = FALSE
    GROUP BY v.assignment_id, v.format, a.status, a.editor_id
) sub;

-- 12c: Non-raw videos → deliverable_versions
INSERT INTO deliverable_versions (id, deliverable_id, version, storage_key, storage_type,
    file_name, file_size_bytes, mime_type, duration_seconds, width, height,
    uploaded_by, is_current, created_at, updated_at)
SELECT v.id, d.id, v.version, v.storage_key, v.storage_type,
    v.file_name, v.file_size_bytes, v.mime_type, v.duration_seconds, v.width, v.height,
    v.uploaded_by, v.is_current, v.created_at, v.updated_at
FROM videos v
JOIN deliverables d ON d.assignment_id = v.assignment_id AND d.format::text = v.format::text
WHERE v.format != 'raw' AND v.is_deleted = FALSE;
