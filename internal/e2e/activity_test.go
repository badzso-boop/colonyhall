// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/server"
)

func TestAgentActivityLifecycleAndWebSocket(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	general := e.generalID()

	// 1. WebSocket connection for a member who can read the general channel
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
	var welcome map[string]any
	if err := wsjson.Read(ctx, ws, &welcome); err != nil || welcome["type"] != "welcome" {
		t.Fatalf("expected welcome frame, got: %v, err: %v", welcome, err)
	}

	// Read replay_done frame
	var replayDone map[string]any
	if err := wsjson.Read(ctx, ws, &replayDone); err != nil || replayDone["type"] != "replay_done" {
		t.Fatalf("expected replay_done frame, got: %v, err: %v", replayDone, err)
	}

	// 2. Non-agent (human) calling /api/v1/agent/activity returns 403
	humanStatus := e.do("POST", "/api/v1/agent/activity", server.DevAdminToken, map[string]any{
		"channel_id": general,
		"state":      "working",
	}, nil)
	if humanStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for human actor reporting activity, got %d", humanStatus)
	}

	// 3. Agent reporting invalid state returns 400
	badStateStatus := e.do("POST", "/api/v1/agent/activity", server.DevAgentToken, map[string]any{
		"channel_id": general,
		"state":      "invalid-state",
	}, nil)
	if badStateStatus != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid state, got %d", badStateStatus)
	}

	// 4. Agent reporting activity on a channel it CAN read succeeds (204)
	agentStatus := e.do("POST", "/api/v1/agent/activity", server.DevAgentToken, map[string]any{
		"channel_id": general,
		"state":      "working",
	}, nil)
	if agentStatus != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent, got %d", agentStatus)
	}

	// 5. Verify the WebSocket client receives the ephemeral "agent.activity" frame
	var actFrame map[string]any
	if err := wsjson.Read(ctx, ws, &actFrame); err != nil {
		t.Fatalf("failed to read activity frame: %v", err)
	}
	if actFrame["type"] != "agent.activity" {
		t.Fatalf("expected type agent.activity, got %v", actFrame["type"])
	}
	if actFrame["state"] != "working" {
		t.Fatalf("expected state working, got %v", actFrame["state"])
	}
	if actFrame["channel_id"] != general {
		t.Fatalf("expected channel_id %s, got %v", general, actFrame["channel_id"])
	}

	// 6. Verify IsAgentWorking reflects the state
	fakeAgentActor, err := e.app.Identity.Authenticate(ctx, server.DevAgentToken)
	if err != nil {
		t.Fatal(err)
	}
	if !e.app.Gateway.IsAgentWorking(fakeAgentActor.ID) {
		t.Fatal("expected IsAgentWorking to be true")
	}

	// 7. Advance manual clock by 61 seconds -> activity expires
	e.clock.Advance(61 * time.Second)
	if e.app.Gateway.IsAgentWorking(fakeAgentActor.ID) {
		t.Fatal("expected IsAgentWorking to be false after 61s expiry")
	}

	// 8. Agent reporting on a private channel it is NOT a member of returns 403
	var privChan struct{ ID string }
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "admin-only", "kind": "private"}, &privChan); s != http.StatusCreated {
		t.Fatalf("create private channel: %d", s)
	}
	privStatus := e.do("POST", "/api/v1/agent/activity", server.DevAgentToken, map[string]any{
		"channel_id": privChan.ID,
		"state":      "working",
	}, nil)
	if privStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for channel agent cannot read, got %d", privStatus)
	}
}
