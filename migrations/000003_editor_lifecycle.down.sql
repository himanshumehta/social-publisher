DROP INDEX IF EXISTS idx_videos_status;

DROP INDEX IF EXISTS idx_reviews_parent;

ALTER TABLE reviews DROP COLUMN IF EXISTS parent_id;

ALTER TABLE videos DROP COLUMN IF EXISTS status;

DROP TYPE IF EXISTS video_status;
