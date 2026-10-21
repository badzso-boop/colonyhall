package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
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
	addStatus := e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAgentToken, apispec.AddMemberRequest{
		ActorId:  alice.Actor.Id,
		CanWrite: &canWrite,
	}, nil)
	if addStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for agent adding member, got %d", addStatus)
	}

	// 6. Non-admin human (alice) cannot add members (403)
	addStatus = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), alice.Token, apispec.AddMemberRequest{
		ActorId:  bobAdmin.Actor.Id,
		CanWrite: &canWrite,
	}, nil)
	if addStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for alice adding member, got %d", addStatus)
	}

	// 7. Admin adds alice and bobAdmin to the channel
	status = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  alice.Actor.Id,
		CanWrite: &canWrite,
	}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 for adding alice, got %d", status)
	}

	status = e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", createdChan.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  bobAdmin.Actor.Id,
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
