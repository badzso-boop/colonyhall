-- +goose Up
-- Phase 0: workspace, actors, channels, messages, event log, delivery.
-- Append-only rule: message, message_revision and event rows are never
-- updated or deleted (counters and soft-delete markers excepted).

CREATE TABLE workspace (
    id          uuid PRIMARY KEY,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL
);

CREATE TABLE actor (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    kind          text NOT NULL CHECK (kind IN ('human', 'agent')),
    handle        text NOT NULL,
    display_name  text NOT NULL,
    role          text NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'observer')),
    state         text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'paused', 'stopped')),
    created_at    timestamptz NOT NULL,
    UNIQUE (workspace_id, handle)
);

-- Bearer tokens for agents and (in phase 0) humans. Only the SHA-256 hash is stored.
CREATE TABLE api_token (
    id          uuid PRIMARY KEY,
    actor_id    uuid NOT NULL REFERENCES actor(id),
    token_hash  bytea NOT NULL UNIQUE,
    label       text NOT NULL,
    created_at  timestamptz NOT NULL,
    revoked_at  timestamptz
);

CREATE TABLE channel (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspace(id),
    kind          text NOT NULL CHECK (kind IN ('public', 'private', 'dm')),
    name          text NOT NULL,
    msg_count     bigint NOT NULL DEFAULT 0,
    created_by    uuid NOT NULL REFERENCES actor(id),
    created_at    timestamptz NOT NULL,
    UNIQUE (workspace_id, name)
);

CREATE TABLE channel_member (
    channel_id     uuid NOT NULL REFERENCES channel(id),
    actor_id       uuid NOT NULL REFERENCES actor(id),
    can_write      boolean NOT NULL DEFAULT true,
    msg_count      bigint NOT NULL DEFAULT 0,  -- messages seen (Mattermost-style unread counter)
    mention_count  bigint NOT NULL DEFAULT 0,
    joined_at      timestamptz NOT NULL,
    PRIMARY KEY (channel_id, actor_id)
);
CREATE INDEX channel_member_actor_idx ON channel_member (actor_id);

CREATE TABLE message (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL REFERENCES workspace(id),
    channel_id      uuid NOT NULL REFERENCES channel(id),
    thread_root_id  uuid REFERENCES message(id),
    author_id       uuid NOT NULL REFERENCES actor(id),
    client_msg_id   text,
    kind            text NOT NULL CHECK (kind IN ('text', 'system')),
    body            text NOT NULL,
    mentions        uuid[] NOT NULL DEFAULT '{}',
    reply_count     integer NOT NULL DEFAULT 0,
    last_reply_at   timestamptz,
    created_at      timestamptz NOT NULL,
    deleted_at      timestamptz
);
CREATE INDEX message_channel_idx ON message (channel_id, id);
CREATE INDEX message_thread_idx ON message (thread_root_id, id) WHERE thread_root_id IS NOT NULL;
CREATE UNIQUE INDEX message_client_id_idx ON message (author_id, client_msg_id) WHERE client_msg_id IS NOT NULL;

-- Event log (transactional outbox). seq is assigned under an advisory lock.
CREATE SEQUENCE event_seq;
CREATE TABLE event (
    seq             bigint PRIMARY KEY,
    id              uuid NOT NULL UNIQUE,
    workspace_id    uuid NOT NULL,
    type            text NOT NULL,
    actor_id        uuid NOT NULL,
    channel_id      uuid,
    aggregate_type  text NOT NULL,
    aggregate_id    uuid NOT NULL,
    payload         jsonb NOT NULL,
    schema_version  integer NOT NULL,
    at              timestamptz NOT NULL
);

CREATE TABLE consumer_offset (
    consumer_name  text PRIMARY KEY,
    last_seq       bigint NOT NULL,
    updated_at     timestamptz NOT NULL
);

-- One row per recipient per item (Zulip UserMessage-style flags).
-- flags bits: 1 unread, 2 mentioned, 4 wake, 8 dm.
CREATE TABLE delivery (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL,
    recipient_id  uuid NOT NULL REFERENCES actor(id),
    item_kind     text NOT NULL,
    item_id       uuid NOT NULL,
    channel_id    uuid,
    flags         smallint NOT NULL,
    event_seq     bigint NOT NULL,
    created_at    timestamptz NOT NULL,
    acked_at      timestamptz,
    UNIQUE (recipient_id, item_kind, item_id)
);
CREATE INDEX delivery_pending_idx ON delivery (recipient_id, id) WHERE acked_at IS NULL;

-- +goose Down
DROP TABLE delivery;
DROP TABLE consumer_offset;
DROP TABLE event;
DROP SEQUENCE event_seq;
DROP TABLE message;
DROP TABLE channel_member;
DROP TABLE channel;
DROP TABLE api_token;
DROP TABLE actor;
DROP TABLE workspace;
