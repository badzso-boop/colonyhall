-- +goose Up
-- Admin controls: emergency stop per workspace, token revocation info and
-- the hash-chained audit log (built from the event log by the audit module).

ALTER TABLE workspace
    ADD COLUMN emergency_stop_at timestamptz,
    ADD COLUMN emergency_stop_by uuid REFERENCES actor(id);

CREATE INDEX api_token_actor_idx ON api_token (actor_id);

-- Each entry hashes the previous one: altering or deleting a row breaks
-- the chain, which /admin/audit/verify detects.
CREATE TABLE audit_log (
    seq           bigint PRIMARY KEY,        -- the event's seq
    workspace_id  uuid NOT NULL,
    actor_id      uuid NOT NULL,
    action        text NOT NULL,             -- the event type
    target_type   text NOT NULL,
    target_id     uuid NOT NULL,
    payload       bytea NOT NULL,            -- exact bytes that were hashed (jsonb would reformat them)
    at            timestamptz NOT NULL,
    prev_hash     bytea NOT NULL,
    hash          bytea NOT NULL
);
CREATE INDEX audit_log_workspace_idx ON audit_log (workspace_id, seq);

-- +goose Down
DROP TABLE audit_log;
DROP INDEX api_token_actor_idx;
ALTER TABLE workspace DROP COLUMN emergency_stop_by, DROP COLUMN emergency_stop_at;
