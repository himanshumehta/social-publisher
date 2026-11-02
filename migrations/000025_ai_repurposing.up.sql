-- AI Content Repurposing tables
CREATE TABLE IF NOT EXISTS repurpose_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_post_id UUID NOT NULL REFERENCES posts(id),
    user_id UUID NOT NULL REFERENCES users(id),
    ai_job_id UUID REFERENCES ai_jobs(id),
    strategy VARCHAR(50) NOT NULL, -- 'long_to_short', 'blog_to_thread', 'video_to_caption', 'podcast_to_blog', 'cross_platform'
    target_platforms TEXT[] NOT NULL,
    options JSONB DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_repurpose_jobs_user ON repurpose_jobs(user_id);
CREATE INDEX idx_repurpose_jobs_source ON repurpose_jobs(source_post_id);

CREATE TABLE IF NOT EXISTS repurpose_outputs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    repurpose_job_id UUID NOT NULL REFERENCES repurpose_jobs(id) ON DELETE CASCADE,
    target_platform VARCHAR(20) NOT NULL,
    generated_text TEXT NOT NULL,
    generated_hashtags TEXT[] DEFAULT '{}',
    generated_title VARCHAR(300),
    media_suggestions JSONB DEFAULT '{}',
    accepted BOOLEAN DEFAULT FALSE,
    accepted_post_id UUID REFERENCES posts(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_repurpose_outputs_job ON repurpose_outputs(repurpose_job_id);
