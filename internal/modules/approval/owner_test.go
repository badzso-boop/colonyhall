// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package approval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type readAll struct{}

func (readAll) CanRead(context.Context, identity.Actor, ids.ID) (bool, error)  { return true, nil }
func (readAll) CanWrite(context.Context, identity.Actor, ids.ID) (bool, error) { return true, nil }
func (readAll) SendMessage(context.Context, identity.Actor, conversation.SendInput) (conversation.SendResult, error) {
	return conversation.SendResult{}, nil
}

// oneRequest is a store holding a single request.
type oneRequest struct{ r approval.ApprovalRequest }

func (s *oneRequest) Create(_ context.Context, r approval.ApprovalRequest) error { s.r = r; return nil }
func (s *oneRequest) ByID(_ context.Context, id ids.ID) (approval.ApprovalRequest, error) {
	if id != s.r.ID {
		return approval.ApprovalRequest{}, db.ErrNotFound
	}
	return s.r, nil
}
func (s *oneRequest) Decide(context.Context, ids.ID, string, ids.ID, string, time.Time) error {
	return nil
}
func (s *oneRequest) ExpirePendingByWorkspace(context.Context, ids.ID, time.Time) (int64, error) {
	return 0, nil
}
func (s *oneRequest) ListPendingByWorkspace(context.Context, ids.ID) ([]approval.ApprovalRequest, error) {
	return nil, nil
}

type agentDir map[ids.ID]identity.Actor

func (d agentDir) ActorByID(_ context.Context, id ids.ID) (identity.Actor, error) {
	a, ok := d[id]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, nil
}

// Round 38: approving a tool use is giving the agent a task, so only its
// owner or an admin sees and decides the request, even in a shared channel.
func TestApprovalOfAnotherPersonsAgentIsNotFound(t *testing.T) {
	ctx := context.Background()
	ws, member, agent := ids.ID{1}, ids.ID{2}, ids.ID{3}
	store := &oneRequest{}
	req := approval.ApprovalRequest{ID: ids.ID{4}, WorkspaceID: ws, AgentID: agent, ChannelID: ids.ID{5}, ToolName: "Bash",
		State: approval.StatePending, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := store.Create(ctx, req); err != nil {
		t.Fatal(err)
	}
	dir := agentDir{agent: {ID: agent, WorkspaceID: ws, Kind: identity.KindAgent}}
	svc := &approval.Service{Store: store, Conversation: readAll{}, Agents: dir}
	by := identity.Actor{ID: member, WorkspaceID: ws, Kind: identity.KindHuman, Role: identity.RoleMember}

	if _, err := svc.Get(ctx, by, req.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Decide(ctx, by, req.ID, approval.DecisionAllow, ""); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("decide: %v", err)
	}
	// Its owner sees it.
	dir[agent] = identity.Actor{ID: agent, WorkspaceID: ws, Kind: identity.KindAgent, OwnerID: member}
	if _, err := svc.Get(ctx, by, req.ID); err != nil {
		t.Fatalf("owner get: %v", err)
	}
}
