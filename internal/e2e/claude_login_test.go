// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// lockedBuffer collects the server log of a test.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// newLoggedEnv is newEnv with the server log, at debug level, kept.
func newLoggedEnv(t *testing.T) (*env, *lockedBuffer) {
	t.Helper()
	ctx := context.Background()
	logs := &lockedBuffer{}
	clk := clock.NewManual(time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC))
	app, err := server.New(ctx, server.Options{Store: "memory", Clock: clk, IDs: &ids.Sequential{},
		Logger: slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	return &env{t: t, app: app, srv: srv, router: specRouter(t, srv.URL), clock: clk}, logs
}

type loginCall struct {
	code int
	out  apispec.ClaudeLoginState
	prob problemOut
}

func (e *env) setLoginAsync(hostID, token, secret string) chan loginCall {
	ch := make(chan loginCall, 1)
	go func() {
		var raw struct {
			apispec.ClaudeLoginState
			problemOut
		}
		code := e.do("PUT", "/api/v1/hosts/"+hostID+"/claude-login", token, map[string]any{"token": secret}, &raw)
		ch <- loginCall{code: code, out: raw.ClaudeLoginState, prob: raw.problemOut}
	}()
	return ch
}

// guardServe plays the guard: it takes one login request frame, fetches the
// request (the token once) and answers with res.
func (e *env) guardServe(ctx context.Context, ws *websocket.Conn, hostToken string, res map[string]any) apispec.HostClaudeLoginRequest {
	e.t.Helper()
	var frame struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	for frame.Type != "host.claude_login.request" {
		if err := wsjson.Read(ctx, ws, &frame); err != nil {
			e.t.Fatalf("login frame: %v", err)
		}
	}
	var req apispec.HostClaudeLoginRequest
	if c := e.do("GET", "/api/v1/host/claude-login/"+frame.RequestID, hostToken, nil, &req); c != http.StatusOK {
		e.t.Fatalf("guard get: %d", c)
	}
	// The token is handed out once.
	var again apispec.HostClaudeLoginRequest
	if c := e.do("GET", "/api/v1/host/claude-login/"+frame.RequestID, hostToken, nil, &again); c != http.StatusOK || again.Token != nil {
		e.t.Fatalf("second read: %d %v", c, again.Token != nil)
	}
	if c := e.do("POST", "/api/v1/host/claude-login/"+frame.RequestID+"/result", hostToken, res, nil); c != http.StatusNoContent {
		e.t.Fatalf("guard result: %d", c)
	}
	return req
}

func TestClaudeLoginRelayNeverStoresTheToken(t *testing.T) {
	e, logs := newLoggedEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hostID, hostToken := e.enroll("homelab-colleague")
	otherHost, _ := e.enroll("homelab")
	colleagueID, colleagueTok := e.createActor("human", "kollega", "member")
	if c := e.do("PUT", "/api/v1/hosts/"+hostID+"/owner", server.DevAdminToken, map[string]any{"owner_id": colleagueID}, nil); c != http.StatusOK {
		t.Fatalf("owner: %d", c)
	}
	const secret = "sk-ant-oat01-TESTTOKEN-never-stored-anywhere-0123456789"
	body := map[string]any{"token": secret}

	// Offline guard: 409, nothing kept.
	if c := e.do("PUT", "/api/v1/hosts/"+hostID+"/claude-login", colleagueTok, body, nil); c != http.StatusConflict {
		t.Fatalf("offline: %d", c)
	}
	if n := e.app.HostLogin.Pending(); n != 0 {
		t.Fatalf("a token stayed in memory after an offline attempt: %d", n)
	}
	// Another person's machine is not found; agents are refused.
	if c := e.do("PUT", "/api/v1/hosts/"+otherHost+"/claude-login", colleagueTok, body, nil); c != http.StatusNotFound {
		t.Fatalf("another person's machine: %d", c)
	}
	if c := e.do("PUT", "/api/v1/hosts/"+hostID+"/claude-login", server.DevAgentToken, body, nil); c != http.StatusForbidden {
		t.Fatalf("agent: %d", c)
	}
	if c := e.do("PUT", "/api/v1/hosts/"+hostID+"/claude-login", colleagueTok, map[string]any{"token": "short"}, nil); c != http.StatusBadRequest {
		t.Fatalf("short token: %d", c)
	}

	ws, _ := e.hostWS(ctx, hostToken)
	e.waitHostConnected(hostID)

	// The guard's check fails: 422, the old token stays (on the guard).
	call := e.setLoginAsync(hostID, colleagueTok, secret)
	req := e.guardServe(ctx, ws, hostToken, map[string]any{"ok": false, "error_code": "check_failed"})
	if req.Op != apispec.Set || req.Token == nil || *req.Token != secret {
		t.Fatalf("guard got: %+v", req)
	}
	if r := <-call; r.code != http.StatusUnprocessableEntity || !strings.Contains(r.prob.Detail, "check_failed") {
		t.Fatalf("failed check: %+v", r)
	}

	// The check passes: 200 with the time, audited without the token.
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	call = e.setLoginAsync(hostID, colleagueTok, secret)
	e.guardServe(ctx, ws, hostToken, map[string]any{"ok": true, "set_at": at})
	if r := <-call; r.code != http.StatusOK || r.out.SetAt == nil || !r.out.SetAt.Equal(at) {
		t.Fatalf("set: %+v", r)
	}
	if n := e.app.HostLogin.Pending(); n != 0 {
		t.Fatalf("a token stayed in memory: %d", n)
	}

	// Removal by the owner.
	done := make(chan int, 1)
	go func() { done <- e.do("DELETE", "/api/v1/hosts/"+hostID+"/claude-login", colleagueTok, nil, nil) }()
	if req := e.guardServe(ctx, ws, hostToken, map[string]any{"ok": true}); req.Op != apispec.Clear || req.Token != nil {
		t.Fatalf("clear request: %+v", req)
	}
	if c := <-done; c != http.StatusNoContent {
		t.Fatalf("clear: %d", c)
	}

	// The token is nowhere: not in an event (the audit is built from them),
	// a report or the server log; the audit has who set it on which machine.
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	evs, err := e.app.Events.Since(ctx, 0, 100000)
	if err != nil {
		t.Fatal(err)
	}
	set, cleared := 0, 0
	for _, ev := range evs {
		raw, _ := json.Marshal(ev)
		if strings.Contains(string(raw), "TESTTOKEN") {
			t.Fatalf("the token is in event %s", ev.Type)
		}
		switch ev.Type {
		case "host.claude_login_set":
			set++
			if ev.ActorID.String() != colleagueID || ev.AggregateID.String() != hostID {
				t.Fatalf("audit of the set: %+v", ev)
			}
		case "host.claude_login_cleared":
			cleared++
		}
	}
	if set != 1 || cleared != 1 {
		t.Fatalf("login events: set %d cleared %d", set, cleared)
	}
	var hosts apispec.HostList
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	if raw, _ := json.Marshal(hosts); strings.Contains(string(raw), "TESTTOKEN") {
		t.Fatal("the token is in the machines list")
	}
	if strings.Contains(logs.String(), "TESTTOKEN") {
		t.Fatal("the token is in the server log")
	}

	// Rate limit: the person's attempts in the window are counted.
	for i := 0; i < 10; i++ {
		c := e.do("PUT", "/api/v1/hosts/"+otherHost+"/claude-login", server.DevAdminToken, map[string]any{"token": "x" + strings.Repeat("y", 30)}, nil)
		if c == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatal("no rate limit")
}
