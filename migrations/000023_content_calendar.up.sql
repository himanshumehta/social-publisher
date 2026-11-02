-- Add 'scheduled' to post_status enum
-- NOTE: ALTER TYPE ... ADD VALUE cannot run inside a transaction block.
ALTER TYPE post_status ADD VALUE IF NOT EXISTS 'scheduled';

BEGIN;

-- Add scheduling columns to posts
ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS timezone VARCHAR(50) DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS pre_schedule_status VARCHAR(50);

-- Partial index for the schedule worker to efficiently find due posts
CREATE INDEX IF NOT EXISTS idx_posts_scheduled_due
    ON posts (scheduled_at)
    WHERE status = 'scheduled';

COMMIT;
