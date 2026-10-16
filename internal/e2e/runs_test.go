package e2e

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

type runTrackingAdapter struct {
	decideResult guard.RunResult
	decideErr    error
	workResult   guard.RunResult
	workErr      error
	decideCalls  int
	workCalls    int
}

func (a *runTrackingAdapter) Decide(_ context.Context, req guard.DecideRequest) (guard.RunResult, error) {
	a.decideCalls++
	_, refs := guard.BuildPrompt(req.Handle, req.Items, false)
	res := a.decideResult
	res.Refs = refs
	return res, a.decideErr
}

func (a *runTrackingAdapter) Work(_ context.Context, _ guard.WorkRequest) (guard.RunResult, error) {
	a.workCalls++
	return a.workResult, a.workErr
}

func (a *runTrackingAdapter) Direct(_ context.Context, _ guard.DirectRequest) (guard.RunResult, error) {
	return guard.RunResult{Text: "ok"}, nil
}

func TestRunLoggingFromGuard(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, _ = e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent hello"})

	ad := &runTrackingAdapter{
		decideResult: guard.RunResult{
			Decision: guard.Decision{
				Actions: []guard.Action{
					{Ref: "1", Kind: guard.ActReply, Text: "hi back"},
				},
			},
			Usage: guard.Usage{
				Input:      120,
				Output:     45,
				CacheRead:  15,
				CacheWrite: 5,
			},
			Duration: 350 * time.Millisecond,
		},
	}
	agent := e.guardAgent(ad, false)
	e.tick(agent)

	if ad.decideCalls != 1 {
		t.Fatalf("decide calls: got %d want 1", ad.decideCalls)
	}

	// 1. Human lists runs
	var rl apispec.RunList
	status := e.do("GET", "/api/v1/runs?limit=10", server.DevAdminToken, nil, &rl)
	if status != http.StatusOK {
		t.Fatalf("GET /runs status: %d", status)
	}
	if len(rl.Runs) != 1 {
		t.Fatalf("runs count: got %d want 1", len(rl.Runs))
	}
	r := rl.Runs[0]
	if r.Phase != "decide" || r.Status != "ok" || r.Items != 1 {
		t.Fatalf("run metadata: %+v", r)
	}
	if r.InputTokens != 120 || r.OutputTokens != 45 || r.CacheReadTokens != 15 || r.CacheWriteTokens != 5 {
		t.Fatalf("run tokens mismatch: in=%d out=%d cr=%d cw=%d", r.InputTokens, r.OutputTokens, r.CacheReadTokens, r.CacheWriteTokens)
	}
	if r.DurationMs != 350 {
		t.Fatalf("run duration: got %d want 350", r.DurationMs)
	}

	// Filter by agent_id
	var filtered apispec.RunList
	e.do("GET", "/api/v1/runs?agent_id="+r.AgentId.String(), server.DevAdminToken, nil, &filtered)
	if len(filtered.Runs) != 1 {
		t.Fatalf("filtered runs: got %d want 1", len(filtered.Runs))
	}

	// 2. Human queries usage
	var uResp apispec.UsageResponse
	status = e.do("GET", "/api/v1/usage?days=7", server.DevAdminToken, nil, &uResp)
	if status != http.StatusOK {
		t.Fatalf("GET /usage status: %d", status)
	}
	if len(uResp.Usage) != 1 {
		t.Fatalf("usage entries: got %d want 1", len(uResp.Usage))
	}
	u := uResp.Usage[0]
	if u.AgentId != r.AgentId {
		t.Fatalf("usage agent id: got %s want %s", u.AgentId, r.AgentId)
	}
	if u.InputTokens != 120 || u.OutputTokens != 45 || u.CacheReadTokens != 15 || u.CacheWriteTokens != 5 || u.TotalTokens != 185 {
		t.Fatalf("usage tokens mismatch: %+v", u)
	}
	if u.RunCount != 1 || u.ErrorCount != 0 {
		t.Fatalf("usage counts: runs=%d errors=%d", u.RunCount, u.ErrorCount)
	}
}

func TestRunLoggingWorkError(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, _ = e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent do some heavy work"})

	ad := &runTrackingAdapter{
		decideResult: guard.RunResult{
			Decision: guard.Decision{
				Actions: []guard.Action{
					{Ref: "1", Kind: guard.ActWork, Task: "crunch numbers"},
				},
			},
			Usage: guard.Usage{Input: 50, Output: 20},
		},
		workResult: guard.RunResult{
			Usage:    guard.Usage{Input: 500, Output: 100},
			Duration: 1200 * time.Millisecond,
		},
		workErr: errors.New("out of memory while compiling"),
	}
	agent := e.guardAgent(ad, true)
	e.tick(agent)

	if ad.workCalls != 1 {
		t.Fatalf("work calls: got %d want 1", ad.workCalls)
	}

	var rl apispec.RunList
	e.do("GET", "/api/v1/runs?limit=10", server.DevAdminToken, nil, &rl)
	if len(rl.Runs) != 2 {
		t.Fatalf("runs count: got %d want 2 (decide + work)", len(rl.Runs))
	}

	// Newest first: rl.Runs[0] is work, rl.Runs[1] is decide
	workRun := rl.Runs[0]
	if workRun.Phase != "work" || workRun.Status != "error" {
		t.Fatalf("work run phase/status: %+v", workRun)
	}
	if workRun.ErrorId == nil {
		t.Fatalf("expected error_id to be set on failed work run")
	}

	// Verify error details can be read by human
	var runErr apispec.RunError
	e.do("GET", "/api/v1/errors/"+workRun.ErrorId.String(), server.DevAdminToken, nil, &runErr)
	if runErr.Id != *workRun.ErrorId || runErr.Summary != "work run failed" {
		t.Fatalf("error details mismatch: %+v", runErr)
	}

	// Check usage sums include error count
	var uResp apispec.UsageResponse
	e.do("GET", "/api/v1/usage?days=7", server.DevAdminToken, nil, &uResp)
	if len(uResp.Usage) != 1 {
		t.Fatalf("usage entries: got %d want 1", len(uResp.Usage))
	}
	u := uResp.Usage[0]
	if u.RunCount != 2 || u.ErrorCount != 1 {
		t.Fatalf("usage counts: runs=%d errors=%d", u.RunCount, u.ErrorCount)
	}
}

