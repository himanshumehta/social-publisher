BEGIN;

-- Push notification device tokens
CREATE TABLE IF NOT EXISTS push_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id   VARCHAR(200) NOT NULL,
    platform    VARCHAR(10) NOT NULL CHECK (platform IN ('ios', 'android', 'web')),
    token       TEXT NOT NULL,
    is_active   BOOLEAN DEFAULT TRUE,
    last_used_at TIMESTAMPTZ DEFAULT NOW(),
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_push_tokens_user_active
    ON push_tokens (user_id, is_active)
    WHERE is_active = TRUE;

-- Push notification delivery queue
CREATE TABLE IF NOT EXISTS push_queue (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title         VARCHAR(200) NOT NULL,
    body          TEXT NOT NULL,
    data          JSONB DEFAULT '{}',
    priority      VARCHAR(10) DEFAULT 'normal' CHECK (priority IN ('normal', 'high')),
    status        VARCHAR(20) DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    error_message TEXT,
    sent_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_push_queue_pending
    ON push_queue (status, created_at)
    WHERE status = 'pending';

-- Offline sync queue (cursor-based)
CREATE TABLE IF NOT EXISTS sync_queue (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type  VARCHAR(30) NOT NULL CHECK (entity_type IN ('post', 'project', 'notification', 'channel')),
    entity_id    UUID NOT NULL,
    action       VARCHAR(10) NOT NULL CHECK (action IN ('create', 'update', 'delete')),
    payload      JSONB DEFAULT '{}',
    sequence_num BIGSERIAL,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sync_queue_user_seq
    ON sync_queue (user_id, sequence_num);

COMMIT;
