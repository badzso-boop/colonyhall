// Package e2e runs whole flows against an in-process server with the
// in-memory stores, a manual clock and sequential IDs, so every run is
// deterministic and needs no database or AI.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/fakeagent"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

type env struct {
	t   *testing.T
	app *server.App
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	app, err := server.New(ctx, server.Options{
		Store: "memory",
		Clock: clock.NewManual(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)),
		IDs:   &ids.Sequential{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	return &env{t: t, app: app, srv: srv}
}

func (e *env) do(method, path, token string, body any, out any) int {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

func (e *env) generalID() string {
	var r struct {
		Channels []struct{ ID, Name string } `json:"channels"`
	}
	e.do("GET", "/api/v1/channels", server.DevAdminToken, nil, &r)
	for _, c := range r.Channels {
		if c.Name == "general" {
			return c.ID
		}
	}
	e.t.Fatal("#general not found")
	return ""
}

type message struct {
	ID           string `json:"id"`
	AuthorID     string `json:"author_id"`
	ThreadRootID string `json:"thread_root_id"`
	Body         string `json:"body"`
}

// TestMentionWakesAgentAndReplyArrives is the phase 0 walking skeleton:
// a human mentions an agent, the agent reads its inbox over MCP, replies in
// the thread, acknowledges, and a browser sees both messages live.
func TestMentionWakesAgentAndReplyArrives(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	// A browser connects and waits for live events.
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = ws.CloseNow() }()
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": server.DevAdminToken}); err != nil {
		t.Fatal(err)
	}
	expectFrame(t, ctx, ws, "welcome")
	expectFrame(t, ctx, ws, "replay_done")

	// The human mentions the agent.
	var sent message
	status := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken,
		map[string]any{"body": "@fake-agent ping", "client_msg_id": "c1"}, &sent)
	if status != http.StatusCreated {
		t.Fatalf("send: status %d", status)
	}
	if err := e.app.DeliveryConsumer.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	// The fake agent handles its inbox over MCP.
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	scenario, err := fakeagent.Load("../../scenarios/ping.yaml")
	if err != nil {
		t.Fatal(err)
	}
	agent := &fakeagent.Agent{Session: session, Scenario: scenario}
	if n, err := agent.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("first run: handled %d, err %v", n, err)
	}
	if n, err := agent.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("second run must find an empty inbox: handled %d, err %v", n, err)
	}

	// The reply is in the thread of the original message.
	var list struct{ Messages []message }
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &list)
	if len(list.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(list.Messages))
	}
	reply := list.Messages[1]
	if reply.Body != "pong" || reply.ThreadRootID != sent.ID {
		t.Fatalf("unexpected reply: %+v", reply)
	}

	// The browser saw both messages live, in order.
	var bodies []string
	for len(bodies) < 2 {
		f := expectFrame(t, ctx, ws, "event")
		if f.EventType == "message.created" {
			var p struct{ Body string }
			_ = json.Unmarshal(f.Data, &p)
			bodies = append(bodies, p.Body)
		}
	}
	if bodies[0] != "@fake-agent ping" || bodies[1] != "pong" {
		t.Fatalf("live events: %v", bodies)
	}
}

func TestSendIsIdempotent(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	var a, b message
	e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, map[string]any{"body": "hello", "client_msg_id": "same"}, &a)
	e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, map[string]any{"body": "hello", "client_msg_id": "same"}, &b)
	if a.ID == "" || a.ID != b.ID {
		t.Fatalf("resend created a duplicate: %s vs %s", a.ID, b.ID)
	}
}

func TestRejectsBadTokenAndForeignChannel(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	if s := e.do("GET", "/api/v1/me", "col_wrong", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("bad token: status %d", s)
	}
	// Create a private channel without the agent; the agent must not read it.
	var ch struct{ ID string }
	e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "secret", "kind": "private"}, &ch)
	if s := e.do("GET", "/api/v1/channels/"+ch.ID+"/messages", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("foreign channel: status %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "  "}, nil); s != http.StatusBadRequest {
		t.Fatalf("empty body: status %d", s)
	}
}

type frame struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
}

func expectFrame(t *testing.T, ctx context.Context, ws *websocket.Conn, typ string) frame {
	t.Helper()
	for {
		var f frame
		if err := wsjson.Read(ctx, ws, &f); err != nil {
			t.Fatalf("waiting for %q: %v", typ, err)
		}
		if f.Type == typ {
			return f
		}
	}
}

// TestWorkspaceIsolation: an admin of another workspace must not see or
// touch this workspace's channels, even with a known channel ID.
func TestWorkspaceIsolation(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	const otherAdmin = "col_other_workspace_admin_token"
	if _, _, _, err := e.app.Identity.Bootstrap(context.Background(), "other", "eve", "Eve", otherAdmin); err != nil {
		t.Fatal(err)
	}
	if s := e.do("GET", "/api/v1/channels/"+general+"/messages", otherAdmin, nil, nil); s != http.StatusForbidden {
		t.Fatalf("read foreign channel: status %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", otherAdmin, map[string]any{"body": "hi"}, nil); s != http.StatusNotFound {
		t.Fatalf("write foreign channel: status %d", s)
	}
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", otherAdmin, nil, &me)
	if s := e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": me.ID}, nil); s != http.StatusNotFound {
		t.Fatalf("add foreign actor: status %d", s)
	}
	var list struct{ Channels []struct{ Name string } }
	e.do("GET", "/api/v1/channels", otherAdmin, nil, &list)
	if len(list.Channels) != 0 {
		t.Fatalf("other admin sees foreign channels: %+v", list.Channels)
	}
}
