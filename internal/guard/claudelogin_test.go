package guard

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
)

const testLoginToken = "sk-ant-oat01-guard-test-token-0123456789"

func TestClaudeLoginFailingCheckKeepsTheOldToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colonyhall", "claude.token")
	l, err := LoadClaudeLogin(path)
	if err != nil || l.Token() != "" || l.SetAt() != nil {
		t.Fatalf("missing file is no token: %v %q", err, l.Token())
	}
	set := func(tok string) apispec.HostClaudeLoginRequest {
		return apispec.HostClaudeLoginRequest{Op: apispec.Set, Token: &tok}
	}
	ok := func(context.Context, string) bool { return true }
	bad := func(context.Context, string) bool { return false }

	if r := serveClaudeLogin(context.Background(), l, ok, set("old-token-0123456789-abcdef")); !r.Ok || r.SetAt == nil {
		t.Fatalf("first set: %+v", r)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("token file mode: %v %v", st, err)
	}
	// A token the check refuses changes nothing.
	r := serveClaudeLogin(context.Background(), l, bad, set(testLoginToken))
	if r.Ok || r.ErrorCode == nil || *r.ErrorCode != apispec.CheckFailed {
		t.Fatalf("refused token: %+v", r)
	}
	if raw, _ := os.ReadFile(path); strings.TrimSpace(string(raw)) != "old-token-0123456789-abcdef" || l.Token() != "old-token-0123456789-abcdef" {
		t.Fatalf("the old token was replaced: %q %q", raw, l.Token())
	}
	// A multi-line paste is refused before any check.
	if r := serveClaudeLogin(context.Background(), l, ok, set("a b")); r.Ok {
		t.Fatal("a token with spaces was saved")
	}
	// The new one, then removal.
	if r := serveClaudeLogin(context.Background(), l, ok, set(testLoginToken)); !r.Ok || l.Token() != testLoginToken {
		t.Fatalf("new token: %+v", r)
	}
	if again, _ := LoadClaudeLogin(path); again.Token() != testLoginToken || again.SetAt() == nil {
		t.Fatal("the saved token is not read back")
	}
	if r := serveClaudeLogin(context.Background(), l, ok, apispec.HostClaudeLoginRequest{Op: apispec.Clear}); !r.Ok || l.Token() != "" || r.SetAt != nil {
		t.Fatalf("clear: %+v", r)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("token file still there: %v", err)
	}
	// No claude_token_file: not configured.
	if r := serveClaudeLogin(context.Background(), &ClaudeLogin{}, ok, set(testLoginToken)); r.Ok || *r.ErrorCode != apispec.NotConfigured {
		t.Fatalf("not configured: %+v", r)
	}
}

func TestLoginCheckerPassesTheCandidateOnlyToClaude(t *testing.T) {
	var f fakeRunner
	f.stdout = `{"type":"result","result":"Current session: 5% used\nResets 3pm (Europe/Budapest)"}`
	check := NewLoginChecker(f.run, "claude", filepath.Join(t.TempDir(), "login-check"))
	check(context.Background(), testLoginToken)
	if !slices.Equal(f.args, claudeUsageArgs) {
		t.Fatalf("not a /usage call: %v", f.args)
	}
	if !slices.Contains(f.env, "CLAUDE_CODE_OAUTH_TOKEN="+testLoginToken) {
		t.Fatal("the candidate token is not in the child's environment")
	}
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == testLoginToken {
		t.Fatal("the token leaked into the guard's own environment")
	}
}

func TestClaudeRunsUseTheLoginToken(t *testing.T) {
	var f fakeRunner
	f.stdout = `{"result":"{\"actions\":[]}","structured_output":{"actions":[]}}`
	tok := ""
	c := &ClaudeCode{Run: f.run, Token: func() string { return tok }}
	_, _ = c.Decide(context.Background(), DecideRequest{Handle: "bot"})
	for _, e := range f.env {
		if strings.HasPrefix(e, "CLAUDE_CODE_OAUTH_TOKEN=") {
			t.Fatal("no login set, yet a token was passed")
		}
	}
	tok = testLoginToken // set from the PWA while the guard runs
	_, _ = c.Decide(context.Background(), DecideRequest{Handle: "bot"})
	if !slices.Contains(f.env, "CLAUDE_CODE_OAUTH_TOKEN="+testLoginToken) {
		t.Fatalf("decide run without the token")
	}
	_, _ = c.Direct(context.Background(), DirectRequest{Prompt: "x", SessionID: "s1"})
	if !slices.Contains(f.env, "CLAUDE_CODE_OAUTH_TOKEN="+testLoginToken) {
		t.Fatalf("direct run without the token")
	}
}

func TestSandboxUsesTheLoginTokenInsteadOfTheLoginFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"secret":"guard-user"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tok := ""
	s := &Sandbox{Bwrap: "/usr/bin/bwrap", Home: home, Root: t.TempDir(), TokenSource: func() string { return tok }}
	agentDir := filepath.Join(s.Root, "bot")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, env, err := s.clientMounts("claude-code", agentDir, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfgDir := strings.TrimPrefix(env[0], "CLAUDE_CONFIG_DIR=")
	if _, err := os.Stat(filepath.Join(cfgDir, ".credentials.json")); err != nil {
		t.Fatalf("without a token the user's login is copied: %v", err)
	}
	tok = testLoginToken
	_, env, err = s.clientMounts("claude-code", agentDir, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env, "CLAUDE_CODE_OAUTH_TOKEN="+testLoginToken) {
		t.Fatalf("sandbox env: %v", env)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, ".credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("the guard user's login stays in the agent's folder: %v", err)
	}
}
