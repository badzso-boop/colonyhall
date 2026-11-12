-- +goose Up
-- Round 50: one sample of the database size a day, for the System status
-- page's growth line. Written by every server's maintenance loop; the first
-- write of a day wins (ON CONFLICT DO NOTHING). Kept for 400 days.
CREATE TABLE db_size_sample (
    sampled_on  date PRIMARY KEY,
    size_bytes  bigint NOT NULL
);

-- +goose Down
DROP TABLE db_size_sample;
