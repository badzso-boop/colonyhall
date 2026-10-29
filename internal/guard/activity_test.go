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

func TestGuardActivityDecideRun(t *testing.T) {
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

	decideStarted := make(chan struct{})
	decideRelease := make(chan struct{})

	ad := &directTestAdapter{
		decideImpl: func(_ context.Context, _ DecideRequest) (RunResult, error) {
			close(decideStarted)
			<-decideRelease
			return RunResult{
				Decision: Decision{
					Actions: []Action{{Ref: "1", Kind: ActReply, Text: "hi"}},
				},
			}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", HaltCheck: time.Hour, DecideTimeout: time.Minute},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{{ID: "it-3", AuthorKind: "human", ItemID: "m3", Channel: "general", ChannelID: "ch-1", ThreadRootID: "t-1", Body: "decide"}}

	done := make(chan error, 1)
	go func() {
		done <- agent.handle(context.Background(), items)
	}()

	<-decideStarted
	// Before decide returns: "working" must be posted
	mu.Lock()
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity report while deciding, got %d: %v", len(activities), activities)
	}
	if !containsSubstring(activities[0], `"working"`) || !containsSubstring(activities[0], `"ch-1"`) || !containsSubstring(activities[0], `"t-1"`) {
		t.Errorf("expected working for ch-1:t-1, got %s", activities[0])
	}
	mu.Unlock()

	close(decideRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(activities) != 2 {
		t.Fatalf("expected 2 activity reports (working, idle), got %d: %v", len(activities), activities)
	}
	if !containsSubstring(activities[1], `"idle"`) || !containsSubstring(activities[1], `"ch-1"`) || !containsSubstring(activities[1], `"t-1"`) {
		t.Errorf("expected idle for ch-1:t-1, got %s", activities[1])
	}
}

func TestGuardActivityDecideBatchTwoThreads(t *testing.T) {
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

	decideStarted := make(chan struct{})
	decideRelease := make(chan struct{})

	ad := &directTestAdapter{
		decideImpl: func(_ context.Context, _ DecideRequest) (RunResult, error) {
			close(decideStarted)
			<-decideRelease
			return RunResult{
				Decision: Decision{
					Actions: []Action{
						{Ref: "1", Kind: ActReply, Text: "hi 1"},
						{Ref: "2", Kind: ActReply, Text: "hi 2"},
					},
				},
			}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", HaltCheck: time.Hour, DecideTimeout: time.Minute},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{
		{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "ch1", ChannelID: "c1", ThreadRootID: "t1", Body: "msg 1"},
		{ID: "it-2", AuthorKind: "human", ItemID: "m2", Channel: "ch2", ChannelID: "c2", ThreadRootID: "t2", Body: "msg 2"},
	}

	done := make(chan error, 1)
	go func() {
		done <- agent.handle(context.Background(), items)
	}()

	<-decideStarted
	// Both threads must be reported as working
	mu.Lock()
	if len(activities) != 2 {
		t.Fatalf("expected 2 working activity reports, got %d: %v", len(activities), activities)
	}
	mu.Unlock()

	close(decideRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(activities) != 4 {
		t.Fatalf("expected 4 activity reports (2 working, 2 idle), got %d: %v", len(activities), activities)
	}
	// Check that both c1:t1 and c2:t2 got idle
	hasIdle1 := false
	hasIdle2 := false
	for _, a := range activities[2:] {
		if containsSubstring(a, `"idle"`) && containsSubstring(a, `"c1"`) && containsSubstring(a, `"t1"`) {
			hasIdle1 = true
		}
		if containsSubstring(a, `"idle"`) && containsSubstring(a, `"c2"`) && containsSubstring(a, `"t2"`) {
			hasIdle2 = true
		}
	}
	if !hasIdle1 || !hasIdle2 {
		t.Errorf("expected idle for both threads, got: %v", activities[2:])
	}
}

func TestGuardActivityDecideFollowedByWorkNoFlicker(t *testing.T) {
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

	workStarted := make(chan struct{})
	workRelease := make(chan struct{})

	ad := &directTestAdapter{
		decideImpl: func(_ context.Context, _ DecideRequest) (RunResult, error) {
			return RunResult{
				Decision: Decision{
					Actions: []Action{{Ref: "1", Kind: ActWork, Task: "run tests"}},
				},
			}, nil
		},
		workImpl: func(_ context.Context, _ WorkRequest) (RunResult, error) {
			close(workStarted)
			<-workRelease
			return RunResult{Text: "tests passed"}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", AllowWork: true, Dir: "/tmp", HaltCheck: time.Hour, DecideTimeout: time.Minute, WorkTimeout: time.Minute},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{{ID: "it-w", AuthorKind: "human", ItemID: "mw", Channel: "general", ChannelID: "ch-1", ThreadRootID: "t-1", Body: "do work"}}

	done := make(chan error, 1)
	go func() {
		done <- agent.handle(context.Background(), items)
	}()

	<-workStarted
	// While work is executing: verify that NO idle has been sent yet!
	mu.Lock()
	for _, a := range activities {
		if containsSubstring(a, `"idle"`) {
			mu.Unlock()
			t.Fatalf("idle was sent before work finished (flicker!): %v", activities)
		}
	}
	if len(activities) != 1 {
		t.Fatalf("expected exactly 1 working report so far, got %d: %v", len(activities), activities)
	}
	mu.Unlock()

	close(workRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(activities) != 2 {
		t.Fatalf("expected exactly 2 activity reports (1 working, 1 idle), got %d: %v", len(activities), activities)
	}
	if !containsSubstring(activities[0], `"working"`) {
		t.Errorf("first must be working, got %s", activities[0])
	}
	if !containsSubstring(activities[1], `"idle"`) {
		t.Errorf("second must be idle, got %s", activities[1])
	}
}

func TestGuardActivityDMTopLevelThread(t *testing.T) {
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

	decideStarted := make(chan struct{})
	decideRelease := make(chan struct{})

	ad := &directTestAdapter{
		decideImpl: func(_ context.Context, _ DecideRequest) (RunResult, error) {
			close(decideStarted)
			<-decideRelease
			return RunResult{
				Decision: Decision{
					Actions: []Action{{Ref: "1", Kind: ActReply, Text: "hi dm"}},
				},
			}, nil
		},
	}

	agent := &Agent{
		Cfg:      AgentConfig{Client: "claude-code", HaltCheck: time.Hour, DecideTimeout: time.Minute},
		Client:   &Client{BaseURL: ts.URL, Token: "token", HTTP: ts.Client()},
		Adapter:  ad,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	// DM top-level item: DM is true, ThreadRootID is empty, ChannelID is "dm-channel-1"
	items := []Item{{ID: "it-dm", AuthorKind: "human", ItemID: "mdm", Channel: "dm-user", ChannelID: "dm-channel-1", DM: true, ThreadRootID: "", Body: "hi dm"}}

	done := make(chan error, 1)
	go func() {
		done <- agent.handle(context.Background(), items)
	}()

	<-decideStarted
	mu.Lock()
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity report, got %d: %v", len(activities), activities)
	}
	// thread_root_id must match ChannelID ("dm-channel-1"), as direct runs do
	if !containsSubstring(activities[0], `"thread_root_id":"dm-channel-1"`) {
		t.Errorf("expected thread_root_id to match ChannelID for top-level DM, got %s", activities[0])
	}
	mu.Unlock()

	close(decideRelease)
	if err := <-done; err != nil {
		t.Fatal(err)
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
