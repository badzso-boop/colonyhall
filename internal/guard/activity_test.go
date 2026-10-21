package guard

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestGuardActivityDirectRun(t *testing.T) {
	var mu sync.Mutex
	var activities []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/activity" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			activities = append(activities, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	ad := &directTestAdapter{
		impl: func(_ context.Context, _ DirectRequest) (RunResult, error) {
			return RunResult{Text: "done"}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "antigravity", HaltCheck: time.Hour},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: t.TempDir(),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "general", ChannelID: "ch-1", Body: "hi"}}
	if err := agent.handleDirect(context.Background(), items); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(activities) < 2 {
		t.Fatalf("expected at least 2 activity reports (working, idle), got %d: %v", len(activities), activities)
	}
	if !containsSubstring(activities[0], `"working"`) {
		t.Errorf("first activity should be working, got %s", activities[0])
	}
	if !containsSubstring(activities[len(activities)-1], `"idle"`) {
		t.Errorf("last activity should be idle, got %s", activities[len(activities)-1])
	}
}

func TestGuardActivityWorkRun(t *testing.T) {
	var mu sync.Mutex
	var activities []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/activity" && r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			activities = append(activities, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	ad := &directTestAdapter{
		workImpl: func(_ context.Context, _ WorkRequest) (RunResult, error) {
			return RunResult{Text: "work done"}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", HaltCheck: time.Hour, WorkTimeout: time.Minute},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	it := PromptItem{
		Item: Item{ID: "it-2", AuthorKind: "human", ItemID: "m2", Channel: "general", ChannelID: "ch-1", Body: "do work"},
	}
	if err := agent.work(context.Background(), it, "do work"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(activities) < 2 {
		t.Fatalf("expected at least 2 activity reports, got %d: %v", len(activities), activities)
	}
	if !containsSubstring(activities[0], `"working"`) {
		t.Errorf("first activity should be working, got %s", activities[0])
	}
	if !containsSubstring(activities[len(activities)-1], `"idle"`) {
		t.Errorf("last activity should be idle, got %s", activities[len(activities)-1])
	}
}

func TestGuardActivityNotForDecisionRun(t *testing.T) {
	var mu sync.Mutex
	var activities []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/activity" {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			activities = append(activities, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	ad := &directTestAdapter{
		decideImpl: func(_ context.Context, _ DecideRequest) (RunResult, error) {
			return RunResult{Text: `{"actions":[{"ref":"1","kind":"reply","text":"hi"}]}`}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", HaltCheck: time.Hour},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{{ID: "it-3", AuthorKind: "human", ItemID: "m3", Channel: "general", ChannelID: "ch-1", Body: "decide"}}
	_ = agent.handle(context.Background(), items)

	mu.Lock()
	defer mu.Unlock()
	if len(activities) != 0 {
		t.Fatalf("decision run must not report activity, got: %v", activities)
	}
}

func TestGuardActivityKeepsGoingOnFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/activity" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail":"server error"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	ad := &directTestAdapter{
		impl: func(_ context.Context, _ DirectRequest) (RunResult, error) {
			return RunResult{Text: "ran successfully"}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "antigravity", HaltCheck: time.Hour},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: t.TempDir(),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{{ID: "it-4", AuthorKind: "human", ItemID: "m4", Channel: "general", ChannelID: "ch-1", Body: "test error"}}
	// Run must succeed despite activity endpoint failing
	if err := agent.handleDirect(context.Background(), items); err != nil {
		t.Fatalf("run failed when activity failed: %v", err)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
