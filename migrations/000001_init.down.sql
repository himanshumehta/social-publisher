-- Reverse of 000001_init.up.sql — drops everything in dependency order.

DROP TRIGGER IF EXISTS trg_posts_search_vector ON posts;
DROP FUNCTION IF EXISTS posts_search_vector_update();

DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS posts;
DROP TABLE IF EXISTS payouts;
DROP TABLE IF EXISTS youtube_analytics;
DROP TABLE IF EXISTS publish_records;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS multipart_uploads;
DROP TABLE IF EXISTS videos;
DROP TABLE IF EXISTS assignments;
DROP TABLE IF EXISTS banking_info;
DROP TABLE IF EXISTS creator_categories;
DROP TABLE IF EXISTS creator_profiles;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS oauth_accounts;
DROP TABLE IF EXISTS user_categories;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS categories;

DROP TYPE IF EXISTS audit_action;
DROP TYPE IF EXISTS post_status;
DROP TYPE IF EXISTS payout_status;
DROP TYPE IF EXISTS review_action;
DROP TYPE IF EXISTS video_format;
DROP TYPE IF EXISTS assignment_status;
DROP TYPE IF EXISTS banking_status;
DROP TYPE IF EXISTS oauth_provider;
DROP TYPE IF EXISTS user_role;
