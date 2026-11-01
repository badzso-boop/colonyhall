package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func TestLiveProgressE2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	general := e.generalID()
	agentID := e.agentID()

	// 1. Connect WebSocket as admin (reader of general)
	wsAdmin, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = wsAdmin.CloseNow() }()
	if err := wsjson.Write(ctx, wsAdmin, map[string]any{"type": "hello", "token": server.DevAdminToken}); err != nil {
		t.Fatal(err)
	}
	var welcome map[string]any
	if err := wsjson.Read(ctx, wsAdmin, &welcome); err != nil || welcome["type"] != "welcome" {
		t.Fatalf("expected welcome frame, got: %v, err: %v", welcome, err)
	}
	var replayDone map[string]any
	if err := wsjson.Read(ctx, wsAdmin, &replayDone); err != nil || replayDone["type"] != "replay_done" {
		t.Fatalf("expected replay_done frame, got: %v, err: %v", replayDone, err)
	}

	// 2. Non-agent (human) calling POST /api/v1/agent/progress returns 403
	humanStatus := e.do("POST", "/api/v1/agent/progress", server.DevAdminToken, map[string]any{
		"channel_id": general,
		"run_key":    "test-run",
		"steps":      []any{},
	}, nil)
	if humanStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for human actor reporting progress, got %d", humanStatus)
	}

	// 3. Agent reporting on private channel it cannot read returns 403
	var privChan struct{ ID string }
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "secret-ops", "kind": "private"}, &privChan); s != http.StatusCreated {
		t.Fatalf("create private channel: %d", s)
	}
	privStatus := e.do("POST", "/api/v1/agent/progress", server.DevAgentToken, map[string]any{
		"channel_id": privChan.ID,
		"run_key":    "test-run",
		"steps":      []any{},
	}, nil)
	if privStatus != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for channel agent cannot read, got %d", privStatus)
	}

	// 4. Create second user bob (not in private channel) and connect websocket
	var bobCreated struct {
		Actor struct{ ID string }
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{"kind": "human", "handle": "bob", "role": "member"}, &bobCreated); s != http.StatusCreated {
		t.Fatalf("create bob: %d", s)
	}
	var bobIssued struct {
		Secret string
	}
	if s := e.do("POST", "/api/v1/actors/"+bobCreated.Actor.ID+"/tokens", server.DevAdminToken, map[string]any{"label": "bob-tok"}, &bobIssued); s != http.StatusCreated {
		t.Fatalf("issue bob token: %d", s)
	}
	wsBob, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = wsBob.CloseNow() }()
	if err := wsjson.Write(ctx, wsBob, map[string]any{"type": "hello", "token": bobIssued.Secret}); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, wsBob, &welcome); err != nil || welcome["type"] != "welcome" {
		t.Fatalf("bob expected welcome frame, got: %v", welcome)
	}
	if err := wsjson.Read(ctx, wsBob, &replayDone); err != nil || replayDone["type"] != "replay_done" {
		t.Fatalf("bob expected replay_done frame, got: %v", replayDone)
	}

	// Add agent to private channel
	if s := e.do("POST", "/api/v1/channels/"+privChan.ID+"/members", server.DevAdminToken, map[string]any{"actor_id": agentID}, nil); s != http.StatusNoContent {
		t.Fatalf("add agent to priv chan: %d", s)
	}

	// 5. Agent reports progress in private channel -> 204
	nowStr := time.Now().UTC().Format(time.RFC3339)
	step1 := map[string]any{
		"kind":        "read",
		"label":       "secret.txt",
		"path":        "secret.txt",
		"state":       "done",
		"started_at":  nowStr,
		"duration_ms": 120,
	}
	agentStatus := e.do("POST", "/api/v1/agent/progress", server.DevAgentToken, map[string]any{
		"channel_id": privChan.ID,
		"run_key":    "run-priv-1",
		"steps":      []any{step1},
		"done":       false,
	}, nil)
	if agentStatus != http.StatusNoContent {
		t.Fatalf("expected 204 NoContent, got %d", agentStatus)
	}

	// Admin (member of privChan) must receive the agent.progress frame
	var progFrame map[string]any
	for {
		var f map[string]any
		if err := wsjson.Read(ctx, wsAdmin, &f); err != nil {
			t.Fatalf("admin failed to read progress frame: %v", err)
		}
		if f["type"] == "agent.progress" {
			progFrame = f
			break
		}
	}
	if progFrame["run_key"] != "run-priv-1" {
		t.Fatalf("expected run_key run-priv-1, got %v", progFrame["run_key"])
	}

	// Bob (NOT a member of privChan) must NOT receive the agent.progress frame
	readCtx, readCancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	for {
		var f map[string]any
		err := wsjson.Read(readCtx, wsBob, &f)
		if err != nil {
			// Timed out without receiving agent.progress -> success!
			break
		}
		if f["type"] == "agent.progress" {
			readCancel()
			t.Fatalf("bob without channel access must not receive progress frame, got: %v", f)
		}
	}
	readCancel()

	// 6. Test GET /api/v1/channels/{id}/progress
	// Bob calling it on privChan -> 403 Forbidden
	bobProgStatus := e.do("GET", "/api/v1/channels/"+privChan.ID+"/progress", bobIssued.Secret, nil, nil)
	if bobProgStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized human calling channel progress, got %d", bobProgStatus)
	}

	// Admin calling it on privChan -> 200 OK with active run
	var adminProgResp apispec.ChannelProgressResponse
	adminProgStatus := e.do("GET", "/api/v1/channels/"+privChan.ID+"/progress", server.DevAdminToken, nil, &adminProgResp)
	if adminProgStatus != http.StatusOK {
		t.Fatalf("expected 200 for admin calling channel progress, got %d", adminProgStatus)
	}
	if len(adminProgResp.Runs) != 1 {
		t.Fatalf("expected 1 run in progress, got %d", len(adminProgResp.Runs))
	}
	if adminProgResp.Runs[0].RunKey != "run-priv-1" {
		t.Fatalf("expected run_key run-priv-1, got %s", adminProgResp.Runs[0].RunKey)
	}
	if len(adminProgResp.Runs[0].Steps) != 1 || adminProgResp.Runs[0].Steps[0].Label != "secret.txt" {
		t.Fatalf("expected 1 step secret.txt, got %+v", adminProgResp.Runs[0].Steps)
	}

	// 7. Store Run Report with steps and reply_message_id
	// First send a reply message from agent
	var sendRes struct {
		Message struct {
			Id string `json:"id"`
		} `json:"message"`
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, map[string]any{
		"body":          "Here is the result of my work.",
		"client_msg_id": "test-reply-1",
	}, &sendRes); s != http.StatusCreated {
		t.Fatalf("send agent reply message: %d", s)
	}
	replyMsgID := sendRes.Message.Id
	if replyMsgID == "" {
		t.Fatal("empty replyMsgID")
	}

	reportReq := map[string]any{
		"phase":              "direct",
		"client":             "claude-code",
		"model":              "claude-3-7-sonnet",
		"session_id":         "sess-123",
		"channel_id":         general,
		"items":              1,
		"input_tokens":       1500,
		"output_tokens":      450,
		"cache_read_tokens":  0,
		"cache_write_tokens": 0,
		"duration_ms":        2500,
		"status":             "ok",
		"reply_message_id":   replyMsgID,
		"steps": []any{
			map[string]any{
				"kind":        "read",
				"label":       "file1.go",
				"path":        "file1.go",
				"state":       "done",
				"started_at":  nowStr,
				"duration_ms": 300,
			},
			map[string]any{
				"kind":        "command",
				"label":       "go test ./...",
				"state":       "done",
				"started_at":  nowStr,
				"duration_ms": 2200,
			},
		},
	}
	var runRef struct {
		Id string `json:"id"`
	}
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, reportReq, &runRef); s != http.StatusCreated {
		t.Fatalf("expected 201 for report run, got %d", s)
	}

	// 8. Verify GET /api/v1/runs returns stored steps and reply_message_id
	var runList apispec.RunList
	if s := e.do("GET", "/api/v1/runs?limit=1", server.DevAdminToken, nil, &runList); s != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/v1/runs, got %d", s)
	}
	if len(runList.Runs) == 0 {
		t.Fatal("expected at least 1 run in list")
	}
	latestRun := runList.Runs[0]
	if latestRun.ReplyMessageId == nil || latestRun.ReplyMessageId.String() != replyMsgID {
		t.Fatalf("expected reply_message_id %s, got %v", replyMsgID, latestRun.ReplyMessageId)
	}
	if latestRun.Steps == nil || len(*latestRun.Steps) != 2 {
		t.Fatalf("expected 2 steps stored, got %v", latestRun.Steps)
	}
	if (*latestRun.Steps)[0].Label != "file1.go" || (*latestRun.Steps)[1].Label != "go test ./..." {
		t.Fatalf("unexpected steps content: %+v", *latestRun.Steps)
	}

	// 9. Verify GET /api/v1/agents/{id}/overview returns steps in last_runs
	var agentOverview apispec.AgentOverview
	if s := e.do("GET", "/api/v1/agents/"+agentID+"/overview", server.DevAdminToken, nil, &agentOverview); s != http.StatusOK {
		t.Fatalf("expected 200 for agent overview, got %d", s)
	}
	if len(agentOverview.Usage.LastRuns) == 0 {
		t.Fatal("expected last runs in agent overview")
	}
	firstRun := agentOverview.Usage.LastRuns[0]
	if firstRun.Steps == nil || len(*firstRun.Steps) != 2 {
		t.Fatalf("expected 2 steps in overview run, got: %v", firstRun.Steps)
	}
}

