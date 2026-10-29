package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/apispec"
)

func TestParseClaudeRateLimit_RealFixture(t *testing.T) {
	fixture := `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1793278800,"rateLimitType":"five_hour","overageStatus":"rejected","overageDisabledReason":"org_level_disabled","isUsingOverage":false,"unifiedWindows":{"five_hour":{"utilization":0.1,"resetsAt":1793278800,"status":"allowed"},"seven_day":{"utilization":0.73,"resetsAt":1793574000,"status":"allowed"}}},"uuid":"25d299a0-04d1-4beb-be8b-8c446167f446","session_id":"912e14cb-8af4-4ab2-a602-646a580376d6"}`

	var sl claudeStreamLine
	if err := json.Unmarshal([]byte(fixture), &sl); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	rl := parseClaudeRateLimit(sl, fixture)
	if rl.Status != "allowed" {
		t.Errorf("expected status 'allowed', got %q", rl.Status)
	}
	expectedReset := time.Unix(1793278800, 0)
	if !rl.ResetsAt.Equal(expectedReset) {
		t.Errorf("expected resetsAt %v, got %v", expectedReset, rl.ResetsAt)
	}
	if rl.FiveHour == nil {
		t.Fatal("expected five_hour window, got nil")
	}
	if rl.FiveHour.Utilization != 0.1 {
		t.Errorf("expected five_hour utilization 0.1, got %f", rl.FiveHour.Utilization)
	}
	if rl.FiveHour.Status != "allowed" {
		t.Errorf("expected five_hour status 'allowed', got %q", rl.FiveHour.Status)
	}
	if rl.SevenDay == nil {
		t.Fatal("expected seven_day window, got nil")
	}
	if rl.SevenDay.Utilization != 0.73 {
		t.Errorf("expected seven_day utilization 0.73, got %f", rl.SevenDay.Utilization)
	}
	if rl.SevenDay.Status != "allowed" {
		t.Errorf("expected seven_day status 'allowed', got %q", rl.SevenDay.Status)
	}
}

