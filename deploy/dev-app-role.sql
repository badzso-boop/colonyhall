-- Generated from internal/setup/approle.go (TestDevAppRoleScriptMatchesTheInstaller).
-- `make dev` runs it before the server starts: the server connects as
-- colony_app, a role row-level security applies to (docs/23-elkulonites.md).
\set ON_ERROR_STOP on
DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'colony_app') THEN
    CREATE ROLE colony_app LOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END $$;
ALTER ROLE colony_app NOSUPERUSER NOBYPASSRLS PASSWORD 'colony_app';
ALTER DATABASE colony OWNER TO colony_app;
ALTER SCHEMA public OWNER TO colony_app;
DO $$ DECLARE r record; BEGIN
  FOR r IN SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE n.nspname = 'public' AND pg_get_userbyid(c.relowner) <> 'colony_app'
             AND (c.relkind IN ('r', 'p', 'v')
                  -- a sequence of its own; a table's serial sequence follows its table
                  OR c.relkind = 'S' AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.objid = c.oid AND d.deptype IN ('a', 'i'))) LOOP
    EXECUTE format('ALTER %s %I OWNER TO colony_app', CASE r.relkind WHEN 'v' THEN 'VIEW' WHEN 'S' THEN 'SEQUENCE' ELSE 'TABLE' END, r.relname);
  END LOOP;
  FOR r IN SELECT p.oid::regprocedure AS fn FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
           WHERE n.nspname = 'public' AND pg_get_userbyid(p.proowner) <> 'colony_app' LOOP
    EXECUTE format('ALTER FUNCTION %s OWNER TO colony_app', r.fn);
  END LOOP;
END $$;
