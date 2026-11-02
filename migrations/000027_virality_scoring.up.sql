-- 000027_virality_scoring.up.sql

CREATE TABLE virality_scores (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id    UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE UNIQUE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    score      INT  NOT NULL DEFAULT 0 CHECK (score >= 0 AND score <= 100),
    factors    JSONB NOT NULL DEFAULT '{}',
    suggestions TEXT[] DEFAULT '{}',
    scored_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE trending_hashtags (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hashtag      VARCHAR(100) NOT NULL,
    platform     VARCHAR(20)  NOT NULL,
    trend_score  NUMERIC(10,2) DEFAULT 0,
    sample_count INT DEFAULT 0,
    first_seen   TIMESTAMPTZ DEFAULT NOW(),
    last_seen    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (hashtag, platform)
);

CREATE INDEX idx_virality_scores_user_score ON virality_scores (user_id, score DESC);
CREATE INDEX idx_trending_hashtags_platform  ON trending_hashtags (platform, trend_score DESC);