// Steps are for humans only: another agent's guard connection, even in a
// channel it can read, never gets agent.progress frames; and a terminal
// event for a channel the agent cannot read is refused.
func TestProgressNotSentToAgentsAndTerminalEventChannelChecked(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	general := e.generalID()

	var other struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "worker", "api_key": true}, &other); s != http.StatusCreated {
		t.Fatalf("create worker: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": other.Actor.ID, "can_write": true}, nil); s != http.StatusNoContent {
		t.Fatalf("add worker: %d", s)
	}

	// @fake-agent's guard connection, a reader of #general.
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	defer func() { _ = ws.CloseNow() }()
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": server.DevAgentToken}); err != nil {
		t.Fatal(err)
	}
	expectFrame(t, ctx, ws, "welcome")
	expectFrame(t, ctx, ws, "replay_done")

	// @worker reports a step, then a human posts a sentinel message.
	if s := e.do("POST", "/api/v1/agent/progress", other.Token, map[string]any{
		"channel_id": general, "run_key": "r1",
		"steps": []any{map[string]any{"kind": "read", "label": "secret/plan.md", "state": "done", "started_at": "2026-10-26T10:00:00Z"}},
	}, nil); s != http.StatusNoContent && s != http.StatusOK && s != http.StatusAccepted {
		t.Fatalf("progress: %d", s)
	}
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "sentinel"}); s != http.StatusCreated {
		t.Fatalf("sentinel: %d", s)
	}
	for {
		var f map[string]any
		if err := wsjson.Read(ctx, ws, &f); err != nil {
			t.Fatalf("waiting for the sentinel: %v", err)
		}
		if f["type"] == "agent.progress" {
			t.Fatalf("an agent connection got another agent's steps: %v", f)
		}
		if f["type"] == "event" && f["event_type"] == "message.created" {
			break
		}
	}

	// A terminal event for a private channel the agent cannot read.
	var priv struct{ ID string }
	if s := e.do("POST", "/api/v1/channels", server.DevAdminToken, map[string]any{"name": "ops-private", "kind": "private"}, &priv); s != http.StatusCreated {
		t.Fatalf("private channel: %d", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-events", server.DevAgentToken, map[string]any{
		"name": "x", "channel_id": priv.ID, "duration_ms": 1, "tail": "done",
	}, nil); s != http.StatusForbidden {
		t.Fatalf("terminal event for an unreadable channel: want 403, got %d", s)
	}
}

