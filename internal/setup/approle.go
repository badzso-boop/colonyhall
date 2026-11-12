package setup

import "fmt"

// AppRole is the database role the server connects as on installs made
// since round 45: not a superuser and without BYPASSRLS, so row-level
// security applies to it (docs/23-elkulonites.md). Older installs keep
// connecting as the database superuser and get a warning at start-up.
const AppRole = "colony_app"

// appRoleSQL creates (or updates) the application role and hands it the
// database and everything in its public schema; it runs on every install
// and upgrade, so it is idempotent. The password is hex, safe in a literal;
// the SQL goes to psql on stdin, never on a command line.
func appRoleSQL(database, password string) string {
	return fmt.Sprintf(`\set ON_ERROR_STOP on
DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%[1]s') THEN
    CREATE ROLE %[1]s LOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END $$;
ALTER ROLE %[1]s NOSUPERUSER NOBYPASSRLS PASSWORD '%[3]s';
ALTER DATABASE %[2]s OWNER TO %[1]s;
ALTER SCHEMA public OWNER TO %[1]s;
DO $$ DECLARE r record; BEGIN
  FOR r IN SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE n.nspname = 'public' AND pg_get_userbyid(c.relowner) <> '%[1]s'
             AND (c.relkind IN ('r', 'p', 'v')
                  -- a sequence of its own; a table's serial sequence follows its table
                  OR c.relkind = 'S' AND NOT EXISTS (SELECT FROM pg_depend d WHERE d.objid = c.oid AND d.deptype IN ('a', 'i'))) LOOP
    EXECUTE format('ALTER %%s %%I OWNER TO %[1]s', CASE r.relkind WHEN 'v' THEN 'VIEW' WHEN 'S' THEN 'SEQUENCE' ELSE 'TABLE' END, r.relname);
  END LOOP;
  FOR r IN SELECT p.oid::regprocedure AS fn FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
           WHERE n.nspname = 'public' AND pg_get_userbyid(p.proowner) <> '%[1]s' LOOP
    EXECUTE format('ALTER FUNCTION %%s OWNER TO %[1]s', r.fn);
  END LOOP;
END $$;
`, AppRole, database, password)
}
