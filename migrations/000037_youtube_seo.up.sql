-- YouTube SEO & Content Strategy AI
CREATE TABLE seo_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects(id),
    post_id UUID REFERENCES posts(id),
    created_by UUID NOT NULL REFERENCES users(id),
    platform VARCHAR(20) NOT NULL DEFAULT 'youtube',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    original_title TEXT,
    optimized_title TEXT,
    original_description TEXT,
    optimized_description TEXT,
    original_tags TEXT[],
    optimized_tags TEXT[],
    title_score INT NOT NULL DEFAULT 0,
    description_score INT NOT NULL DEFAULT 0,
    overall_score INT NOT NULL DEFAULT 0,
    suggestions JSONB DEFAULT '[]',
    keyword_data JSONB DEFAULT '{}',
    competitor_data JSONB DEFAULT '{}',
    credits_used INT NOT NULL DEFAULT 0,
    error_message TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT seo_analyses_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE TABLE seo_keywords (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    keyword VARCHAR(200) NOT NULL,
    search_volume INT NOT NULL DEFAULT 0,
    competition VARCHAR(10) NOT NULL DEFAULT 'medium',
    trend_direction VARCHAR(10) NOT NULL DEFAULT 'stable',
    relevance_score REAL NOT NULL DEFAULT 0,
    platform VARCHAR(20) NOT NULL DEFAULT 'youtube',
    category VARCHAR(100),
    last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT seo_keywords_competition_check CHECK (competition IN ('low', 'medium', 'high')),
    CONSTRAINT seo_keywords_trend_check CHECK (trend_direction IN ('rising', 'stable', 'declining'))
);

CREATE TABLE content_suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    suggestion_type VARCHAR(30) NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    keywords TEXT[] DEFAULT '{}',
    estimated_views INT,
    confidence REAL NOT NULL DEFAULT 0,
    trending_score INT NOT NULL DEFAULT 0,
    source VARCHAR(50),
    metadata JSONB DEFAULT '{}',
    is_dismissed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT content_suggestions_type_check CHECK (suggestion_type IN (
        'trending_topic', 'competitor_gap', 'series_idea', 'seasonal', 'evergreen', 'collaboration'
    ))
);

CREATE TABLE competitor_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    platform VARCHAR(20) NOT NULL DEFAULT 'youtube',
    channel_name VARCHAR(200) NOT NULL,
    channel_url TEXT,
    platform_channel_id VARCHAR(100),
    subscriber_count INT,
    video_count INT,
    avg_views INT,
    top_keywords TEXT[] DEFAULT '{}',
    last_analyzed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_seo_analyses_project ON seo_analyses(project_id);
CREATE INDEX idx_seo_analyses_created_by ON seo_analyses(created_by);
CREATE INDEX idx_seo_analyses_status ON seo_analyses(status) WHERE status IN ('pending', 'processing');
CREATE INDEX idx_seo_keywords_user ON seo_keywords(user_id);
CREATE INDEX idx_seo_keywords_keyword ON seo_keywords(keyword);
CREATE INDEX idx_content_suggestions_user ON content_suggestions(user_id, is_dismissed);
CREATE INDEX idx_competitor_channels_user ON competitor_channels(user_id);
