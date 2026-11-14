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
)

type capturingPusher struct {
	ch chan string
}

func (p *capturingPusher) PushFsRequest(_ ids.ID, requestID string) bool {
	select {
	case p.ch <- requestID:
	default:
	}
	return true
}

func TestFsBrowseE2E(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	adminToken := server.DevAdminToken
	agentToken := server.DevAgentToken

	agent, err := e.app.Identity.Authenticate(ctx, agentToken)
	if err != nil {
		t.Fatal(err)
	}

	general := e.generalID()

	// 1. Agent token on human endpoints -> 403
	code := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", agentToken, map[string]any{
		"channel_id": general,
		"op":         "list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("POST /agents/{agentId}/fs with agent token: got %d, want 403", code)
	}

	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/file-fetch", agentToken, map[string]any{
		"channel_id": general,
		"path":       "/home/bot/main.go",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("POST /agents/{agentId}/file-fetch with agent token: got %d, want 403", code)
	}

	// 2. Human token on agent endpoints -> 403
	code = e.do("GET", "/api/v1/agent/fs/dummy-req", adminToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("GET /agent/fs/{requestId} with human token: got %d, want 403", code)
	}

	code = e.do("POST", "/api/v1/agent/fs/dummy-req/result", adminToken, map[string]any{
		"op": "list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("POST /agent/fs/{requestId}/result with human token: got %d, want 403", code)
	}

	// 3. Human without channel access -> 403
	var privChan apispec.Channel
	code = e.do("POST", "/api/v1/channels", adminToken, map[string]any{
		"name": "secret-fs",
		"kind": "private",
	}, &privChan)
	if code != http.StatusCreated {
		t.Fatalf("create channel: got %d", code)
	}

	// Ensure agent is in private channel
	_ = e.app.Conversation.EnsureMember(ctx, privChan.Id, agent.ID)

	// Create user Bob (not in private channel)
	admin, _ := e.app.Identity.Authenticate(ctx, adminToken)
	_, bobToken, err := e.app.Identity.CreateActor(ctx, admin, identity.NewActorInput{
		Kind:        identity.KindHuman,
		Handle:      "bob",
		DisplayName: "Bob",
		Role:        identity.RoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}

	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", bobToken, map[string]any{
		"channel_id": privChan.Id.String(),
		"op":         "list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("human without channel access POST /fs: got %d, want 403", code)
	}

	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/file-fetch", bobToken, map[string]any{
		"channel_id": privChan.Id.String(),
		"path":       "/home/bot/main.go",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("human without channel access POST /file-fetch: got %d, want 403", code)
	}

	// 4. Agent not in channel -> 403
	agent2Actor, agent2Token, err := e.app.Identity.CreateAgentActor(ctx, admin, identity.AgentActorInput{
		Handle:      "other-bot",
		DisplayName: "Other Bot",
		WithToken:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Assign host for both agents
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
	_ = e.app.Hosts.Store.SetAssignment(ctx, hosts.Assignment{
		AgentID:     agent2Actor.ID,
		HostID:      hostID,
		WorkspaceID: agent.WorkspaceID,
		CreatedAt:   time.Now(),
	})

	code = e.do("POST", "/api/v1/agents/"+agent2Actor.ID.String()+"/fs", adminToken, map[string]any{
		"channel_id": privChan.Id.String(),
		"op":         "list",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("agent not in channel POST /fs: got %d, want 403", code)
	}

	code = e.do("POST", "/api/v1/agents/"+agent2Actor.ID.String()+"/file-fetch", adminToken, map[string]any{
		"channel_id": privChan.Id.String(),
		"path":       "/home/other/file.txt",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("agent not in channel POST /file-fetch: got %d, want 403", code)
	}

	// 5. Success flow: human with access -> answer from fake guard
	pusher := &capturingPusher{ch: make(chan string, 10)}
	e.app.Files.FsPusher = pusher

	type fsOutcome struct {
		code int
		resp apispec.AgentFsResponse
	}
	outcomeCh := make(chan fsOutcome, 1)

	go func() {
		var resp apispec.AgentFsResponse
		c := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", adminToken, map[string]any{
			"channel_id": general,
			"op":         "list",
			"path":       "src",
		}, &resp)
		outcomeCh <- fsOutcome{code: c, resp: resp}
	}()

	var reqID string
	select {
	case reqID = <-pusher.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for pusher to capture request id")
	}

	// 6. Other agent tries to read this request -> 404
	code = e.do("GET", "/api/v1/agent/fs/"+reqID, agent2Token, nil, nil)
	if code != http.StatusNotFound {
		t.Fatalf("other agent GET /agent/fs/{reqID}: got %d, want 404", code)
	}

	// Other agent tries to answer this request -> 404
	code = e.do("POST", "/api/v1/agent/fs/"+reqID+"/result", agent2Token, map[string]any{
		"op": "list",
	}, nil)
	if code != http.StatusNotFound {
		t.Fatalf("other agent POST /agent/fs/{reqID}/result: got %d, want 404", code)
	}

	// 7. Legitimate agent reads pending request details
	var pending apispec.AgentFsPendingRequest
	code = e.do("GET", "/api/v1/agent/fs/"+reqID, agentToken, nil, &pending)
	if code != http.StatusOK {
		t.Fatalf("legitimate agent GET /agent/fs/{reqID}: got %d, want 200", code)
	}
	if pending.RequestId != reqID || pending.Op != apispec.AgentFsPendingRequestOpList || pending.Path == nil || *pending.Path != "src" {
		t.Fatalf("unexpected pending request details: %+v", pending)
	}

	// Legitimate agent answers the request
	code = e.do("POST", "/api/v1/agent/fs/"+reqID+"/result", agentToken, map[string]any{
		"op": "list",
		"entries": []map[string]any{
			{
				"name": "main.go",
				"kind": "file",
				"size": 420,
			},
		},
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("legitimate agent POST result: got %d, want 200", code)
	}

	// Human call returns 200 OK with the answer
	var outcome fsOutcome
	select {
	case outcome = <-outcomeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for human POST /fs response")
	}
	if outcome.code != http.StatusOK {
		t.Fatalf("human POST /fs: got %d, want 200", outcome.code)
	}
	if outcome.resp.Entries == nil || len(*outcome.resp.Entries) != 1 || (*outcome.resp.Entries)[0].Name != "main.go" {
		t.Fatalf("unexpected entries in human response: %+v", outcome.resp)
	}

	// 8. Timeout -> 504 and late answer -> 410
	e.app.Files.FsTimeout = 50 * time.Millisecond
	go func() {
		c := e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/fs", adminToken, map[string]any{
			"channel_id": general,
			"op":         "search",
			"query":      "needle",
		}, nil)
		outcomeCh <- fsOutcome{code: c}
	}()

	var timeoutReqID string
	select {
	case timeoutReqID = <-pusher.ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for pusher to capture timeout request id")
	}

	// Wait for human call to complete with 504 Gateway Timeout
	select {
	case outcome = <-outcomeCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for 504 outcome")
	}
	if outcome.code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504 Gateway Timeout, got: %d", outcome.code)
	}

	// Guard attempts late delivery -> 410 Gone
	code = e.do("POST", "/api/v1/agent/fs/"+timeoutReqID+"/result", agentToken, map[string]any{
		"op": "search",
	}, nil)
	if code != http.StatusGone {
		t.Fatalf("late answer to timed out request: got %d, want 410", code)
	}

	// 9. Agent file-fetch endpoint
	var fetchReq apispec.FileFetchRequest
	code = e.do("POST", "/api/v1/agents/"+agent.ID.String()+"/file-fetch", adminToken, map[string]any{
		"channel_id": general,
		"path":       "/home/fake-agent/scheduler.log",
	}, &fetchReq)
	if code != http.StatusCreated {
		t.Fatalf("POST /agents/{agentId}/file-fetch: got %d, want 201", code)
	}
	if fetchReq.Path != "/home/fake-agent/scheduler.log" || fetchReq.State != apispec.FileFetchRequestStatePending {
		t.Fatalf("unexpected fetch request: %+v", fetchReq)
	}
}
