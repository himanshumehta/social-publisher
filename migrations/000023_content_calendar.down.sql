BEGIN;

DROP INDEX IF EXISTS idx_posts_scheduled_due;

ALTER TABLE posts
    DROP COLUMN IF EXISTS scheduled_at,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS pre_schedule_status;

-- Cannot remove enum values in PostgreSQL; 'scheduled' remains but is harmless.

COMMIT;