// Run keys come from the guards: another agent using the same run key must
// not add steps to someone else's run.
func TestProgressRunKeyScopedToAgent(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	var other struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "intruder", "api_key": true}, &other); s != http.StatusCreated {
		t.Fatalf("create intruder: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": other.Actor.ID, "can_write": true}, nil); s != http.StatusNoContent {
		t.Fatalf("add intruder: %d", s)
	}
	step := func(label string) []any {
		return []any{map[string]any{"kind": "read", "label": label, "state": "done", "started_at": "2026-10-26T10:00:00Z"}}
	}
	e.do("POST", "/api/v1/agent/progress", server.DevAgentToken, map[string]any{"channel_id": general, "run_key": "shared", "steps": step("real.md")}, nil)
	e.do("POST", "/api/v1/agent/progress", other.Token, map[string]any{"channel_id": general, "run_key": "shared", "steps": step("forged.md")}, nil)

	var res struct {
		Runs []struct {
			AgentID string `json:"agent_id"`
			Steps   []struct{ Label string }
		}
	}
	if s := e.do("GET", "/api/v1/channels/"+general+"/progress", server.DevAdminToken, nil, &res); s != http.StatusOK {
		t.Fatalf("progress: %d", s)
	}
	agentID := e.agentID()
	for _, r := range res.Runs {
		if r.AgentID != agentID {
			continue
		}
		for _, st := range r.Steps {
			if st.Label == "forged.md" {
				t.Fatalf("another agent's step landed in @fake-agent's run: %+v", r)
			}
		}
	}
}
