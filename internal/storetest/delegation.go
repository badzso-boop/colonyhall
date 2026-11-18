// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

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
	"colonyhall/internal/platform/ids"
)

func testDelegation(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	ctx = db.WithWorkspace(ctx, f.ws.ID) // round 45: every store call is scoped

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

	// -------------------------------------------------------------
	// ListPendingByPairAndConversation
	// -------------------------------------------------------------
	msg2 := conversation.Message{
		ID:          gen.New(),
		WorkspaceID: f.ws.ID,
		ChannelID:   f.channel.ID,
		AuthorID:    f.agent.ID,
		Kind:        "text",
		Body:        "Task 2",
		CreatedAt:   t0.Add(2 * time.Minute),
	}
	must(t, b.Conversation.InsertMessage(ctx, msg2))

	q2ID := gen.New()
	must(t, b.Question.Insert(ctx, question.Question{
		ID:           q2ID,
		Number:       num + 1,
		WorkspaceID:  f.ws.ID,
		AskedBy:      f.agent.ID,
		ToHuman:      true,
		ChannelID:    f.channel.ID,
		ThreadRootID: msg.ID,
		Text:         "Approve task 2?",
		Options:      []string{"Jóváhagyom", "Elutasítom"},
		State:        question.StateOpen,
		CreatedAt:    t0.Add(2 * time.Minute),
	}))

	d2ID := gen.New()
	d2 := delegation.Delegation{
		ID:           d2ID,
		WorkspaceID:  f.ws.ID,
		MessageID:    msg2.ID,
		ChannelID:    f.channel.ID,
		ThreadRootID: msg.ID,
		FromAgentID:  f.agent.ID,
		ToAgentID:    agent2.ID,
		QuestionID:   q2ID,
		State:        delegation.StatePending,
		CreatedAt:    t0.Add(2 * time.Minute),
	}
	must(t, b.Delegation.Insert(ctx, d2))

	pendingPair, err := b.Delegation.ListPendingByPairAndConversation(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, msg.ID)
	must(t, err)
	if len(pendingPair) != 1 || pendingPair[0].ID != d2ID {
		t.Fatalf("expected pendingPair to contain d2, got %+v", pendingPair)
	}

	// Unordered pair check (agent2, f.agent)
	pendingPairRev, err := b.Delegation.ListPendingByPairAndConversation(ctx, f.ws.ID, agent2.ID, f.agent.ID, f.channel.ID, msg.ID)
	must(t, err)
	if len(pendingPairRev) != 1 || pendingPairRev[0].ID != d2ID {
		t.Fatalf("expected reversed pair to find d2, got %+v", pendingPairRev)
	}

	// Different conversation returns empty
	otherConvPending, err := b.Delegation.ListPendingByPairAndConversation(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, gen.New())
	must(t, err)
	if len(otherConvPending) != 0 {
		t.Fatalf("expected empty for other conversation, got %d", len(otherConvPending))
	}

	// ApproveViaGrant: only from pending, and the flag is stored.
	if updated.ViaGrant {
		t.Fatalf("an admin approval must not be marked via_grant")
	}
	aff, err = b.Delegation.ApproveViaGrant(ctx, dID, f.human.ID, now)
	must(t, err)
	if aff != 0 {
		t.Fatalf("ApproveViaGrant on an approved delegation: expected 0 affected, got %d", aff)
	}
	aff, err = b.Delegation.ApproveViaGrant(ctx, d2ID, f.human.ID, now)
	must(t, err)
	if aff != 1 {
		t.Fatalf("ApproveViaGrant: expected 1 affected, got %d", aff)
	}
	viaGrant, err := b.Delegation.GetByID(ctx, d2ID)
	must(t, err)
	if viaGrant.State != delegation.StateApproved || !viaGrant.ViaGrant || viaGrant.DecidedBy == nil || *viaGrant.DecidedBy != f.human.ID {
		t.Fatalf("ApproveViaGrant: unexpected delegation %+v", viaGrant)
	}

	// -------------------------------------------------------------
	// Settings
	// -------------------------------------------------------------
	// GetSettings before insert returns ErrNotFound
	_, err = b.Delegation.GetSettings(ctx, f.ws.ID)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing settings, got %v", err)
	}

	st := delegation.Settings{
		WorkspaceID:      f.ws.ID,
		GrantIdleMinutes: 45,
		GrantMaxHours:    8,
		UpdatedAt:        t0,
	}
	must(t, b.Delegation.UpsertSettings(ctx, st))

	gotSt, err := b.Delegation.GetSettings(ctx, f.ws.ID)
	must(t, err)
	if gotSt.GrantIdleMinutes != 45 || gotSt.GrantMaxHours != 8 {
		t.Fatalf("unexpected settings: %+v", gotSt)
	}

	// Update settings
	st.GrantIdleMinutes = 30
	must(t, b.Delegation.UpsertSettings(ctx, st))
	gotSt, err = b.Delegation.GetSettings(ctx, f.ws.ID)
	must(t, err)
	if gotSt.GrantIdleMinutes != 30 {
		t.Fatalf("expected updated idle minutes 30, got %d", gotSt.GrantIdleMinutes)
	}

	// -------------------------------------------------------------
	// Grants
	// -------------------------------------------------------------
	canonA, canonB := delegation.CanonicalPair(f.agent.ID, agent2.ID)
	grantID := gen.New()
	grant := delegation.Grant{
		ID:           grantID,
		WorkspaceID:  f.ws.ID,
		ChannelID:    f.channel.ID,
		ThreadRootID: msg.ID,
		AgentAID:     canonA,
		AgentBID:     canonB,
		DelegationID: dID,
		ApprovedBy:   f.human.ID,
		ApprovedAt:   t0,
		LastUsedAt:   t0,
		ExpiresAt:    t0.Add(8 * time.Hour),
	}
	must(t, b.Delegation.InsertGrant(ctx, grant))

	// GetGrantByID
	gotGrant, err := b.Delegation.GetGrantByID(ctx, grantID)
	must(t, err)
	if gotGrant.ID != grantID || gotGrant.AgentAID != canonA || gotGrant.AgentBID != canonB {
		t.Fatalf("unexpected grant retrieved: %+v", gotGrant)
	}

	// GetActiveGrant
	activeG, err := b.Delegation.GetActiveGrant(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, msg.ID, t0.Add(time.Hour))
	must(t, err)
	if activeG.ID != grantID {
		t.Fatalf("expected grant %s, got %s", grantID, activeG.ID)
	}

	// Reverse pair lookup works
	activeGRev, err := b.Delegation.GetActiveGrant(ctx, f.ws.ID, agent2.ID, f.agent.ID, f.channel.ID, msg.ID, t0.Add(time.Hour))
	must(t, err)
	if activeGRev.ID != grantID {
		t.Fatalf("expected grant %s on reverse lookup, got %s", grantID, activeGRev.ID)
	}

	// Different conversation returns not found
	_, err = b.Delegation.GetActiveGrant(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, gen.New(), t0.Add(time.Hour))
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for wrong thread, got %v", err)
	}

	// Lookup after ExpiresAt returns ErrNotFound
	_, err = b.Delegation.GetActiveGrant(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, msg.ID, t0.Add(9*time.Hour))
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after expires_at, got %v", err)
	}

	// Update last used
	usedAt := t0.Add(2 * time.Hour)
	must(t, b.Delegation.UpdateGrantLastUsed(ctx, grantID, usedAt))
	gotGrant, err = b.Delegation.GetGrantByID(ctx, grantID)
	must(t, err)
	if !gotGrant.LastUsedAt.Equal(usedAt) {
		t.Fatalf("expected lastUsedAt %v, got %v", usedAt, gotGrant.LastUsedAt)
	}

	// ListGrants
	allGrants, err := b.Delegation.ListGrants(ctx, f.ws.ID, false, t0.Add(10*time.Hour))
	must(t, err)
	if len(allGrants) != 1 {
		t.Fatalf("expected 1 grant in allGrants, got %d", len(allGrants))
	}

	activeGrants, err := b.Delegation.ListGrants(ctx, f.ws.ID, true, t0.Add(3*time.Hour))
	must(t, err)
	if len(activeGrants) != 1 {
		t.Fatalf("expected 1 active grant at 3h, got %d", len(activeGrants))
	}

	// Revoke grant
	revokeTime := t0.Add(3 * time.Hour)
	affRevoke, err := b.Delegation.RevokeGrantConditional(ctx, grantID, f.human.ID, revokeTime)
	must(t, err)
	if affRevoke != 1 {
		t.Fatalf("expected 1 affected row on revoke, got %d", affRevoke)
	}

	// Revoke again should fail (aff = 0)
	affRevokeAgain, err := b.Delegation.RevokeGrantConditional(ctx, grantID, f.human.ID, revokeTime.Add(time.Minute))
	must(t, err)
	if affRevokeAgain != 0 {
		t.Fatalf("expected 0 affected on second revoke, got %d", affRevokeAgain)
	}

	// GetActiveGrant now returns ErrNotFound
	_, err = b.Delegation.GetActiveGrant(ctx, f.ws.ID, f.agent.ID, agent2.ID, f.channel.ID, msg.ID, revokeTime)
	if !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for revoked grant, got %v", err)
	}

	// One answer per request (2026-11-18): the first claim wins, the same
	// answer again still counts, another message does not.
	ans1 := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: agent2.ID, Kind: "text", Body: "done", CreatedAt: t0}
	ans2 := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: agent2.ID, Kind: "text", Body: "and more", CreatedAt: t0}
	must(t, b.Conversation.InsertMessage(ctx, ans1))
	must(t, b.Conversation.InsertMessage(ctx, ans2))
	for i, c := range []struct {
		answer ids.ID
		want   bool
	}{{ans1.ID, true}, {ans1.ID, true}, {ans2.ID, false}} {
		got, err := b.Delegation.ClaimAnswer(ctx, f.ws.ID, msg.ID, agent2.ID, c.answer, t0)
		must(t, err)
		if got != c.want {
			t.Fatalf("claim %d: got %v, want %v", i, got, c.want)
		}
	}
}
