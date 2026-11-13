package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/server"
)

func TestChannelsAndMembersE2E(t *testing.T) {
	e := newEnv(t)

	// 1. Agent cannot create a channel
	agentChanReq := apispec.CreateChannelRequest{
		Name: "agent-channel",
	}
	agentCreateStatus := e.do("POST", "/api/v1/channels", server.DevAgentToken, agentChanReq, nil)
	if agentCreateStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for agent POST /channels, got %d", agentCreateStatus)
	}

	// 2. Admin creates a private channel with a description
	privKind := apispec.Private
	desc := "Internal ops discussion"
	createReq := apispec.CreateChannelRequest{
		Name:        "ops-team",
		Kind:        &privKind,
		Description: &desc,
	}
	var createdChan apispec.Channel
	status := e.do("POST", "/api/v1/channels", server.DevAdminToken, createReq, &createdChan)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created for channel creation, got %d", status)
	}
	if createdChan.Name != "ops-team" || createdChan.Description == nil || *createdChan.Description != desc {
		t.Fatalf("unexpected channel: %+v", createdChan)
	}

	// 3. Create a non-admin human actor (member role) and another admin human actor
	memberRole := apispec.Member
	var alice apispec.CreateActorResponse
	status = e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind:   apispec.ActorKindHuman,
		Handle: "alice",
		Role:   &memberRole,
	}, &alice)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for creating alice, got %d", status)
	}

	adminRole := apispec.Admin
	var bobAdmin apispec.CreateActorResponse
	status = e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind:   apispec.ActorKindHuman,
		Handle: "bob-admin",
		Role:   &adminRole,
	}, &bobAdmin)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for creating bob-admin, got %d", status)
	}

	// 4. Non-member (alice) cannot read the private channel or list its members
	var memList apispec.ChannelMemberList
	status = e.do("GET", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), alice.Token, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for alice listing members of private channel, got %d", status)
	}

	// 5. Agent cannot add members (403)
	canWrite := true
	addStatus := e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAgentToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(alice.Actor.Id),
		CanWrite: &canWrite,
	}, nil)
	if addStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for agent adding member, got %d", addStatus)
	}

	// 6. Non-admin human (alice) cannot add members (403)
	addStatus = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), alice.Token, apispec.AddMemberRequest{ActorId: httpx.Ptr(bobAdmin.Actor.Id),
		CanWrite: &canWrite,
	}, nil)
	if addStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for alice adding member, got %d", addStatus)
	}

	// 7. Admin adds alice and bobAdmin to the channel
	status = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(alice.Actor.Id),
		CanWrite: &canWrite,
	}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 for adding alice, got %d", status)
	}

	status = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAdminToken, apispec.AddMemberRequest{ActorId: httpx.Ptr(bobAdmin.Actor.Id),
		CanWrite: &canWrite,
	}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 for adding bobAdmin, got %d", status)
	}

	// Now alice is a member, so she can list members
	status = e.do("GET", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), alice.Token, nil, &memList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for alice listing members, got %d", status)
	}
	if len(memList.Members) != 3 {
		t.Fatalf("expected 3 members (admin, alice, bobAdmin), got %d", len(memList.Members))
	}

	// 8. Non-admin human cannot remove members (403)
	delStatus := e.do("DELETE", fmt.Sprintf("/api/v1/channels/%s/members/%s", createdChan.Id, bobAdmin.Actor.Id), alice.Token, nil, nil)
	if delStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for alice deleting member, got %d", delStatus)
	}

	// 9. Admin removes alice
	delStatus = e.do("DELETE", fmt.Sprintf("/api/v1/channels/%s/members/%s", createdChan.Id, alice.Actor.Id), server.DevAdminToken, nil, nil)
	if delStatus != http.StatusNoContent {
		t.Fatalf("expected 204 for removing alice, got %d", delStatus)
	}

	// Alice is no longer a member, so she loses read access
	status = e.do("GET", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), alice.Token, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for removed alice accessing private channel, got %d", status)
	}

	// 10. Removing the last human admin of a private channel is refused
	// We have 2 admins right now: Admin and bobAdmin. Removing bobAdmin should succeed.
	delStatus = e.do("DELETE", fmt.Sprintf("/api/v1/channels/%s/members/%s", createdChan.Id, bobAdmin.Actor.Id), server.DevAdminToken, nil, nil)
	if delStatus != http.StatusNoContent {
		t.Fatalf("expected 204 for removing bobAdmin, got %d", delStatus)
	}

	// Now only 1 human admin (Admin) remains in the private channel.
	// Removing Admin should return 409 Conflict.
	var meAdmin apispec.Actor
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &meAdmin)
	delStatus = e.do("DELETE", fmt.Sprintf("/api/v1/channels/%s/members/%s", createdChan.Id, meAdmin.Id), server.DevAdminToken, nil, nil)
	if delStatus != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when removing the last human admin of a private channel, got %d", delStatus)
	}
}

