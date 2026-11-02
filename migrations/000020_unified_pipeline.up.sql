-- Unified Video Publishing Pipeline: project_type, publish_mode, claim flow

-- Project type and publish mode columns
ALTER TABLE projects ADD COLUMN IF NOT EXISTS project_type VARCHAR(20) NOT NULL DEFAULT 'self';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS publish_mode VARCHAR(20) NOT NULL DEFAULT 'own_channel';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS agreement_accepted BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE projects ADD CONSTRAINT projects_project_type_check
    CHECK (project_type IN ('assignment', 'self'));
ALTER TABLE projects ADD CONSTRAINT projects_publish_mode_check
    CHECK (publish_mode IN ('official', 'own_channel'));

-- 'open' already exists in project_status enum (from migration 000005, never removed).
-- Migration 000015 remapped rows from open→active, but the enum value remains. No ALTER TYPE needed.

-- Index for assignment feed: open assignments ordered by priority
CREATE INDEX IF NOT EXISTS idx_projects_open_assignments
    ON projects(priority DESC, created_at DESC)
    WHERE status = 'open' AND project_type = 'assignment';

-- Index for posts by project (project_id column already exists on posts table)
CREATE INDEX IF NOT EXISTS idx_posts_project_id
    ON posts(project_id) WHERE project_id IS NOT NULL;

-- Backfill existing data: assignments created by admin for a different creator
UPDATE projects SET project_type = 'assignment', publish_mode = 'official'
WHERE creator_id IS NOT NULL AND created_by != creator_id;

-- Self-service projects: creator made it themselves
UPDATE projects SET project_type = 'self', publish_mode = 'own_channel'
WHERE creator_id IS NOT NULL AND created_by = creator_id;

-- Unassigned projects (admin-created, no creator yet) are assignments
UPDATE projects SET project_type = 'assignment', publish_mode = 'official'
WHERE creator_id IS NULL;
