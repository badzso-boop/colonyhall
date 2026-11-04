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
)

const docsOnlyHelp = "Usage: claude [options]\n  --append-system-prompt <prompt>\n  --permission-mode <mode> (choices: \"acceptEdits\", \"auto\", \"bypassPermissions\", \"manual\", \"dontAsk\", \"plan\")\n" +
	"  --permission-prompts <target>\n  --restricted\n  --settings <file-or-json>\n  --strict-mcp-config\n"

func countFlag(args []string, flag string) int {
	n := 0
	for _, a := range args {
		if a == flag {
			n++
		}
	}
	return n
}

// TestClaudeDirectDocsOnlyArgs: a docs-only planner starts with no command
// tool, writes allowed only for Markdown in its root, refusals instead of
// prompts, and neither its own permission mode nor a permission prompt
// tool, even when the request carries them.
func TestClaudeDirectDocsOnlyArgs(t *testing.T) {
	f := &fakeRunner{stdout: `{"type":"result","result":"ok","session_id":"s1"}`, helpStdout: docsOnlyHelp}
	c := &ClaudeCode{Run: f.run}
	dir := t.TempDir()
	_, err := c.Direct(context.Background(), DirectRequest{
		Prompt: "plan it", Dir: dir, SessionID: "s1", MCPConfigPath: "/state/mcp.json",
		PermissionMode: "bypassPermissions", PermissionPromptTool: "mcp__colonyhall__approve", DocsOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	args := f.args
	if slices.Contains(args, "--permission-prompt-tool") || strings.Contains(strings.Join(args, " "), "approve") {
		t.Fatalf("a docs-only run must never get a permission prompt tool: %v", args)
	}
	if countFlag(args, "--permission-mode") != 1 {
		t.Fatalf("exactly one permission mode: %v", args)
	}
	for flag, want := range map[string]string{
		"--permission-mode":    "dontAsk",
		"--permission-prompts": "none",
		"--tools":              "Read,Write,Edit,Glob,Grep",
		"--allowedTools":       "mcp__colonyhall",
		"--mcp-config":         "/state/mcp.json",
	} {
		if v, ok := flagValue(args, flag); !ok || v != want {
			t.Errorf("%s = %q, want %q (%v)", flag, v, want, args)
		}
	}
	for _, flag := range []string{"--restricted", "--strict-mcp-config"} {
		if !slices.Contains(args, flag) {
			t.Errorf("missing %s: %v", flag, args)
		}
	}
	denied, _ := flagValue(args, "--disallowedTools")
	for _, tool := range []string{"Bash", "NotebookEdit", "Agent"} {
		if !slices.Contains(strings.Split(denied, ","), tool) {
			t.Errorf("--disallowedTools lacks %s: %q", tool, denied)
		}
	}
	tools, _ := flagValue(args, "--tools")
	if strings.Contains(tools, "Bash") || strings.Contains(tools, "Notebook") {
		t.Errorf("command tools offered: %q", tools)
	}

	raw, ok := flagValue(args, "--settings")
	if !ok {
		t.Fatalf("no --settings: %v", args)
	}
	var s struct {
		Permissions struct {
			DefaultMode string   `json:"defaultMode"`
			Allow       []string `json:"allow"`
			Deny        []string `json:"deny"`
			Ask         []string `json:"ask"`
		} `json:"permissions"`
		DisableAllHooks bool `json:"disableAllHooks"`
	}
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("settings are not JSON: %v %q", err, raw)
	}
	if s.Permissions.DefaultMode != "dontAsk" || !s.DisableAllHooks || len(s.Permissions.Ask) != 0 {
		t.Errorf("settings: %+v", s)
	}
	for _, want := range []string{"Edit(/" + dir + "/**/*.md)", "Write(/" + dir + "/**/*.md)", "mcp__colonyhall"} {
		if !slices.Contains(s.Permissions.Allow, want) {
			t.Errorf("allow lacks %q: %v", want, s.Permissions.Allow)
		}
	}
	for _, a := range s.Permissions.Allow {
		if (strings.HasPrefix(a, "Edit") || strings.HasPrefix(a, "Write")) && !strings.HasSuffix(a, "/**/*.md)") {
			t.Errorf("a write allow rule wider than Markdown: %q", a)
		}
		if strings.HasPrefix(a, "Bash") || strings.HasPrefix(a, "Read") {
			t.Errorf("unexpected allow rule %q", a)
		}
	}
	// deny > ask > allow: a deny (or ask) rule on Edit/Write would override
	// the Markdown allow, so there must be none.
	for _, d := range s.Permissions.Deny {
		if strings.HasPrefix(d, "Edit") || strings.HasPrefix(d, "Write") {
			t.Errorf("deny rule %q would override the .md allow", d)
		}
	}
	if !slices.Contains(s.Permissions.Deny, "Bash") || !slices.Contains(s.Permissions.Deny, "NotebookEdit") {
		t.Errorf("deny: %v", s.Permissions.Deny)
	}
}

// TestClaudeDirectDocsOnlyRefusesOldClient: without the flags that make
// the restriction hold, the run is refused instead of started unrestricted.
func TestClaudeDirectDocsOnlyRefusesOldClient(t *testing.T) {
	f := &fakeRunner{stdout: `{"type":"result","result":"ok"}`}
	c := &ClaudeCode{Run: f.run}
	_, err := c.Direct(context.Background(), DirectRequest{Prompt: "x", Dir: t.TempDir(), SessionID: "s1", DocsOnly: true})
	if !errors.Is(err, ErrDocsOnlyUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if f.args != nil {
		t.Fatalf("claude ran: %v", f.args)
	}
}

func TestDocsOnlySettingsRoots(t *testing.T) {
	for _, bad := range []string{"relative/dir", "/", "/a b", "/a,b", "/a/*", "/a/[x]", "/a/(x)", "/a/../b", "/a/{x}", "/a\"b"} {
		if _, err := DocsOnlySettings(bad); !errors.Is(err, ErrDocsOnlyRoot) {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
	// A symlinked root gets a rule for the resolved path too.
	base := t.TempDir()
	real := filepath.Join(base, "real")
	link := filepath.Join(base, "link")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	raw, err := DocsOnlySettings(link)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "Edit(/"+link+"/**/*.md)") || !strings.Contains(raw, "Edit(/"+real+"/**/*.md)") {
		t.Fatalf("settings: %s", raw)
	}
}

// TestDirectDocsOnlyAgent: the guard marks a docs-only agent's run, drops
// the chat approval tool even when approvals are chat, and gives it no
// terminal MCP server (terminals run commands) and no exchange folder.
func TestDirectDocsOnlyAgent(t *testing.T) {
	stateDir, root, exchange := t.TempDir(), t.TempDir(), t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	var seen DirectRequest
	var mcpDoc mcpConfigDoc
	ad := &directTestAdapter{}
	ad.impl = func(_ context.Context, req DirectRequest) (RunResult, error) {
		seen = req
		if raw, err := os.ReadFile(req.MCPConfigPath); err == nil {
			_ = json.Unmarshal(raw, &mcpDoc)
		}
		return RunResult{Text: "planned", SessionID: "sess-1"}, nil
	}
	agent := &Agent{
		Cfg:         AgentConfig{Client: "claude-code", Mode: "direct", Approvals: "chat", WriteScope: WriteScopeDocs, PermissionMode: "auto", HaltCheck: time.Hour},
		Client:      &Client{BaseURL: ts.URL, Token: "t", HTTP: ts.Client()},
		Adapter:     ad,
		StateDir:    stateDir,
		RootDir:     root,
		ExchangeDir: exchange,
		ExchangeOK:  true,
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts:    map[string]int{},
	}
	items := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "eng", ChannelID: "chan-1", ThreadRootID: "root-1", Body: "plan the feature"}}
	if err := agent.handleDirect(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if !seen.DocsOnly || seen.PermissionPromptTool != "" || seen.Dir != root {
		t.Fatalf("request: docs_only=%v prompt_tool=%q dir=%q", seen.DocsOnly, seen.PermissionPromptTool, seen.Dir)
	}
	if _, ok := mcpDoc.MCPServers["colony-terminal"]; ok {
		t.Fatalf("a docs-only agent got the terminal server: %+v", mcpDoc.MCPServers)
	}
	if _, ok := mcpDoc.MCPServers["colonyhall"]; !ok {
		t.Fatalf("no colonyhall server: %+v", mcpDoc.MCPServers)
	}
	if strings.Contains(seen.Prompt, exchange) {
		t.Fatalf("the prompt names the exchange folder outside its root")
	}

	// A full-access agent with the same settings keeps its approval tool
	// and terminals: the docs-only branch is what removes them.
	agent.Cfg.WriteScope = WriteScopeAll
	mcpDoc = mcpConfigDoc{}
	items[0].ID, items[0].ItemID = "it-2", "m2"
	if err := agent.handleDirect(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if seen.DocsOnly || seen.PermissionPromptTool != "mcp__colonyhall__approve" {
		t.Fatalf("full access: %+v", seen)
	}
	if _, ok := mcpDoc.MCPServers["colony-terminal"]; !ok {
		t.Fatalf("full access lost the terminal server: %+v", mcpDoc.MCPServers)
	}
}

// TestDirectDocsOnlyRefusesOtherClients: only Claude Code can enforce it.
func TestDirectDocsOnlyRefusesOtherClients(t *testing.T) {
	ad := &directTestAdapter{}
	agent := &Agent{
		Cfg:      AgentConfig{Client: "antigravity", Mode: "direct", WriteScope: WriteScopeDocs, HaltCheck: time.Hour},
		Client:   &Client{BaseURL: "http://127.0.0.1:1", Token: "t"},
		Adapter:  ad,
		StateDir: t.TempDir(),
		RootDir:  t.TempDir(),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}
	items := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "eng", ChannelID: "chan-1", ThreadRootID: "root-1", Body: "x"}}
	_ = agent.handleDirect(context.Background(), items)
	if len(ad.calls) != 0 {
		t.Fatalf("an antigravity docs-only agent ran: %+v", ad.calls)
	}
}

func TestAgentConfigWriteScope(t *testing.T) {
	for _, tc := range []struct {
		cfg AgentConfig
		ok  bool
	}{
		{AgentConfig{Client: "claude-code", Mode: "direct"}, true},
		{AgentConfig{Client: "claude-code", Mode: "direct", WriteScope: "docs"}, true},
		{AgentConfig{Client: "claude-code", Mode: "compact", WriteScope: "docs"}, false},
		{AgentConfig{Client: "antigravity", Mode: "direct", WriteScope: "docs"}, false},
		{AgentConfig{Client: "claude-code", Mode: "direct", WriteScope: "code"}, false},
	} {
		err := tc.cfg.defaults()
		if (err == nil) != tc.ok {
			t.Errorf("%+v: err = %v", tc.cfg, err)
		}
	}
}
