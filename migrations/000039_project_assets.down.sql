DROP INDEX IF EXISTS idx_projects_source_slug;
ALTER TABLE projects DROP COLUMN IF EXISTS kit_completed_at;
ALTER TABLE projects DROP COLUMN IF EXISTS kit_progress;
ALTER TABLE projects DROP COLUMN IF EXISTS kit_status;
ALTER TABLE projects DROP COLUMN IF EXISTS source_vehicle_slug;
DROP TABLE IF EXISTS project_asset_changelog;
DROP TABLE IF EXISTS project_assets;
