CREATE TYPE video_status AS ENUM ('processing', 'ready', 'approved', 'rejected');

ALTER TABLE videos ADD COLUMN status video_status NOT NULL DEFAULT 'processing';

ALTER TABLE reviews ADD COLUMN parent_id UUID REFERENCES reviews(id);

CREATE INDEX idx_reviews_parent ON reviews(parent_id) WHERE parent_id IS NOT NULL;

CREATE INDEX idx_videos_status ON videos(assignment_id, status) WHERE is_deleted = FALSE;

ALTER TYPE review_action ADD VALUE IF NOT EXISTS 'transition';

ALTER TABLE multipart_uploads ADD COLUMN IF NOT EXISTS format text NOT NULL DEFAULT 'raw';
