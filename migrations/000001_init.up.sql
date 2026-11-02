-- social-publisher — consolidated init schema
-- All extensions, enums, tables, indexes, triggers, and seed data.

-- =============================================================================
-- Extensions
-- =============================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- =============================================================================
-- Enum Types
-- =============================================================================
CREATE TYPE user_role AS ENUM ('creator', 'editor', 'admin', 'super_admin');
CREATE TYPE oauth_provider AS ENUM ('google', 'facebook', 'linkedin');
CREATE TYPE banking_status AS ENUM ('pending', 'submitted', 'verified', 'rejected');
CREATE TYPE assignment_status AS ENUM (
    'draft', 'open', 'claimed', 'raw_uploaded', 'editing', 'edited',
    'creator_review', 'creator_approved', 'admin_review',
    'approved', 'published', 'rejected', 'cancelled'
);
CREATE TYPE video_format AS ENUM (
    'raw', 'youtube_16_9', 'instagram_reel', 'youtube_short',
    'facebook_feed', 'facebook_reel'
);
CREATE TYPE review_action AS ENUM ('comment', 'request_changes', 'approve', 'reject', 'transition');
CREATE TYPE payout_status AS ENUM ('pending', 'approved', 'processing', 'completed', 'failed', 'cancelled');
CREATE TYPE post_status AS ENUM (
    'ai_draft', 'creator_editing', 'creator_submitted', 'admin_review',
    'approved', 'published', 'rejected'
);
CREATE TYPE audit_action AS ENUM (
    'login', 'logout', 'profile_update', 'banking_update',
    'assignment_create', 'assignment_update', 'assignment_claim', 'assignment_cancel',
    'video_upload', 'video_edit_upload', 'video_delete',
    'review_comment', 'review_approve', 'review_reject', 'review_request_changes',
    'publish_youtube', 'publish_instagram',
    'payout_calculate', 'payout_approve', 'payout_process', 'payout_complete', 'payout_fail',
    'post_create', 'post_ai_generate', 'post_submit', 'post_approve', 'post_publish',
    'user_create', 'user_update', 'user_role_change',
    'category_create', 'category_update', 'channel_create', 'channel_update'
);

-- =============================================================================
-- Tables
-- =============================================================================

CREATE TABLE categories (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name          TEXT NOT NULL,
    slug          TEXT NOT NULL UNIQUE,
    description   TEXT,
    icon_url      TEXT,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    display_order INTEGER NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE channels (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_id      UUID NOT NULL REFERENCES categories(id),
    platform         TEXT NOT NULL,
    channel_name     TEXT NOT NULL,
    channel_id       TEXT,
    channel_url      TEXT,
    access_token     TEXT,
    refresh_token    TEXT,
    token_expires_at TIMESTAMPTZ,
    scopes           TEXT[],
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    metadata         JSONB DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (category_id, platform)
);

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    display_name  TEXT,
    avatar_url    TEXT,
    bio           TEXT,
    phone         TEXT,
    password_hash TEXT,
    role          user_role NOT NULL DEFAULT 'creator',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    is_onboarded  BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_categories (
    user_id     UUID NOT NULL REFERENCES users(id),
    category_id UUID NOT NULL REFERENCES categories(id),
    PRIMARY KEY (user_id, category_id)
);

CREATE TABLE oauth_accounts (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL REFERENCES users(id),
    provider         oauth_provider NOT NULL,
    provider_id      TEXT NOT NULL,
    access_token     TEXT,
    refresh_token    TEXT,
    token_expires_at TIMESTAMPTZ,
    raw_profile      JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_id)
);

