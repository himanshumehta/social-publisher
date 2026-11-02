BEGIN;

-- Drop indexes
DROP INDEX IF EXISTS idx_posts_org;
DROP INDEX IF EXISTS idx_projects_org;
DROP INDEX IF EXISTS idx_org_invoices_org;
DROP INDEX IF EXISTS idx_org_subscriptions_org_status;
DROP INDEX IF EXISTS idx_org_members_user;
DROP INDEX IF EXISTS idx_org_members_org_status;

-- Remove org_id from projects and posts
ALTER TABLE posts    DROP COLUMN IF EXISTS org_id;
ALTER TABLE projects DROP COLUMN IF EXISTS org_id;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS org_invoices;
DROP TABLE IF EXISTS org_subscriptions;
DROP TABLE IF EXISTS org_members;
DROP TABLE IF EXISTS organizations;

COMMIT;
