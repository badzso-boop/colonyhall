-- +goose Up
CREATE TABLE delegation (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    message_id      uuid NOT NULL REFERENCES message(id) ON DELETE CASCADE,
    channel_id      uuid NOT NULL REFERENCES channel(id) ON DELETE CASCADE,
    thread_root_id  uuid,
    from_agent_id   uuid NOT NULL REFERENCES actor(id),
    to_agent_id     uuid NOT NULL REFERENCES actor(id),
    question_id     uuid NOT NULL REFERENCES question(id) ON DELETE CASCADE,
    state           text NOT NULL,
    decided_by      uuid REFERENCES actor(id),
    decided_at      timestamptz,
    created_at      timestamptz NOT NULL,
    CONSTRAINT delegation_state_check CHECK (state IN ('pending', 'approved', 'denied', 'expired')),
    CONSTRAINT delegation_message_target_unique UNIQUE (message_id, to_agent_id)
);

CREATE INDEX delegation_workspace_state_idx ON delegation(workspace_id, state);
CREATE INDEX delegation_question_idx ON delegation(question_id);
CREATE INDEX delegation_message_idx ON delegation(message_id);

ALTER TABLE delivery ADD COLUMN delegated_by uuid REFERENCES actor(id);
ALTER TABLE question ADD COLUMN allow_expire boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE question DROP COLUMN IF EXISTS allow_expire;
ALTER TABLE delivery DROP COLUMN IF EXISTS delegated_by;
DROP TABLE IF EXISTS delegation;
