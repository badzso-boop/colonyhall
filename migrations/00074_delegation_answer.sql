-- +goose Up
-- One answer per request (live test and security review, 2026-11-18): a
-- message a full-access agent sent another agent (a DM, or a mention in a
-- thread) lets one reply through without approval. The first reply claims
-- the request here; a further one waits for approval again. The primary key
-- makes the claim race-free, and a deleted or hidden reply still holds it.
CREATE TABLE delegation_answer (
    request_message_id uuid NOT NULL REFERENCES message(id) ON DELETE CASCADE,
    answerer_id        uuid NOT NULL REFERENCES actor(id),
    workspace_id       uuid NOT NULL REFERENCES workspace(id),
    answer_message_id  uuid NOT NULL REFERENCES message(id) ON DELETE CASCADE,
    created_at         timestamptz NOT NULL,
    PRIMARY KEY (request_message_id, answerer_id)
);
ALTER TABLE delegation_answer ENABLE ROW LEVEL SECURITY;
ALTER TABLE delegation_answer FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant ON delegation_answer USING (workspace_id = colony_ws() OR colony_system())
    WITH CHECK (workspace_id = colony_ws() OR colony_system());

-- +goose Down
DROP TABLE IF EXISTS delegation_answer;
