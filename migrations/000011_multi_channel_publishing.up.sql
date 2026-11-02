BEGIN;

-- 1. Drop unique constraint to allow multiple channels per platform per category
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_category_id_platform_key;
CREATE INDEX IF NOT EXISTS idx_channels_category_platform ON channels(category_id, platform);

-- 2. Relax channel_targets CHECK to allow project-level targeting (all content refs NULL)
ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_content_check;
ALTER TABLE channel_targets ADD CONSTRAINT channel_targets_content_check CHECK (
  (video_id IS NOT NULL AND deliverable_id IS NULL AND deliverable_version_id IS NULL) OR
  (video_id IS NULL AND deliverable_id IS NOT NULL AND deliverable_version_id IS NULL) OR
  (video_id IS NULL AND deliverable_id IS NULL AND deliverable_version_id IS NOT NULL) OR
  (video_id IS NULL AND deliverable_id IS NULL AND deliverable_version_id IS NULL)
);

-- 3. Add unique index for project-level targets (prevent duplicate project+channel combos)
CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_targets_project_channel
ON channel_targets(project_id, channel_id)
WHERE video_id IS NULL AND deliverable_id IS NULL AND deliverable_version_id IS NULL;

-- 4. Add review workflow columns to channel_targets
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'creator';
ALTER TABLE channel_targets ADD CONSTRAINT channel_targets_source_check
  CHECK (source IN ('creator', 'admin', 'suggested'));

ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS review_status VARCHAR(20) NOT NULL DEFAULT 'pending';
ALTER TABLE channel_targets ADD CONSTRAINT channel_targets_review_status_check
  CHECK (review_status IN ('pending', 'approved', 'rejected'));

ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS reviewed_by UUID REFERENCES users(id);
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS per_channel_title TEXT;
ALTER TABLE channel_targets ADD COLUMN IF NOT EXISTS per_channel_description TEXT;

-- 5. Add auto-approve config to categories
ALTER TABLE categories ADD COLUMN IF NOT EXISTS auto_approve_hours INT NOT NULL DEFAULT 24;

COMMIT;
