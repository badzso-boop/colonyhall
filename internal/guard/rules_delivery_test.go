package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClaudeCode_AppendSystemPromptFlag(t *testing.T) {
	ctx := context.Background()

	// 1. Direct run with flag supported in claude --help
	{
		var f fakeRunner
		f.stdout = `{"result":"done","session_id":"s1"}`
		c := &ClaudeCode{Run: f.run}
		_, err := c.Direct(ctx, DirectRequest{
			Prompt:    "hello",
			Rules:     "be polite",
			SessionID: "s1",
		})
		if err != nil {
			t.Fatal(err)
		}
		val, ok := flagValue(f.args, "--append-system-prompt")
		if !ok || val != "be polite" {
			t.Fatalf("expected --append-system-prompt 'be polite', got val=%q ok=%v in args: %v", val, ok, f.args)
		}
	}

	// 2. Direct run with flag NOT supported in claude --help
	{
		var f fakeRunner
		f.noHelp = true
		f.stdout = `{"result":"done","session_id":"s1"}`
		c := &ClaudeCode{Run: f.run}
		_, err := c.Direct(ctx, DirectRequest{
			Prompt:    "hello",
			Rules:     "be polite",
			SessionID: "s1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(f.args, "--append-system-prompt") {
			t.Fatalf("did not expect --append-system-prompt when flag is not supported: %v", f.args)
		}
	}

	// 3. Work run with flag supported
	{
		var f fakeRunner
		f.stdout = `{"result":"done"}`
		c := &ClaudeCode{Run: f.run}
		_, err := c.Work(ctx, WorkRequest{
			Prompt: "work task",
			Rules:  "be safe",
		})
		if err != nil {
			t.Fatal(err)
		}
		val, ok := flagValue(f.args, "--append-system-prompt")
		if !ok || val != "be safe" {
			t.Fatalf("expected --append-system-prompt 'be safe' in work run, got val=%q ok=%v in args: %v", val, ok, f.args)
		}
	}

	// 4. Work run with flag NOT supported
	{
		var f fakeRunner
		f.noHelp = true
		f.stdout = `{"result":"done"}`
		c := &ClaudeCode{Run: f.run}
		_, err := c.Work(ctx, WorkRequest{
			Prompt: "work task",
			Rules:  "be safe",
		})
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(f.args, "--append-system-prompt") {
			t.Fatalf("did not expect --append-system-prompt in work run when flag is not supported: %v", f.args)
		}
	}
}

func TestAntigravity_RulesPrepending(t *testing.T) {
	ctx := context.Background()

	// 1. Direct run with rules
	{
		var f fakeRunner
		f.stdout = `{"status":"SUCCESS","response":"ok","conversation_id":"conv-1"}`
		ag := &Antigravity{Run: f.run}
		_, err := ag.Direct(ctx, DirectRequest{
			Prompt:       "work on file.go",
			Rules:        "don't delete data",
			RulesVersion: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		p, ok := flagValue(f.args, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		wantPrefix := "<colonyhall_rules version=\"3\">\ndon't delete data\n</colonyhall_rules>\n\n"
		if !strings.HasPrefix(p, wantPrefix) {
			t.Fatalf("expected prompt to start with rules block, got:\n%s", p)
		}
	}

	// 2. Direct run without rules (resumed run)
	{
		var f fakeRunner
		f.stdout = `{"status":"SUCCESS","response":"ok","conversation_id":"conv-1"}`
		ag := &Antigravity{Run: f.run}
		_, err := ag.Direct(ctx, DirectRequest{
			Prompt:         "work on file.go",
			ConversationID: "conv-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		p, ok := flagValue(f.args, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if strings.Contains(p, "<colonyhall_rules") {
			t.Fatalf("did not expect rules in resumed prompt, got:\n%s", p)
		}
	}

	// 3. Work run with rules
	{
		var f fakeRunner
		f.stdout = `{"status":"SUCCESS","response":"ok"}`
		ag := &Antigravity{Run: f.run}
		_, err := ag.Work(ctx, WorkRequest{
			Prompt:       "do something",
			Rules:        "shared rules",
			RulesVersion: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		p, ok := flagValue(f.args, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		wantPrefix := "<colonyhall_rules version=\"5\">\nshared rules\n</colonyhall_rules>\n\n"
		if !strings.HasPrefix(p, wantPrefix) {
			t.Fatalf("expected work prompt to start with rules block, got:\n%s", p)
		}
	}
}

func TestRulesDelivery_ClaudeAndAntigravityLifecycle(t *testing.T) {
	var rulesVer atomic.Int32
	rulesVer.Store(1)
	var rulesText atomic.Pointer[string]
	v1 := "Shared Rules v1"
	v2 := "Shared Rules v2"
	rulesText.Store(&v1)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/v1/agent/instructions"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": rulesVer.Load(),
				"text":    *rulesText.Load(),
			})
		case strings.HasSuffix(r.URL.Path, "/inbox/ack"):
			_ = json.NewEncoder(w).Encode(map[string]any{"acked": 1})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-reply"})
		case strings.HasSuffix(r.URL.Path, "/runs"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer ts.Close()

	ctx := context.Background()

	// 1. Claude Code: gets --append-system-prompt on every run
	t.Run("ClaudeCode_EveryRun", func(t *testing.T) {
		var f fakeRunner
		f.stdout = `{"result":"ok","session_id":"sess-1","usage":{"input_tokens":10,"output_tokens":10}}`
		c := &ClaudeCode{Run: f.run}
		stateDir := t.TempDir()
		agent := &Agent{
			Cfg:      AgentConfig{Client: "claude-code", Name: "claudebot", HaltCheck: time.Hour},
			Client:   &Client{BaseURL: ts.URL, Token: "tok-claude", HTTP: ts.Client()},
			Adapter:  c,
			StateDir: stateDir,
			Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
			attempts: map[string]int{},
		}

		// Run 1
		items1 := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "dev", ChannelID: "c1", Body: "task 1"}}
		if err := agent.handleDirect(ctx, items1); err != nil {
			t.Fatal(err)
		}
		val, ok := flagValue(f.args, "--append-system-prompt")
		if !ok || val != "Shared Rules v1" {
			t.Fatalf("run 1: expected --append-system-prompt 'Shared Rules v1', got val=%q ok=%v in args: %v", val, ok, f.args)
		}

		// Run 2 on same thread
		items2 := []Item{{ID: "it-2", AuthorKind: "human", ItemID: "m2", ThreadRootID: "m1", Channel: "dev", ChannelID: "c1", Body: "task 2"}}
		if err := agent.handleDirect(ctx, items2); err != nil {
			t.Fatal(err)
		}
		val, ok = flagValue(f.args, "--append-system-prompt")
		if !ok || val != "Shared Rules v1" {
			t.Fatalf("run 2: expected --append-system-prompt 'Shared Rules v1' on resumed run, got val=%q ok=%v in args: %v", val, ok, f.args)
		}
	})

	// 2. Antigravity: gets rules on first run, version change, and rotation
	t.Run("Antigravity_FirstRun_VersionChange_Rotation", func(t *testing.T) {
		var lastArgs []string
		var f fakeRunner
		f.stdout = `{"status":"SUCCESS","response":"ok","conversation_id":"conv-100","usage":{"input_tokens":50,"output_tokens":50}}`
		ag := &Antigravity{
			Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
				lastArgs = slices.Clone(args)
				return f.run(ctx, dir, name, args, env)
			},
		}
		stateDir := t.TempDir()
		var hostVer atomic.Pointer[string]
		hv1 := "cfg-v1"
		hostVer.Store(&hv1)

		agent := &Agent{
			Cfg: AgentConfig{
				Client:           "antigravity",
				Name:             "agybot",
				HaltCheck:        time.Hour,
				MaxSessionTokens: 200, // rotate after reaching 200 tokens
			},
			Client:            &Client{BaseURL: ts.URL, Token: "tok-agy", HTTP: ts.Client()},
			Adapter:           ag,
			StateDir:          stateDir,
			Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
			attempts:          map[string]int{},
			HostConfigVersion: func() string { return *hostVer.Load() },
		}

		// Run 1: First run on thread m1 -> rules MUST be prepended
		items1 := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "dev", ChannelID: "c1", Body: "first message"}}
		if err := agent.handleDirect(ctx, items1); err != nil {
			t.Fatal(err)
		}
		p, ok := flagValue(lastArgs, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if !strings.HasPrefix(p, "<colonyhall_rules version=\"1\">\nShared Rules v1\n</colonyhall_rules>") {
			t.Fatalf("run 1 (first run): expected rules in prompt, got:\n%s", p)
		}

		// Run 2: Second run on same thread -> rules MUST NOT be prepended
		items2 := []Item{{ID: "it-2", AuthorKind: "human", ItemID: "m2", ThreadRootID: "m1", Channel: "dev", ChannelID: "c1", Body: "second message"}}
		if err := agent.handleDirect(ctx, items2); err != nil {
			t.Fatal(err)
		}
		p, ok = flagValue(lastArgs, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if strings.Contains(p, "<colonyhall_rules") {
			t.Fatalf("run 2 (resumed run): rules should NOT be prepended, got:\n%s", p)
		}

		// Update rules to v2 on server and bump host config version
		rulesVer.Store(2)
		rulesText.Store(&v2)
		hv2 := "cfg-v2"
		hostVer.Store(&hv2)

		// Run 3: Version changed to 2 -> rules v2 MUST be prepended
		items3 := []Item{{ID: "it-3", AuthorKind: "human", ItemID: "m3", ThreadRootID: "m1", Channel: "dev", ChannelID: "c1", Body: "third message"}}
		if err := agent.handleDirect(ctx, items3); err != nil {
			t.Fatal(err)
		}
		p, ok = flagValue(lastArgs, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if !strings.HasPrefix(p, "<colonyhall_rules version=\"2\">\nShared Rules v2\n</colonyhall_rules>") {
			t.Fatalf("run 3 (version changed): expected rules v2 in prompt, got:\n%s", p)
		}

		// Run 4: Same version 2 again -> rules MUST NOT be prepended
		items4 := []Item{{ID: "it-4", AuthorKind: "human", ItemID: "m4", ThreadRootID: "m1", Channel: "dev", ChannelID: "c1", Body: "fourth message"}}
		if err := agent.handleDirect(ctx, items4); err != nil {
			t.Fatal(err)
		}
		p, ok = flagValue(lastArgs, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if strings.Contains(p, "<colonyhall_rules") {
			t.Fatalf("run 4 (resumed v2): rules should NOT be prepended, got:\n%s", p)
		}

		// Simulate session rotation by artificially setting LastRunTokens >= MaxSessionTokens
		stFile := stateDir + "/direct_state.json"
		st, err := LoadDirectState(stFile)
		if err != nil {
			t.Fatal(err)
		}
		st.LastRunTokens["m1"] = 300 // exceeds MaxSessionTokens (200)
		if err := st.Save(stFile); err != nil {
			t.Fatal(err)
		}

		// Run 5: After session rotation -> rules MUST be prepended
		items5 := []Item{{ID: "it-5", AuthorKind: "human", ItemID: "m5", ThreadRootID: "m1", Channel: "dev", ChannelID: "c1", Body: "fifth message"}}
		if err := agent.handleDirect(ctx, items5); err != nil {
			t.Fatal(err)
		}
		p, ok = flagValue(lastArgs, "-p")
		if !ok {
			t.Fatal("missing -p")
		}
		if !strings.HasPrefix(p, "<colonyhall_rules version=\"2\">\nShared Rules v2\n</colonyhall_rules>") {
			t.Fatalf("run 5 (rotation): expected rules v2 in prompt after rotation, got:\n%s", p)
		}
	})
}

func TestAgent_FetchRules_CachingAndErrorHandling(t *testing.T) {
	var reqCount atomic.Int32
	var failServer atomic.Bool
	var rulesVer atomic.Int32
	rulesVer.Store(1)
	rulesText := "Initial Rules"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v1/agent/instructions") {
			reqCount.Add(1)
			if failServer.Load() {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": rulesVer.Load(),
				"text":    rulesText,
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	ctx := context.Background()
	now := time.Now()
	curTime := now
	var hostVer string

	agent := &Agent{
		Client:            &Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()},
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:               func() time.Time { return curTime },
		HostConfigVersion: func() string { return hostVer },
	}

	// 1. Initial fetch
	v, txt := agent.fetchRules(ctx)
	if v != 1 || txt != "Initial Rules" {
		t.Fatalf("initial fetch: got v=%d txt=%q", v, txt)
	}
	if reqCount.Load() != 1 {
		t.Fatalf("expected 1 request, got %d", reqCount.Load())
	}

	// 2. Fetch within 10 minutes -> cache hit, no new request
	curTime = curTime.Add(5 * time.Minute)
	v, txt = agent.fetchRules(ctx)
	if v != 1 || txt != "Initial Rules" {
		t.Fatalf("cached fetch: got v=%d txt=%q", v, txt)
	}
	if reqCount.Load() != 1 {
		t.Fatalf("expected still 1 request, got %d", reqCount.Load())
	}

	// 3. Advance time >= 10 minutes -> refetch
	curTime = curTime.Add(6 * time.Minute) // 11 mins total
	rulesVer.Store(2)
	rulesText = "Updated Rules"
	v, txt = agent.fetchRules(ctx)
	if v != 2 || txt != "Updated Rules" {
		t.Fatalf("refetch after 10m: got v=%d txt=%q", v, txt)
	}
	if reqCount.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", reqCount.Load())
	}

	// 4. Host config version changes -> immediate refetch
	hostVer = "new-host-v2"
	rulesVer.Store(3)
	rulesText = "Rules v3"
	v, txt = agent.fetchRules(ctx)
	if v != 3 || txt != "Rules v3" {
		t.Fatalf("refetch on host config change: got v=%d txt=%q", v, txt)
	}
	if reqCount.Load() != 3 {
		t.Fatalf("expected 3 requests, got %d", reqCount.Load())
	}

	// 5. Server error when cached rules exist -> keep last good text, never block run
	curTime = curTime.Add(15 * time.Minute)
	failServer.Store(true)
	v, txt = agent.fetchRules(ctx)
	if v != 3 || txt != "Rules v3" {
		t.Fatalf("expected cached rules v3 on server error, got v=%d txt=%q", v, txt)
	}

	// 6. Fresh agent with server error from the start -> returns 0, "", never blocks
	freshAgent := &Agent{
		Client: &Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return curTime },
	}
	v, txt = freshAgent.fetchRules(ctx)
	if v != 0 || txt != "" {
		t.Fatalf("expected 0, '' on initial server error, got v=%d txt=%q", v, txt)
	}
}
