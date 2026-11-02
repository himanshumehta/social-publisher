-- Migration 000010: Clean up obsolete thumbnail columns
-- The videos table already has thumbnail_key from the init migration.
-- This migration drops the incorrect thumbnail_url and thumbnail_storage_key
-- columns if they were added by a previous version of this migration.

BEGIN;

-- Drop obsolete columns (safe: IF EXISTS handles fresh installs)
ALTER TABLE videos DROP COLUMN IF EXISTS thumbnail_url;
ALTER TABLE videos DROP COLUMN IF EXISTS thumbnail_storage_key;
DROP INDEX IF EXISTS idx_videos_thumbnail_storage_key;

COMMIT;
