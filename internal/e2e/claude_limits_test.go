package e2e

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

func TestRateLimitHostReportAndOverview(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	// 1. Create claude agent
	claudeID, _ := e.createActor("agent", "claude-bot", "member")

	// 2. Pair host
	base := t.TempDir()
	root := filepath.Join(base, "project")
	_ = os.MkdirAll(root, 0o700)
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "testhost", tokenFile); err != nil {
		t.Fatalf("enroll: %v", err)
	}

	hostTokenBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("read host token: %v", err)
	}
	hostToken := strings.TrimSpace(string(hostTokenBytes))

	var hl struct {
		Hosts []struct {
			ID string `json:"id"`
		} `json:"hosts"`
	}
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hl)
	if len(hl.Hosts) == 0 {
		t.Fatal("no hosts found")
	}
	hostID := hl.Hosts[0].ID

	// Assign claude agent to this host
	e.do("PUT", "/api/v1/agents/"+claudeID+"/host", server.DevAdminToken, map[string]any{
		"host_id":         hostID,
		"client":          "claude-code",
		"mode":            "direct",
		"root_dir":        root,
		"sandbox":         true,
		"permission_mode": "auto",
	}, nil)

	// 3. Guard sends report carrying ClaudeLimits
	hostClient := &guard.Client{
		BaseURL: e.srv.URL,
		Token:   hostToken,
	}

	paused := true
	reserve := 90
	warn := 80
	resetsAt := time.Date(2026, 10, 28, 18, 0, 0, 0, time.UTC)
	limits := &apispec.ClaudeLimits{
		Paused:           &paused,
		RateLimitReserve: &reserve,
		RateLimitWarn:    &warn,
		FiveHour: &apispec.ClaudeRateLimitWindow{
			Utilization: 0.91,
			ResetsAt:    &resetsAt,
			Status:      "allowed",
		},
		SevenDay: &apispec.ClaudeRateLimitWindow{
			Utilization: 0.59,
			Status:      "allowed",
		},
	}

	err = hostClient.Report(ctx, guard.HostReport{
		GuardVersion: "test-guard",
		Roots:        []string{root},
		ClaudeLimits: limits,
	})
	if err != nil {
		t.Fatalf("host report: %v", err)
	}

	// 4. Verify GET /api/v1/hosts returns ClaudeLimits
	var hlAfter struct {
		Hosts []apispec.Host `json:"hosts"`
	}
	status := e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hlAfter)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/hosts: status %d", status)
	}
	if len(hlAfter.Hosts) == 0 {
		t.Fatal("no hosts returned")
	}
	hReport := hlAfter.Hosts[0].Report
	if hReport.ClaudeLimits == nil {
		t.Fatal("expected claude_limits on host report, got nil")
	}
	if hReport.ClaudeLimits.Paused == nil || !*hReport.ClaudeLimits.Paused {
		t.Errorf("expected Paused=true, got %v", hReport.ClaudeLimits.Paused)
	}
	if hReport.ClaudeLimits.FiveHour == nil || hReport.ClaudeLimits.FiveHour.Utilization != 0.91 {
		t.Errorf("expected 5h utilization 0.91, got %v", hReport.ClaudeLimits.FiveHour)
	}
	if hReport.ClaudeLimits.SevenDay == nil || hReport.ClaudeLimits.SevenDay.Utilization != 0.59 {
		t.Errorf("expected 7d utilization 0.59, got %v", hReport.ClaudeLimits.SevenDay)
	}

	// 5. Verify GET /api/v1/agents/{id}/overview returns ClaudeLimits
	var overview apispec.AgentOverview
	status = e.do("GET", fmt.Sprintf("/api/v1/agents/%s/overview", claudeID), server.DevAdminToken, nil, &overview)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agents/{id}/overview: status %d", status)
	}
	if overview.Assignment == nil {
		t.Fatal("expected overview.Assignment to be non-nil")
	}
	if overview.Assignment.ClaudeLimits == nil {
		t.Fatal("expected assignment.claude_limits to be non-nil")
	}
	if overview.Assignment.ClaudeLimits.FiveHour == nil || overview.Assignment.ClaudeLimits.FiveHour.Utilization != 0.91 {
		t.Errorf("expected assignment 5h utilization 0.91, got %v", overview.Assignment.ClaudeLimits.FiveHour)
	}
}

