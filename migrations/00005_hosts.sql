-- +goose Up
-- Machines running a guard, which agents run where, and which folders
-- each agent may use on its machine.

CREATE TABLE host_pairing_code (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    code_hash     bytea NOT NULL UNIQUE,
    created_by    uuid NOT NULL REFERENCES actor(id),
    created_at    timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz
);

CREATE TABLE host (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    name          text NOT NULL,
    token_hash    bytea NOT NULL UNIQUE,
    state         text NOT NULL,              -- approved | revoked
    created_by    uuid NOT NULL REFERENCES actor(id),
    created_at    timestamptz NOT NULL,
    last_seen_at  timestamptz,
    report        jsonb NOT NULL DEFAULT '{}'
);

-- One agent runs on at most one host at a time.
CREATE TABLE agent_host (
    agent_id        uuid PRIMARY KEY REFERENCES actor(id),
    host_id         uuid NOT NULL REFERENCES host(id),
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    client          text NOT NULL,
    model           text NOT NULL,
    work_model      text NOT NULL,
    allow_work      boolean NOT NULL,
    permission_mode text NOT NULL,
    created_by      uuid NOT NULL REFERENCES actor(id),
    created_at      timestamptz NOT NULL
);
CREATE INDEX agent_host_host_idx ON agent_host (host_id);

-- Revoked grants are kept (revoked_at), never deleted.
CREATE TABLE folder_grant (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    agent_id      uuid NOT NULL REFERENCES actor(id),
    host_id       uuid NOT NULL REFERENCES host(id),
    path          text NOT NULL,
    mode          text NOT NULL,              -- read | write
    created_by    uuid NOT NULL REFERENCES actor(id),
    created_at    timestamptz NOT NULL,
    revoked_at    timestamptz
);
CREATE INDEX folder_grant_active_idx ON folder_grant (host_id, agent_id) WHERE revoked_at IS NULL;

-- Short-lived agent tokens a guard requests for its assigned agents.
ALTER TABLE api_token ADD COLUMN host_id uuid REFERENCES host(id);
ALTER TABLE api_token ADD COLUMN expires_at timestamptz;

-- +goose Down
ALTER TABLE api_token DROP COLUMN expires_at;
ALTER TABLE api_token DROP COLUMN host_id;
DROP TABLE folder_grant;
DROP TABLE agent_host;
DROP TABLE host;
DROP TABLE host_pairing_code;
