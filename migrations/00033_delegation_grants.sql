-- +goose Up
CREATE TABLE delegation_grant (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    channel_id      uuid NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
    thread_root_id  uuid,
    agent_a_id      uuid NOT NULL REFERENCES actor(id),
    agent_b_id      uuid NOT NULL REFERENCES actor(id),
    delegation_id   uuid NOT NULL REFERENCES delegation(id) ON DELETE CASCADE,
    approved_by     uuid NOT NULL REFERENCES actor(id),
    approved_at     timestamptz NOT NULL,
    last_used_at    timestamptz NOT NULL,
    expires_at      timestamptz NOT NULL,
    revoked_by      uuid REFERENCES actor(id),
    revoked_at      timestamptz
);

CREATE INDEX delegation_grant_active_idx ON delegation_grant(workspace_id, channel_id, agent_a_id, agent_b_id) WHERE revoked_at IS NULL;
CREATE INDEX delegation_grant_delegation_idx ON delegation_grant(delegation_id);

CREATE TABLE delegation_settings (
    workspace_id       uuid PRIMARY KEY REFERENCES workspace(id) ON DELETE CASCADE,
    grant_idle_minutes integer NOT NULL DEFAULT 60,
    grant_max_hours    integer NOT NULL DEFAULT 12,
    updated_at         timestamptz NOT NULL
);

ALTER TABLE delivery ADD COLUMN via_grant boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE delivery DROP COLUMN IF EXISTS via_grant;
DROP TABLE IF EXISTS delegation_settings;
DROP TABLE IF EXISTS delegation_grant;
