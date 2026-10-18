-- +goose Up
CREATE TABLE lease (
    id UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    resource VARCHAR(100) NOT NULL,
    holder_id UUID NOT NULL REFERENCES actor(id),
    note VARCHAR(200),
    acquired_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    released_at TIMESTAMPTZ,
    released_by UUID REFERENCES actor(id)
);

CREATE INDEX lease_workspace_resource_idx ON lease(workspace_id, resource);
CREATE INDEX lease_active_idx ON lease(workspace_id, released_at, expires_at);
CREATE INDEX lease_holder_idx ON lease(workspace_id, holder_id) WHERE released_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS lease;
