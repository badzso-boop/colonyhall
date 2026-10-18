-- +goose Up
CREATE TABLE agent_rules (
    workspace_id UUID NOT NULL REFERENCES workspace(id),
    version INT NOT NULL,
    text VARCHAR(8000) NOT NULL,
    created_by UUID NOT NULL REFERENCES actor(id),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, version)
);

CREATE INDEX agent_rules_workspace_version_idx ON agent_rules(workspace_id, version DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_rules;
