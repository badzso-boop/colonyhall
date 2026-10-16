package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type budgetTestServer struct {
	mu         sync.Mutex
	agentID    string
	handle     string
	inboxItems []Item
	ackedIDs   []string
	postedBody []string
	limit      int64
	usedToday  int64
	remaining  int64
	resetsAt   time.Time
}

func (s *budgetTestServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		s.mu.Lock()
		defer s.mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": s.agentID, "handle": s.handle, "state": "active"})
		case strings.HasSuffix(r.URL.Path, "/actors"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"actors": []map[string]any{
					{"id": "u1", "handle": "alice"},
				},
			})
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
		case strings.Contains(r.URL.Path, "/messages") && strings.HasSuffix(r.URL.Path, "/thread"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"root":    map[string]any{"id": "m1", "author_id": "u1", "body": "first message"},
				"replies": []map[string]any{},
			})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if txt, ok := body["body"].(string); ok {
				s.postedBody = append(s.postedBody, txt)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-reply"})
		case strings.HasSuffix(r.URL.Path, "/agent/budget"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"limit":      s.limit,
				"used_today": s.usedToday,
				"remaining":  s.remaining,
				"resets_at":  s.resetsAt.Format(time.RFC3339),
			})
		case strings.HasSuffix(r.URL.Path, "/agent/runs"):
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})
}

type scriptedAdapter struct {
	decideFn func(ctx context.Context, req DecideRequest) (RunResult, error)
	workFn   func(ctx context.Context, req WorkRequest) (RunResult, error)
	directFn func(ctx context.Context, req DirectRequest) (RunResult, error)
}

func (s *scriptedAdapter) Decide(ctx context.Context, req DecideRequest) (RunResult, error) {
	if s.decideFn != nil {
		return s.decideFn(ctx, req)
	}
	return RunResult{}, nil
}

func (s *scriptedAdapter) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	if s.workFn != nil {
		return s.workFn(ctx, req)
	}
	return RunResult{}, nil
}

func (s *scriptedAdapter) Direct(ctx context.Context, req DirectRequest) (RunResult, error) {
	if s.directFn != nil {
		return s.directFn(ctx, req)
	}
	return RunResult{}, nil
}

