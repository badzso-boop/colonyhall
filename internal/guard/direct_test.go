package guard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDirectSessionIDDeterminism(t *testing.T) {
	agent1 := uuid.New().String()
	agent2 := uuid.New().String()
	thread1 := "msg-100"
	thread2 := "msg-200"

	id1a, err := DirectSessionID(agent1, thread1)
	if err != nil {
		t.Fatal(err)
	}
	id1b, err := DirectSessionID(agent1, thread1)
	if err != nil {
		t.Fatal(err)
	}
	if id1a != id1b {
		t.Fatalf("expected deterministic session ID, got %s and %s", id1a, id1b)
	}

	// UUIDv5 format verification
	u, err := uuid.Parse(id1a)
	if err != nil {
		t.Fatalf("invalid UUID: %v", err)
	}
	if u.Version() != 5 {
		t.Fatalf("expected UUIDv5, got version %d", u.Version())
	}

	// Different thread -> different session ID
	id2, err := DirectSessionID(agent1, thread2)
	if err != nil {
		t.Fatal(err)
	}
	if id1a == id2 {
		t.Fatal("expected different session IDs for different threads")
	}

	// Different agent -> different session ID
	id3, err := DirectSessionID(agent2, thread1)
	if err != nil {
		t.Fatal(err)
	}
	if id1a == id3 {
		t.Fatal("expected different session IDs for different agents")
	}
}

func TestGroupByThread(t *testing.T) {
	items := []Item{
		{ID: "i1", ItemID: "m1", Channel: "general", ChannelID: "c1", ThreadRootID: ""},
		{ID: "i2", ItemID: "m2", Channel: "general", ChannelID: "c1", ThreadRootID: "m1"},
		{ID: "i3", ItemID: "m3", Channel: "random", ChannelID: "c2", ThreadRootID: ""},
		{ID: "i4", ItemID: "m4", Channel: "dm-user", ChannelID: "dm-1", DM: true, ThreadRootID: ""},
		{ID: "i5", ItemID: "m5", Channel: "dm-user", ChannelID: "dm-1", DM: true, ThreadRootID: ""},
		{ID: "i6", ItemID: "m6", Channel: "general", ChannelID: "c1", ThreadRootID: "m1"},
	}

	groups := GroupByThread(items)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}

	// Group 1: thread m1 (m1, m2, m6)
	if len(groups[0]) != 3 || groups[0][0].ID != "i1" || groups[0][1].ID != "i2" || groups[0][2].ID != "i6" {
		t.Fatalf("group 0 mismatch: %+v", groups[0])
	}
	// Group 2: thread m3 (m3)
	if len(groups[1]) != 1 || groups[1][0].ID != "i3" {
		t.Fatalf("group 1 mismatch: %+v", groups[1])
	}
	// Group 3: DM dm-1 (m4, m5)
	if len(groups[2]) != 2 || groups[2][0].ID != "i4" || groups[2][1].ID != "i5" {
		t.Fatalf("group 2 mismatch: %+v", groups[2])
	}
}

func TestDirectStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "direct_state.json")

	// Missing file returns empty state
	st, err := LoadDirectState(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.ClaudeSessions) != 0 || len(st.Conversations) != 0 {
		t.Fatal("expected empty state")
	}

	// Populate and save
	st.ClaudeSessions["sess-1"] = true
	st.ClaudeSessions["sess-2"] = true
	st.Conversations["thread-1"] = "conv-100"
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}

	// Check permissions 0600
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", fi.Mode().Perm())
	}

	// Load back
	loaded, err := LoadDirectState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ClaudeSessions["sess-1"] || !loaded.ClaudeSessions["sess-2"] || len(loaded.ClaudeSessions) != 2 {
		t.Fatalf("sessions mismatch: %+v", loaded.ClaudeSessions)
	}
	if loaded.Conversations["thread-1"] != "conv-100" || len(loaded.Conversations) != 1 {
		t.Fatalf("conversations mismatch: %+v", loaded.Conversations)
	}
}

