-- +goose Up
-- Round 34 ("Szálaim"): how far a viewer has read each thread.
CREATE TABLE thread_read (
    actor_id       UUID NOT NULL REFERENCES actor(id),
    thread_root_id UUID NOT NULL REFERENCES message(id),
    last_read_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (actor_id, thread_root_id)
);
-- The "Szálaim" query looks threads up by participant.
CREATE INDEX message_thread_author_idx ON message (thread_root_id, author_id) WHERE thread_root_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS message_thread_author_idx;
DROP TABLE IF EXISTS thread_read;
