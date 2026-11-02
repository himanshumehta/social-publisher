-- =============================================================================
-- 000008_rename_assignment_to_project.down.sql
-- Reverse: rename "project" back to "assignment"
-- =============================================================================

BEGIN;

-- =============================================================================
-- 1. Revert notification data
-- =============================================================================
UPDATE notifications SET link = REPLACE(link, '/projects/', '/assignments/') WHERE link LIKE '%/projects/%';
UPDATE notifications SET type = 'assignment_claimed' WHERE type = 'project_claimed';

-- =============================================================================
-- 2. Rename columns back
-- =============================================================================
ALTER TABLE deliverables RENAME COLUMN project_id TO assignment_id;
ALTER TABLE raw_assets RENAME COLUMN project_id TO assignment_id;
ALTER TABLE archive_tasks RENAME COLUMN project_id TO assignment_id;
ALTER TABLE posts RENAME COLUMN project_id TO assignment_id;
ALTER TABLE publish_records RENAME COLUMN project_id TO assignment_id;
ALTER TABLE reviews RENAME COLUMN project_id TO assignment_id;
ALTER TABLE multipart_uploads RENAME COLUMN project_id TO assignment_id;
ALTER TABLE videos RENAME COLUMN project_id TO assignment_id;

-- =============================================================================
-- 3. Rename table back
-- =============================================================================
ALTER TABLE projects RENAME TO assignments;

-- =============================================================================
-- 4. Rename indexes back
-- =============================================================================
ALTER INDEX deliverables_project_id_format_key RENAME TO deliverables_assignment_id_format_key;
ALTER INDEX idx_deliverables_project RENAME TO idx_deliverables_assignment;
ALTER INDEX idx_raw_assets_project RENAME TO idx_raw_assets_assignment;
ALTER INDEX idx_archive_tasks_project RENAME TO idx_archive_tasks_assignment;
ALTER INDEX idx_publish_records_project RENAME TO idx_publish_records_assignment;
ALTER INDEX idx_reviews_project RENAME TO idx_reviews_assignment;
ALTER INDEX idx_multipart_uploads_project RENAME TO idx_multipart_uploads_assignment;
ALTER INDEX idx_videos_project RENAME TO idx_videos_assignment;
ALTER INDEX idx_projects_open RENAME TO idx_assignments_open;
ALTER INDEX idx_projects_editor RENAME TO idx_assignments_editor;
ALTER INDEX idx_projects_creator RENAME TO idx_assignments_creator;
ALTER INDEX idx_projects_status RENAME TO idx_assignments_status;
ALTER INDEX idx_projects_category RENAME TO idx_assignments_category;

-- =============================================================================
-- 5. Rename audit_action enum values back
-- =============================================================================
ALTER TYPE audit_action RENAME VALUE 'project_create' TO 'assignment_create';
ALTER TYPE audit_action RENAME VALUE 'project_update' TO 'assignment_update';
ALTER TYPE audit_action RENAME VALUE 'project_claim' TO 'assignment_claim';
ALTER TYPE audit_action RENAME VALUE 'project_cancel' TO 'assignment_cancel';

-- =============================================================================
-- 6. Rename enum type back
-- =============================================================================
ALTER TYPE project_status RENAME TO assignment_status;

COMMIT;
