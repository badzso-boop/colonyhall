// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestTerminalRelayE2E(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	adminToken := server.DevAdminToken
	agentToken := server.DevAgentToken

	agent, err := e.app.Identity.Authenticate(ctx, agentToken)
	if err != nil {
		t.Fatal(err)
	}

	general := e.generalID()

	// Assign host for the agent so relay is allowed
	hostID := ids.V7{}.New()
	_ = e.app.Hosts.Store.CreateHost(ctx, hosts.Host{
		ID:          hostID,
		WorkspaceID: agent.WorkspaceID,
		Name:        "homelab",
		CreatedAt:   time.Now(),
	})
	_ = e.app.Hosts.Store.SetAssignment(ctx, hosts.Assignment{
		AgentID:     agent.ID,
		HostID:      hostID,
		WorkspaceID: agent.WorkspaceID,
		CreatedAt:   time.Now(),
	})

	// 1. Agent token on human endpoints -> 403 Forbidden
	code := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", agentToken, map[string]any{
		"channel_id": general,
		"op":         "term_list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("agent token POST /fs term_list: got %d, want 403", code)
	}

	// Round 40: a terminal name that could leave the agent's folder never
	// leaves the server.
	for _, op := range []string{"term_output", "term_kill"} {
		for _, name := range []string{"../other-agent/build", "a/b", "..", "Build", ""} {
			if code := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", adminToken, map[string]any{
				"channel_id": general, "op": op, "name": name,
			}, nil); code != http.StatusBadRequest {
				t.Fatalf("%s with name %q: got %d, want 400", op, name, code)
			}
		}
	}

	// 2. Create regular user (role member)
	admin, _ := e.app.Identity.Authenticate(ctx, adminToken)
	userActor, userToken, err := e.app.Identity.CreateActor(ctx, admin, identity.NewActorInput{
		Kind:        identity.KindHuman,
		Handle:      "regularuser",
		DisplayName: "Regular User",
		Role:        identity.RoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}

	genID, _ := ids.Parse(general)
	_ = e.app.Conversation.EnsureMember(ctx, genID, userActor.ID)

	// 3. Non-admin cannot kill terminal -> 403 Forbidden
	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", userToken, map[string]any{
		"channel_id": general,
		"op":         "term_kill",
		"name":       "ros-build",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("non-admin user POST /fs term_kill: got %d, want 403", code)
	}

	// 4. Human without access to private channel -> 403 Forbidden
	var privChan apispec.Channel
	code = e.do("POST", "/api/v1/channels", adminToken, map[string]any{
		"name": "private-term",
		"kind": "private",
	}, &privChan)
	if code != http.StatusCreated {
		t.Fatalf("create channel: got %d", code)
	}
	_ = e.app.Conversation.EnsureMember(ctx, privChan.Id, agent.ID)

	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", userToken, map[string]any{
		"channel_id": privChan.Id.String(),
		"op":         "term_list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("user without channel access POST /fs: got %d, want 403", code)
	}

	// Another person's agent's terminals are not found (round 38); the
	// user lists them as the agent's owner.
	if code := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", userToken, map[string]any{"channel_id": general, "op": "term_list"}, nil); code != http.StatusNotFound {
		t.Fatalf("term_list of another person's agent: %d", code)
	}
	if code := e.do("PUT", "/api/v1/agents/"+agent.ID.String()+"/owner", server.DevAdminToken, map[string]any{"owner_id": userActor.ID}, nil); code != http.StatusOK {
		t.Fatalf("set owner: %d", code)
	}

	// 5. Successful term_list relay
	pusher := &capturingPusher{ch: make(chan string, 10)}
	e.app.Files.FsPusher = pusher

	type fsOutcome struct {
		code int
		resp apispec.AgentFsResponse
	}
	outcomeCh := make(chan fsOutcome, 1)

	// User with channel access lists terminals
	go func() {
		var resp apispec.AgentFsResponse
		c := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", userToken, map[string]any{
			"channel_id": general,
			"op":         "term_list",
		}, &resp)
		outcomeCh <- fsOutcome{code: c, resp: resp}
	}()

	var reqID string
	select {
	case reqID = <-pusher.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for term_list pusher request id")
	}

	// Guard gets the request
	var pending apispec.AgentFsPendingRequest
	code = e.do("GET", "/api/v1/agent/fs/"+reqID, agentToken, nil, &pending)
	if code != http.StatusOK {
		t.Fatalf("agent GET /agent/fs/{reqID}: got %d, want 200", code)
	}
	if pending.Op != apispec.AgentFsPendingRequestOpTermList {
		t.Fatalf("expected op term_list, got %q", pending.Op)
	}

	// Guard answers with terminal summary
	now := time.Now()
	exit0 := 0
	trUUID := openapi_types.UUID(ids.V7{}.New())
	code = e.do("POST", "/api/v1/agent/fs/"+reqID+"/result", agentToken, apispec.AgentFsResultRequest{
		Op: (*apispec.AgentFsResultRequestOp)(&pending.Op),
		Terminals: &[]apispec.TerminalSummary{
			{
				Name:         "ros-build",
				Status:       apispec.TerminalSummaryStatusRunning,
				Command:      "make build",
				Cwd:          "/home/bot/ws",
				ChannelId:    openapi_types.UUID(genID),
				ThreadRootId: &trUUID,
				StartedAt:    now,
				ExitCode:     &exit0,
				DurationMs:   125000,
			},
		},
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("agent POST result for term_list: got %d, want 200", code)
	}

	var outcome fsOutcome
	select {
	case outcome = <-outcomeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for user term_list response")
	}
	if outcome.code != http.StatusOK {
		t.Fatalf("user term_list: got %d, want 200", outcome.code)
	}
	if outcome.resp.Terminals == nil || len(*outcome.resp.Terminals) != 1 || (*outcome.resp.Terminals)[0].Name != "ros-build" {
		t.Fatalf("unexpected terminals in response: %+v", outcome.resp)
	}

	// 6. Successful term_output relay
	go func() {
		var resp apispec.AgentFsResponse
		lines := 50
		c := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", userToken, map[string]any{
			"channel_id": general,
			"op":         "term_output",
			"name":       "ros-build",
			"lines":      lines,
		}, &resp)
		outcomeCh <- fsOutcome{code: c, resp: resp}
	}()

	select {
	case reqID = <-pusher.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for term_output pusher request id")
	}

	code = e.do("GET", "/api/v1/agent/fs/"+reqID, agentToken, nil, &pending)
	if code != http.StatusOK {
		t.Fatalf("agent GET /agent/fs/{reqID}: got %d, want 200", code)
	}
	if pending.Op != apispec.AgentFsPendingRequestOpTermOutput {
		t.Fatalf("expected op term_output, got %q", pending.Op)
	}
	if pending.Name == nil || *pending.Name != "ros-build" {
		t.Fatalf("expected name 'ros-build', got %+v", pending.Name)
	}
	if pending.Lines == nil || *pending.Lines != 50 {
		t.Fatalf("expected lines 50, got %+v", pending.Lines)
	}

	testOutput := "Step 1: compiling...\nStep 2: done."
	code = e.do("POST", "/api/v1/agent/fs/"+reqID+"/result", agentToken, apispec.AgentFsResultRequest{
		Op:     (*apispec.AgentFsResultRequestOp)(&pending.Op),
		Output: &testOutput,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("agent POST result for term_output: got %d, want 200", code)
	}

	select {
	case outcome = <-outcomeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for user term_output response")
	}
	if outcome.code != http.StatusOK {
		t.Fatalf("user term_output: got %d, want 200", outcome.code)
	}
	if outcome.resp.Output == nil || *outcome.resp.Output != testOutput {
		t.Fatalf("unexpected output in response: %+v", outcome.resp)
	}

	// 7. Successful term_kill relay by admin
	go func() {
		var resp apispec.AgentFsResponse
		c := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", adminToken, map[string]any{
			"channel_id": general,
			"op":         "term_kill",
			"name":       "ros-build",
		}, &resp)
		outcomeCh <- fsOutcome{code: c, resp: resp}
	}()

	select {
	case reqID = <-pusher.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for term_kill pusher request id")
	}

	// Guard reads pending request: must have name and killed_by
	code = e.do("GET", "/api/v1/agent/fs/"+reqID, agentToken, nil, &pending)
	if code != http.StatusOK {
		t.Fatalf("agent GET /agent/fs/{reqID}: got %d, want 200", code)
	}
	if pending.Op != apispec.AgentFsPendingRequestOpTermKill {
		t.Fatalf("expected op term_kill, got %q", pending.Op)
	}
	if pending.Name == nil || *pending.Name != "ros-build" {
		t.Fatalf("expected name 'ros-build', got %+v", pending.Name)
	}
	if pending.KilledBy == nil || *pending.KilledBy != "@"+admin.Handle {
		t.Fatalf("expected killed_by '@%s', got %+v", admin.Handle, pending.KilledBy)
	}

	// Guard answers killed: true
	killed := true
	code = e.do("POST", "/api/v1/agent/fs/"+reqID+"/result", agentToken, apispec.AgentFsResultRequest{
		Op:     (*apispec.AgentFsResultRequestOp)(&pending.Op),
		Killed: &killed,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("agent POST result for term_kill: got %d, want 200", code)
	}

	select {
	case outcome = <-outcomeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for admin term_kill response")
	}
	if outcome.code != http.StatusOK {
		t.Fatalf("admin term_kill: got %d, want 200", outcome.code)
	}
	if outcome.resp.Killed == nil || !*outcome.resp.Killed {
		t.Fatalf("expected killed: true in response, got %+v", outcome.resp)
	}
}
