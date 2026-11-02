-- 000031_revenue_attribution.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_affiliate_links_tracking_code;
DROP INDEX IF EXISTS idx_affiliate_links_user;
DROP TABLE IF EXISTS affiliate_links;

DROP INDEX IF EXISTS idx_revenue_goals_user_status;
DROP TABLE IF EXISTS revenue_goals;

DROP INDEX IF EXISTS idx_revenue_sources_source_type;
DROP INDEX IF EXISTS idx_revenue_sources_user_period;
DROP TABLE IF EXISTS revenue_sources;

COMMIT;
