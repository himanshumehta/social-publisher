-- Migration 000013 DOWN: Recreate deliverable types, tables, and columns

-- 1. Recreate enums
CREATE TYPE deliverable_status AS ENUM ('queued', 'editing', 'in_review', 'revision', 'approved', 'published');
CREATE TYPE deliverable_format AS ENUM ('youtube_16_9', 'instagram_reel', 'youtube_short', 'facebook_feed', 'facebook_reel');

-- 2. Recreate deliverables table
CREATE TABLE IF NOT EXISTS deliverables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    format deliverable_format NOT NULL,
    status deliverable_status NOT NULL DEFAULT 'queued',
    editor_id UUID REFERENCES users(id),
    current_version INT NOT NULL DEFAULT 0,
    creator_approved BOOLEAN NOT NULL DEFAULT FALSE,
    admin_approved BOOLEAN NOT NULL DEFAULT FALSE,
    total_revenue_paise BIGINT NOT NULL DEFAULT 0,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 3. Recreate deliverable_versions table
CREATE TABLE IF NOT EXISTS deliverable_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deliverable_id UUID NOT NULL REFERENCES deliverables(id) ON DELETE CASCADE,
    version INT NOT NULL,
    storage_key TEXT NOT NULL,
    storage_type TEXT NOT NULL DEFAULT 'hot',
    file_name TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL DEFAULT 0,
    mime_type TEXT NOT NULL DEFAULT 'video/mp4',
    duration_seconds INT,
    width INT,
    height INT,
    uploaded_by UUID NOT NULL REFERENCES users(id),
    is_current BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 4. Re-add columns to publish_records
ALTER TABLE publish_records ADD COLUMN IF NOT EXISTS deliverable_id UUID REFERENCES deliverables(id);
ALTER TABLE publish_records ADD COLUMN IF NOT EXISTS deliverable_version_id UUID REFERENCES deliverable_versions(id);

-- 5. Re-add deliverable_id to reviews
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS deliverable_id UUID REFERENCES deliverables(id);

-- 6. Rewire upload_queue: video_id → deliverable_version_id
ALTER TABLE upload_queue ADD COLUMN IF NOT EXISTS deliverable_version_id UUID REFERENCES deliverable_versions(id);
ALTER TABLE upload_queue DROP COLUMN IF EXISTS video_id;

-- 7. Re-add deliverable columns to channel_targets
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS deliverable_id UUID REFERENCES deliverables(id);
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS deliverable_version_id UUID REFERENCES deliverable_versions(id);
