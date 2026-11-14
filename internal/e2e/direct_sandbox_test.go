// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
)

// Round 40: the guard never wraps a direct-mode run in the sandbox, so an
// agent assigned "direct" with the sandbox flag is full-access for every
// rule: another agent's task waits for a human, and the directory does not
// call it sandboxed (Norbi's decision: keep the combination, treat it as
// full-access).
func TestSandboxFlaggedDirectAgentIsFullAccess(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	hostID, _ := e.enroll("machine-1")
	_, sourceTok := createSandboxedAgent(e, hostID, "projects", "Projects Agent")
	targetID, targetTok := e.newAgent("direct-boxed", "Direct Boxed", map[string]any{
		"host_id": hostID, "client": "claude-code", "model": "haiku", "allow_work": true,
		"mode": "direct", "root_dir": "/app", "sandbox": true,
	}, nil)

	full, err := e.app.Hosts.IsFullAccess(ctx, mustID(targetID))
	if err != nil || !full {
		t.Fatalf("a direct agent with the sandbox flag must be full-access: %v %v", full, err)
	}

	var msg apispec.Message
	if s := e.do("POST", "/api/v1/channels/"+e.generalID()+"/messages", sourceTok, apispec.SendMessageRequest{
		Body: "@direct-boxed run the deploy script",
	}, &msg); s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var inbox apispec.Inbox
	if s := e.do("GET", "/api/v1/agent/inbox", targetTok, nil, &inbox); s != http.StatusOK {
		t.Fatalf("inbox: %d", s)
	}
	if inbox.Total != 0 {
		t.Fatalf("an agent's task reached a direct agent without a human: %d items", inbox.Total)
	}
}
