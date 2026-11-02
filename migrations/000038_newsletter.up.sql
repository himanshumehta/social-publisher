-- Newsletter & Email Marketing
CREATE TABLE newsletters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(200) NOT NULL,
    description TEXT,
    from_name VARCHAR(100),
    from_email VARCHAR(200),
    reply_to VARCHAR(200),
    logo_url TEXT,
    color_primary VARCHAR(7) DEFAULT '#2563EB',
    color_secondary VARCHAR(7) DEFAULT '#1E40AF',
    footer_text TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    subscriber_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE newsletter_subscribers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    newsletter_id UUID NOT NULL REFERENCES newsletters(id) ON DELETE CASCADE,
    email VARCHAR(200) NOT NULL,
    name VARCHAR(200),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    source VARCHAR(50) DEFAULT 'manual',
    metadata JSONB DEFAULT '{}',
    subscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    unsubscribed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT newsletter_subscribers_status_check CHECK (status IN ('active', 'unsubscribed', 'bounced')),
    CONSTRAINT newsletter_subscribers_unique UNIQUE (newsletter_id, email)
);

CREATE TABLE newsletter_issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    newsletter_id UUID NOT NULL REFERENCES newsletters(id) ON DELETE CASCADE,
    subject VARCHAR(500) NOT NULL,
    preview_text VARCHAR(200),
    html_content TEXT NOT NULL,
    plain_text_content TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    source_post_id UUID REFERENCES posts(id),
    source_project_id UUID REFERENCES projects(id),
    sent_count INT NOT NULL DEFAULT 0,
    open_count INT NOT NULL DEFAULT 0,
    click_count INT NOT NULL DEFAULT 0,
    bounce_count INT NOT NULL DEFAULT 0,
    scheduled_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT newsletter_issues_status_check CHECK (status IN ('draft', 'scheduled', 'sending', 'sent', 'failed'))
);

CREATE TABLE newsletter_analytics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id UUID NOT NULL REFERENCES newsletter_issues(id) ON DELETE CASCADE,
    subscriber_id UUID NOT NULL REFERENCES newsletter_subscribers(id) ON DELETE CASCADE,
    event_type VARCHAR(20) NOT NULL,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT newsletter_analytics_event_check CHECK (event_type IN ('sent', 'opened', 'clicked', 'bounced', 'unsubscribed'))
);

CREATE INDEX idx_newsletters_user ON newsletters(user_id);
CREATE INDEX idx_newsletter_subscribers_newsletter ON newsletter_subscribers(newsletter_id, status);
CREATE INDEX idx_newsletter_subscribers_email ON newsletter_subscribers(email);
CREATE INDEX idx_newsletter_issues_newsletter ON newsletter_issues(newsletter_id);
CREATE INDEX idx_newsletter_issues_status ON newsletter_issues(status) WHERE status IN ('scheduled', 'sending');
CREATE INDEX idx_newsletter_analytics_issue ON newsletter_analytics(issue_id);
