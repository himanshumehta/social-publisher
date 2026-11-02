-- Commerce integrations: connect creator accounts to external platforms
CREATE TABLE IF NOT EXISTS commerce_integrations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id),
    channel_id       UUID REFERENCES channels(id),
    platform         VARCHAR(20) NOT NULL,   -- youtube, instagram, shopify, gumroad, patreon
    integration_type VARCHAR(30) NOT NULL,   -- super_chat, membership, shopping, digital_product, subscription
    credentials      JSONB DEFAULT '{}',     -- encrypted API keys / tokens (TODO: encrypt)
    status           VARCHAR(20) DEFAULT 'active', -- active, paused, disconnected, error
    last_sync_at     TIMESTAMPTZ,
    sync_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, platform, integration_type)
);

CREATE INDEX idx_commerce_integrations_user_status ON commerce_integrations(user_id, status);

-- Commerce transactions: individual revenue events pulled from platforms
CREATE TABLE IF NOT EXISTS commerce_transactions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id          UUID NOT NULL REFERENCES commerce_integrations(id),
    user_id                 UUID NOT NULL REFERENCES users(id),
    platform                VARCHAR(20) NOT NULL,
    transaction_type        VARCHAR(30) NOT NULL, -- super_chat, membership_payment, product_sale, subscription_payment, tip, donation
    external_transaction_id VARCHAR(200),
    amount_paise            BIGINT NOT NULL,
    currency                VARCHAR(3) DEFAULT 'INR',
    customer_name           VARCHAR(200),
    customer_id             VARCHAR(200),
    product_name            VARCHAR(200),
    metadata                JSONB DEFAULT '{}',
    transaction_date        TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(integration_id, external_transaction_id)
);

CREATE INDEX idx_commerce_txns_user_date ON commerce_transactions(user_id, transaction_date DESC);
CREATE INDEX idx_commerce_txns_integration_date ON commerce_transactions(integration_id, transaction_date DESC);
CREATE INDEX idx_commerce_txns_platform_type ON commerce_transactions(platform, transaction_type);
