-- Project Assets: stores creator kit files uploaded by content-agent
CREATE TABLE IF NOT EXISTS project_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    category VARCHAR(50) NOT NULL,
    subcategory VARCHAR(100) DEFAULT '',
    filename VARCHAR(255) NOT NULL,
    file_url TEXT NOT NULL,
    thumbnail_url TEXT,
    alt_text TEXT DEFAULT '',
    file_size BIGINT DEFAULT 0,
    mime_type VARCHAR(100) DEFAULT '',
    metadata JSONB DEFAULT '{}',
    sort_order INT DEFAULT 0,
    status VARCHAR(20) DEFAULT 'active',
    version INT DEFAULT 1,
    uploaded_by UUID REFERENCES users(id),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_project_assets_project ON project_assets(project_id);
CREATE INDEX idx_project_assets_category ON project_assets(project_id, category, subcategory);
CREATE INDEX idx_project_assets_status ON project_assets(project_id, status);
CREATE INDEX idx_project_assets_filename ON project_assets(project_id, filename);

-- Changelog for asset operations
CREATE TABLE IF NOT EXISTS project_asset_changelog (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    action VARCHAR(20) NOT NULL,
    category VARCHAR(50),
    asset_count INT DEFAULT 1,
    summary TEXT,
    triggered_by VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_changelog_project ON project_asset_changelog(project_id, created_at DESC);

-- Add kit tracking columns to projects
ALTER TABLE projects ADD COLUMN IF NOT EXISTS source_vehicle_slug VARCHAR(255);
ALTER TABLE projects ADD COLUMN IF NOT EXISTS kit_status VARCHAR(20) DEFAULT 'not_started';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS kit_progress JSONB DEFAULT '{}';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS kit_completed_at TIMESTAMPTZ;

CREATE INDEX idx_projects_source_slug ON projects(source_vehicle_slug) WHERE source_vehicle_slug IS NOT NULL;
