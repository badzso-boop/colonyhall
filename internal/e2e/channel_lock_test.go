package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

func TestChannelAgentLock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()
	_, memberTok := e.createUser("anna", "Anna")
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "before"})
	// A pending wake item from before the lock waits for the unlock.
	before, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent look at this"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Only admins lock; agents never do.
	for _, tok := range []string{memberTok, server.DevAgentToken} {
		if s := e.do("POST", "/api/v1/channels/"+general+"/lock", tok, nil, nil); s != http.StatusForbidden {
			t.Fatalf("lock by non-admin: %d", s)
		}
	}
	var c apispec.Channel
	if s := e.do("POST", "/api/v1/channels/"+general+"/lock", server.DevAdminToken, nil, &c); s != http.StatusOK || c.AgentLockedAt == nil {
		t.Fatalf("lock: %d %+v", s, c)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// Agents cannot write: send, react, upload, MCP, decisions.
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "x"}, nil); s != http.StatusConflict {
		t.Fatalf("agent send: %d", s)
	}
	if s := e.do("PUT", "/api/v1/messages/"+root.ID+"/reactions/%F0%9F%91%8D", server.DevAgentToken, nil, nil); s != http.StatusConflict {
		t.Fatalf("agent reaction: %d", s)
	}
	if s, _ := uploadMultipart(t, e, general, "a.txt", []byte("x"), server.DevAgentToken); s != http.StatusConflict {
		t.Fatalf("agent upload: %d", s)
	}
	sess, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sess.Close() }()
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "general", "text": "x"}})
	if err != nil || !res.IsError {
		t.Fatalf("MCP send in a locked channel: %v %+v", err, res)
	}
	res, err = sess.CallTool(ctx, &mcp.CallToolParams{Name: "decision_add", Arguments: map[string]any{"text": "x", "rationale": "y", "thread": root.ID}})
	if err != nil || !res.IsError {
		t.Fatalf("decision in a locked channel: %v %+v", err, res)
	}
	// Reads stay allowed.
	if s := e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, nil, nil); s != http.StatusOK {
		t.Fatalf("agent read: %d", s)
	}
	res, err = sess.CallTool(ctx, &mcp.CallToolParams{Name: "channel_info", Arguments: map[string]any{"channel": "general"}})
	if err != nil || res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "Locked against agents") {
		t.Fatalf("channel_info: %v %+v", err, res)
	}

	// Humans keep talking, and it wakes no agent, not even after the unlock.
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent while locked"}); s != http.StatusCreated {
		t.Fatalf("human send: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if items := e.inbox(server.DevAgentToken); len(items) != 0 {
		t.Fatalf("a locked channel woke the agent: %+v", items)
	}

	if s := e.do("POST", "/api/v1/channels/"+general+"/unlock", memberTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("unlock by member: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/unlock", server.DevAdminToken, nil, &c); s != http.StatusOK {
		t.Fatalf("unlock: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	items := e.inbox(server.DevAgentToken)
	if len(items) != 1 || items[0].ItemID != before.ID {
		t.Fatalf("after the unlock only the item from before the lock wakes: %+v", items)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "back"}, nil); s != http.StatusCreated {
		t.Fatalf("agent send after unlock: %d", s)
	}
}

func TestChannelLockRefusesDMsAndArchived(t *testing.T) {
	e := newEnv(t)
	annaID, _ := e.createUser("anna", "Anna")
	var dm struct{ ID string }
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{annaID}}, &dm)
	if s := e.do("POST", "/api/v1/channels/"+dm.ID+"/lock", server.DevAdminToken, nil, nil); s != http.StatusConflict {
		t.Fatalf("locking a DM: %d", s)
	}
	general := e.generalID()
	e.do("POST", "/api/v1/channels/"+general+"/archive", server.DevAdminToken, nil, nil)
	if s := e.do("POST", "/api/v1/channels/"+general+"/lock", server.DevAdminToken, nil, nil); s != http.StatusConflict {
		t.Fatalf("locking an archived channel: %d", s)
	}
}
