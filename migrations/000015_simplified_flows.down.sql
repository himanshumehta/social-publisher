-- =============================================================================
-- Migration 000015 DOWN: Revert simplified flows
-- NOTE: Enum values cannot be removed in PostgreSQL without recreating the type.
-- This rollback restores columns and tables but old enum values remain.
-- =============================================================================

BEGIN;

-- Restore indexes
DROP INDEX IF EXISTS idx_projects_in_review;
DROP INDEX IF EXISTS idx_projects_active_creator;
CREATE INDEX IF NOT EXISTS idx_projects_open ON projects(status, priority DESC, created_at DESC)
    WHERE status = 'open';

-- Restore channel_targets review columns
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'creator';
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS review_status VARCHAR(20) NOT NULL DEFAULT 'pending';
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS reviewed_by UUID REFERENCES users(id);
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

-- Drop editor_alignments
DROP TABLE IF EXISTS editor_alignments;

-- Restore channels
ALTER TABLE channels DROP COLUMN IF EXISTS user_id;
ALTER TABLE channels DROP COLUMN IF EXISTS channel_type;
ALTER TABLE channels ALTER COLUMN category_id SET NOT NULL;
DROP INDEX IF EXISTS idx_channels_user_id;

-- Restore editing fields
ALTER TABLE projects DROP COLUMN IF EXISTS editing_requested;
ALTER TABLE projects DROP COLUMN IF EXISTS editing_notes;
ALTER TABLE projects DROP COLUMN IF EXISTS editing_completed_at;

-- Restore workflow_type
ALTER TABLE projects ADD COLUMN IF NOT EXISTS workflow_type TEXT NOT NULL DEFAULT 'standard';
ALTER TABLE projects ADD CONSTRAINT projects_workflow_type_check
    CHECK (workflow_type IN ('standard', 'direct_publish', 'auto_clip'));
CREATE INDEX IF NOT EXISTS idx_projects_workflow_type ON projects(workflow_type);

COMMIT;