func TestRunLoggingPermissions(t *testing.T) {
	e := newEnv(t)

	// Agents cannot read GET /runs
	status := e.do("GET", "/api/v1/runs", server.DevAgentToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("agent GET /runs: got %d want 403", status)
	}

	// Agents cannot read GET /usage
	status = e.do("GET", "/api/v1/usage", server.DevAgentToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("agent GET /usage: got %d want 403", status)
	}

	// Humans cannot call POST /agent/runs
	validRun := map[string]any{
		"phase":              "decide",
		"client":             "claude-code",
		"model":              "claude-3-7-sonnet",
		"session_id":         "sess-1",
		"items":              1,
		"input_tokens":       100,
		"output_tokens":      50,
		"cache_read_tokens":  0,
		"cache_write_tokens": 0,
		"duration_ms":        500,
		"status":             "ok",
	}
	status = e.do("POST", "/api/v1/agent/runs", server.DevAdminToken, validRun, nil)
	if status != http.StatusForbidden {
		t.Fatalf("human POST /agent/runs: got %d want 403", status)
	}
}

func TestRunLoggingValidation(t *testing.T) {
	e := newEnv(t)

	baseRun := map[string]any{
		"phase":              "decide",
		"client":             "claude-code",
		"model":              "claude-3-7-sonnet",
		"session_id":         "sess-1",
		"items":              1,
		"input_tokens":       100,
		"output_tokens":      50,
		"cache_read_tokens":  0,
		"cache_write_tokens": 0,
		"duration_ms":        500,
		"status":             "ok",
	}

	// Negative tokens
	badTokens := copyMap(baseRun)
	badTokens["input_tokens"] = -5
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, badTokens, nil); s != http.StatusBadRequest {
		t.Fatalf("negative tokens: got %d want 400", s)
	}

	// Tokens > 10^9
	tooManyTokens := copyMap(baseRun)
	tooManyTokens["input_tokens"] = 1_000_000_001
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, tooManyTokens, nil); s != http.StatusBadRequest {
		t.Fatalf("excessive tokens: got %d want 400", s)
	}

	// Bad phase
	badPhase := copyMap(baseRun)
	badPhase["phase"] = "magic"
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, badPhase, nil); s != http.StatusBadRequest {
		t.Fatalf("bad phase: got %d want 400", s)
	}

	// Bad status
	badStatus := copyMap(baseRun)
	badStatus["status"] = "waiting"
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, badStatus, nil); s != http.StatusBadRequest {
		t.Fatalf("bad status: got %d want 400", s)
	}

	// Duration > 24 hours
	badDuration := copyMap(baseRun)
	badDuration["duration_ms"] = 24*3600*1000 + 1
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, badDuration, nil); s != http.StatusBadRequest {
		t.Fatalf("bad duration: got %d want 400", s)
	}

	// Usage days bounds
	if s := e.do("GET", "/api/v1/usage?days=0", server.DevAdminToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("days=0: got %d want 400", s)
	}
	if s := e.do("GET", "/api/v1/usage?days=91", server.DevAdminToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("days=91: got %d want 400", s)
	}
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// TestRunReportReferencesAreChecked: a run can only point at the caller's
// own error and a channel it can read; text fields are bounded. Before,
// a foreign or unknown id was stored or broke the insert (500).
func TestRunReportReferencesAreChecked(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, otherToken := e.createActor("agent", "other-bot", "member")
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", otherToken, nil, &me)
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": me.ID}, nil)
	var foreignErr struct{ ID string }
	e.do("POST", "/api/v1/agent/errors", otherToken, map[string]any{"channel_id": general, "summary": "x"}, &foreignErr)
	base := map[string]any{"phase": "direct", "client": "claude-code", "status": "error", "items": 1,
		"input_tokens": 1, "output_tokens": 1, "cache_read_tokens": 0, "cache_write_tokens": 0, "duration_ms": 1}
	with := func(k string, v any) map[string]any {
		m := map[string]any{}
		for kk, vv := range base {
			m[kk] = vv
		}
		m[k] = v
		return m
	}
	for name, body := range map[string]map[string]any{
		"another agent's error": with("error_id", foreignErr.ID),
		"unknown error":         with("error_id", "00000000-0000-0000-0000-0000000000ff"),
		"unknown channel":       with("channel_id", "00000000-0000-0000-0000-0000000000fe"),
	} {
		if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, body, nil); s != http.StatusBadRequest {
			t.Errorf("%s: %d", name, s)
		}
	}
	if s := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, with("model", strings.Repeat("m", 5000)), nil); s != http.StatusCreated {
		t.Fatalf("long model is clipped, not rejected: %d", s)
	}
}
