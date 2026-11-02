-- 000030_marketplace_enhancements.down.sql

DROP INDEX IF EXISTS idx_marketplace_templates_category_active;
DROP INDEX IF EXISTS idx_portfolio_items_portfolio_sort;
DROP INDEX IF EXISTS idx_editor_skills_skill_name;
DROP INDEX IF EXISTS idx_editor_skills_editor_id;

DROP TABLE IF EXISTS marketplace_templates;
DROP TABLE IF EXISTS editor_skills;
DROP TABLE IF EXISTS portfolio_items;
DROP TABLE IF EXISTS editor_portfolios;
