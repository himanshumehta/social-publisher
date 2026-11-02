-- A/B Testing tables
CREATE TABLE ab_tests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    original_post_id UUID NOT NULL REFERENCES posts(id),
    name            VARCHAR(200) NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    metric          VARCHAR(30) NOT NULL DEFAULT 'engagement',
    duration_hours  INT NOT NULL DEFAULT 24,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    winner_variant_id UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ab_tests_status_check
        CHECK (status IN ('draft', 'running', 'evaluating', 'completed', 'cancelled')),
    CONSTRAINT ab_tests_metric_check
        CHECK (metric IN ('engagement', 'clicks', 'impressions', 'reach')),
    CONSTRAINT ab_tests_duration_check
        CHECK (duration_hours >= 1 AND duration_hours <= 720)
);

CREATE TABLE ab_test_variants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ab_test_id      UUID NOT NULL REFERENCES ab_tests(id) ON DELETE CASCADE,
    variant_label   VARCHAR(10) NOT NULL,
    post_id         UUID NOT NULL REFERENCES posts(id),
    is_control      BOOLEAN NOT NULL DEFAULT FALSE,
    views           INT NOT NULL DEFAULT 0,
    likes           INT NOT NULL DEFAULT 0,
    comments        INT NOT NULL DEFAULT 0,
    shares          INT NOT NULL DEFAULT 0,
    clicks          INT NOT NULL DEFAULT 0,
    impressions     INT NOT NULL DEFAULT 0,
    engagement_rate NUMERIC(8,4) NOT NULL DEFAULT 0,
    is_winner       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT ab_test_variants_label_check
        CHECK (variant_label IN ('A', 'B', 'C', 'D')),
    CONSTRAINT ab_test_variants_unique
        UNIQUE (ab_test_id, variant_label)
);

-- Indexes
CREATE INDEX idx_ab_tests_user_id ON ab_tests(user_id);
CREATE INDEX idx_ab_tests_running ON ab_tests(status) WHERE status = 'running';
CREATE INDEX idx_ab_test_variants_post_id ON ab_test_variants(post_id);