func TestRateLimitNoteEndpoint(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	// 1. Create channel and agent
	pubKind := apispec.Public
	var ch apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "test-ratelimit-channel",
		Kind: &pubKind,
	}, &ch)

	agentID, agentToken := e.createActor("agent", "claude-bot", "member")
	canWrite := true
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", ch.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  uuid.MustParse(agentID),
		CanWrite: &canWrite,
	}, nil)

	// Post root message
	var sendRes apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", ch.Id), server.DevAdminToken, apispec.SendMessageRequest{
		Body: "hello agent",
	}, &sendRes)
	rootMsgID := sendRes.Message.Id

	// 2. Agent posts 5-hour rate limit note
	loc, _ := time.LoadLocation("Europe/Budapest")
	reset5h := time.Date(2026, 10, 28, 18, 0, 0, 0, loc)

	req5h := apispec.RateLimitNoteRequest{
		ChannelId:    ch.Id,
		ThreadRootId: &rootMsgID,
		Window:       apispec.FiveHour,
		Utilization:  91,
		ResetsAt:     reset5h,
	}

	status := e.do("POST", "/api/v1/agent/ratelimit-note", agentToken, req5h, nil)
	if status != http.StatusNoContent {
		t.Fatalf("POST /api/v1/agent/ratelimit-note: expected 204, got %d", status)
	}

	var th5h apispec.Thread
	e.do("GET", fmt.Sprintf("/api/v1/messages/%s/thread", rootMsgID), server.DevAdminToken, nil, &th5h)
	var noteMsg5h *apispec.Message
	for i := range th5h.Replies {
		if th5h.Replies[i].ThreadRootId != nil && *th5h.Replies[i].ThreadRootId == rootMsgID {
			noteMsg5h = &th5h.Replies[i]
			break
		}
	}
	if noteMsg5h == nil {
		t.Fatal("expected 5h rate limit note in thread, found none")
	}
	expectedBody5h := "⏸ A Claude 5 órás kerete 91%-on, 18:00-kor folytatom."
	if noteMsg5h.Body != expectedBody5h {
		t.Errorf("expected body %q, got %q", expectedBody5h, noteMsg5h.Body)
	}

	// 3. Agent posts weekly rate limit note (Monday reset)
	// 2026-11-02 was a Monday
	resetWeekly := time.Date(2026, 11, 2, 18, 0, 0, 0, loc)
	reqWeekly := apispec.RateLimitNoteRequest{
		ChannelId:    ch.Id,
		ThreadRootId: &rootMsgID,
		Window:       apispec.SevenDay,
		Utilization:  95,
		ResetsAt:     resetWeekly,
	}

	status = e.do("POST", "/api/v1/agent/ratelimit-note", agentToken, reqWeekly, nil)
	if status != http.StatusNoContent {
		t.Fatalf("POST /api/v1/agent/ratelimit-note weekly: expected 204, got %d", status)
	}

	var thWeekly apispec.Thread
	e.do("GET", fmt.Sprintf("/api/v1/messages/%s/thread", rootMsgID), server.DevAdminToken, nil, &thWeekly)
	var noteMsgWeekly *apispec.Message
	for i := range thWeekly.Replies {
		if thWeekly.Replies[i].Id != noteMsg5h.Id && thWeekly.Replies[i].ThreadRootId != nil && *thWeekly.Replies[i].ThreadRootId == rootMsgID {
			noteMsgWeekly = &thWeekly.Replies[i]
			break
		}
	}
	if noteMsgWeekly == nil {
		t.Fatal("expected weekly rate limit note in thread, found none")
	}
	expectedBodyWeekly := "⏸ A Claude heti kerete 95%-on, hétfőn 18:00-kor folytatom."
	if noteMsgWeekly.Body != expectedBodyWeekly {
		t.Errorf("expected weekly body %q, got %q", expectedBodyWeekly, noteMsgWeekly.Body)
	}
}

