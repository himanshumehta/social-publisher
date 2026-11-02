-- Generalized AI job system (replaces single-purpose clip_jobs)
CREATE TABLE IF NOT EXISTS ai_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    video_id UUID REFERENCES videos(id),
    created_by UUID NOT NULL REFERENCES users(id),
    job_type VARCHAR(50) NOT NULL, -- 'auto_clip', 'auto_subtitle', 'ai_brief'
    provider VARCHAR(50) NOT NULL DEFAULT 'internal', -- 'whisper', 'claude', 'ffmpeg'
    config JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    result JSONB DEFAULT NULL,
    error_message TEXT,
    credits_used INT NOT NULL DEFAULT 0,
    processing_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_jobs_status ON ai_jobs(status, created_at) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_ai_jobs_project ON ai_jobs(project_id);
CREATE INDEX idx_ai_jobs_created_by ON ai_jobs(created_by);

-- AI job output artifacts (clips, subtitles, briefs)
CREATE TABLE IF NOT EXISTS ai_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ai_job_id UUID NOT NULL REFERENCES ai_jobs(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id),
    asset_type VARCHAR(50) NOT NULL, -- 'clip_video', 'subtitle_srt', 'subtitle_vtt', 'brief_json'
    storage_key TEXT, -- S3 key for binary assets
    content JSONB, -- for structured content (briefs, subtitles as JSON)
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_assets_job ON ai_assets(ai_job_id);

-- User credit system for AI features
CREATE TABLE IF NOT EXISTS ai_credits (
    user_id UUID PRIMARY KEY REFERENCES users(id),
    balance INT NOT NULL DEFAULT 0,
    total_purchased INT NOT NULL DEFAULT 0,
    total_used INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Credit transaction audit log
CREATE TABLE IF NOT EXISTS ai_credit_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    amount INT NOT NULL, -- positive = credit, negative = debit
    balance_after INT NOT NULL,
    reason VARCHAR(200) NOT NULL,
    ai_job_id UUID REFERENCES ai_jobs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ai_credit_txns_user ON ai_credit_transactions(user_id, created_at DESC);

-- Migrate existing clip_jobs to ai_jobs
INSERT INTO ai_jobs (id, project_id, video_id, created_by, job_type, provider, config, status, error_message, credits_used, processing_started_at, completed_at, created_at, updated_at)
SELECT id, project_id, source_video_id, created_by, 'auto_clip', COALESCE(NULLIF(provider, ''), 'internal'), COALESCE(config, '{}'), status::text, error_message, 0, started_at, completed_at, created_at, updated_at
FROM clip_jobs
ON CONFLICT DO NOTHING;
