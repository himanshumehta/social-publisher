-- AI Regional Language Dubbing
CREATE TABLE dub_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    video_id UUID NOT NULL REFERENCES videos(id),
    created_by UUID NOT NULL REFERENCES users(id),
    source_language VARCHAR(10) NOT NULL DEFAULT 'en',
    target_language VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    provider VARCHAR(50) NOT NULL DEFAULT 'internal',
    config JSONB NOT NULL DEFAULT '{}',
    voice_profile VARCHAR(50),
    preserve_background_audio BOOLEAN NOT NULL DEFAULT TRUE,
    lip_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    credits_used INT NOT NULL DEFAULT 0,
    progress INT NOT NULL DEFAULT 0,
    error_message TEXT,
    result JSONB,
    output_video_url TEXT,
    output_audio_url TEXT,
    duration_seconds INT,
    processing_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT dub_jobs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'cancelled')),
    CONSTRAINT dub_jobs_language_check CHECK (target_language IN ('hi', 'ta', 'te', 'kn', 'ml', 'bn', 'mr', 'gu', 'pa'))
);

CREATE TABLE dub_outputs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dub_job_id UUID NOT NULL REFERENCES dub_jobs(id) ON DELETE CASCADE,
    language VARCHAR(10) NOT NULL,
    audio_url TEXT,
    video_url TEXT,
    subtitle_url TEXT,
    transcript TEXT,
    quality_score REAL,
    duration_seconds INT,
    file_size_bytes BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT dub_outputs_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX idx_dub_jobs_project ON dub_jobs(project_id);
CREATE INDEX idx_dub_jobs_created_by ON dub_jobs(created_by);
CREATE INDEX idx_dub_jobs_status ON dub_jobs(status, created_at) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_dub_outputs_job ON dub_outputs(dub_job_id);