func TestGuard_ClaudeLimiterPause_E2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	pubKind := apispec.Public
	var ch apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{
		Name: "test-claude-limit",
		Kind: &pubKind,
	}, &ch)

	claudeID, _ := e.createActor("agent", "claude-bot", "member")
	canWrite := true
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/members", ch.Id), server.DevAdminToken, apispec.AddMemberRequest{
		ActorId:  uuid.MustParse(claudeID),
		CanWrite: &canWrite,
	}, nil)

	base := t.TempDir()
	root := filepath.Join(base, "project")
	_ = os.MkdirAll(root, 0o700)
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "claude-box", tokenFile); err != nil {
		t.Fatal(err)
	}

	var hosts struct {
		Hosts []struct {
			ID string `json:"id"`
		}
	}
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	e.do("PUT", "/api/v1/agents/"+claudeID+"/host", server.DevAdminToken, map[string]any{
		"host_id":         hostID,
		"client":          "claude-code",
		"mode":            "direct",
		"root_dir":        root,
		"sandbox":         true,
		"permission_mode": "auto",
	}, nil)

	loc, _ := time.LoadLocation("Europe/Budapest")
	simTime := time.Date(2026, 10, 28, 12, 0, 0, 0, loc)
	timeMu := sync.Mutex{}
	mockNow := func() time.Time {
		timeMu.Lock()
		defer timeMu.Unlock()
		return simTime
	}

	resetTime := simTime.Add(1 * time.Hour) // resets at 13:00

	var mu sync.Mutex
	runCalls := 0
	ad := &scriptedAdapter{
		direct: func(ctx context.Context, req guard.DirectRequest) (guard.RunResult, error) {
			mu.Lock()
			runCalls++
			callNum := runCalls
			mu.Unlock()

			if callNum == 1 {
				// Simulate emitting rate limit event at 91%
				if req.OnRateLimit != nil {
					req.OnRateLimit(guard.RateLimit{
						Status: "allowed",
						FiveHour: &guard.RateLimitWindow{
							Utilization: 0.91,
							ResetsAt:    resetTime,
							Status:      "allowed",
						},
					})
				}
				return guard.RunResult{
					Text: "First run done",
					RateLimit: &guard.RateLimit{
						Status: "allowed",
						FiveHour: &guard.RateLimitWindow{
							Utilization: 0.91,
							ResetsAt:    resetTime,
							Status:      "allowed",
						},
					},
				}, nil
			}
			return guard.RunResult{Text: "Second run done after reset"}, nil
		},
	}

	off := false
	sup := &guard.Supervisor{
		Cfg: guard.Config{
			Server:           e.srv.URL,
			HostTokenFile:    tokenFile,
			Roots:            []string{root},
			SandboxDir:       filepath.Join(base, "sandbox"),
			RequireSandbox:   &off,
			AllowUnsandboxed: true,
			ConfigPoll:       20 * time.Millisecond,
			RateLimitReserve: 90,
			RateLimitWarn:    80,
			Defaults: guard.AgentConfig{
				Poll:      20 * time.Millisecond,
				Debounce:  time.Millisecond,
				HaltCheck: 10 * time.Millisecond,
			},
		},
		Now:        mockNow,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	// 1. Post first message to start thread
	var sendRes1 apispec.SendResult
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", ch.Id), server.DevAdminToken, apispec.SendMessageRequest{
		Body: "msg 1: do work @claude-bot",
	}, &sendRes1)
	threadID := sendRes1.Message.Id

	// Wait for first run to complete
	eventually(t, "first run completed", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runCalls == 1
	})

	// 2. Post second message in thread while paused (at 91% reserve)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", ch.Id), server.DevAdminToken, apispec.SendMessageRequest{
		Body:         "msg 2: do more work @claude-bot",
		ThreadRootId: &threadID,
	}, nil)

	// Guard should pause, NOT run second run yet, and post rate limit note!
	eventually(t, "rate limit note posted in thread", func() bool {
		var th apispec.Thread
		e.do("GET", fmt.Sprintf("/api/v1/messages/%s/thread", threadID), server.DevAdminToken, nil, &th)
		for _, rep := range th.Replies {
			if strings.Contains(rep.Body, "⏸ A Claude 5 órás kerete 91%-on") {
				return true
			}
		}
		return false
	})

	// Run calls must still be 1 (paused, not started!)
	mu.Lock()
	if runCalls != 1 {
		t.Fatalf("expected runCalls to stay 1 while paused, got %d", runCalls)
	}
	mu.Unlock()

	// 3. Advance clock past resetTime + 1m
	timeMu.Lock()
	simTime = resetTime.Add(2 * time.Minute)
	timeMu.Unlock()
	sup.WakeAll()

	// Wait for second run to execute automatically after resume
	eventually(t, "second run completed after reset", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return runCalls == 2
	})

	// Verify reply appears in thread
	var thFinal apispec.Thread
	e.do("GET", fmt.Sprintf("/api/v1/messages/%s/thread", threadID), server.DevAdminToken, nil, &thFinal)
	foundReply := false
	for _, rep := range thFinal.Replies {
		if strings.Contains(rep.Body, "Second run done after reset") {
			foundReply = true
			break
		}
	}
	if !foundReply {
		t.Fatal("expected second run reply in thread, not found")
	}
}
