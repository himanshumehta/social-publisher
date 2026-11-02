-- =============================================================================
-- 000008_rename_assignment_to_project.up.sql
-- Rename "assignment" domain concept to "project" across entire schema
-- =============================================================================

BEGIN;

-- =============================================================================
-- 1. Rename enum type
-- =============================================================================
ALTER TYPE assignment_status RENAME TO project_status;

-- =============================================================================
-- 2. Rename audit_action enum values
-- =============================================================================
ALTER TYPE audit_action RENAME VALUE 'assignment_create' TO 'project_create';
ALTER TYPE audit_action RENAME VALUE 'assignment_update' TO 'project_update';
ALTER TYPE audit_action RENAME VALUE 'assignment_claim' TO 'project_claim';
ALTER TYPE audit_action RENAME VALUE 'assignment_cancel' TO 'project_cancel';

-- =============================================================================
-- 3. Rename indexes (before table/column renames for clarity)
-- =============================================================================
ALTER INDEX idx_assignments_category RENAME TO idx_projects_category;
ALTER INDEX idx_assignments_status RENAME TO idx_projects_status;
ALTER INDEX idx_assignments_creator RENAME TO idx_projects_creator;
ALTER INDEX idx_assignments_editor RENAME TO idx_projects_editor;
ALTER INDEX idx_assignments_open RENAME TO idx_projects_open;
ALTER INDEX idx_videos_assignment RENAME TO idx_videos_project;
ALTER INDEX idx_multipart_uploads_assignment RENAME TO idx_multipart_uploads_project;
ALTER INDEX idx_reviews_assignment RENAME TO idx_reviews_project;
ALTER INDEX idx_publish_records_assignment RENAME TO idx_publish_records_project;
ALTER INDEX idx_archive_tasks_assignment RENAME TO idx_archive_tasks_project;
ALTER INDEX idx_raw_assets_assignment RENAME TO idx_raw_assets_project;
ALTER INDEX idx_deliverables_assignment RENAME TO idx_deliverables_project;

-- Rename auto-generated unique constraint index
ALTER INDEX deliverables_assignment_id_format_key RENAME TO deliverables_project_id_format_key;

-- =============================================================================
-- 4. Rename table
-- =============================================================================
ALTER TABLE assignments RENAME TO projects;

-- =============================================================================
-- 5. Rename columns in related tables
-- =============================================================================
ALTER TABLE videos RENAME COLUMN assignment_id TO project_id;
ALTER TABLE multipart_uploads RENAME COLUMN assignment_id TO project_id;
ALTER TABLE reviews RENAME COLUMN assignment_id TO project_id;
ALTER TABLE publish_records RENAME COLUMN assignment_id TO project_id;
ALTER TABLE posts RENAME COLUMN assignment_id TO project_id;
ALTER TABLE archive_tasks RENAME COLUMN assignment_id TO project_id;
ALTER TABLE raw_assets RENAME COLUMN assignment_id TO project_id;
ALTER TABLE deliverables RENAME COLUMN assignment_id TO project_id;

-- =============================================================================
-- 6. Update notification data
-- =============================================================================
UPDATE notifications SET type = 'project_claimed' WHERE type = 'assignment_claimed';
UPDATE notifications SET link = REPLACE(link, '/assignments/', '/projects/') WHERE link LIKE '%/assignments/%';

COMMIT;
