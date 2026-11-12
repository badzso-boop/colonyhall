package setup

import (
	"os"
	"testing"
)

// The dev stack's role script (deploy/dev-app-role.sql) is the installer's
// SQL for the dev database; this keeps the two from drifting apart.
func TestDevAppRoleScriptMatchesTheInstaller(t *testing.T) {
	want := "-- Generated from internal/setup/approle.go (TestDevAppRoleScriptMatchesTheInstaller).\n" +
		"-- `make dev` runs it before the server starts: the server connects as\n" +
		"-- colony_app, a role row-level security applies to (docs/23-elkulonites.md).\n" +
		appRoleSQL("colony", "colony_app")
	got, err := os.ReadFile("../../deploy/dev-app-role.sql")
	if os.Getenv("UPDATE_DEV_APP_ROLE") == "1" {
		if err := os.WriteFile("../../deploy/dev-app-role.sql", []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil || string(got) != want {
		t.Fatalf("deploy/dev-app-role.sql is out of date; run UPDATE_DEV_APP_ROLE=1 go test ./internal/setup/ -run TestDevAppRole (%v)", err)
	}
}
