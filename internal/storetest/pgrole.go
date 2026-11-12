package storetest

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"
)

// AppRole is the role the Postgres tests connect as: not a superuser and
// without BYPASSRLS, so row-level security applies to it (round 45).
const AppRole = "colony_app"

// AppRoleDatabase prepares the test database behind adminURL (a role that
// may create roles, e.g. the superuser of the test container) for the
// application role: it creates AppRole if missing, drops the public schema
// and recreates it owned by AppRole, and returns the URL to connect as
// AppRole. The migrations then run as AppRole, which owns the tables, and
// FORCE ROW LEVEL SECURITY applies to it as it does to the server.
func AppRoleDatabase(t *testing.T, adminURL string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, q := range []string{
		`DO $$ BEGIN
		   IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '` + AppRole + `') THEN
		     CREATE ROLE ` + AppRole + ` LOGIN PASSWORD '` + AppRole + `' NOSUPERUSER NOBYPASSRLS;
		   END IF;
		 END $$`,
		`ALTER ROLE ` + AppRole + ` NOSUPERUSER NOBYPASSRLS PASSWORD '` + AppRole + `'`,
		`DROP SCHEMA IF EXISTS public CASCADE`,
		`CREATE SCHEMA public AUTHORIZATION ` + AppRole,
	} {
		if _, err := conn.Exec(ctx, q); err != nil {
			t.Fatalf("prepare %s: %v", AppRole, err)
		}
	}
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(AppRole, AppRole)
	return u.String()
}
