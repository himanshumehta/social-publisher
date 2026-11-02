DELETE FROM revenue_config;
DROP TABLE IF EXISTS revenue_config;
ALTER TABLE channels DROP COLUMN IF EXISTS analytics_sync_error;
ALTER TABLE channels DROP COLUMN IF EXISTS last_analytics_sync;
