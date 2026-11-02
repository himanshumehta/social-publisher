BEGIN;

ALTER TABLE categories DROP COLUMN IF EXISTS auto_approve_hours;

ALTER TABLE channel_targets DROP COLUMN IF EXISTS per_channel_description;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS per_channel_title;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS reviewed_by;

ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_review_status_check;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS review_status;

ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_source_check;
ALTER TABLE channel_targets DROP COLUMN IF EXISTS source;

DROP INDEX IF EXISTS idx_channel_targets_project_channel;

-- Restore original CHECK constraint (no all-null case)
ALTER TABLE channel_targets DROP CONSTRAINT IF EXISTS channel_targets_content_check;
ALTER TABLE channel_targets ADD CONSTRAINT channel_targets_content_check CHECK (
  (video_id IS NOT NULL AND deliverable_id IS NULL AND deliverable_version_id IS NULL) OR
  (video_id IS NULL AND deliverable_id IS NULL AND deliverable_version_id IS NOT NULL) OR
  (video_id IS NULL AND deliverable_id IS NOT NULL AND deliverable_version_id IS NULL)
);

DROP INDEX IF EXISTS idx_channels_category_platform;
-- Restore unique constraint (may fail if data violates it)
-- ALTER TABLE channels ADD CONSTRAINT channels_category_id_platform_key UNIQUE (category_id, platform);

COMMIT;
