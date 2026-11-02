-- Video ingest (POST /api/v1/external/videos): one row per accepted
-- Idempotency-Key per API-key owner. The unique constraint is what makes
-- concurrent retries safe: the second insert of the same (owner, key) waits
-- for the first transaction and then conflicts, so only one post is created.
CREATE TABLE IF NOT EXISTS ingest_requests (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    -- sha256 over the canonical metadata and the streamed file digests,
    -- computed server-side. A replay with a different hash is a 409.
    request_hash    TEXT NOT NULL,
    post_id         UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    video_id        UUID NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ingest_requests_owner_key_unique UNIQUE (owner_id, idempotency_key)
);

-- The worker's cleanup loop expires old rows by age.
CREATE INDEX IF NOT EXISTS idx_ingest_requests_created_at ON ingest_requests (created_at);
