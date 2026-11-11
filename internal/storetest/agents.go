package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/agents"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
)

// testAgentLifecycle: archiving frees the handle (round 33), lookups by
// handle see the active actor, and an agent's own instructions are kept.
func testAgentLifecycle(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	ctx = db.WithWorkspace(ctx, f.ws.ID) // round 45: every store call is scoped
	id := b.Identity

	// The old agent has a token; archiving revokes it and frees "bot".
	tok := identity.Token{ID: gen.New(), ActorID: f.agent.ID, Hash: []byte("bot-token"), Label: "default", CreatedAt: t0}
	must(t, id.CreateToken(ctx, tok))
	dup := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindAgent, Handle: "bot", DisplayName: "Bot 2",
		Role: identity.RoleMember, State: "active", CreatedAt: t0.Add(time.Hour)}
	if err := id.CreateActor(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("handle of an active agent: %v", err)
	}
	must(t, id.ArchiveActor(ctx, f.agent.ID, f.human.ID, t0.Add(time.Minute)))
	must(t, id.RevokeActorTokens(ctx, f.agent.ID, t0.Add(time.Minute)))
	old, err := id.ActorByID(ctx, f.agent.ID)
	if err != nil || old.ArchivedAt == nil || !old.ArchivedAt.Equal(t0.Add(time.Minute)) || old.ArchivedBy != f.human.ID || old.State != "stopped" {
		t.Fatalf("archived: %+v %v", old, err)
	}
	if _, err := id.ActorByTokenHash(ctx, []byte("bot-token"), t0.Add(time.Hour)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("token of an archived agent: %v", err)
	}
	if _, err := id.ActorByHandle(ctx, f.ws.ID, "bot"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("an archived agent is not found by handle: %v", err)
	}
	must(t, id.CreateActor(ctx, dup))
	got, err := id.ActorByHandle(ctx, f.ws.ID, "bot")
	if err != nil || got.ID != dup.ID {
		t.Fatalf("the handle points to the new agent: %+v %v", got, err)
	}
	active, err := id.ListActors(ctx, f.ws.ID)
	must(t, err)
	all, err := id.ListAllActors(ctx, f.ws.ID)
	must(t, err)
	if len(active) != 2 || len(all) != 3 || all[0].ID != f.agent.ID || all[1].ID != dup.ID {
		t.Fatalf("lists: active %d, all %+v", len(active), all)
	}
	// Archiving twice keeps the first time.
	must(t, id.ArchiveActor(ctx, f.agent.ID, f.human.ID, t0.Add(2*time.Hour)))
	if again, _ := id.ActorByID(ctx, f.agent.ID); !again.ArchivedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("archived again: %+v", again)
	}

	// Own instructions: none, then version 1; a version is written once.
	if _, err := b.Agents.InstructionsByAgent(ctx, dup.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("no instructions: %v", err)
	}
	in := agents.Instructions{AgentID: dup.ID, WorkspaceID: f.ws.ID, Version: 1, Text: "Írj tömören.", CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, b.Agents.CreateInstructions(ctx, in))
	if err := b.Agents.CreateInstructions(ctx, in); err == nil {
		t.Fatal("the same version twice")
	}
	gotIn, err := b.Agents.InstructionsByAgent(ctx, dup.ID)
	if err != nil || gotIn.Text != "Írj tömören." || gotIn.Version != 1 || gotIn.CreatedBy != f.human.ID || !gotIn.CreatedAt.Equal(t0) {
		t.Fatalf("instructions: %+v %v", gotIn, err)
	}
}
