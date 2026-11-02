-- =============================================================================
-- 000005_state_refactor.down.sql — Rollback to old 13-state model
-- =============================================================================

-- Remove deliverable_id from reviews
DROP INDEX IF EXISTS idx_reviews_deliverable;
ALTER TABLE reviews DROP COLUMN IF EXISTS deliverable_id;

-- Drop new tables
DROP TABLE IF EXISTS deliverable_versions;
DROP TABLE IF EXISTS deliverables;
DROP TABLE IF EXISTS raw_assets;

-- Recreate old assignment_status enum
CREATE TYPE assignment_status_old AS ENUM (
    'draft', 'open', 'claimed', 'raw_uploaded', 'editing', 'edited',
    'creator_review', 'creator_approved', 'admin_review',
    'approved', 'published', 'rejected', 'cancelled'
);

DROP INDEX IF EXISTS idx_assignments_open;

-- Reverse-migrate assignments.status
ALTER TABLE assignments
    ALTER COLUMN status TYPE assignment_status_old
    USING (
        CASE status::text
            WHEN 'in_production' THEN 'raw_uploaded'
            WHEN 'in_editing'    THEN 'editing'
            WHEN 'in_review'     THEN 'admin_review'
            WHEN 'on_hold'       THEN 'draft'
            ELSE status::text
        END::assignment_status_old
    );
ALTER TABLE assignments ALTER COLUMN status SET DEFAULT 'draft'::assignment_status_old;

-- Reverse-migrate reviews
ALTER TABLE reviews ALTER COLUMN from_status TYPE text USING from_status::text;
ALTER TABLE reviews ALTER COLUMN to_status TYPE text USING to_status::text;

UPDATE reviews SET from_status = CASE from_status
    WHEN 'in_production' THEN 'raw_uploaded'
    WHEN 'in_editing'    THEN 'editing'
    WHEN 'in_review'     THEN 'admin_review'
    WHEN 'on_hold'       THEN 'draft'
    ELSE from_status
END WHERE from_status IS NOT NULL;

UPDATE reviews SET to_status = CASE to_status
    WHEN 'in_production' THEN 'raw_uploaded'
    WHEN 'in_editing'    THEN 'editing'
    WHEN 'in_review'     THEN 'admin_review'
    WHEN 'on_hold'       THEN 'draft'
    ELSE to_status
END WHERE to_status IS NOT NULL;

ALTER TABLE reviews
    ALTER COLUMN from_status TYPE assignment_status_old USING from_status::assignment_status_old;
ALTER TABLE reviews
    ALTER COLUMN to_status TYPE assignment_status_old USING to_status::assignment_status_old;

-- Swap types
DROP TYPE assignment_status;
ALTER TYPE assignment_status_old RENAME TO assignment_status;

-- Recreate index
CREATE INDEX idx_assignments_open ON assignments(status, priority DESC, created_at DESC)
    WHERE status = 'open';

-- Drop new enum types
DROP TYPE IF EXISTS deliverable_format;
DROP TYPE IF EXISTS deliverable_status;
DROP TYPE IF EXISTS raw_asset_status;