CREATE TABLE refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL UNIQUE,
    family_id  UUID NOT NULL,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE creator_profiles (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL UNIQUE REFERENCES users(id),
    slug             TEXT NOT NULL UNIQUE,
    city             TEXT,
    state            TEXT,
    specializations  TEXT[],
    youtube_channel  TEXT,
    instagram_handle TEXT,
    total_videos     INTEGER NOT NULL DEFAULT 0,
    total_views      BIGINT NOT NULL DEFAULT 0,
    total_earnings   BIGINT NOT NULL DEFAULT 0,
    is_public        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE creator_categories (
    creator_profile_id UUID NOT NULL REFERENCES creator_profiles(id),
    category_id        UUID NOT NULL REFERENCES categories(id),
    PRIMARY KEY (creator_profile_id, category_id)
);

CREATE TABLE banking_info (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL UNIQUE REFERENCES users(id),
    account_holder   TEXT NOT NULL,
    bank_name        TEXT NOT NULL,
    account_number   TEXT NOT NULL,
    ifsc_code        TEXT NOT NULL,
    upi_id           TEXT,
    pan_number       TEXT,
    status           banking_status NOT NULL DEFAULT 'pending',
    verified_at      TIMESTAMPTZ,
    verified_by      UUID REFERENCES users(id),
    rejection_reason TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE assignments (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_id     UUID REFERENCES categories(id),
    title           TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    description     TEXT,
    subject_name    TEXT,
    subject_type    TEXT,
    subject_ref_id  TEXT,
    requirements    JSONB NOT NULL DEFAULT '{}',
    budget_amount   BIGINT NOT NULL DEFAULT 0,
    budget_currency TEXT NOT NULL DEFAULT 'INR',
    deadline_at     TIMESTAMPTZ,
    publish_target  TIMESTAMPTZ,
    status          assignment_status NOT NULL DEFAULT 'draft',
    creator_id      UUID REFERENCES users(id),
    editor_id       UUID REFERENCES users(id),
    created_by      UUID NOT NULL REFERENCES users(id),
    priority        INTEGER NOT NULL DEFAULT 0,
    tags            TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE videos (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id   UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    uploaded_by     UUID NOT NULL REFERENCES users(id),
    format          video_format NOT NULL,
    version         INTEGER NOT NULL DEFAULT 1,
    storage_key     TEXT NOT NULL,
    storage_type    TEXT NOT NULL DEFAULT 'spaces',
    file_name       TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type       TEXT NOT NULL,
    duration_seconds INTEGER,
    width           INTEGER,
    height          INTEGER,
    fps             NUMERIC(6,2),
    codec           TEXT,
    bitrate_kbps    INTEGER,
    thumbnail_key   TEXT,
    is_current      BOOLEAN NOT NULL DEFAULT TRUE,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE multipart_uploads (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    upload_id       TEXT NOT NULL UNIQUE,
    assignment_id   UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id),
    storage_key     TEXT NOT NULL,
    file_name       TEXT NOT NULL,
    file_size_bytes BIGINT NOT NULL,
    mime_type       TEXT NOT NULL,
    total_parts     INTEGER NOT NULL,
    status          TEXT NOT NULL DEFAULT 'in_progress',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE reviews (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    video_id      UUID REFERENCES videos(id),
    reviewer_id   UUID NOT NULL REFERENCES users(id),
    action        review_action NOT NULL,
    comment       TEXT,
    timestamp_sec INTEGER,
    from_status   assignment_status,
    to_status     assignment_status,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE publish_records (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id       UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    video_id            UUID REFERENCES videos(id),
    channel_id          UUID NOT NULL REFERENCES channels(id),
    platform_content_id TEXT,
    platform_url        TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    error_message       TEXT,
    published_at        TIMESTAMPTZ,
    metadata            JSONB,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE youtube_analytics (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    publish_record_id       UUID NOT NULL REFERENCES publish_records(id) ON DELETE CASCADE,
    youtube_video_id        TEXT NOT NULL,
    period_start            DATE NOT NULL,
    period_end              DATE NOT NULL,
    views                   BIGINT NOT NULL DEFAULT 0,
    watch_time_minutes      BIGINT NOT NULL DEFAULT 0,
    likes                   INTEGER NOT NULL DEFAULT 0,
    comments_count          INTEGER NOT NULL DEFAULT 0,
    shares                  INTEGER NOT NULL DEFAULT 0,
    subscribers_gained      INTEGER NOT NULL DEFAULT 0,
    estimated_revenue_paise BIGINT NOT NULL DEFAULT 0,
    ad_revenue_paise        BIGINT NOT NULL DEFAULT 0,
    raw_response            JSONB,
    fetched_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (youtube_video_id, period_start)
);

CREATE TABLE payouts (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    creator_id           UUID NOT NULL REFERENCES users(id),
    period_start         DATE NOT NULL,
    period_end           DATE NOT NULL,
    gross_amount         BIGINT NOT NULL,
    platform_fee         BIGINT NOT NULL DEFAULT 0,
    tds_amount           BIGINT NOT NULL DEFAULT 0,
    net_amount           BIGINT NOT NULL,
    status               payout_status NOT NULL DEFAULT 'pending',
    approved_by          UUID REFERENCES users(id),
    approved_at          TIMESTAMPTZ,
    transfer_mode        TEXT,
    upi_transaction_ref  TEXT,
    processed_by         UUID REFERENCES users(id),
    razorpay_payout_id   TEXT,
    razorpay_fund_acc_id TEXT,
    razorpay_status      TEXT,
    failure_reason       TEXT,
    breakdown            JSONB NOT NULL DEFAULT '[]',
    processed_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (creator_id, period_start, period_end)
);

CREATE TABLE posts (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_id     UUID REFERENCES categories(id),
    assignment_id   UUID REFERENCES assignments(id),
    author_id       UUID NOT NULL REFERENCES users(id),
    title           TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    content         TEXT,
    excerpt         TEXT,
    cover_image_url TEXT,
    subject_name    TEXT,
    subject_type    TEXT,
    ai_provider     TEXT,
    ai_model        TEXT,
    ai_prompt       TEXT,
    ai_generated_at TIMESTAMPTZ,
    is_ai_generated BOOLEAN NOT NULL DEFAULT FALSE,
    status          post_status NOT NULL DEFAULT 'creator_editing',
    meta_title      TEXT,
    meta_description TEXT,
    tags            TEXT[],
    search_vector   TSVECTOR,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE audit_logs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID REFERENCES users(id),
    action      audit_action NOT NULL,
    entity_type TEXT,
    entity_id   UUID,
    ip_address  INET,
    user_agent  TEXT,
    request_id  TEXT,
    old_values  JSONB,
    new_values  JSONB,
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    body       TEXT,
    link       TEXT,
    is_read    BOOLEAN NOT NULL DEFAULT FALSE,
    type       TEXT,
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Indexes
-- =============================================================================

CREATE INDEX idx_assignments_category ON assignments(category_id);
CREATE INDEX idx_assignments_status   ON assignments(status);
CREATE INDEX idx_assignments_creator  ON assignments(creator_id);
CREATE INDEX idx_assignments_editor   ON assignments(editor_id);
CREATE INDEX idx_assignments_open     ON assignments(status, priority DESC, created_at DESC)
    WHERE status = 'open';

CREATE INDEX idx_videos_assignment ON videos(assignment_id);
CREATE INDEX idx_videos_current    ON videos(assignment_id, format)
    WHERE is_current = TRUE AND is_deleted = FALSE;

CREATE INDEX idx_multipart_uploads_assignment ON multipart_uploads(assignment_id);
CREATE INDEX idx_multipart_uploads_user       ON multipart_uploads(user_id);
CREATE INDEX idx_multipart_uploads_status     ON multipart_uploads(status);

CREATE INDEX idx_reviews_assignment ON reviews(assignment_id);

CREATE INDEX idx_publish_records_assignment ON publish_records(assignment_id);
CREATE INDEX idx_publish_records_channel    ON publish_records(channel_id);

CREATE INDEX idx_posts_published ON posts(published_at DESC) WHERE status = 'published';
CREATE INDEX idx_posts_search    ON posts USING GIN (search_vector);

CREATE INDEX idx_audit_logs_user    ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action  ON audit_logs(action);
CREATE INDEX idx_audit_logs_entity  ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);

CREATE INDEX idx_notifications_user ON notifications(user_id, is_read, created_at DESC);

-- =============================================================================
-- Triggers
-- =============================================================================

CREATE OR REPLACE FUNCTION posts_search_vector_update() RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector :=
        setweight(to_tsvector('english', COALESCE(NEW.title, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(NEW.excerpt, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.subject_name, '')), 'B') ||
        setweight(to_tsvector('english', COALESCE(NEW.content, '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_posts_search_vector
    BEFORE INSERT OR UPDATE OF title, excerpt, subject_name, content
    ON posts
    FOR EACH ROW
    EXECUTE FUNCTION posts_search_vector_update();

-- =============================================================================
-- Seed Data
-- =============================================================================

INSERT INTO categories (name, slug, description, display_order)
VALUES ('General', 'general', 'Default content category', 0);
