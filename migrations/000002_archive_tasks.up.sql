CREATE TYPE archive_task_status AS ENUM ('pending', 'in_progress', 'completed', 'failed');

CREATE TABLE archive_tasks (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    assignment_id   UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    initiated_by    UUID NOT NULL REFERENCES users(id),
    status          archive_task_status NOT NULL DEFAULT 'pending',
    total_files     INTEGER NOT NULL DEFAULT 0,
    files_processed INTEGER NOT NULL DEFAULT 0,
    total_bytes     BIGINT NOT NULL DEFAULT 0,
    bytes_processed BIGINT NOT NULL DEFAULT 0,
    error_message   TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_archive_tasks_pending ON archive_tasks(status, created_at)
    WHERE status = 'pending';
CREATE INDEX idx_archive_tasks_assignment ON archive_tasks(assignment_id);
