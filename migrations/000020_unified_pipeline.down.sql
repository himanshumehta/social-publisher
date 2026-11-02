-- Rollback unified pipeline columns
DROP INDEX IF EXISTS idx_projects_open_assignments;
DROP INDEX IF EXISTS idx_posts_project_id;

ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_project_type_check;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_publish_mode_check;

ALTER TABLE projects DROP COLUMN IF EXISTS project_type;
ALTER TABLE projects DROP COLUMN IF EXISTS publish_mode;
ALTER TABLE projects DROP COLUMN IF EXISTS claimed_at;
ALTER TABLE projects DROP COLUMN IF EXISTS agreement_accepted;