// TestRemovedMemberStopsReceivingLiveEvents: a member removed from a private
// channel must not keep getting its events over an open WebSocket (the
// connection caches read access per channel).
func TestRemovedMemberStopsReceivingLiveEvents(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	var priv struct{ ID string }
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "secret", "kind": "private"}, &priv); s != http.StatusCreated {
		t.Fatalf("create private channel: %d", s)
	}
	memberRole := apispec.Member
	var alice apispec.CreateActorResponse
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind: apispec.ActorKindHuman, Handle: "alice", Role: &memberRole,
	}, &alice); s != http.StatusCreated {
		t.Fatalf("create alice: %d", s)
	}
	for _, ch := range []string{priv.ID, general} {
		if s := e.do("POST", "/api/v1/channels/"+ch+"/members", server.DevAdminToken, map[string]any{"actor_id": alice.Actor.Id}, nil); s != http.StatusNoContent {
			t.Fatalf("add alice: %d", s)
		}
	}

	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = ws.CloseNow() }()
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": alice.Token}); err != nil {
		t.Fatal(err)
	}
	expectFrame(t, ctx, ws, "welcome")
	expectFrame(t, ctx, ws, "replay_done")

	// While a member, alice sees the private channel live (and the
	// connection caches her read access).
	if _, s := e.send(server.DevAdminToken, priv.ID, map[string]any{"body": "before", "client_msg_id": "p1"}); s != http.StatusCreated {
		t.Fatalf("send before: %d", s)
	}
	if f := nextMessageFrame(t, ctx, ws); f["channel_id"] != priv.ID {
		t.Fatalf("want the private message first, got %v", f)
	}

	if s := e.do("DELETE", "/api/v1/channels/"+priv.ID+"/members/"+alice.Actor.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("remove alice: %d", s)
	}
	if _, s := e.send(server.DevAdminToken, priv.ID, map[string]any{"body": "after", "client_msg_id": "p2"}); s != http.StatusCreated {
		t.Fatalf("send after: %d", s)
	}
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "public", "client_msg_id": "g1"}); s != http.StatusCreated {
		t.Fatalf("send public: %d", s)
	}
	// The next message alice sees must be the public one.
	if f := nextMessageFrame(t, ctx, ws); f["channel_id"] != general {
		t.Fatalf("removed member still got a private event: %v", f)
	}
}

// nextMessageFrame skips to the next message.created event frame.
func nextMessageFrame(t *testing.T, ctx context.Context, ws *websocket.Conn) map[string]any {
	t.Helper()
	for {
		var f map[string]any
		if err := wsjson.Read(ctx, ws, &f); err != nil {
			t.Fatalf("waiting for a message event: %v", err)
		}
		if f["type"] == "event" && f["event_type"] == "message.created" {
			return f
		}
	}
}

// Round 40: only public and private channels are created by name.
func TestCreateChannelRefusesOtherKinds(t *testing.T) {
	e := newEnv(t)
	for _, kind := range []string{"dm", "foo"} {
		if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "fake-" + kind, "kind": kind}, nil); s != http.StatusBadRequest {
			t.Fatalf("kind %s: %d, want 400", kind, s)
		}
	}
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "secret", "kind": "private"}, nil); s != http.StatusCreated {
		t.Fatalf("private: %d", s)
	}
}
