// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	convpg "colonyhall/internal/modules/conversation/postgres"
	"colonyhall/internal/modules/identity"
	idpg "colonyhall/internal/modules/identity/postgres"
	"colonyhall/internal/modules/operator"
	oppg "colonyhall/internal/modules/operator/postgres"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/db/opscope"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/migrations"
)

// Tables without a tenant policy, on purpose (docs/23-elkulonites.md, 3.2).
var noTenantPolicy = map[string]bool{
	"goose_db_version": true, // the migration tool's own table
	"consumer_offset":  true, // consumer names and sequence numbers, no customer data
	"db_size_sample":   true, // the whole database's daily size (round 50), no customer data
}

// TestPostgresRowLevelSecurity proves the second line of defence (round
// 45): as the application role, a query sees and writes only its own
// workspace's rows, nothing without a scope, and everything in system mode.
func TestPostgresRowLevelSecurity(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	url = AppRoleDatabase(t, url)
	ctx := context.Background()
	pg, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Pool.Close)
	if err := migrations.Up(ctx, pg.Pool); err != nil {
		t.Fatal(err)
	}

	var super, bypass bool
	must(t, pg.Pool.QueryRow(ctx, "SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&super, &bypass))
	if super || bypass {
		t.Fatalf("the tests must run as a role row-level security applies to (super %v, bypassrls %v)", super, bypass)
	}

	// Every table is covered: a new table without a policy fails here.
	rows, err := pg.Pool.Query(ctx, `SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity, count(p.polname)
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_policy p ON p.polrelid = c.oid
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') GROUP BY 1, 2, 3 ORDER BY 1`)
	must(t, err)
	tables := 0
	for rows.Next() {
		var name string
		var enabled, forced bool
		var policies int
		must(t, rows.Scan(&name, &enabled, &forced, &policies))
		tables++
		if noTenantPolicy[name] {
			continue
		}
		if !enabled || !forced || policies == 0 {
			t.Errorf("table %s: row-level security enabled %v, forced %v, %d policies; every table needs ENABLE, FORCE and a policy (docs/23-elkulonites.md)", name, enabled, forced, policies)
		}
	}
	must(t, rows.Err())
	if tables < 50 {
		t.Fatalf("only %d tables found", tables)
	}

	ids := &idpg.Store{DB: pg}
	conv := &convpg.Store{DB: pg}
	a := identity.Workspace{ID: gen.New(), Name: "acme", CreatedAt: t0, Slug: "acme"}
	b := identity.Workspace{ID: gen.New(), Name: "beta", CreatedAt: t0, Slug: "beta"}
	inA, inB := db.WithWorkspace(ctx, a.ID), db.WithWorkspace(ctx, b.ID)

	// Without a scope nothing can be written, not even a workspace.
	if err := ids.CreateWorkspace(ctx, a); err == nil {
		t.Fatal("an unscoped insert must be refused")
	}
	// A scope cannot write another workspace's row.
	if err := ids.CreateWorkspace(inA, b); err == nil {
		t.Fatal("workspace A's scope wrote workspace B")
	}
	must(t, ids.CreateWorkspace(inA, a))
	must(t, ids.CreateWorkspace(inB, b))
	alice := identity.Actor{ID: gen.New(), WorkspaceID: a.ID, Kind: identity.KindHuman, Handle: "alice", DisplayName: "Alice", Role: identity.RoleOwner, State: "active", CreatedAt: t0}
	bob := identity.Actor{ID: gen.New(), WorkspaceID: b.ID, Kind: identity.KindHuman, Handle: "bob", DisplayName: "Bob", Role: identity.RoleOwner, State: "active", CreatedAt: t0}
	must(t, ids.CreateActor(inA, alice))
	must(t, ids.CreateActor(inB, bob))
	if err := ids.CreateActor(inA, identity.Actor{ID: gen.New(), WorkspaceID: b.ID, Kind: identity.KindHuman, Handle: "mallory",
		DisplayName: "M", Role: identity.RoleMember, State: "active", CreatedAt: t0}); err == nil {
		t.Fatal("workspace A's scope created an actor in workspace B")
	}
	chA := conversation.Channel{ID: gen.New(), WorkspaceID: a.ID, Kind: conversation.KindPublic, Name: "general", CreatedBy: alice.ID, CreatedAt: t0}
	must(t, conv.CreateChannel(inA, chA))
	must(t, conv.AddMember(inA, conversation.Member{ChannelID: chA.ID, ActorID: alice.ID, CanWrite: true, JoinedAt: t0}))
	// The composite foreign key: B's actor cannot join A's channel, even in system mode.
	if err := conv.AddMember(system.Context(ctx), conversation.Member{ChannelID: chA.ID, ActorID: bob.ID, CanWrite: true, JoinedAt: t0}); err == nil {
		t.Fatal("an actor of workspace B joined a channel of workspace A")
	}

	// Reads: each scope sees its own rows only; no scope sees nothing.
	if _, err := ids.ActorByID(inB, alice.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("workspace B read workspace A's actor: %v", err)
	}
	if _, err := conv.ChannelByID(inB, chA.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("workspace B read workspace A's channel: %v", err)
	}
	if _, err := ids.ActorByID(ctx, alice.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("an unscoped read found a row: %v", err)
	}
	if got, err := ids.ActorByID(inA, alice.ID); err != nil || got.ID != alice.ID {
		t.Fatalf("workspace A reads its own actor: %v", err)
	}
	if got, err := ids.ActorByID(system.Context(ctx), bob.ID); err != nil || got.ID != bob.ID {
		t.Fatalf("system mode reads every workspace: %v", err)
	}
	// Updates do not reach another workspace's rows either.
	must(t, ids.SetActorState(inB, alice.ID, identity.StatePaused))
	if got, _ := ids.ActorByID(inA, alice.ID); got.State == identity.StatePaused {
		t.Fatal("workspace B changed workspace A's actor")
	}

	// The narrow lookups work in any scope and answer only "which workspace".
	if ws, err := ids.HandleWorkspaces(ctx, "bob"); err != nil || len(ws) != 1 || ws[0].ID != b.ID {
		t.Fatalf("handle lookup without a scope: %+v %v", ws, err)
	}
	if w, err := ids.WorkspaceBySlug(inA, "beta"); err != nil || w.ID != b.ID {
		t.Fatalf("slug lookup from another scope: %+v %v", w, err)
	}
	// They leave no system mode behind in the caller's transaction.
	must(t, pg.InTx(inA, func(ctx context.Context) error {
		if _, err := ids.HandleWorkspaces(ctx, "bob"); err != nil {
			return err
		}
		if _, err := ids.ActorByID(ctx, bob.ID); !errors.Is(err, db.ErrNotFound) {
			t.Errorf("system mode leaked out of a lookup function: %v", err)
		}
		return nil
	}))
	// Round 40: the limit functions work before authentication and leave
	// no system mode behind either.
	must(t, pg.InTx(inA, func(ctx context.Context) error {
		if err := ids.RecordAuthFailure(ctx, identity.AuthFailure{ID: gen.New(), Scope: identity.LimitLogin, Handle: "ghost", IP: "192.0.2.1", At: t0}); err != nil {
			return err
		}
		if _, err := ids.AuthFailures(ctx, identity.LimitLogin, "ghost", "192.0.2.1", t0.Add(-time.Hour)); err != nil {
			return err
		}
		if err := ids.ClearAuthFailures(ctx, identity.LimitLogin, "ghost", t0); err != nil {
			return err
		}
		if _, err := ids.UnattributedLockouts(ctx, t0.Add(-time.Hour)); err != nil {
			return err
		}
		if _, err := ids.ActorByID(ctx, bob.ID); !errors.Is(err, db.ErrNotFound) {
			t.Errorf("system mode leaked out of a limit function: %v", err)
		}
		return nil
	}))
	// A row without a workspace is invisible to every workspace scope.
	var n int
	must(t, pg.InTx(inA, func(ctx context.Context) error {
		return pg.Q(ctx).QueryRow(ctx, "SELECT count(*) FROM auth_failure").Scan(&n)
	}))
	if n != 0 {
		t.Fatalf("a workspace scope sees %d unattributed failure rows", n)
	}

	// Round 51: the operator scope sees the operator tables and no
	// workspace; a workspace scope does not see the operator tables.
	op := opscope.Context(ctx)
	ops := &oppg.Store{DB: pg}
	o := operator.Operator{ID: gen.New(), Handle: "ops", DisplayName: "Ops", PasswordHash: "x",
		TOTP: secretbox.Sealed{KeyID: "k1", Nonce: []byte("n"), Ciphertext: []byte("c")}, State: operator.StateActive, CreatedAt: t0}
	if err := ops.CreateOperator(inA, o); err == nil {
		t.Fatal("a workspace scope created an operator")
	}
	must(t, ops.CreateOperator(op, o))
	if _, err := ops.OperatorByID(inA, o.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("a workspace scope read an operator: %v", err)
	}
	if _, err := ops.OperatorByID(ctx, o.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("an unscoped read found an operator: %v", err)
	}
	if _, err := ops.OperatorByID(op, o.ID); err != nil {
		t.Fatalf("the operator scope reads its table: %v", err)
	}
	if _, err := ids.ActorByID(op, alice.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("the operator scope read a workspace's actor: %v", err)
	}
	if _, err := ids.WorkspaceByID(op, a.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("the operator scope read a workspace row: %v", err)
	}
	for _, table := range []string{"message", "channel", "actor", "event", "file", "workspace_invite"} {
		var n int
		must(t, pg.Q(op).QueryRow(op, "SELECT count(*) FROM "+table).Scan(&n))
		if n != 0 {
			t.Fatalf("the operator scope sees %d rows of %s", n, table)
		}
	}

	// Round 51: the operator's overview reads in the operator scope itself
	// through colony_operator_overview(): the numbers are right although
	// the scope sees no workspace rows, no other scope may call it, and it
	// leaves no system mode behind on the connection.
	var overview operator.Overview = ops
	got, err := overview.Customers(op, a.ID, t0.Add(-time.Minute))
	if err != nil || len(got) != 1 || got[0].ID != a.ID || got[0].Name != "acme" || got[0].Slug != "acme" ||
		got[0].Users != 1 || !got[0].HasOwner || got[0].Agents != 0 || got[0].Hosts != 0 || got[0].Invite != nil {
		t.Fatalf("overview of workspace A: %+v %v", got, err)
	}
	everyone := a.ID // the ids package name is taken by the store here
	clear(everyone[:])
	all, err := overview.Customers(op, everyone, t0.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[[16]byte]bool{}
	for _, c := range all {
		seen[c.ID] = true
	}
	if !seen[a.ID] || !seen[b.ID] {
		t.Fatalf("overview of every workspace misses A or B: %+v", all)
	}
	for name, c := range map[string]context.Context{"workspace": inA, "unscoped": ctx, "system": system.Context(ctx)} {
		if _, err := overview.Customers(c, a.ID, t0); err == nil {
			t.Fatalf("a %s scope ran the operator's overview", name)
		}
	}
	var left int
	must(t, pg.Q(op).QueryRow(op, "SELECT count(*) FROM actor").Scan(&left))
	if left != 0 {
		t.Fatalf("the overview left system mode on: the operator scope now sees %d actors", left)
	}

	// Round 51: a nested transaction with another scope fails instead of
	// running with the outer one (system mode here would see workspace B).
	err = pg.InTx(system.Context(ctx), func(ctx context.Context) error {
		return pg.InTx(db.WithWorkspace(ctx, a.ID), func(ctx context.Context) error {
			_, err := ids.ActorByID(ctx, bob.ID)
			return err
		})
	})
	if !errors.Is(err, db.ErrScopeChanged) {
		t.Fatalf("a nested transaction with another scope: %v", err)
	}

	// Round 51b: a read-only context (the operator's break-glass session)
	// gets connections on which Postgres itself refuses every write, in a
	// transaction or not; reads work; the next plain user writes again.
	ro := db.ReadOnly(inA)
	if _, err := ids.ActorByID(ro, alice.ID); err != nil {
		t.Fatalf("a read-only read: %v", err)
	}
	carol := identity.Actor{ID: gen.New(), WorkspaceID: a.ID, Kind: identity.KindHuman, Handle: "carol", DisplayName: "Carol",
		Role: identity.RoleMember, State: "active", CreatedAt: t0}
	if err := ids.CreateActor(ro, carol); err == nil {
		t.Fatal("a read-only context wrote a row")
	}
	if err := pg.InTx(ro, func(ctx context.Context) error { return ids.CreateActor(ctx, carol) }); err == nil {
		t.Fatal("a read-only transaction wrote a row")
	}
	must(t, ids.CreateActor(inA, carol))
}
