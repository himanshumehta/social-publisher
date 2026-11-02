-- Seed data for E2E testing against a pre-populated database.
-- Guarded: this is a no-op on a fresh database (the referenced users do not exist).
-- Uses existing user/category/channel UUIDs from prior seed data
-- Password for all test users: TestPass123

-- ── UUID Reference (from existing data) ────────────────────────────
-- Users:
--   super-admin@example.com  = b0000000-0000-0000-0000-000000000001
--   admin@example.com       = b0000000-0000-0000-0000-000000000002
--   editor@example.com      = b0000000-0000-0000-0000-000000000003
--   creator@example.com     = b0000000-0000-0000-0000-000000000004
--   creator2@example.com    = b0000000-0000-0000-0000-000000000005
-- Categories:
--   general                = a0000000-0000-0000-0000-000000000001
-- Channels:
--   youtube (Indian)       = e0000000-0000-0000-0000-000000000001
--   instagram              = e0000000-0000-0000-0000-000000000002
-- Creator profiles:
--   creator (demo-creator) = c0000000-0000-0000-0000-000000000001
--   creator2 (demo-creator-2)= c0000000-0000-0000-0000-000000000002

DO $seed$
BEGIN
IF EXISTS (SELECT 1 FROM users WHERE id = 'b0000000-0000-0000-0000-000000000004')
   AND EXISTS (SELECT 1 FROM categories WHERE id = 'a0000000-0000-0000-0000-000000000001') THEN

-- 1. Update all user passwords to TestPass123 & ensure active + onboarded
UPDATE users SET
  password_hash = '$2a$10$mpn6RGYfh79XRa1cdsNxK.6HLI5AcDeJLeYGjFvVn76gxv1gtBqau',
  is_active = TRUE,
  is_onboarded = TRUE
WHERE email IN ('super-admin@example.com', 'admin@example.com', 'editor@example.com', 'creator@example.com', 'creator2@example.com');

-- 2. Ensure user-category associations exist (general)
INSERT INTO user_categories (user_id, category_id)
VALUES
  ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001'),
  ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001'),
  ('b0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001'),
  ('b0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000001'),
  ('b0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

-- 3. Editor alignment (editor → creator, aligned_by superadmin)
INSERT INTO editor_alignments (id, creator_id, editor_id, aligned_by, is_active)
VALUES
  ('d0000000-0000-0000-0000-000000000001',
   'b0000000-0000-0000-0000-000000000004',
   'b0000000-0000-0000-0000-000000000003',
   'b0000000-0000-0000-0000-000000000001',
   TRUE)
ON CONFLICT DO NOTHING;

-- 4. Banking info for creator (verified — needed for payouts)
INSERT INTO banking_info (id, user_id, account_holder, bank_name, ifsc_code, account_number, upi_id, status)
VALUES
  ('d0000000-0000-0000-0000-000000000010',
   'b0000000-0000-0000-0000-000000000004',
   'Demo Creator', 'Example Bank', 'EXMP0000000', '0000000000000000', 'demo@upi', 'verified')
ON CONFLICT (user_id) DO NOTHING;

-- 5. AI credits for creator (start with 50 credits)
INSERT INTO ai_credits (user_id, balance, total_purchased, total_used)
VALUES
  ('b0000000-0000-0000-0000-000000000004', 50, 50, 0)
ON CONFLICT (user_id) DO NOTHING;

-- 6. Sample project (active, ready for testing)
INSERT INTO projects (id, category_id, creator_id, created_by, title, slug, description, status, editing_requested)
VALUES
  ('d0000000-0000-0000-0000-000000000100',
   'a0000000-0000-0000-0000-000000000001',
   'b0000000-0000-0000-0000-000000000004',
   'b0000000-0000-0000-0000-000000000004',
   'Demo Project',
   'demo-project',
   'Sample project for local testing.',
   'active',
   FALSE)
ON CONFLICT (slug) DO NOTHING;

END IF;
END
$seed$;
