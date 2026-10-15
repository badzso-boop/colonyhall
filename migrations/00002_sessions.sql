-- +goose Up
-- Password login for humans: one credential per actor, browser sessions.

CREATE TABLE credential (
    actor_id       uuid PRIMARY KEY REFERENCES actor(id),
    password_hash  text NOT NULL,          -- argon2id, PHC string format
    updated_at     timestamptz NOT NULL
);

CREATE TABLE session (
    id            uuid PRIMARY KEY,
    actor_id      uuid NOT NULL REFERENCES actor(id),
    token_hash    bytea NOT NULL UNIQUE,   -- SHA-256 of the cookie value
    csrf_token    text NOT NULL,
    user_agent    text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL,
    revoked_at    timestamptz
);
CREATE INDEX session_actor_idx ON session (actor_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE session;
DROP TABLE credential;
