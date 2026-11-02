-- 000029_collaboration_workflows.up.sql

-- ── Team Members ────────────────────────────────────────────────────────

CREATE TABLE team_members (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      UUID        NOT NULL REFERENCES users(id),
    member_id     UUID        NOT NULL REFERENCES users(id),
    role          VARCHAR(30) NOT NULL DEFAULT 'viewer',
    invited_by    UUID        NOT NULL REFERENCES users(id),
    invited_email VARCHAR(255),
    status        VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_team_owner_member UNIQUE (owner_id, member_id),
    CONSTRAINT chk_team_role CHECK (role IN ('viewer', 'contributor', 'manager', 'admin')),
    CONSTRAINT chk_team_status CHECK (status IN ('pending', 'active', 'removed'))
);

CREATE INDEX idx_team_members_owner_status ON team_members (owner_id, status);
CREATE INDEX idx_team_members_member       ON team_members (member_id);

-- ── Approval Workflows ──────────────────────────────────────────────────

CREATE TABLE approval_workflows (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   UUID         NOT NULL REFERENCES users(id),
    name       VARCHAR(100) NOT NULL,
    steps      JSONB        NOT NULL DEFAULT '[]',
    is_active  BOOLEAN      NOT NULL DEFAULT TRUE,
    auto_apply BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- ── Approval Requests ───────────────────────────────────────────────────

CREATE TABLE approval_requests (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id      UUID        NOT NULL REFERENCES posts(id),
    workflow_id  UUID        NOT NULL REFERENCES approval_workflows(id),
    submitted_by UUID        NOT NULL REFERENCES users(id),
    current_step INT         NOT NULL DEFAULT 1,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_approval_status CHECK (
        status IN ('pending', 'in_review', 'approved', 'rejected', 'changes_requested')
    )
);

CREATE INDEX idx_approval_requests_post   ON approval_requests (post_id);
CREATE INDEX idx_approval_requests_status ON approval_requests (status);

-- ── Approval Actions ────────────────────────────────────────────────────

CREATE TABLE approval_actions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id  UUID        NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    step_number INT         NOT NULL,
    actor_id    UUID        NOT NULL REFERENCES users(id),
    action      VARCHAR(20) NOT NULL,
    comment     TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_action_type CHECK (
        action IN ('approve', 'reject', 'request_changes')
    )
);

CREATE INDEX idx_approval_actions_request ON approval_actions (request_id);
