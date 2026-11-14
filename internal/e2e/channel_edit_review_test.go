// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// An outsider must not learn that a private channel is archived.
func TestArchivedChannelOutsiderGetsForbidden(t *testing.T) {
	e := newEnv(t)
	_, outsiderTok := e.createUser("outsider", "Outsider")
	privKind := apispec.Private
	var secret apispec.Channel
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{Name: "secret", Kind: &privKind}, &secret); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	msg, _ := e.send(server.DevAdminToken, secret.Id.String(), map[string]any{"body": "hello"})
	if s := e.do("POST", "/api/v1/channels/"+secret.Id.String()+"/archive", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatalf("archive: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+secret.Id.String()+"/messages", outsiderTok, map[string]any{"body": "x"}, nil); s != http.StatusForbidden {
		t.Fatalf("outsider send: %d, want 403", s)
	}
	if s := e.do("PUT", "/api/v1/messages/"+msg.ID+"/reactions/%F0%9F%91%8D", outsiderTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("outsider reaction: %d, want 403", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+secret.Id.String()+"/messages", server.DevAdminToken, map[string]any{"body": "x"}, nil); s != http.StatusConflict {
		t.Fatalf("admin send: %d, want 409", s)
	}
}

// A role does not make an agent a channel editor: agents never change a channel.
func TestChannelEditorRefusesAnAgentWithAdminRole(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin, err := e.app.Identity.Authenticate(ctx, server.DevAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	agent := identity.Actor{ID: ids.ID(uuid.New()), WorkspaceID: admin.WorkspaceID, Kind: identity.KindAgent, Role: identity.RoleAdmin, Handle: "root-bot"}
	general, err := ids.Parse(e.generalID())
	if err != nil {
		t.Fatal(err)
	}
	name := "renamed"
	if _, err := e.app.Conversation.UpdateChannel(ctx, agent, general, conversation.ChannelUpdate{Name: &name}); err == nil {
		t.Fatal("an agent renamed a channel")
	}
	if _, err := e.app.Conversation.ArchiveChannel(ctx, agent, general); err == nil {
		t.Fatal("an agent archived a channel")
	}
	if err := e.app.Conversation.AddMembers(ctx, agent, general, []ids.ID{admin.ID}, true); err == nil {
		t.Fatal("an agent added a member")
	}
	if err := e.app.Conversation.RemoveChannelMember(ctx, agent, general, admin.ID); err == nil {
		t.Fatal("an agent removed a member")
	}
}

// A reply to a request whose origin channel was archived stays a plain DM
// item: the agent could not answer in the origin. A terminal in an archived
// channel wakes nobody.
func TestArchivedOriginAndTerminalWake(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()
	askerID, askerTok := e.createActor("agent", "asker", "member")
	_, helperTok := e.createActor("agent", "helper", "member")
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": askerID, "can_write": true}, nil)
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@asker coordinate"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	asker, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", askerTok, headerTripper{
		headers: map[string]string{"X-Colonyhall-Channel": general, "X-Colonyhall-Thread": root.ID}, base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = asker.Close() }()
	if res, err := asker.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "@helper", "text": "run the tests"}}); err != nil || res.IsError {
		t.Fatalf("request: %v %+v", err, res)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/archive", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatalf("archive: %d", s)
	}
	helper, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", helperTok, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = helper.Close() }()
	if res, err := helper.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "@asker", "text": "30/30 green"}}); err != nil || res.IsError {
		t.Fatalf("reply: %v %+v", err, res)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var ib apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", askerTok, nil, &ib)
	found := false
	for _, it := range ib.Items {
		if it.Author == "helper" {
			found = true
			if it.ReplyOrigin != nil {
				t.Fatalf("the reply was routed to the archived channel: %+v", it.ReplyOrigin)
			}
		}
	}
	if !found {
		t.Fatalf("the reply did not arrive: %+v", ib.Items)
	}
	before := ib.Total
	s := e.do("POST", "/api/v1/agent/terminal-events", askerTok, map[string]any{"name": "build", "channel_id": general, "duration_ms": 1, "tail": "done"}, nil)
	if s != http.StatusConflict {
		t.Fatalf("terminal in an archived channel: %d, want 409", s)
	}
	e.do("GET", "/api/v1/agent/inbox", askerTok, nil, &ib)
	if ib.Total != before {
		t.Fatalf("the terminal woke the agent: %d items, was %d", ib.Total, before)
	}
}
