package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/db"
)

func testDelegation(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)

	agent2 := identity.Actor{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		Kind:        identity.KindAgent,
		Handle:      "target-bot",
		DisplayName: "Target Bot",
		Role:        identity.RoleMember,
		State:       "active",
		CreatedAt:   t0,
	}
	must(t, b.Identity.CreateActor(ctx, agent2))
	must(t, b.Conversation.AddMember(ctx, conversation.Member{ChannelID: f.channel.ID, ActorID: agent2.ID, CanWrite: true, JoinedAt: t0}))

	msg := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.agent.ID,
		Kind:        "text",
		Body:        "Please run build",
		CreatedAt:   t0,
	}
	must(t, b.Conversation.InsertMessage(ctx, msg))

	num, err := b.Question.NextNumber(ctx, f.ws.ID)
	must(t, err)

	qID := gen.New()
	q := question.Question{
		ID:           qID,
		Number:       num,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToHuman:      true,
		ChannelID:    f.channel.ID,
		ThreadRootID: msg.ID,
		Text:         "Approve task?",
		Options:      []string{"Jóváhagyom", "Elutasítom"},
		State:        question.StateOpen,
		CreatedAt:    t0,
	}
	must(t, b.Question.Insert(ctx, q))

	dID := gen.New()
	d := delegation.Delegation{
		ID:           dID,
		WorkspaceID:  f.ws.ID,
		MessageID:    msg.ID,
		ChannelID:    f.channel.ID,
		ThreadRootID: msg.ID,
		FromAgentID:  f.agent.ID,
		ToAgentID:    agent2.ID,
		QuestionID:   qID,
		State:        delegation.StatePending,
		CreatedAt:    t0,
	}

	must(t, b.Delegation.Insert(ctx, d))

	// Duplicate insert is a no-op
	must(t, b.Delegation.Insert(ctx, d))

	// GetByID
	got, err := b.Delegation.GetByID(ctx, dID)
	must(t, err)
	if got.ID != dID || got.State != delegation.StatePending || got.FromAgentID != f.agent.ID || got.ToAgentID != agent2.ID {
		t.Fatalf("unexpected delegation retrieved: %+v", got)
	}

	// GetByQuestionID
	byQ, err := b.Delegation.GetByQuestionID(ctx, qID)
	must(t, err)
	if byQ.ID != dID {
		t.Fatalf("expected id %s, got %s", dID, byQ.ID)
	}

	// GetByMessageAndTarget
	byMsg, err := b.Delegation.GetByMessageAndTarget(ctx, msg.ID, agent2.ID)
	must(t, err)
	if byMsg.ID != dID {
		t.Fatalf("expected id %s, got %s", dID, byMsg.ID)
	}

	// ListByWorkspace with pending
	pendingState := delegation.StatePending
	list, err := b.Delegation.ListByWorkspace(ctx, f.ws.ID, &pendingState)
	must(t, err)
	if len(list) != 1 {
		t.Fatalf("expected 1 pending delegation, got %d", len(list))
	}

	// Conditional update with wrong expected state should fail (affected = 0)
	now := t0.Add(time.Minute)
	aff, err := b.Delegation.UpdateStateConditional(ctx, dID, delegation.StateApproved, delegation.StateDenied, &f.human.ID, now)
	must(t, err)
	if aff != 0 {
		t.Fatalf("expected 0 affected, got %d", aff)
	}

	// Conditional update with correct state should succeed (affected = 1)
	aff, err = b.Delegation.UpdateStateConditional(ctx, dID, delegation.StatePending, delegation.StateApproved, &f.human.ID, now)
	must(t, err)
	if aff != 1 {
		t.Fatalf("expected 1 affected, got %d", aff)
	}

	// Verify updated state
	updated, err := b.Delegation.GetByID(ctx, dID)
	must(t, err)
	if updated.State != delegation.StateApproved {
		t.Fatalf("expected state approved, got %s", updated.State)
	}
	if updated.DecidedBy == nil || *updated.DecidedBy != f.human.ID {
		t.Fatalf("expected decided by human %s, got %+v", f.human.ID, updated.DecidedBy)
	}

	// List pending now should return 0
	list, err = b.Delegation.ListByWorkspace(ctx, f.ws.ID, &pendingState)
	must(t, err)
	if len(list) != 0 {
		t.Fatalf("expected 0 pending, got %d", len(list))
	}

	// Non-existent ID lookup returns ErrNotFound
	_, err = b.Delegation.GetByID(ctx, gen.New())
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-existent delegation, got %v", err)
	}
}
