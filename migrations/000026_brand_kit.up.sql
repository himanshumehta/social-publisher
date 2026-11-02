-- Brand kits: reusable brand identity per creator
CREATE TABLE IF NOT EXISTS brand_kits (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    name            VARCHAR(100) NOT NULL,
    logo_url        TEXT,
    primary_color   VARCHAR(7),   -- hex #RRGGBB
    secondary_color VARCHAR(7),
    accent_color    VARCHAR(7),
    font_heading    VARCHAR(100),
    font_body       VARCHAR(100),
    tone            VARCHAR(50) DEFAULT 'professional',
    voice_description TEXT,
    tagline         VARCHAR(200),
    hashtags        TEXT[] DEFAULT '{}',
    cta_default     TEXT,
    is_default      BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE INDEX idx_brand_kits_user ON brand_kits(user_id);

-- Content templates: parameterized text skeletons per brand kit + platform
CREATE TABLE IF NOT EXISTS content_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    brand_kit_id    UUID NOT NULL REFERENCES brand_kits(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id),
    name            VARCHAR(100) NOT NULL,
    platform        VARCHAR(20) NOT NULL,
    template_body   TEXT NOT NULL,          -- contains {{variable}} placeholders
    variables       TEXT[] NOT NULL DEFAULT '{}',
    example_output  TEXT,
    category        VARCHAR(50) DEFAULT 'general',
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_content_templates_brand_kit ON content_templates(brand_kit_id);
CREATE INDEX idx_content_templates_user_platform ON content_templates(user_id, platform);

-- Link posts to their brand kit
ALTER TABLE posts ADD COLUMN IF NOT EXISTS brand_kit_id UUID REFERENCES brand_kits(id);
