-- AI Smart Clip Generator (Better than OpusClip)
CREATE TABLE smart_clip_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    video_id UUID NOT NULL REFERENCES videos(id),
    created_by UUID NOT NULL REFERENCES users(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    provider VARCHAR(50) NOT NULL DEFAULT 'internal',
    clip_count INT NOT NULL DEFAULT 5,
    min_duration_seconds INT NOT NULL DEFAULT 15,
    max_duration_seconds INT NOT NULL DEFAULT 60,
    target_aspect_ratio VARCHAR(10) NOT NULL DEFAULT '9:16',
    auto_captions BOOLEAN NOT NULL DEFAULT TRUE,
    caption_language VARCHAR(10) NOT NULL DEFAULT 'en',
    smart_crop BOOLEAN NOT NULL DEFAULT TRUE,
    detect_hooks BOOLEAN NOT NULL DEFAULT TRUE,
    config JSONB NOT NULL DEFAULT '{}',
    credits_used INT NOT NULL DEFAULT 0,
    progress INT NOT NULL DEFAULT 0,
    error_message TEXT,
    processing_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT smart_clip_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    CONSTRAINT smart_clip_jobs_aspect_check CHECK (target_aspect_ratio IN ('9:16', '1:1', '4:5', '16:9'))
);

CREATE TABLE smart_clips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES smart_clip_jobs(id) ON DELETE CASCADE,
    title VARCHAR(500),
    start_time_ms INT NOT NULL DEFAULT 0,
    end_time_ms INT NOT NULL DEFAULT 0,
    duration_ms INT NOT NULL DEFAULT 0,
    hook_score INT NOT NULL DEFAULT 0,
    virality_score INT NOT NULL DEFAULT 0,
    engagement_prediction REAL NOT NULL DEFAULT 0,
    aspect_ratio VARCHAR(10) NOT NULL DEFAULT '9:16',
    has_captions BOOLEAN NOT NULL DEFAULT FALSE,
    caption_data JSONB,
    video_url TEXT,
    thumbnail_url TEXT,
    transcript TEXT,
    hook_text TEXT,
    tags TEXT[] DEFAULT '{}',
    detected_moments JSONB DEFAULT '[]',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    rank INT NOT NULL DEFAULT 0,
    file_size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT smart_clips_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE TABLE clip_moments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES smart_clip_jobs(id) ON DELETE CASCADE,
    moment_type VARCHAR(30) NOT NULL,
    start_time_ms INT NOT NULL,
    end_time_ms INT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    description TEXT,
    hook_score INT NOT NULL DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT clip_moments_type_check CHECK (moment_type IN (
        'hook', 'climax', 'reveal', 'reaction', 'tutorial_step',
        'comparison', 'before_after', 'highlight', 'quote', 'cta'
    ))
);

CREATE INDEX idx_smart_clip_jobs_project ON smart_clip_jobs(project_id);
CREATE INDEX idx_smart_clip_jobs_created_by ON smart_clip_jobs(created_by);
CREATE INDEX idx_smart_clip_jobs_status ON smart_clip_jobs(status, created_at) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_smart_clips_job ON smart_clips(job_id);
CREATE INDEX idx_smart_clips_virality ON smart_clips(virality_score DESC);
CREATE INDEX idx_clip_moments_job ON clip_moments(job_id);
