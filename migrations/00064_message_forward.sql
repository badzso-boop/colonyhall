-- +goose Up
-- Round 55, part F: a forwarded message names the message it quotes. The
-- quote itself is in the body (with its @mentions defused); this column is
-- for the "Továbbítva" mark and is shown only to readers of the source.
ALTER TABLE message ADD COLUMN forwarded_from_id uuid REFERENCES message(id);

-- +goose Down
ALTER TABLE message DROP COLUMN IF EXISTS forwarded_from_id;
