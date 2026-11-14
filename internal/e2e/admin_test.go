// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/fakeagent"
	auditmem "colonyhall/internal/modules/audit/memory"
	"colonyhall/internal/server"
)

func (e *env) agentID() string {
	var list struct {
		Actors []struct{ ID, Handle string }
	}
	e.do("GET", "/api/v1/actors", server.DevAdminToken, nil, &list)
	for _, a := range list.Actors {
		if a.Handle == "fake-agent" {
			return a.ID
		}
	}
	e.t.Fatal("fake-agent not found")
	return ""
}

func TestTokenLifecycle(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	var issued struct {
		Token  struct{ ID string }
		Secret string
	}
	if s := e.do("POST", "/api/v1/actors/"+agent+"/tokens", server.DevAdminToken, map[string]any{"label": "laptop"}, &issued); s != http.StatusCreated {
		t.Fatalf("issue: status %d", s)
	}
	if s := e.do("GET", "/api/v1/me", issued.Secret, nil, nil); s != http.StatusOK {
		t.Fatalf("new token must work: status %d", s)
	}
	if s := e.do("DELETE", "/api/v1/tokens/"+issued.Token.ID, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("revoke: status %d", s)
	}
	if s := e.do("GET", "/api/v1/me", issued.Secret, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("revoked token must fail: status %d", s)
	}
	var list struct {
		Tokens []struct {
			Label     string
			RevokedAt *time.Time `json:"revoked_at"`
		}
	}
	e.do("GET", "/api/v1/actors/"+agent+"/tokens", server.DevAdminToken, nil, &list)
	if len(list.Tokens) != 2 || list.Tokens[1].Label != "laptop" || list.Tokens[1].RevokedAt == nil {
		t.Fatalf("token list: %+v", list.Tokens)
	}
	if s := e.do("POST", "/api/v1/actors/"+agent+"/tokens", server.DevAgentToken, map[string]any{}, nil); s != http.StatusForbidden {
		t.Fatalf("agent issues itself a token: status %d", s)
	}
}

func TestPauseAndStopAgent(t *testing.T) {
	e := newEnv(t)
	agent, general := e.agentID(), e.generalID()
	setState := func(state string) {
		if s := e.do("PUT", "/api/v1/actors/"+agent+"/state", server.DevAdminToken, map[string]any{"state": state}, nil); s != http.StatusNoContent {
			t.Fatalf("set %s: status %d", state, s)
		}
	}
	setState("paused")
	if s := e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, nil, nil); s != http.StatusOK {
		t.Fatalf("paused agent reads: status %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "hi"}, nil); s != http.StatusForbidden {
		t.Fatalf("paused agent writes: status %d", s)
	}
	setState("stopped")
	if s := e.do("GET", "/api/v1/me", server.DevAgentToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("stopped agent: status %d", s)
	}
	setState("active")
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "back"}, nil); s != http.StatusCreated {
		t.Fatalf("reactivated agent writes: status %d", s)
	}
}

// TestEmergencyStop: one switch halts every agent, including an MCP
// session that was opened before the stop.
func TestEmergencyStop(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent triggers emergency stop: status %d", s)
	}
	var st struct {
		EmergencyStop struct{ Active bool } `json:"emergency_stop"`
	}
	if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, nil, &st); s != http.StatusOK || !st.EmergencyStop.Active {
		t.Fatalf("emergency stop: status %d %+v", s, st)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{"body": "x"}, nil); s != http.StatusForbidden {
		t.Fatalf("agent writes during stop (REST): status %d", s)
	}
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "general", "text": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("agent writes during stop over an already open MCP session")
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, map[string]any{"body": "admin still talks"}, nil); s != http.StatusCreated {
		t.Fatalf("human admin writes during stop: status %d", s)
	}

	b := e.browser()
	b.login("norbi", server.DevAdminPassword)
	if s := b.do("POST", "/api/v1/admin/emergency-clear", map[string]any{"password": "wrong-password-here"}, nil); s != http.StatusUnauthorized {
		t.Fatalf("clear with wrong password: status %d", s)
	}
	if s := b.do("POST", "/api/v1/admin/emergency-clear", map[string]any{"password": server.DevAdminPassword}, &st); s != http.StatusOK || st.EmergencyStop.Active {
		t.Fatalf("clear: status %d %+v", s, st)
	}
	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "general", "text": "back to work"}})
	if err != nil || res.IsError {
		t.Fatalf("agent writes after clear: %v %+v", err, res)
	}
}

