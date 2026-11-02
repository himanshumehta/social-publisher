-- Editor marketplace: job board, applications, assignments

-- Editing jobs posted by creators
CREATE TABLE IF NOT EXISTS editing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    creator_id UUID NOT NULL REFERENCES users(id),
    title VARCHAR(200) NOT NULL,
    brief TEXT NOT NULL DEFAULT '',
    complexity VARCHAR(20) NOT NULL DEFAULT 'standard', -- simple, standard, complex
    budget_min INT NOT NULL DEFAULT 0, -- paise
    budget_max INT NOT NULL DEFAULT 0,
    suggested_price INT NOT NULL DEFAULT 0,
    deadline TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'open', -- open, in_progress, completed, cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_editing_jobs_status ON editing_jobs(status) WHERE status = 'open';
CREATE INDEX idx_editing_jobs_creator ON editing_jobs(creator_id);

-- Editor applications to jobs
CREATE TABLE IF NOT EXISTS job_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES editing_jobs(id) ON DELETE CASCADE,
    editor_id UUID NOT NULL REFERENCES users(id),
    proposed_price INT NOT NULL DEFAULT 0,
    cover_note TEXT NOT NULL DEFAULT '',
    portfolio_links TEXT[] DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, shortlisted, accepted, rejected, withdrawn
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(job_id, editor_id)
);

CREATE INDEX idx_job_applications_job ON job_applications(job_id);
CREATE INDEX idx_job_applications_editor ON job_applications(editor_id);

-- Accepted assignments with agreed pricing
CREATE TABLE IF NOT EXISTS editing_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES editing_jobs(id),
    application_id UUID NOT NULL REFERENCES job_applications(id),
    editor_id UUID NOT NULL REFERENCES users(id),
    creator_id UUID NOT NULL REFERENCES users(id),
    agreed_price INT NOT NULL DEFAULT 0,
    platform_fee INT NOT NULL DEFAULT 0,
    total_charged INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'assigned', -- assigned, in_progress, in_review, completed, disputed, cancelled
    delivery_notes TEXT DEFAULT '',
    revision_notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_editing_assignments_editor ON editing_assignments(editor_id);
CREATE INDEX idx_editing_assignments_creator ON editing_assignments(creator_id);

-- Video call rooms for negotiation
CREATE TABLE IF NOT EXISTS video_call_rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES editing_jobs(id),
    creator_id UUID NOT NULL REFERENCES users(id),
    editor_id UUID NOT NULL REFERENCES users(id),
    provider VARCHAR(20) NOT NULL DEFAULT '100ms', -- 100ms, daily
    room_id VARCHAR(200) NOT NULL DEFAULT '',
    room_url TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'created', -- created, active, ended
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at TIMESTAMPTZ
);

-- Editor ratings (mutual)
CREATE TABLE IF NOT EXISTS editor_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id UUID NOT NULL REFERENCES editing_assignments(id),
    rated_by UUID NOT NULL REFERENCES users(id),
    rated_user UUID NOT NULL REFERENCES users(id),
    rating INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
    review_text TEXT DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(assignment_id, rated_by)
);

CREATE INDEX idx_editor_ratings_user ON editor_ratings(rated_user);

-- Marketplace transactions
CREATE TABLE IF NOT EXISTS marketplace_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    assignment_id UUID NOT NULL REFERENCES editing_assignments(id),
    creator_id UUID NOT NULL REFERENCES users(id),
    editor_id UUID NOT NULL REFERENCES users(id),
    amount INT NOT NULL DEFAULT 0,
    editor_payout INT NOT NULL DEFAULT 0,
    platform_fee INT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, held, released, refunded, disputed
    gateway_ref VARCHAR(200) DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Editing price config (admin-managed)
CREATE TABLE IF NOT EXISTS editing_price_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    duration_bracket VARCHAR(20) NOT NULL, -- '<5m', '5-15m', '15-30m', '30m+'
    complexity VARCHAR(20) NOT NULL, -- simple, standard, complex
    suggested_price INT NOT NULL DEFAULT 0, -- paise
    UNIQUE(duration_bracket, complexity)
);

-- Seed price config
INSERT INTO editing_price_config (duration_bracket, complexity, suggested_price) VALUES
    ('<5m', 'simple', 50000), ('<5m', 'standard', 100000), ('<5m', 'complex', 200000),
    ('5-15m', 'simple', 100000), ('5-15m', 'standard', 200000), ('5-15m', 'complex', 400000),
    ('15-30m', 'simple', 200000), ('15-30m', 'standard', 400000), ('15-30m', 'complex', 800000),
    ('30m+', 'simple', 400000), ('30m+', 'standard', 800000), ('30m+', 'complex', 1500000)
ON CONFLICT DO NOTHING;
