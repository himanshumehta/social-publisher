-- =============================================================================
-- Migration 000015: Simplified Flows
-- Implements Flows 1-5: simplified statuses, creator channels, editor alignment
-- =============================================================================

-- 1. Add new project_status enum values (must be outside transaction)
ALTER TYPE project_status ADD VALUE IF NOT EXISTS 'active';
ALTER TYPE project_status ADD VALUE IF NOT EXISTS 'archived';

BEGIN;

-- =============================================================================
-- 2. Remap existing project statuses to new simplified set
-- =============================================================================
UPDATE projects SET status = 'active' WHERE status IN ('draft', 'open', 'claimed', 'in_production', 'in_editing');
UPDATE projects SET status = 'archived' WHERE status IN ('cancelled', 'on_hold');
-- 'in_review' and 'published' remain unchanged

-- Remap review audit rows
UPDATE reviews SET from_status = 'active'
WHERE from_status::text IN ('draft', 'open', 'claimed', 'in_production', 'in_editing');
UPDATE reviews SET to_status = 'active'
WHERE to_status::text IN ('draft', 'open', 'claimed', 'in_production', 'in_editing');
UPDATE reviews SET from_status = 'archived'
WHERE from_status::text IN ('cancelled', 'on_hold');
UPDATE reviews SET to_status = 'archived'
WHERE to_status::text IN ('cancelled', 'on_hold');

-- =============================================================================
-- 3. Drop workflow_type from projects
-- =============================================================================
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_workflow_type_check;
DROP INDEX IF EXISTS idx_projects_workflow_type;
ALTER TABLE projects DROP COLUMN IF EXISTS workflow_type;

-- =============================================================================
-- 4. Add editing readiness fields to projects (Flow 3)
-- =============================================================================
ALTER TABLE projects ADD COLUMN IF NOT EXISTS editing_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS editing_notes TEXT;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS editing_completed_at TIMESTAMPTZ;

-- =============================================================================
-- 5. Creator channel linking (Flow 4) — modify channels table
-- =============================================================================
-- Add owner concept: NULL = system channel, SET = creator-owned
ALTER TABLE channels ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id);
ALTER TABLE channels ADD COLUMN IF NOT EXISTS channel_type VARCHAR(20) NOT NULL DEFAULT 'system';
ALTER TABLE channels ADD CONSTRAINT channels_channel_type_check
    CHECK (channel_type IN ('system', 'creator'));

-- Creator channels don't need category_id
ALTER TABLE channels ALTER COLUMN category_id DROP NOT NULL;

-- Index for fast creator channel lookup
CREATE INDEX IF NOT EXISTS idx_channels_user_id ON channels(user_id) WHERE user_id IS NOT NULL;

-- Mark existing channels as system
UPDATE channels SET channel_type = 'system' WHERE user_id IS NULL;

-- =============================================================================
-- 6. Editor alignment model (Flow 3) — new table
-- =============================================================================
CREATE TABLE IF NOT EXISTS editor_alignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id UUID NOT NULL REFERENCES users(id),
    editor_id UUID NOT NULL REFERENCES users(id),
    aligned_by UUID NOT NULL REFERENCES users(id),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(creator_id, editor_id)
);

CREATE INDEX IF NOT EXISTS idx_editor_alignments_creator ON editor_alignments(creator_id) WHERE is_active = TRUE;
CREATE INDEX IF NOT EXISTS idx_editor_alignments_editor ON editor_alignments(editor_id) WHERE is_active = TRUE;

-- =============================================================================
-- 7. Simplify channel_targets — drop per-target review (Flow 5)
-- =============================================================================
ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_review_status_check;
ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_source_check;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS review_status;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS reviewed_by;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS source;

-- =============================================================================
-- 8. Update indexes for new status model
-- =============================================================================
-- Drop old open-projects index (no more 'open' status)
DROP INDEX IF EXISTS idx_projects_open;

-- New index for review queue
CREATE INDEX IF NOT EXISTS idx_projects_in_review
    ON projects(updated_at ASC)
    WHERE status = 'in_review';

-- New index for active projects by creator
CREATE INDEX IF NOT EXISTS idx_projects_active_creator
    ON projects(creator_id, updated_at DESC)
    WHERE status = 'active';

COMMIT;
