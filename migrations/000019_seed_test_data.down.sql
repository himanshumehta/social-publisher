-- Reverse seed data (order matters for FK constraints)
DELETE FROM projects WHERE slug = 'demo-project';
DELETE FROM ai_credits WHERE user_id = 'b0000000-0000-0000-0000-000000000004';
DELETE FROM banking_info WHERE user_id = 'b0000000-0000-0000-0000-000000000004';
DELETE FROM editor_alignments WHERE id = 'd0000000-0000-0000-0000-000000000001';
-- Note: user_categories and password updates are not reversed to preserve existing state
