package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestOverviewE2E(t *testing.T) {
	e := newEnv(t)
	agentIDStr := e.agentID()
	agentID, _ := ids.Parse(agentIDStr)
	generalIDStr := e.generalID()
	generalID, _ := ids.Parse(generalIDStr)

	// 1. Agent calling /api/v1/agents/{id}/overview returns 403 Forbidden
	agentCallStatus := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", agentIDStr), server.DevAgentToken, nil, nil)
	if agentCallStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when agent calls overview, got %d", agentCallStatus)
	}

	// 2. Non-existent actor returns 404
	randomID := ids.V7{}.New()
	notFoundStatus := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", randomID), server.DevAdminToken, nil, nil)
	if notFoundStatus != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown actor id, got %d", notFoundStatus)
	}

	// 3. Another workspace's agent returns 404
	const otherAdmin = "col_other_workspace_admin_token"
	if _, _, _, err := e.app.Identity.Bootstrap(context.Background(), "other", "eve", "Eve", otherAdmin); err != nil {
		t.Fatal(err)
	}
	var meEve struct{ Id openapi_types.UUID }
	e.do("GET", "/api/v1/me", otherAdmin, nil, &meEve)
	foreignStatus := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", meEve.Id), server.DevAdminToken, nil, nil)
	if foreignStatus != http.StatusNotFound {
		t.Fatalf("expected 404 for actor from another workspace, got %d", foreignStatus)
	}

	// 4. Human actor overview
	var meAdmin struct{ Id openapi_types.UUID }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &meAdmin)

	var humanOverview apispec.AgentOverview
	humanStatus := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", meAdmin.Id), server.DevAdminToken, nil, &humanOverview)
	if humanStatus != http.StatusOK {
		t.Fatalf("expected 200 for human overview, got %d", humanStatus)
	}
	if humanOverview.Actor.Id != meAdmin.Id {
		t.Fatalf("expected actor ID %s, got %s", meAdmin.Id, humanOverview.Actor.Id)
	}
	if humanOverview.State != "active" {
		t.Fatalf("expected state active, got %s", humanOverview.State)
	}
	if humanOverview.CommonChannels == nil || len(*humanOverview.CommonChannels) == 0 {
		t.Fatalf("expected common channels for human overview, got none")
	}
	if humanOverview.Assignment != nil {
		t.Fatalf("human overview must not have assignment")
	}

	// 5. Agent actor overview: configure assignment, activity, runs, notes, leases, questions
	adminActor, err := e.app.Identity.ActorByID(context.Background(), ids.ID(meAdmin.Id))
	if err != nil {
		t.Fatal(err)
	}
	agentActor, err := e.app.Identity.ActorByID(context.Background(), agentID)
	if err != nil {
		t.Fatal(err)
	}

	// Enroll a host and assign the agent
	code, _, err := e.app.Hosts.CreatePairingCode(context.Background(), adminActor)
	if err != nil {
		t.Fatal(err)
	}
	host, secret, err := e.app.Hosts.Enroll(context.Background(), code, "dev-machine")
	if err != nil {
		t.Fatal(err)
	}
	sandbox := false
	maxSession := 150000
	dailyLimit := int64(500000)
	_, err = e.app.Hosts.Assign(context.Background(), adminActor, agentID, hosts.AssignInput{
		HostID:           host.ID,
		Client:           "claude-code",
		Model:            "claude-3-5-sonnet",
		WorkModel:        "claude-3-7-sonnet",
		AllowWork:        true,
		PermissionMode:   "default",
		Mode:             "direct",
		RootDir:          "/workspace",
		WorktreeRepo:     "/workspace/repo",
		Sandbox:          &sandbox,
		Approvals:        "none",
		MaxSessionTokens: &maxSession,
		DailyTokenLimit:  &dailyLimit,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Host saves report with worktree
	err = e.app.Hosts.SaveReport(context.Background(), host, hosts.Report{
		Roots:   []string{"/workspace"},
		Clients: []hosts.Client{{Name: "claude-code", Version: "1.0.0"}},
		Worktrees: []hosts.Worktree{
			{
				AgentID: agentID.String(),
				Path:    "/workspace/repo-wt",
				Branch:  "feature/awesome",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add a folder grant
	_, err = e.app.Hosts.AddGrant(context.Background(), adminActor, agentID, "/workspace/data", "read")
	if err != nil {
		t.Fatal(err)
	}

	// Agent reports working activity
	actStatus := e.do("POST", "/api/v1/agent/activity", server.DevAgentToken, map[string]any{
		"channel_id": generalIDStr,
		"state":      "working",
	}, nil)
	if actStatus != http.StatusNoContent {
		t.Fatalf("expected 204 for reporting activity, got %d", actStatus)
	}

	// Post message to create a thread root
	msg, s := e.send(server.DevAdminToken, generalIDStr, map[string]any{
		"body": "Thread starter message",
	})
	if s != http.StatusCreated {
		t.Fatalf("failed to post message: %d", s)
	}
	threadRootID, _ := ids.Parse(msg.ID)

	// Agent reports a finished run
	runStatus := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, map[string]any{
		"phase":              "work",
		"client":             "claude-code",
		"model":              "claude-3-5-sonnet",
		"session_id":         "sess-abc-123",
		"channel_id":         generalIDStr,
		"thread_root_id":     msg.ID,
		"items":              1,
		"input_tokens":       10000,
		"output_tokens":      5000,
		"cache_read_tokens":  2000,
		"cache_write_tokens": 1000,
		"context_tokens":     12500,
		"duration_ms":        2500,
		"status":             "ok",
	}, nil)
	if runStatus != http.StatusCreated {
		t.Fatalf("expected 201 for report run, got %d", runStatus)
	}

	// Agent sets a note
	_, err = e.app.Notes.SetNote(context.Background(), agentActor, threadRootID, "Overview test note")
	if err != nil {
		t.Fatal(err)
	}

	// Agent acquires a lease
	_, err = e.app.Leases.Acquire(context.Background(), agentActor, "test/lock", 60, "running migration")
	if err != nil {
		t.Fatal(err)
	}

	// Ask a question from agent
	_, err = e.app.Question.Ask(context.Background(), agentActor, question.AskInput{
		ChannelID:    generalID,
		ThreadRootID: threadRootID,
		To:           "human",
		Text:         "Should we proceed with step 2?",
		Options:      []string{"yes", "no"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Query agent overview
	var agentOverview apispec.AgentOverview
	agentOverviewStatus := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", agentIDStr), server.DevAdminToken, nil, &agentOverview)
	if agentOverviewStatus != http.StatusOK {
		t.Fatalf("expected 200 for agent overview, got %d", agentOverviewStatus)
	}

	// Validate overview fields:
	// Actor
	if agentOverview.Actor.Id != openapi_types.UUID(agentID) {
		t.Fatalf("expected actor ID %s, got %s", agentID, agentOverview.Actor.Id)
	}
	if agentOverview.State != "active" {
		t.Fatalf("expected state active, got %s", agentOverview.State)
	}

	// Activity
	if agentOverview.Activity == nil || !agentOverview.Activity.Working {
		t.Fatalf("expected activity working=true, got %+v", agentOverview.Activity)
	}

	// Assignment
	if agentOverview.Assignment == nil {
		t.Fatalf("expected assignment to be present")
	}
	asgnSpec := agentOverview.Assignment
	if asgnSpec.HostName == nil || *asgnSpec.HostName != "dev-machine" {
		t.Fatalf("expected host_name 'dev-machine', got %v", asgnSpec.HostName)
	}
	if asgnSpec.WorktreeBranch == nil || *asgnSpec.WorktreeBranch != "feature/awesome" {
		t.Fatalf("expected worktree_branch 'feature/awesome', got %v", asgnSpec.WorktreeBranch)
	}
	if asgnSpec.WorktreeRepo != "/workspace/repo" {
		t.Fatalf("expected worktree_repo '/workspace/repo', got %s", asgnSpec.WorktreeRepo)
	}
	if asgnSpec.MaxSessionTokens != 150000 || asgnSpec.DailyTokenLimit != 500000 {
		t.Fatalf("unexpected token limits: max_session=%d, daily=%d", asgnSpec.MaxSessionTokens, asgnSpec.DailyTokenLimit)
	}
	if len(asgnSpec.Grants) != 1 || asgnSpec.Grants[0].Path != "/workspace/data" {
		t.Fatalf("expected 1 folder grant for /workspace/data, got %+v", asgnSpec.Grants)
	}

	// Usage
	if agentOverview.Usage == nil {
		t.Fatalf("expected usage to be present")
	}
	// Total tokens: 10000 + 5000 + 2000 + 1000 = 18000
	if agentOverview.Usage.TodayTokens != 18000 {
		t.Fatalf("expected today_tokens=18000, got %d", agentOverview.Usage.TodayTokens)
	}
	if agentOverview.Usage.DailyTokenLimit != 500000 {
		t.Fatalf("expected daily_token_limit=500000, got %d", agentOverview.Usage.DailyTokenLimit)
	}
	if len(agentOverview.Usage.LastRuns) != 1 || agentOverview.Usage.LastRuns[0].Tokens != 18000 {
		t.Fatalf("expected 1 last run with 18000 tokens, got %+v", agentOverview.Usage.LastRuns)
	}

	// Threads
	if agentOverview.Threads == nil || len(*agentOverview.Threads) != 1 {
		t.Fatalf("expected 1 thread context, got %+v", agentOverview.Threads)
	}
	th := (*agentOverview.Threads)[0]
	if th.SessionId != "sess-abc-123" || th.LastRunTokens != 18000 || th.MaxSessionTokens != 150000 {
		t.Fatalf("unexpected thread context: %+v", th)
	}
	// The session limit compares the context size, not the summed usage.
	if th.ContextTokens == nil || *th.ContextTokens != 12500 {
		t.Fatalf("thread context_tokens: got %v, want 12500", th.ContextTokens)
	}

	// Collaboration
	if agentOverview.Collaboration == nil {
		t.Fatalf("expected collaboration to be present")
	}
	collab := agentOverview.Collaboration
	if len(collab.Notes) != 1 || collab.Notes[0].Text != "Overview test note" {
		t.Fatalf("expected 1 note 'Overview test note', got %+v", collab.Notes)
	}
	if len(collab.ActiveLeases) != 1 || collab.ActiveLeases[0].Resource != "test/lock" {
		t.Fatalf("expected 1 active lease 'test/lock', got %+v", collab.ActiveLeases)
	}
	if len(collab.OpenQuestions) != 1 || collab.OpenQuestions[0].Text != "Should we proceed with step 2?" {
		t.Fatalf("expected 1 open question, got %+v", collab.OpenQuestions)
	}

	// A non-admin human who cannot read #general sees no machine, folders,
	// runs, notes or questions from it (the hosts API is admin-only).
	memberRole := apispec.Member
	var carol apispec.CreateActorResponse
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind: apispec.ActorKindHuman, Handle: "carol", Role: &memberRole,
	}, &carol); s != http.StatusCreated {
		t.Fatalf("create carol: %d", s)
	}
	var limited apispec.AgentOverview
	if s := e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", agentIDStr), carol.Token, nil, &limited); s != http.StatusOK {
		t.Fatalf("member overview: %d", s)
	}
	if limited.Assignment != nil {
		t.Fatalf("non-admin must not see the assignment: %+v", limited.Assignment)
	}
	if limited.Threads != nil && len(*limited.Threads) != 0 {
		t.Fatalf("non-admin must not see threads of unreadable channels: %+v", *limited.Threads)
	}
	if len(limited.Usage.LastRuns) != 0 || len(limited.Collaboration.Notes) != 0 || len(limited.Collaboration.OpenQuestions) != 0 {
		t.Fatalf("non-admin saw runs, notes or questions: %+v %+v", limited.Usage.LastRuns, limited.Collaboration)
	}
	_ = secret
	_ = conversation.KindPublic
}
