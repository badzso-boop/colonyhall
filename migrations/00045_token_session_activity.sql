-- +goose Up
-- The first characters of a token (never enough to use it), and when tokens
-- and sessions were last used; the PWA's Tokens and Users pages show them.
-- Writes are throttled to one a minute per token or session.
ALTER TABLE api_token ADD COLUMN prefix text;
ALTER TABLE api_token ADD COLUMN last_used_at timestamptz;
ALTER TABLE session ADD COLUMN last_seen_at timestamptz;
-- The instructions version a work or direct run got (round 33).
ALTER TABLE agent_run ADD COLUMN instructions_version integer;

-- +goose Down
ALTER TABLE agent_run DROP COLUMN instructions_version;
ALTER TABLE session DROP COLUMN last_seen_at;
ALTER TABLE api_token DROP COLUMN last_used_at;
ALTER TABLE api_token DROP COLUMN prefix;
