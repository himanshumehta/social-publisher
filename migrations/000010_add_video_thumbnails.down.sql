-- Rollback migration 000010: No-op (columns were dropped as cleanup)
-- The thumbnail_key column from init migration is untouched.

BEGIN;
-- Nothing to roll back - the dropped columns were obsolete.
COMMIT;
