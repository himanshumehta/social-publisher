-- Add analytics sync tracking to channels
ALTER TABLE channels ADD COLUMN IF NOT EXISTS last_analytics_sync TIMESTAMPTZ;
ALTER TABLE channels ADD COLUMN IF NOT EXISTS analytics_sync_error TEXT DEFAULT '';

-- Revenue configuration table (admin-managed)
CREATE TABLE IF NOT EXISTS revenue_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    config_key VARCHAR(100) UNIQUE NOT NULL,
    config_value JSONB NOT NULL DEFAULT '{}',
    updated_by UUID REFERENCES users(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed default revenue config
INSERT INTO revenue_config (config_key, config_value) VALUES
    ('creator_share_pct', '70'),
    ('tds_rate_pct', '10'),
    ('min_payout_amount', '50000')
ON CONFLICT (config_key) DO NOTHING;
