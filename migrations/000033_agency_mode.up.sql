BEGIN;

-- Organizations table
CREATE TABLE IF NOT EXISTS organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(100) NOT NULL UNIQUE,
    logo_url   TEXT,
    domain     VARCHAR(200),
    owner_id   UUID NOT NULL REFERENCES users(id),
    plan       VARCHAR(30) NOT NULL DEFAULT 'starter',
    max_members  INT NOT NULL DEFAULT 5,
    max_projects INT NOT NULL DEFAULT 10,
    settings   JSONB NOT NULL DEFAULT '{}',
    is_active  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Organization members
CREATE TABLE IF NOT EXISTS org_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id),
    role       VARCHAR(30) NOT NULL DEFAULT 'member',
    invited_by UUID REFERENCES users(id),
    status     VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, user_id)
);

-- Organization subscriptions
CREATE TABLE IF NOT EXISTS org_subscriptions (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES organizations(id),
    plan                    VARCHAR(30) NOT NULL,
    amount_paise            BIGINT NOT NULL DEFAULT 0,
    billing_cycle           VARCHAR(20) NOT NULL DEFAULT 'monthly',
    status                  VARCHAR(20) NOT NULL DEFAULT 'active',
    current_period_start    DATE NOT NULL,
    current_period_end      DATE NOT NULL,
    gateway_subscription_id VARCHAR(200),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Organization invoices
CREATE TABLE IF NOT EXISTS org_invoices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    subscription_id UUID REFERENCES org_subscriptions(id),
    amount_paise    BIGINT NOT NULL,
    tax_paise       BIGINT NOT NULL DEFAULT 0,
    total_paise     BIGINT NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft',
    invoice_number  VARCHAR(50) NOT NULL UNIQUE,
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    paid_at         TIMESTAMPTZ,
    due_date        DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add org_id to projects and posts for org-scoped content
ALTER TABLE projects ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);
ALTER TABLE posts    ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_org_members_org_status
    ON org_members (org_id, status);

CREATE INDEX IF NOT EXISTS idx_org_members_user
    ON org_members (user_id);

CREATE INDEX IF NOT EXISTS idx_org_subscriptions_org_status
    ON org_subscriptions (org_id, status);

CREATE INDEX IF NOT EXISTS idx_org_invoices_org
    ON org_invoices (org_id);

CREATE INDEX IF NOT EXISTS idx_projects_org
    ON projects (org_id)
    WHERE org_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_posts_org
    ON posts (org_id)
    WHERE org_id IS NOT NULL;

COMMIT;