func TestAuditLogIsTamperEvident(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, map[string]any{"body": "for the record"}, nil)
	e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, nil, nil)
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Entries []struct {
			Seq    int64
			Action string
		}
	}
	if s := e.do("GET", "/api/v1/admin/audit?limit=5", server.DevAdminToken, nil, &page); s != http.StatusOK {
		t.Fatalf("audit list: status %d", s)
	}
	if len(page.Entries) == 0 || page.Entries[0].Action != "workspace.emergency_activated" {
		t.Fatalf("newest audit entry: %+v", page.Entries)
	}
	if s := e.do("GET", "/api/v1/admin/audit", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent reads audit: status %d", s)
	}
	var v struct {
		OK       bool  `json:"ok"`
		Checked  int   `json:"checked"`
		BrokenAt int64 `json:"broken_at"`
	}
	e.do("GET", "/api/v1/admin/audit/verify", server.DevAdminToken, nil, &v)
	if !v.OK || v.Checked < 5 {
		t.Fatalf("intact chain: %+v", v)
	}
	// Someone edits a stored entry behind the server's back.
	victim := page.Entries[1].Seq
	e.app.Audit.Store.(*auditmem.Store).Tamper(victim, []byte(`{"body":"rewritten"}`))
	e.do("GET", "/api/v1/admin/audit/verify", server.DevAdminToken, nil, &v)
	if v.OK || v.BrokenAt != victim {
		t.Fatalf("tampering not detected: %+v (edited seq %d)", v, victim)
	}
}

// TestPausedAdminCannotAdminister: pausing an admin takes away every
// administrative write, not only chat messages.
func TestPausedAdminCannotAdminister(t *testing.T) {
	e := newEnv(t)
	agent, general := e.agentID(), e.generalID()
	var created struct {
		Actor struct{ ID string }
		Token string
	}
	e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{"kind": "human", "handle": "anna", "role": "admin"}, &created)
	admin := created.Token
	if s := e.do("PUT", "/api/v1/actors/"+created.Actor.ID+"/state", server.DevAdminToken, map[string]any{"state": "paused"}, nil); s != http.StatusNoContent {
		t.Fatalf("pause admin: status %d", s)
	}
	checks := []struct {
		name, method, path string
		body               any
	}{
		{"issue token", "POST", "/api/v1/actors/" + agent + "/tokens", map[string]any{}},
		{"change agent state", "PUT", "/api/v1/actors/" + agent + "/state", map[string]any{"state": "stopped"}},
		{"create actor", "POST", "/api/v1/actors", map[string]any{"kind": "agent", "handle": "sneaky"}},
		{"add member", "POST", "/api/v1/channels/" + general + "/members", map[string]any{"actor_id": created.Actor.ID}},
	}
	for _, c := range checks {
		if s := e.do(c.method, c.path, admin, c.body, nil); s != http.StatusForbidden {
			t.Errorf("paused admin can %s: status %d", c.name, s)
		}
	}
}

// An unknown role is a validation error (400), not a database error (500):
// the dev page once sent role "agent", which only the DB constraint caught.
func TestCreateActorRejectsUnknownRole(t *testing.T) {
	e := newEnv(t)
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken,
		map[string]any{"kind": "human", "handle": "bad-role", "role": "agent"}, nil); s != http.StatusBadRequest {
		t.Fatalf("role agent: got %d, want 400", s)
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken,
		map[string]any{"kind": "human", "handle": "good-role", "role": "member"}, nil); s != http.StatusCreated {
		t.Fatalf("role member: got %d, want 201", s)
	}
}
