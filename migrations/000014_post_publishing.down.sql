BEGIN;

DROP TRIGGER IF EXISTS trg_post_publish_links_updated_at ON post_publish_links;
DROP FUNCTION IF EXISTS update_post_publish_links_updated_at();
DROP TABLE IF EXISTS post_publish_links;
DROP TABLE IF EXISTS api_keys;

ALTER TABLE posts
    DROP COLUMN IF EXISTS source_url,
    DROP COLUMN IF EXISTS source_metadata,
    DROP COLUMN IF EXISTS platform_posts,
    DROP COLUMN IF EXISTS target_platforms,
    DROP COLUMN IF EXISTS linked_video_url,
    DROP COLUMN IF EXISTS linked_video_id,
    DROP COLUMN IF EXISTS tone,
    DROP COLUMN IF EXISTS brand_voice,
    DROP COLUMN IF EXISTS system_review_status,
    DROP COLUMN IF EXISTS system_review_notes,
    DROP COLUMN IF EXISTS system_review_at,
    DROP COLUMN IF EXISTS publish_results,
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS retry_count,
    DROP COLUMN IF EXISTS processing_started_at,
    DROP COLUMN IF EXISTS auto_publish;

DROP INDEX IF EXISTS idx_posts_pipeline_queue;
DROP INDEX IF EXISTS idx_posts_review_queue;
DROP INDEX IF EXISTS idx_posts_source_url_unique;

-- NOTE: Cannot remove enum values in PostgreSQL.
-- The added post_status values remain but are harmless.

COMMIT;
