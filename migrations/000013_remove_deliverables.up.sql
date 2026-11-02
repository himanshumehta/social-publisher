-- Migration 000013: Remove deliverable abstraction, consolidate into Video
-- The Deliverable + DeliverableVersion layer duplicates what Video already does.
-- This migration drops deliverable columns/tables and rewires remaining FKs.

-- 1. Drop columns referencing deliverables from publish_records
ALTER TABLE publish_records DROP COLUMN IF EXISTS deliverable_id;
ALTER TABLE publish_records DROP COLUMN IF EXISTS deliverable_version_id;

-- 2. Drop deliverable_id from reviews
ALTER TABLE reviews DROP COLUMN IF EXISTS deliverable_id;

-- 3. Rewire upload_queue: deliverable_version_id → video_id
ALTER TABLE upload_queue ADD COLUMN IF NOT EXISTS video_id UUID REFERENCES videos(id);
ALTER TABLE upload_queue DROP COLUMN IF EXISTS deliverable_version_id;

-- 4. Rewire channel_targets: drop deliverable columns and update CHECK constraint
ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_content_check;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS deliverable_id;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS deliverable_version_id;

-- 5. Drop deliverable tables (CASCADE handles remaining FKs)
DROP TABLE IF EXISTS deliverable_versions CASCADE;
DROP TABLE IF EXISTS deliverables CASCADE;

-- 6. Drop deliverable enums
DROP TYPE IF EXISTS deliverable_format;
DROP TYPE IF EXISTS deliverable_status;