func TestParseClaudeRateLimit_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
	}{
		{
			name:    "empty object",
			jsonStr: `{"type":"rate_limit_event"}`,
		},
		{
			name:    "rate_limit_info empty",
			jsonStr: `{"type":"rate_limit_event","rate_limit_info":{}}`,
		},
		{
			name:    "missing unifiedWindows",
			jsonStr: `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":1793278800}}`,
		},
		{
			name:    "partial unifiedWindows without utilization or resetsAt",
			jsonStr: `{"type":"rate_limit_event","rate_limit_info":{"unifiedWindows":{"five_hour":{"status":"allowed"}}}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sl claudeStreamLine
			if err := json.Unmarshal([]byte(tc.jsonStr), &sl); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			rl := parseClaudeRateLimit(sl, tc.jsonStr)
			if rl.Raw != tc.jsonStr {
				t.Errorf("expected Raw preserved, got %q", rl.Raw)
			}
		})
	}
}

func TestParseResetTimeFromMessage(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Budapest")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	now := time.Date(2026, 10, 28, 12, 58, 0, 0, loc)

	tests := []struct {
		msg          string
		expectedHour int
		expectedMin  int
		expectOK     bool
	}{
		{msg: "You've hit your session limit · resets 1pm", expectedHour: 13, expectedMin: 0, expectOK: true},
		{msg: "resets 1:30pm", expectedHour: 13, expectedMin: 30, expectOK: true},
		{msg: "resets 18:00", expectedHour: 18, expectedMin: 0, expectOK: true},
		{msg: "resets 11:45am", expectedHour: 11, expectedMin: 45, expectOK: true},
		{msg: "resets 12am", expectedHour: 0, expectedMin: 0, expectOK: true},
		{msg: "resets 12pm", expectedHour: 12, expectedMin: 0, expectOK: true},
		{msg: "random error without reset", expectOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.msg, func(t *testing.T) {
			parsed, ok := parseResetTimeFromMessage(tc.msg, now)
			if ok != tc.expectOK {
				t.Fatalf("expected ok=%v, got %v", tc.expectOK, ok)
			}
			if ok {
				inLoc := parsed.In(loc)
				if inLoc.Hour() != tc.expectedHour || inLoc.Minute() != tc.expectedMin {
					t.Errorf("expected %02d:%02d, got %02d:%02d", tc.expectedHour, tc.expectedMin, inLoc.Hour(), inLoc.Minute())
				}
			}
		})
	}
}

func TestIsClaudeLimitMessage(t *testing.T) {
	positives := []string{
		"You've hit your session limit · resets 1pm",
		"Claude usage limit reached",
		"API rate limit exceeded",
		"You have hit your limit for this week",
	}
	for _, s := range positives {
		if !isClaudeLimitMessage(s) {
			t.Errorf("expected %q to be recognized as claude limit message", s)
		}
	}

	negatives := []string{
		"syntax error in file.go",
		"connection refused",
		"context deadline exceeded",
	}
	for _, s := range negatives {
		if isClaudeLimitMessage(s) {
			t.Errorf("expected %q to NOT be recognized as claude limit message", s)
		}
	}
}

func TestClaudeLimiter_ReservePauseAndResume(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "claude_usage.json")

	fixedNow := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	nowMu := sync.Mutex{}
	mockNow := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return fixedNow
	}
	advanceNow := func(d time.Duration) {
		nowMu.Lock()
		defer nowMu.Unlock()
		fixedNow = fixedNow.Add(d)
	}

	limiter := NewClaudeLimiter(stateFile, 90, 80, nil, mockNow)

	// Below reserve: 85%
	resetTime := fixedNow.Add(2 * time.Hour)
	limiter.Update(RateLimit{
		Status: "allowed",
		FiveHour: &RateLimitWindow{
			Utilization: 0.85,
			ResetsAt:    resetTime,
			Status:      "allowed",
		},
	})

	if limiter.IsPaused(mockNow()) {
		t.Fatal("expected limiter not paused at 85%")
	}

	// At or above reserve: 91%
	limiter.Update(RateLimit{
		Status: "allowed",
		FiveHour: &RateLimitWindow{
			Utilization: 0.91,
			ResetsAt:    resetTime,
			Status:      "allowed",
		},
	})

	if !limiter.IsPaused(mockNow()) {
		t.Fatal("expected limiter paused at 91%")
	}
	pauseUntil := limiter.PauseUntil()
	expectedPauseUntil := resetTime.Add(1 * time.Minute)
	if !pauseUntil.Equal(expectedPauseUntil) {
		t.Errorf("expected pause until %v, got %v", expectedPauseUntil, pauseUntil)
	}

	// Advance clock past reset time + 1 min
	advanceNow(2*time.Hour + 2*time.Minute)

	if limiter.IsPaused(mockNow()) {
		t.Fatal("expected limiter to resume after resetsAt + 1m")
	}

	// Test weekly window reserve: 95%
	advanceNow(1 * time.Minute)
	weeklyReset := mockNow().Add(72 * time.Hour)
	limiter.Update(RateLimit{
		Status: "allowed",
		SevenDay: &RateLimitWindow{
			Utilization: 0.95,
			ResetsAt:    weeklyReset,
			Status:      "allowed",
		},
	})
	if !limiter.IsPaused(mockNow()) {
		t.Fatal("expected limiter paused on weekly window")
	}
	info := limiter.PauseInfo()
	if info.Window != "seven_day" {
		t.Errorf("expected window 'seven_day', got %q", info.Window)
	}
	if info.Utilization != 95 {
		t.Errorf("expected utilization 95, got %d", info.Utilization)
	}

	// Newer event shows utilization dropped back under reserve: 40%
	limiter.Update(RateLimit{
		Status: "allowed",
		SevenDay: &RateLimitWindow{
			Utilization: 0.40,
			ResetsAt:    weeklyReset,
			Status:      "allowed",
		},
	})
	if limiter.IsPaused(mockNow()) {
		t.Fatal("expected limiter to unpause when utilization drops back under reserve")
	}
}

func TestClaudeLimiter_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "claude_usage.json")

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	limiter1 := NewClaudeLimiter(stateFile, 90, 80, nil, func() time.Time { return now })

	resetTime := now.Add(3 * time.Hour)
	limiter1.Update(RateLimit{
		Status: "allowed",
		FiveHour: &RateLimitWindow{
			Utilization: 0.92,
			ResetsAt:    resetTime,
			Status:      "allowed",
		},
	})

	if !limiter1.IsPaused(now) {
		t.Fatal("expected limiter1 paused")
	}

	// Restart: create new limiter reading the same stateFile
	limiter2 := NewClaudeLimiter(stateFile, 90, 80, nil, func() time.Time { return now })
	if !limiter2.IsPaused(now) {
		t.Fatal("expected limiter2 to keep pause after restart")
	}

	// Restart after expiration
	afterReset := resetTime.Add(2 * time.Minute)
	limiter3 := NewClaudeLimiter(stateFile, 90, 80, nil, func() time.Time { return afterReset })
	if limiter3.IsPaused(afterReset) {
		t.Fatal("expected limiter3 to unpause on restart after reset time has passed")
	}
}

func TestClaudeLimiter_ReportLimits(t *testing.T) {
	limiter := NewClaudeLimiter("", 90, 80, nil, nil)
	now := time.Now().Truncate(time.Second)
	limiter.Update(RateLimit{
		Status: "allowed",
		FiveHour: &RateLimitWindow{
			Utilization: 0.72,
			ResetsAt:    now.Add(2 * time.Hour),
			Status:      "allowed",
		},
		SevenDay: &RateLimitWindow{
			Utilization: 0.59,
			ResetsAt:    now.Add(48 * time.Hour),
			Status:      "allowed",
		},
	})

	rep := limiter.ReportLimits()
	if rep == nil {
		t.Fatal("expected non-nil ReportLimits")
	}
	if rep.RateLimitReserve == nil || *rep.RateLimitReserve != 90 {
		t.Errorf("expected reserve 90, got %v", rep.RateLimitReserve)
	}
	if rep.RateLimitWarn == nil || *rep.RateLimitWarn != 80 {
		t.Errorf("expected warn 80, got %v", rep.RateLimitWarn)
	}
	if rep.Paused == nil || *rep.Paused {
		t.Errorf("expected Paused=false, got %v", rep.Paused)
	}
	if rep.FiveHour == nil || rep.FiveHour.Utilization != 0.72 {
		t.Errorf("expected 5h utilization 0.72, got %v", rep.FiveHour)
	}
	if rep.SevenDay == nil || rep.SevenDay.Utilization != 0.59 {
		t.Errorf("expected 7d utilization 0.59, got %v", rep.SevenDay)
	}
}

type ratelimitTestServer struct {
	mu         sync.Mutex
	inboxItems []Item
	ackedIDs   []string
	notes      []apispec.RateLimitNoteRequest
	replies    []string
}

func (s *ratelimitTestServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ag1", "handle": "test-claude", "state": "active"})
		case strings.HasSuffix(r.URL.Path, "/actors"):
			_ = json.NewEncoder(w).Encode(map[string]any{"actors": []map[string]any{{"id": "u1", "handle": "alice"}}})
		case strings.Contains(r.URL.Path, "/agent/inbox/ack"):
			var body struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.ackedIDs = append(s.ackedIDs, body.IDs...)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		case strings.Contains(r.URL.Path, "/agent/inbox"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": s.inboxItems,
				"total": len(s.inboxItems),
			})
		case strings.Contains(r.URL.Path, "/agent/ratelimit-note"):
			var req apispec.RateLimitNoteRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			s.notes = append(s.notes, req)
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/budget"):
			_ = json.NewEncoder(w).Encode(map[string]any{"limit": 0, "remaining": 0})
		case strings.Contains(r.URL.Path, "/messages") && strings.HasSuffix(r.URL.Path, "/thread"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"root":    map[string]any{"id": "m1", "author_id": "u1", "body": "hello"},
				"replies": []map[string]any{},
			})
		case strings.Contains(r.URL.Path, "/messages"):
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.replies = append(s.replies, body.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"id": "reply-1"}})
		case strings.Contains(r.URL.Path, "/runs"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
		}
	})
}

func TestAgent_PauseAtReserve_NoRun_NoAck_NotePosted(t *testing.T) {
	ts := &ratelimitTestServer{
		inboxItems: []Item{
			{
				ID:           "item-1",
				Kind:         "message",
				ItemID:       "msg-1",
				ChannelID:    "11111111-1111-1111-1111-111111111111",
				ThreadRootID: "22222222-2222-2222-2222-222222222222",
				Author:       "alice",
				AuthorKind:   "human",
				Body:         "run task",
			},
		},
	}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	nowMu := sync.Mutex{}
	mockNow := func() time.Time {
		nowMu.Lock()
		defer nowMu.Unlock()
		return now
	}

	tempDir := t.TempDir()
	limiter := NewClaudeLimiter(filepath.Join(tempDir, "usage.json"), 90, 80, nil, mockNow)

	// Set limiter above reserve (91%)
	resetTime := now.Add(1 * time.Hour)
	limiter.Update(RateLimit{
		Status: "allowed",
		FiveHour: &RateLimitWindow{
			Utilization: 0.91,
			ResetsAt:    resetTime,
			Status:      "allowed",
		},
	})

	runCalls := 0
	fakeAdapter := &scriptedAdapter{
		directFn: func(ctx context.Context, req DirectRequest) (RunResult, error) {
			runCalls++
			return RunResult{Text: "done"}, nil
		},
	}

	agent := &Agent{
		Cfg: AgentConfig{
			Client: "claude-code",
			Mode:   "direct",
			Batch:  10,
		},
		Client:        &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Adapter:       fakeAdapter,
		ClaudeLimiter: limiter,
		Now:           mockNow,
		StateDir:      tempDir,
		Log:           slog.Default(),
	}

	ctx := context.Background()

	// Tick 1: agent should see it's paused
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 1 failed: %v", err)
	}

	if runCalls != 0 {
		t.Errorf("expected 0 run calls while paused, got %d", runCalls)
	}
	ts.mu.Lock()
	if len(ts.ackedIDs) != 0 {
		t.Errorf("expected 0 acked items, got %v", ts.ackedIDs)
	}
	if len(ts.notes) != 1 {
		t.Fatalf("expected 1 rate limit note posted, got %d", len(ts.notes))
	}
	note := ts.notes[0]
	if note.Utilization != 91 {
		t.Errorf("expected note utilization 91, got %d", note.Utilization)
	}
	if note.Window != apispec.FiveHour {
		t.Errorf("expected note window five_hour, got %s", note.Window)
	}
	ts.mu.Unlock()

	// Tick 2: note should NOT be posted again for the same thread
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 2 failed: %v", err)
	}
	ts.mu.Lock()
	if len(ts.notes) != 1 {
		t.Errorf("expected note to not be posted again, still 1, got %d", len(ts.notes))
	}
	ts.mu.Unlock()

	// Advance clock past resetsAt + 1m
	nowMu.Lock()
	now = resetTime.Add(2 * time.Minute)
	nowMu.Unlock()

	// Tick 3: limiter is resumed, run should execute!
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 3 failed: %v", err)
	}
	if runCalls != 1 {
		t.Errorf("expected 1 run call after resume, got %d", runCalls)
	}
	ts.mu.Lock()
	if len(ts.ackedIDs) != 1 || ts.ackedIDs[0] != "item-1" {
		t.Errorf("expected item-1 acked after run, got %v", ts.ackedIDs)
	}
	ts.mu.Unlock()
}

func TestAgent_LimitFailure_NoAttemptCount_NotePosted(t *testing.T) {
	ts := &ratelimitTestServer{
		inboxItems: []Item{
			{
				ID:           "item-limit-1",
				Kind:         "message",
				ItemID:       "msg-1",
				ChannelID:    "11111111-1111-1111-1111-111111111111",
				ThreadRootID: "22222222-2222-2222-2222-222222222222",
				Author:       "alice",
				AuthorKind:   "human",
				Body:         "run task",
			},
		},
	}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	loc, _ := time.LoadLocation("Europe/Budapest")
	now := time.Date(2026, 10, 28, 12, 58, 0, 0, loc)
	mockNow := func() time.Time { return now }

	tempDir := t.TempDir()
	limiter := NewClaudeLimiter(filepath.Join(tempDir, "usage.json"), 90, 80, nil, mockNow)

	runCalls := 0
	fakeAdapter := &scriptedAdapter{
		directFn: func(ctx context.Context, req DirectRequest) (RunResult, error) {
			runCalls++
			return RunResult{}, fmt.Errorf("claude: run failed (session_limit): You've hit your session limit · resets 1pm")
		},
	}

	agent := &Agent{
		Cfg: AgentConfig{
			Client: "claude-code",
			Mode:   "direct",
			Batch:  10,
		},
		Client:        &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Adapter:       fakeAdapter,
		ClaudeLimiter: limiter,
		Now:           mockNow,
		StateDir:      tempDir,
		Log:           slog.Default(),
	}

	ctx := context.Background()

	// Tick 1: run fails with limit message
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 1 failed: %v", err)
	}

	if runCalls != 1 {
		t.Errorf("expected 1 run call, got %d", runCalls)
	}
	if agent.attempts["item-limit-1"] != 0 {
		t.Errorf("expected attempts to not be incremented, got %d", agent.attempts["item-limit-1"])
	}
	if !limiter.IsPaused(now) {
		t.Errorf("expected limiter to be paused after limit failure")
	}

	ts.mu.Lock()
	if len(ts.ackedIDs) != 0 {
		t.Errorf("expected 0 acked items, got %v", ts.ackedIDs)
	}
	if len(ts.notes) != 1 {
		t.Fatalf("expected 1 rate limit note posted, got %d", len(ts.notes))
	}
	ts.mu.Unlock()

	// Subsequent ticks should not run because limiter is paused
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 2 failed: %v", err)
	}
	if runCalls != 1 {
		t.Errorf("expected still 1 run call, got %d", runCalls)
	}
}
