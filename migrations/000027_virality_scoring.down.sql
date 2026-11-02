-- 000027_virality_scoring.down.sql

DROP INDEX IF EXISTS idx_trending_hashtags_platform;
DROP INDEX IF EXISTS idx_virality_scores_user_score;
DROP TABLE IF EXISTS trending_hashtags;
DROP TABLE IF EXISTS virality_scores;
