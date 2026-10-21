package main

import (
	"context"
	"errors"
	"testing"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/server"
)

func newApp(t *testing.T) *server.App {
	t.Helper()
	app, err := server.New(context.Background(), server.Options{Store: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	return app
}

func TestBootstrapFreshDatabase(t *testing.T) {
	ctx := context.Background()
	app := newApp(t)
	token, err := bootstrap(ctx, app, "colonyhall", "norbi", "Norbi", "a-long-enough-password")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("no owner token")
	}
	owner, _, _, err := app.Identity.Login(ctx, identity.LoginInput{Handle: "norbi", Password: "a-long-enough-password"})
	if err != nil {
		t.Fatalf("login with the bootstrap password: %v", err)
	}
	if owner.Role != identity.RoleOwner || owner.Kind != identity.KindHuman {
		t.Fatalf("owner: %+v", owner)
	}
	if a, err := app.Identity.Authenticate(ctx, token); err != nil || a.ID != owner.ID {
		t.Fatalf("owner token: %v %+v", err, a)
	}

	// A second run must not create a second workspace.
	if _, err := bootstrap(ctx, app, "other", "eve", "", "another-long-password"); !errors.Is(err, errNotEmpty) {
		t.Fatalf("second bootstrap: %v, want errNotEmpty", err)
	}
}

func TestBootstrapRefusesUsedDatabaseAndWeakPassword(t *testing.T) {
	ctx := context.Background()
	app := newApp(t)
	if _, err := bootstrap(ctx, app, "colonyhall", "norbi", "", "short"); !errors.Is(err, identity.ErrWeakPassword) {
		t.Fatalf("weak password: %v", err)
	}
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap(ctx, app, "colonyhall", "norbi", "", "a-long-enough-password"); !errors.Is(err, errNotEmpty) {
		t.Fatalf("after the dev seed: %v, want errNotEmpty", err)
	}
}
