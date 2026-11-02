-- 000031_revenue_attribution.up.sql

BEGIN;

-- ---------------------------------------------------------------
-- revenue_sources: individual revenue entries by source and period
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS revenue_sources (
    id           UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         VARCHAR(100)  NOT NULL,
    source_type  VARCHAR(30)   NOT NULL,
    platform     VARCHAR(20),
    amount_paise BIGINT        NOT NULL DEFAULT 0,
    currency     VARCHAR(3)    DEFAULT 'INR',
    period_start DATE          NOT NULL,
    period_end   DATE          NOT NULL,
    notes        TEXT          DEFAULT '',
    metadata     JSONB         DEFAULT '{}',
    created_at   TIMESTAMPTZ   DEFAULT NOW(),
    updated_at   TIMESTAMPTZ   DEFAULT NOW()
);

CREATE INDEX idx_revenue_sources_user_period
    ON revenue_sources (user_id, period_start);

CREATE INDEX idx_revenue_sources_source_type
    ON revenue_sources (source_type);

-- ---------------------------------------------------------------
-- revenue_goals: time-bound revenue targets
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS revenue_goals (
    id                   UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                 VARCHAR(100)  NOT NULL,
    target_amount_paise  BIGINT        NOT NULL,
    current_amount_paise BIGINT        NOT NULL DEFAULT 0,
    period               VARCHAR(20)   NOT NULL DEFAULT 'monthly',
    period_start         DATE          NOT NULL,
    period_end           DATE          NOT NULL,
    source_types         TEXT[]        DEFAULT '{}',
    status               VARCHAR(20)   DEFAULT 'active',
    achieved_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ   DEFAULT NOW(),
    updated_at           TIMESTAMPTZ   DEFAULT NOW()
);

CREATE INDEX idx_revenue_goals_user_status
    ON revenue_goals (user_id, status);

-- ---------------------------------------------------------------
-- affiliate_links: trackable affiliate/referral links
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS affiliate_links (
    id               UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             VARCHAR(100)  NOT NULL,
    original_url     TEXT          NOT NULL,
    tracking_code    VARCHAR(50)   NOT NULL UNIQUE,
    short_url        TEXT,
    click_count      INT           DEFAULT 0,
    conversion_count INT           DEFAULT 0,
    revenue_paise    BIGINT        DEFAULT 0,
    is_active        BOOLEAN       DEFAULT TRUE,
    created_at       TIMESTAMPTZ   DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   DEFAULT NOW()
);

CREATE INDEX idx_affiliate_links_user
    ON affiliate_links (user_id);

CREATE INDEX idx_affiliate_links_tracking_code
    ON affiliate_links (tracking_code);

COMMIT;
