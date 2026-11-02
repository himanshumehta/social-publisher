-- 000024_cross_platform_analytics.up.sql

CREATE TABLE IF NOT EXISTS analytics_snapshots (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID          NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id      UUID          REFERENCES channels(id) ON DELETE SET NULL,
    platform        VARCHAR(20)   NOT NULL,
    snapshot_date   DATE          NOT NULL,
    followers       INT           DEFAULT 0,
    views           INT           DEFAULT 0,
    likes           INT           DEFAULT 0,
    comments        INT           DEFAULT 0,
    shares          INT           DEFAULT 0,
    engagement_rate NUMERIC(8,4)  DEFAULT 0,
    impressions     INT           DEFAULT 0,
    reach           INT           DEFAULT 0,
    clicks          INT           DEFAULT 0,
    revenue_paise   BIGINT        DEFAULT 0,
    metadata        JSONB         DEFAULT '{}',
    created_at      TIMESTAMPTZ   DEFAULT NOW(),

    CONSTRAINT uq_channel_platform_date UNIQUE (channel_id, platform, snapshot_date)
);

CREATE INDEX idx_analytics_snapshots_user_date
    ON analytics_snapshots (user_id, snapshot_date);

CREATE INDEX idx_analytics_snapshots_platform_date
    ON analytics_snapshots (platform, snapshot_date);
