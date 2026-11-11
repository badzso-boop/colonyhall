-- +goose Up
-- Round 50: the server deletes short-lived guard tokens a week after they
-- expired or were revoked; this partial index keeps that delete cheap.
CREATE INDEX api_token_host_expires_idx ON api_token (expires_at) WHERE host_id IS NOT NULL;

-- +goose Down
DROP INDEX api_token_host_expires_idx;
