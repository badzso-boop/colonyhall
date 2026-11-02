-- +goose Up
-- Round 35: estimated run cost. A price change is a new row with a new
-- valid_from, so old runs keep the price of their time (append-only).
CREATE TABLE model_price (
    id                   UUID PRIMARY KEY,
    workspace_id         UUID NOT NULL REFERENCES workspace(id),
    model                TEXT NOT NULL,
    input_per_mtok       DOUBLE PRECISION NOT NULL CHECK (input_per_mtok >= 0),
    output_per_mtok      DOUBLE PRECISION NOT NULL CHECK (output_per_mtok >= 0),
    cache_read_per_mtok  DOUBLE PRECISION NOT NULL CHECK (cache_read_per_mtok >= 0),
    cache_write_per_mtok DOUBLE PRECISION NOT NULL CHECK (cache_write_per_mtok >= 0),
    valid_from           TIMESTAMPTZ NOT NULL,
    created_by           UUID NOT NULL REFERENCES actor(id),
    created_at           TIMESTAMPTZ NOT NULL
);
CREATE INDEX model_price_workspace ON model_price (workspace_id, model, valid_from);

-- A manual USD to HUF rate: no external rate service, the server is Tailscale-only.
CREATE TABLE workspace_currency (
    workspace_id UUID PRIMARY KEY REFERENCES workspace(id),
    usd_huf      DOUBLE PRECISION NOT NULL CHECK (usd_huf > 0),
    updated_by   UUID NOT NULL REFERENCES actor(id),
    updated_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX agent_run_search ON agent_run (workspace_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS agent_run_search;
DROP TABLE workspace_currency;
DROP TABLE model_price;