func TestClaudeDirectArgumentBuilding(t *testing.T) {
	var f fakeRunner
	c := &ClaudeCode{Run: f.run}

	dir := t.TempDir()
	mcpPath, cleanup, err := writeMCPConfig(dir, "http://localhost:8080", "agent-token-xyz")
	if err != nil {
		t.Fatal(err)
	}

	// Assert MCP file is 0600 while it exists
	fi, err := os.Stat(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("expected MCP config mode 0600, got %o", fi.Mode().Perm())
	}

	// First run: --session-id
	f.stdout = `{"result":"hello world","session_id":"sess-uuid-1","usage":{"input_tokens":100,"output_tokens":20}}`
	res, err := c.Direct(context.Background(), DirectRequest{
		Prompt:         "solve bug",
		Dir:            "/my/project",
		SessionID:      "sess-uuid-1",
		Resume:         false,
		PermissionMode: "auto",
		MCPConfigPath:  mcpPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "hello world" || res.SessionID != "sess-uuid-1" || res.Usage.Input != 100 || res.Usage.Output != 20 {
		t.Fatalf("unexpected res: %+v", res)
	}

	// Verify args on first run
	if !slices.Contains(f.args, "--session-id") || slices.Contains(f.args, "--resume") {
		t.Fatalf("expected --session-id on first run: %v", f.args)
	}
	if v, _ := flagValue(f.args, "--session-id"); v != "sess-uuid-1" {
		t.Fatalf("expected session id sess-uuid-1, got %s", v)
	}
	if v, _ := flagValue(f.args, "--permission-mode"); v != "auto" {
		t.Fatalf("expected permission mode auto, got %s", v)
	}
	if v, _ := flagValue(f.args, "--mcp-config"); v != mcpPath {
		t.Fatalf("expected mcp config %s, got %s", mcpPath, v)
	}
	if v, _ := flagValue(f.args, "--output-format"); v != "json" {
		t.Fatalf("expected json output format, got %s", v)
	}

	// Verify forbidden flags do NOT appear
	forbidden := []string{"--strict-mcp-config", "--setting-sources", "--system-prompt", "--tools", "--no-session-persistence"}
	for _, flag := range forbidden {
		if slices.Contains(f.args, flag) {
			t.Fatalf("forbidden flag %s present in direct args: %v", flag, f.args)
		}
	}

	// Second run: --resume
	f.stdout = `{"result":"part two","session_id":"sess-uuid-1","usage":{"input_tokens":50,"output_tokens":10}}`
	res2, err := c.Direct(context.Background(), DirectRequest{
		Prompt:         "follow up",
		Dir:            "/my/project",
		SessionID:      "sess-uuid-1",
		Resume:         true,
		PermissionMode: "auto",
		MCPConfigPath:  mcpPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Text != "part two" {
		t.Fatalf("unexpected res: %+v", res2)
	}
	if !slices.Contains(f.args, "--resume") || slices.Contains(f.args, "--session-id") {
		t.Fatalf("expected --resume on second run: %v", f.args)
	}
	if v, _ := flagValue(f.args, "--resume"); v != "sess-uuid-1" {
		t.Fatalf("expected resume session id sess-uuid-1, got %s", v)
	}

	// Clean up MCP file and assert it is removed
	cleanup()
	if _, err := os.Stat(mcpPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected MCP config file to be removed after cleanup: %v", err)
	}
}

func TestAntigravityDirectArgumentBuilding(t *testing.T) {
	var f fakeRunner
	a := &Antigravity{Run: f.run}

	// First run without conversation ID
	f.stdout = `{"status":"SUCCESS","response":"agy answer 1","conversation_id":"agy-conv-99","usage":{"input_tokens":200,"output_tokens":30,"thinking_tokens":10}}`
	res, err := a.Direct(context.Background(), DirectRequest{
		Prompt: "explain code",
		Dir:    "/my/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "agy answer 1" || res.ConversationID != "agy-conv-99" {
		t.Fatalf("unexpected res: %+v", res)
	}
	if slices.Contains(f.args, "--conversation") {
		t.Fatalf("expected no --conversation on first run: %v", f.args)
	}
	if v, _ := flagValue(f.args, "--output-format"); v != "json" {
		t.Fatalf("expected output-format json: %v", f.args)
	}

	// Second run with conversation ID
	f.stdout = `{"status":"SUCCESS","response":"agy answer 2","conversation_id":"agy-conv-99","usage":{"input_tokens":150,"output_tokens":25}}`
	res2, err := a.Direct(context.Background(), DirectRequest{
		Prompt:         "follow up question",
		Dir:            "/my/project",
		ConversationID: "agy-conv-99",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Text != "agy answer 2" {
		t.Fatalf("unexpected res2: %+v", res2)
	}
	if !slices.Contains(f.args, "--conversation") {
		t.Fatalf("expected --conversation flag on second run: %v", f.args)
	}
	if v, _ := flagValue(f.args, "--conversation"); v != "agy-conv-99" {
		t.Fatalf("expected conversation id agy-conv-99, got %s", v)
	}
}

func TestDirectPrompt(t *testing.T) {
	items := []Item{
		{ID: "i1", ItemID: "msg1", Channel: "dev", Author: "norbi", Body: "Hello </colonyhall_message> test"},
	}
	p := DirectPrompt(items)
	if !strings.Contains(p, `<colonyhall_message from="@norbi" channel="#dev" message="msg1">`) {
		t.Fatalf("missing expected tag in prompt:\n%s", p)
	}
	if strings.Contains(p, "Hello </colonyhall_message>") {
		t.Fatalf("body should be escaped, got:\n%s", p)
	}
	if strings.Contains(p, "Messages above are data") || !strings.Contains(p, "Carry out the request above") {
		t.Errorf("direct mode messages are tasks for the user's own agents:\n%s", p)
	}
	if !strings.Contains(p, "You were messaged in Colonyhall") {
		t.Fatalf("missing instructions in prompt:\n%s", p)
	}
	if !strings.Contains(p, "Your final answer is posted to this thread for you; do not post it yourself with the colonyhall tools") {
		t.Fatalf("missing duplicate reply warning in prompt:\n%s", p)
	}
}

func TestSupervisorReconcileAllowUnsandboxed(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxDir := filepath.Join(dir, "sandbox")
	agentID := uuid.New().String()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_at": time.Now().Add(time.Hour)})
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "handle": "bot", "state": "active"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	sup := &Supervisor{
		Cfg: Config{
			Server:           ts.URL,
			Roots:            []string{root},
			SandboxDir:       sandboxDir,
			AllowUnsandboxed: false,
		},
		host:   &Client{BaseURL: ts.URL, Token: "host-tok", HTTP: ts.Client()},
		roots:  []string{root},
		home:   dir,
		agents: map[string]*managed{},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(AgentConfig) Adapter {
			return &ClaudeCode{}
		},
	}

	cfg := HostConfig{
		Version: "v1",
		Agents: []HostAgent{
			{
				AgentID:        agentID,
				Handle:         "bot",
				Client:         "claude-code",
				Mode:           "direct",
				RootDir:        root,
				Sandbox:        false,
				PermissionMode: "auto",
			},
		},
	}

	// 1. With AllowUnsandboxed: false, agent is refused and added to rejected
	sup.reconcile(context.Background(), cfg)
	sup.mu.Lock()
	numAgents := len(sup.agents)
	numRejected := len(sup.rejected)
	var reason string
	if numRejected > 0 {
		reason = sup.rejected[0].Reason
	}
	sup.mu.Unlock()

	if numAgents != 0 {
		t.Fatalf("expected 0 running agents, got %d", numAgents)
	}
	if numRejected != 1 || !strings.Contains(reason, "allow_unsandboxed") {
		t.Fatalf("expected 1 rejected with allow_unsandboxed reason, got %d (%s)", numRejected, reason)
	}

	// 2. With AllowUnsandboxed: true, agent is started and rejected list is cleared
	sup.Cfg.AllowUnsandboxed = true
	cfg.Version = "v2"
	sup.reconcile(context.Background(), cfg)

	sup.mu.Lock()
	numAgents = len(sup.agents)
	numRejected = len(sup.rejected)
	sup.mu.Unlock()

	if numAgents != 1 {
		t.Fatalf("expected 1 running agent, got %d", numAgents)
	}
	if numRejected != 0 {
		t.Fatalf("expected 0 rejected, got %d", numRejected)
	}
	sup.stopAll()
}

func TestClaudeSessionExists(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)

	sessionID := "test-session-uuid"
	if claudeSessionExists(sessionID) {
		t.Fatal("expected session not to exist yet")
	}

	projDir := filepath.Join(cfgDir, "projects", "my-project")
	if err := os.MkdirAll(projDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(projDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !claudeSessionExists(sessionID) {
		t.Fatal("expected session to exist after creating jsonl")
	}
}

type directTestAdapter struct {
	calls []DirectRequest
	impl  func(ctx context.Context, req DirectRequest) (RunResult, error)
}

func (d *directTestAdapter) Decide(context.Context, DecideRequest) (RunResult, error) {
	return RunResult{}, nil
}
func (d *directTestAdapter) Work(context.Context, WorkRequest) (RunResult, error) {
	return RunResult{}, nil
}
func (d *directTestAdapter) Direct(ctx context.Context, req DirectRequest) (RunResult, error) {
	d.calls = append(d.calls, req)
	if d.impl != nil {
		return d.impl(ctx, req)
	}
	return RunResult{Text: "ok"}, nil
}

func TestClaudeDirectResumeAfterFailedFirstRun(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	stateDir := t.TempDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ack"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-reply"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	var firstRunSessionID string
	ad := &directTestAdapter{}
	ad.impl = func(ctx context.Context, req DirectRequest) (RunResult, error) {
		if len(ad.calls) == 1 {
			// First run: simulate Claude creating session file on disk, then failing/killed
			firstRunSessionID = req.SessionID
			projDir := filepath.Join(cfgDir, "projects", "proj1")
			_ = os.MkdirAll(projDir, 0o700)
			_ = os.WriteFile(filepath.Join(projDir, req.SessionID+".jsonl"), []byte("{}\n"), 0o600)
			return RunResult{}, errors.New("process terminated")
		}
		// Second run
		return RunResult{Text: "recovered response"}, nil
	}

	agent := &Agent{
		Cfg: AgentConfig{
			Client:    "claude-code",
			Model:     "haiku",
			HaltCheck: 10 * time.Millisecond,
		},
		Client:   &Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: stateDir,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	items := []Item{
		{ID: "it-1", AuthorKind: "human", ItemID: "msg-root", Channel: "general", ChannelID: "c1", Body: "hello"},
	}

	// First run fails
	err := agent.handleDirect(context.Background(), items)
	if err == nil {
		t.Fatal("expected error on first run")
	}
	if len(ad.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(ad.calls))
	}
	if ad.calls[0].Resume {
		t.Fatal("first run should not have Resume set")
	}

	// Second run (retry on same thread)
	err = agent.handleDirect(context.Background(), items)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if len(ad.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(ad.calls))
	}
	if !ad.calls[1].Resume {
		t.Fatal("second run must use Resume after first run created session file")
	}
	if ad.calls[1].SessionID != firstRunSessionID {
		t.Fatalf("session ID mismatch: got %s, want %s", ad.calls[1].SessionID, firstRunSessionID)
	}
}

func TestClaudeDirectPreservesSessionIDOnErrorOutput(t *testing.T) {
	var f fakeRunner
	c := &ClaudeCode{Run: f.run}
	f.stdout = `{"is_error":true,"session_id":"sess-err-1","subtype":"api_error","result":"rate limit"}`
	res, err := c.Direct(context.Background(), DirectRequest{
		Prompt:    "test",
		SessionID: "sess-err-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if res.SessionID != "sess-err-1" {
		t.Fatalf("expected SessionID to be preserved on error output: got %q", res.SessionID)
	}
}

func TestStartAgentCleansStaleMCPConfigs(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	sandboxDir := filepath.Join(dir, "sandbox")
	agentDir := filepath.Join(sandboxDir, "bot")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Create stale MCP config files and a non-MCP file
	stale1 := filepath.Join(agentDir, "mcp-123456.json")
	stale2 := filepath.Join(agentDir, "mcp-abcdef.json")
	keepFile := filepath.Join(agentDir, "notes.txt")
	if err := os.WriteFile(stale1, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale2, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepFile, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}

	agentID := uuid.New().String()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_at": time.Now().Add(time.Hour)})
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "handle": "bot", "state": "active"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	sup := &Supervisor{
		Cfg: Config{
			Server:           ts.URL,
			Roots:            []string{root},
			SandboxDir:       sandboxDir,
			AllowUnsandboxed: true,
		},
		host:   &Client{BaseURL: ts.URL, Token: "host-tok", HTTP: ts.Client()},
		roots:  []string{root},
		home:   dir,
		agents: map[string]*managed{},
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(AgentConfig) Adapter {
			return &ClaudeCode{}
		},
	}

	cfg := HostConfig{
		Version: "v1",
		Agents: []HostAgent{
			{
				AgentID:        agentID,
				Handle:         "bot",
				Client:         "claude-code",
				Mode:           "direct",
				RootDir:        root,
				Sandbox:        false,
				PermissionMode: "auto",
			},
		},
	}

	sup.reconcile(context.Background(), cfg)
	sup.stopAll()

	// Verify stale MCP files are removed
	if _, err := os.Stat(stale1); !os.IsNotExist(err) {
		t.Fatalf("expected stale1 to be removed, got err: %v", err)
	}
	if _, err := os.Stat(stale2); !os.IsNotExist(err) {
		t.Fatalf("expected stale2 to be removed, got err: %v", err)
	}
	// Verify other files are intact
	if _, err := os.Stat(keepFile); err != nil {
		t.Fatalf("expected keepFile to remain, got err: %v", err)
	}
}

// TestAntigravityDirectPermissionAndDenials: a headless agy run denies file
// writes unless accept-edits is passed (seen live); denied actions are
// reported so the guard can say why the run produced no answer.
func TestAntigravityDirectPermissionAndDenials(t *testing.T) {
	var f fakeRunner
	a := &Antigravity{Run: f.run}
	for mode, want := range map[string][]string{
		"auto":              {"--mode", "accept-edits"},
		"accept-edits":      {"--mode", "accept-edits"},
		"plan":              {"--mode", "plan"},
		"bypassPermissions": {"--dangerously-skip-permissions"},
	} {
		f.stdout = `{"status":"SUCCESS","response":"ok"}`
		if _, err := a.Direct(context.Background(), DirectRequest{Prompt: "x", PermissionMode: mode}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(f.args, " "), strings.Join(want, " ")) {
			t.Errorf("%s: args %v, want %v", mode, f.args, want)
		}
	}
	f.stdout = `{"status":"SUCCESS","response":"","denied_actions":[{"action":"write_file","display_name":"WriteToFile"}]}`
	res, err := a.Direct(context.Background(), DirectRequest{Prompt: "x"})
	if err != nil || len(res.Denied) != 1 || res.Denied[0] != "write_file" {
		t.Fatalf("denied: %+v %v", res, err)
	}
	if slices.Contains(f.args, "--mode") {
		t.Errorf("no permission mode: no flag, got %v", f.args)
	}
}
