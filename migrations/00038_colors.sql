-- +goose Up
-- Machine and agent colours (round 33): a fixed palette of names, never hex.
-- Existing machines get a colour by creation order, as new ones will.
ALTER TABLE host ADD COLUMN color TEXT;
UPDATE host SET color = (ARRAY['blue','green','orange','violet','teal','pink','amber','cyan','lime','red'])[((n.rn - 1) % 10) + 1]
FROM (SELECT id, row_number() OVER (PARTITION BY workspace_id ORDER BY created_at, id) AS rn FROM host) n
WHERE host.id = n.id;
ALTER TABLE host
    ALTER COLUMN color SET NOT NULL,
    ADD CONSTRAINT host_color_check CHECK (color IN ('red','orange','amber','lime','green','teal','cyan','blue','violet','pink'));

-- An agent's own colour; without one it shows its machine's.
ALTER TABLE actor
    ADD COLUMN color TEXT,
    ADD CONSTRAINT actor_color_check CHECK (color IS NULL OR (kind = 'agent' AND color IN ('red','orange','amber','lime','green','teal','cyan','blue','violet','pink')));

-- +goose Down
ALTER TABLE actor DROP CONSTRAINT IF EXISTS actor_color_check, DROP COLUMN IF EXISTS color;
ALTER TABLE host DROP CONSTRAINT IF EXISTS host_color_check, DROP COLUMN IF EXISTS color;
