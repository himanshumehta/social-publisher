-- 000024_cross_platform_analytics.down.sql

DROP INDEX IF EXISTS idx_analytics_snapshots_platform_date;
DROP INDEX IF EXISTS idx_analytics_snapshots_user_date;
DROP TABLE IF EXISTS analytics_snapshots;