func TestGuardDailyTokenBudget_Direct(t *testing.T) {
	fakeNow := time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)
	resetsAt := time.Date(2026, 10, 16, 22, 0, 0, 0, time.UTC) // midnight Budapest

	bs := &budgetTestServer{
		agentID:   "00000000-0000-0000-0000-0000000000a1",
		handle:    "worker",
		limit:     100000,
		usedToday: 100000,
		remaining: 0,
		resetsAt:  resetsAt,
		inboxItems: []Item{
			{
				ID:           "item-1",
				Kind:         "message",
				ItemID:       "m1",
				ChannelID:    "c1",
				ThreadRootID: "m1",
				AuthorKind:   "human",
				Body:         "Please do this task",
			},
		},
	}
	ts := httptest.NewServer(bs.handler())
	defer ts.Close()

	directCalls := 0
	ad := &scriptedAdapter{
		directFn: func(ctx context.Context, req DirectRequest) (RunResult, error) {
			directCalls++
			return RunResult{
				Text:  "Task finished.",
				Usage: Usage{Input: 1000, Output: 500},
			}, nil
		},
	}

	stateDir := t.TempDir()
	agent := &Agent{
		Cfg: AgentConfig{
			Name:            "worker",
			Client:          "claude-code",
			Mode:            "direct",
			Dir:             stateDir,
			Poll:            5 * time.Second,
			FallbackPoll:    60 * time.Second,
			DailyTokenLimit: 100000,
		},
		Client: &Client{
			BaseURL: ts.URL,
			Token:   "dummy-token",
			HTTP:    ts.Client(),
		},
		Adapter:  ad,
		Mode:     "direct",
		RootDir:  stateDir,
		StateDir: stateDir,
		Log:      slog.Default(),
		Now: func() time.Time {
			return fakeNow
		},
	}
	agent.me = Me{ID: bs.agentID, Handle: bs.handle, State: "active"}

	ctx := context.Background()

	// 1. Over budget: no run, item not acked, one note posted in thread
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 1 failed: %v", err)
	}
	if directCalls != 0 {
		t.Fatalf("expected 0 direct calls when over budget, got %d", directCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 0 {
		t.Fatalf("expected 0 acked items, got %v", bs.ackedIDs)
	}
	if len(bs.postedBody) != 1 {
		t.Fatalf("expected 1 posted message (budget notice), got %v", bs.postedBody)
	}
	wantNotice := fmt.Sprintf("⏳ My daily token budget is used up; I continue after %s.", resetsAt.Local().Format("15:04"))
	if bs.postedBody[0] != wantNotice {
		t.Fatalf("budget notice mismatch: got %q want %q", bs.postedBody[0], wantNotice)
	}
	bs.mu.Unlock()

	// Verify waitInterval waits until resets_at
	wantWait := resetsAt.Sub(fakeNow)
	if gotWait := agent.waitInterval(); gotWait != wantWait {
		t.Fatalf("waitInterval: got %v want %v", gotWait, wantWait)
	}

	// 2. Same day, tick again: still over budget, no run, still only 1 note total (no duplicate note today)
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 2 failed: %v", err)
	}
	if directCalls != 0 {
		t.Fatalf("expected 0 direct calls, got %d", directCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 0 {
		t.Fatalf("expected 0 acked items, got %v", bs.ackedIDs)
	}
	if len(bs.postedBody) != 1 {
		t.Fatalf("expected still exactly 1 posted message, got %d: %v", len(bs.postedBody), bs.postedBody)
	}
	bs.mu.Unlock()

	// 3. Raised limit: runs again!
	bs.mu.Lock()
	bs.limit = 200000
	bs.remaining = 100000
	bs.mu.Unlock()

	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 3 (raised limit) failed: %v", err)
	}
	if directCalls != 1 {
		t.Fatalf("expected 1 direct call after limit raised, got %d", directCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 1 || bs.ackedIDs[0] != "item-1" {
		t.Fatalf("expected item-1 acked, got %v", bs.ackedIDs)
	}
	if len(bs.postedBody) != 2 || bs.postedBody[1] != "Task finished." {
		t.Fatalf("expected reply posted, got %v", bs.postedBody)
	}
	bs.mu.Unlock()

	// waitInterval should be normal fallbackPoll now (budget cleared)
	if gotWait := agent.waitInterval(); gotWait != 60*time.Second {
		t.Fatalf("waitInterval after budget cleared: got %v want 60s", gotWait)
	}

	// 4. Next day: over budget for day 1, then clock advances across midnight -> runs again
	bs.mu.Lock()
	bs.usedToday = 200000
	bs.remaining = 0
	bs.inboxItems = []Item{
		{
			ID:           "item-2",
			Kind:         "message",
			ItemID:       "m2",
			ChannelID:    "c1",
			ThreadRootID: "m2",
			AuthorKind:   "human",
			Body:         "Task for tomorrow",
		},
	}
	bs.mu.Unlock()

	// Tick on day 1 with new item: over budget
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 4 failed: %v", err)
	}
	if directCalls != 1 { // still 1 from previous step
		t.Fatalf("expected no new direct calls when over budget, got %d", directCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 1 { // only item-1 from step 3
		t.Fatalf("item-2 should not be acked, got %v", bs.ackedIDs)
	}
	bs.mu.Unlock()

	// Advance clock past midnight to day 2
	fakeNow = fakeNow.Add(9 * time.Hour) // 23:00 UTC = 01:00 next day CEST
	resetsAtNext := resetsAt.Add(24 * time.Hour)
	bs.mu.Lock()
	bs.usedToday = 0
	bs.remaining = 200000
	bs.resetsAt = resetsAtNext
	bs.mu.Unlock()

	// Tick on day 2: budget reset -> runs again!
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("tick 5 (next day) failed: %v", err)
	}
	if directCalls != 2 {
		t.Fatalf("expected 2 direct calls after next day reset, got %d", directCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 2 || bs.ackedIDs[1] != "item-2" {
		t.Fatalf("expected item-2 acked, got %v", bs.ackedIDs)
	}
	bs.mu.Unlock()
}

func TestGuardDailyTokenBudget_Compact(t *testing.T) {
	fakeNow := time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)
	resetsAt := time.Date(2026, 10, 16, 22, 0, 0, 0, time.UTC)

	bs := &budgetTestServer{
		agentID:   "00000000-0000-0000-0000-0000000000b2",
		handle:    "compact-bot",
		limit:     50000,
		usedToday: 50000,
		remaining: 0,
		resetsAt:  resetsAt,
		inboxItems: []Item{
			{
				ID:           "item-c1",
				Kind:         "message",
				ItemID:       "m1",
				ChannelID:    "c1",
				ThreadRootID: "m1",
				AuthorKind:   "human",
				Body:         "Hello bot",
			},
		},
	}
	ts := httptest.NewServer(bs.handler())
	defer ts.Close()

	decideCalls := 0
	workCalls := 0
	ad := &scriptedAdapter{
		decideFn: func(ctx context.Context, req DecideRequest) (RunResult, error) {
			decideCalls++
			return RunResult{
				Decision: Decision{
					Actions: []Action{{Ref: "1", Kind: ActReply, Text: "Hi human"}},
				},
			}, nil
		},
		workFn: func(ctx context.Context, req WorkRequest) (RunResult, error) {
			workCalls++
			return RunResult{Text: "Work done"}, nil
		},
	}

	agent := &Agent{
		Cfg: AgentConfig{
			Name:            "compact-bot",
			Client:          "claude-code",
			Mode:            "compact",
			AllowWork:       true,
			Poll:            5 * time.Second,
			FallbackPoll:    60 * time.Second,
			DailyTokenLimit: 50000,
		},

		Client: &Client{
			BaseURL: ts.URL,
			Token:   "dummy-token",
			HTTP:    ts.Client(),
		},
		Adapter: ad,
		Mode:    "compact",
		Log:     slog.Default(),
		Now: func() time.Time {
			return fakeNow
		},
	}
	agent.me = Me{ID: bs.agentID, Handle: bs.handle, State: "active"}

	ctx := context.Background()

	// 1. Over budget before decide: no decide run, no ack, notice posted
	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("compact tick 1: %v", err)
	}
	if decideCalls != 0 {
		t.Fatalf("expected 0 decide calls, got %d", decideCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 0 {
		t.Fatalf("expected 0 acked items, got %v", bs.ackedIDs)
	}
	if len(bs.postedBody) != 1 {
		t.Fatalf("expected 1 notice posted, got %v", bs.postedBody)
	}
	bs.mu.Unlock()

	// 2. Raise limit: decide runs, replies, item acked
	bs.mu.Lock()
	bs.limit = 100000
	bs.remaining = 50000
	bs.mu.Unlock()

	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("compact tick 2: %v", err)
	}
	if decideCalls != 1 {
		t.Fatalf("expected 1 decide call, got %d", decideCalls)
	}
	bs.mu.Lock()
	if len(bs.ackedIDs) != 1 || bs.ackedIDs[0] != "item-c1" {
		t.Fatalf("expected item-c1 acked, got %v", bs.ackedIDs)
	}
	if len(bs.postedBody) != 2 || bs.postedBody[1] != "Hi human" {
		t.Fatalf("expected reply posted, got %v", bs.postedBody)
	}
	bs.mu.Unlock()

	// 3. Work phase budget check: decide returns ActWork, but when work starts, budget is exhausted
	bs.mu.Lock()
	bs.inboxItems = []Item{
		{
			ID:           "item-c2",
			Kind:         "message",
			ItemID:       "m2",
			ChannelID:    "c1",
			ThreadRootID: "m2",
			AuthorKind:   "human",
			Body:         "Please do complex work",
		},
	}
	bs.mu.Unlock()

	ad.decideFn = func(ctx context.Context, req DecideRequest) (RunResult, error) {
		decideCalls++
		// Simulate budget becoming exhausted right after decide
		bs.mu.Lock()
		bs.remaining = 0
		bs.mu.Unlock()
		return RunResult{
			Decision: Decision{
				Actions: []Action{{Ref: "1", Kind: ActWork, Task: "run complex work"}},
			},
		}, nil
	}

	if err := agent.Tick(ctx); err != nil {
		t.Fatalf("compact tick 3: %v", err)
	}
	if decideCalls != 2 {
		t.Fatalf("expected decide call 2, got %d", decideCalls)
	}
	if workCalls != 0 {
		t.Fatalf("expected 0 work calls because budget was exhausted, got %d", workCalls)
	}
	bs.mu.Lock()
	// item-c2 should NOT be acked!
	if len(bs.ackedIDs) != 1 { // still only item-c1 from before
		t.Fatalf("item-c2 should not be acked, got %v", bs.ackedIDs)
	}
	bs.mu.Unlock()
}
