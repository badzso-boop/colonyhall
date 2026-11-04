-- +goose Up
-- The person an agent or a machine belongs to (round 38): a member sees only
-- their own agents' runs, steps, errors and folders, and only their own
-- machines. NULL: nobody's, visible to admins only.
ALTER TABLE actor ADD COLUMN owner_id uuid REFERENCES actor(id);
ALTER TABLE host ADD COLUMN owner_id uuid REFERENCES actor(id);
CREATE INDEX actor_owner_idx ON actor (owner_id) WHERE owner_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS actor_owner_idx;
ALTER TABLE host DROP COLUMN IF EXISTS owner_id;
ALTER TABLE actor DROP COLUMN IF EXISTS owner_id;
