-- +goose Up
-- Round 45: every workspace gets a subdomain name (slug) and a state. In
-- multi-tenant mode the slug picks the workspace from the Host header; in
-- single-tenant mode neither is used. Expand, backfill, contract.
ALTER TABLE workspace ADD COLUMN slug text;
ALTER TABLE workspace ADD COLUMN state text NOT NULL DEFAULT 'active'
    CHECK (state IN ('active', 'suspended'));

-- The slug comes from the name, the same way identity.SlugFromName makes it;
-- an invalid, reserved or duplicate one falls back to identity.IDSlug. A
-- chosen slug is set afterwards with `colonyhall set-slug`.
WITH derived AS (
    SELECT id, created_at,
           trim(both '-' from left(trim(both '-' from regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g')), 40)) AS cand
    FROM workspace
), ranked AS (
    SELECT id, cand, row_number() OVER (PARTITION BY cand ORDER BY created_at, id) AS n FROM derived
)
UPDATE workspace w
SET slug = CASE
    WHEN r.n = 1
     AND r.cand ~ '^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$'
     AND r.cand NOT LIKE '%--%'
     AND r.cand <> ALL (ARRAY['admin','api','app','www','mail','smtp','imap','pop','ftp','ns1','ns2',
         'static','assets','cdn','status','docs','help','support','billing','auth',
         'login','sso','id','operator','root','system','localhost','test','dev'])
    THEN r.cand
    ELSE 'ws-' || left(replace(w.id::text, '-', ''), 12)
END
FROM ranked r WHERE r.id = w.id;

ALTER TABLE workspace ALTER COLUMN slug SET NOT NULL;
ALTER TABLE workspace ADD CONSTRAINT workspace_slug_key UNIQUE (slug);

-- +goose Down
ALTER TABLE workspace DROP CONSTRAINT workspace_slug_key;
ALTER TABLE workspace DROP COLUMN state;
ALTER TABLE workspace DROP COLUMN slug;
