-- 000030_marketplace_enhancements.up.sql

CREATE TABLE IF NOT EXISTS editor_portfolios (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    editor_id       UUID          NOT NULL REFERENCES users(id) UNIQUE,
    bio             TEXT          DEFAULT '',
    specializations TEXT[]        DEFAULT '{}',
    years_experience INT          DEFAULT 0,
    hourly_rate     INT           DEFAULT 0,
    availability    VARCHAR(20)   DEFAULT 'available',
    portfolio_url   TEXT,
    demo_reel_url   TEXT,
    is_featured     BOOLEAN       DEFAULT FALSE,
    created_at      TIMESTAMPTZ   DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS portfolio_items (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    portfolio_id    UUID          NOT NULL REFERENCES editor_portfolios(id) ON DELETE CASCADE,
    title           VARCHAR(200)  NOT NULL,
    description     TEXT          DEFAULT '',
    thumbnail_url   TEXT,
    media_url       TEXT,
    media_type      VARCHAR(20)   DEFAULT 'video',
    platform        VARCHAR(20),
    view_count      INT           DEFAULT 0,
    sort_order      INT           DEFAULT 0,
    created_at      TIMESTAMPTZ   DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS editor_skills (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    editor_id       UUID          NOT NULL REFERENCES users(id),
    skill_name      VARCHAR(50)   NOT NULL,
    proficiency     VARCHAR(20)   DEFAULT 'intermediate',
    verified        BOOLEAN       DEFAULT FALSE,
    UNIQUE(editor_id, skill_name)
);

CREATE TABLE IF NOT EXISTS marketplace_templates (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    creator_id      UUID          NOT NULL REFERENCES users(id),
    title           VARCHAR(200)  NOT NULL,
    description     TEXT          DEFAULT '',
    category        VARCHAR(50)   NOT NULL,
    brief_template  TEXT          NOT NULL,
    price           INT           NOT NULL DEFAULT 0,
    purchase_count  INT           DEFAULT 0,
    rating_avg      NUMERIC(3,2)  DEFAULT 0,
    is_active       BOOLEAN       DEFAULT TRUE,
    created_at      TIMESTAMPTZ   DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   DEFAULT NOW()
);

CREATE INDEX idx_editor_skills_editor_id ON editor_skills (editor_id);
CREATE INDEX idx_editor_skills_skill_name ON editor_skills (skill_name);
CREATE INDEX idx_portfolio_items_portfolio_sort ON portfolio_items (portfolio_id, sort_order);
CREATE INDEX idx_marketplace_templates_category_active ON marketplace_templates (category, is_active);
