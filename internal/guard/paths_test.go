package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCheckGrant(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	root := filepath.Join(base, "projects")
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(root, "app"), filepath.Join(base, "other")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A symlink inside the root that points outside it.
	if err := os.Symlink(filepath.Join(base, "other"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	roots, err := ResolveRoots([]string{root, home})
	if err != nil {
		t.Fatal(err)
	}
	if _, reason := CheckGrant(filepath.Join(root, "app"), roots, home); reason != "" {
		t.Fatalf("valid grant refused: %s", reason)
	}
	for path, want := range map[string]string{
		filepath.Join(base, "other"):  "outside",
		filepath.Join(root, "escape"): "outside", // symlink resolved
		filepath.Join(root, "nope"):   "does not exist",
		filepath.Join(home, ".ssh"):   "deny list",
		"relative/path":               "absolute",
		root + "/app/../app":          "absolute, clean",
	} {
		if _, reason := CheckGrant(path, roots, home); !strings.Contains(reason, want) {
			t.Errorf("%s: reason %q, want %q", path, reason, want)
		}
	}
	if _, err := ResolveRoots([]string{filepath.Join(base, "missing")}); err == nil {
		t.Error("a missing root must be an error")
	}
}

func TestSandboxArgs(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	ro, rw := filepath.Join(base, "docs"), filepath.Join(base, "app")
	for _, d := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".gemini"), filepath.Join(home, ".ssh"), ro, rw} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{filepath.Join(rw, ".env"), filepath.Join(home, ".claude", ".credentials.json")} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sb := &Sandbox{Bwrap: "bwrap", Home: home, Root: filepath.Join(home, ".local/share/colonyhall/sandbox")}
	agentDir := filepath.Join(sb.Root, "bot")
	t.Setenv("ANTHROPIC_API_KEY", "secret-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "secret-token")
	args, env, err := sb.Command("claude-code", agentDir, rw, []Grant{{Path: ro}, {Path: rw, Write: true}}, "claude", []string{"-p", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	cfg := agentDir + ".config/claude"
	for _, want := range []string{
		"--ro-bind " + ro + " " + ro,
		"--bind " + rw + " " + rw,
		"--tmpfs " + filepath.Join(home, ".claude"), // the real config is never visible
		"--tmpfs " + filepath.Join(home, ".gemini"),
		"--tmpfs " + filepath.Join(home, ".ssh"),
		"--ro-bind /dev/null " + filepath.Join(rw, ".env"),
		"--bind " + agentDir + " " + agentDir,
		"--bind " + cfg + " " + cfg,
		"--chdir " + rw + " -- claude -p hi",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{"--clearenv", "--setenv", "secret-token", "secret-key"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("forbidden %q leaked in args:\n%s", forbidden, joined)
		}
	}
	for _, wantEnv := range []string{
		"HOME=" + home,
		"CLAUDE_CONFIG_DIR=" + cfg,
		"CLAUDE_CODE_OAUTH_TOKEN=secret-token",
		"ANTHROPIC_API_KEY=secret-key",
	} {
		if !slices.Contains(env, wantEnv) {
			t.Errorf("missing env %q in %v", wantEnv, env)
		}
	}
	if strings.Contains(joined, "--bind "+filepath.Join(home, ".claude")) {
		t.Error("nothing of the real ~/.claude may be bound writable")
	}
	if raw, err := os.ReadFile(filepath.Join(cfg, ".credentials.json")); err != nil || string(raw) != "x" {
		t.Errorf("the login is copied into the agent's config: %q %v", raw, err)
	}
	// The hiding must come after the grants, or a broad grant reveals it.
	if strings.Index(joined, "--tmpfs "+filepath.Join(home, ".ssh")) < strings.Index(joined, "--bind "+rw) {
		t.Error("deny list must be mounted after the grants")
	}

	// When ClaudeToken is set on the Sandbox, it is passed in env instead of copying credentials.
	sb.ClaudeToken = "explicit-token"
	_, env2, err := sb.Command("claude-code", agentDir, rw, []Grant{{Path: ro}}, "claude", []string{"-p", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(env2, "CLAUDE_CODE_OAUTH_TOKEN=explicit-token") {
		t.Errorf("missing explicit token in env: %v", env2)
	}
}

// TestAgentDirRejectsServerTraversal: a malicious server must not be able
// to point an agent's writable folder outside the sandbox root.
func TestAgentDirRejectsServerTraversal(t *testing.T) {
	root := t.TempDir()
	s := &Supervisor{Cfg: Config{SandboxDir: root}}
	id := "01a13fdf-0d46-7dd6-b537-1436c7ed0491"
	if dir, err := s.agentDir(HostAgent{AgentID: id, Handle: "claude"}); err != nil || dir != filepath.Join(root, "claude") {
		t.Fatalf("valid handle: %s %v", dir, err)
	}
	for _, ha := range []HostAgent{
		{AgentID: id, Handle: "../../etc"},
		{AgentID: id, Handle: ".."},
		{AgentID: id, Handle: "a/b"},
		{AgentID: id, Handle: ""},
		{AgentID: "../hosts", Handle: "claude"},
	} {
		if dir, err := s.agentDir(ha); err == nil {
			t.Errorf("%+v accepted as %s", ha, dir)
		}
	}
}

// TestSandboxConfigSymlinks: links a previous run planted in the agent's
// config must not make the guard write outside it, nor become bind sources.
func TestSandboxConfigSymlinks(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(home, ".gemini", "antigravity-cli"), filepath.Join(home, ".claude"), outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for f, v := range map[string]string{
		filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"): "TOKEN",
		filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"):           "{}",
		filepath.Join(home, ".claude", ".credentials.json"):                          "CREDS",
		filepath.Join(outside, "victim"):                                             "KEEP",
	} {
		if err := os.WriteFile(f, []byte(v), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sb := &Sandbox{Bwrap: "bwrap", Home: home, Root: filepath.Join(base, "sandbox")}
	agentDir := filepath.Join(sb.Root, "bot")
	if _, _, err := sb.Command("antigravity", agentDir, agentDir, nil, "agy", nil); err != nil {
		t.Fatal(err)
	}
	// A run replaces files in its config with links to the host.
	cli := agentDir + ".config/gemini/antigravity-cli"
	_ = os.Remove(filepath.Join(cli, "antigravity-oauth-token"))
	if err := os.Symlink(filepath.Join(outside, "victim"), filepath.Join(cli, "antigravity-oauth-token")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := sb.Command("antigravity", agentDir, agentDir, nil, "agy", nil); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(outside, "victim")); string(raw) != "KEEP" {
		t.Fatalf("the guard wrote through a planted link: %q", raw)
	}
	if fi, err := os.Lstat(filepath.Join(cli, "antigravity-oauth-token")); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the planted link must be replaced by a fresh token copy: %v %v", fi, err)
	}
	// The whole client folder swapped for a link to the host: refused.
	gem := agentDir + ".config/gemini"
	if err := os.RemoveAll(gem); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, gem); err != nil {
		t.Fatal(err)
	}
	if args, _, err := sb.Command("antigravity", agentDir, agentDir, nil, "agy", nil); err == nil {
		t.Fatalf("a linked config folder must be refused, got %v", args)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 1 {
		t.Fatalf("the guard created files outside: %v", entries)
	}
}

// TestPolicyRechecksGrants: a granted folder swapped for a symlink after
// the config arrived is not mounted.
func TestPolicyRechecksGrants(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "projects")
	app, docs, outside := filepath.Join(root, "app"), filepath.Join(root, "app", "docs"), filepath.Join(base, "etc")
	for _, d := range []string{docs, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	roots, _ := ResolveRoots([]string{root})
	off := false
	s := &Supervisor{Cfg: Config{RequireSandbox: &off}, roots: roots, Log: discardLog(), lastOK: time.Now()}
	m := &managed{conf: HostAgent{Handle: "bot", AllowWork: true}, grants: []Grant{{Path: app, Write: true}, {Path: docs}}}
	if p := s.policy(m, filepath.Join(base, "sb", "bot")); !p.Allowed || len(p.AddDirs) != 1 {
		t.Fatalf("before: %+v", p)
	}
	// With write access to app, a run swaps app/docs for a link to the host.
	if err := os.RemoveAll(docs); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, docs); err != nil {
		t.Fatal(err)
	}
	if p := s.policy(m, filepath.Join(base, "sb", "bot")); len(p.AddDirs) != 0 || p.Dir != app {
		t.Fatalf("the swapped grant must be dropped: %+v", p)
	}
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestSandboxedRunsCannotExecute: sandboxed agents read and write their
// folders but run no commands (decision, 2026-10-15).
func TestSandboxedRunsCannotExecute(t *testing.T) {
	// Claude: only file tools, Bash explicitly disallowed.
	f := &fakeRunner{stdout: `{"result":"ok"}`}
	c := &ClaudeCode{Run: f.run}
	if _, err := c.Work(context.Background(), WorkRequest{Prompt: "x", NoExec: true}); err != nil {
		t.Fatal(err)
	}
	if v, _ := flagValue(f.args, "--tools"); v != ClaudeFileTools {
		t.Errorf("--tools %q", v)
	}
	if v, _ := flagValue(f.args, "--disallowedTools"); v != "Bash" {
		t.Errorf("--disallowedTools %q", v)
	}
	// A permission mode that skips checks is not honoured in the sandbox.
	if _, err := c.Work(context.Background(), WorkRequest{Prompt: "x", NoExec: true, PermissionMode: "bypassPermissions"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := flagValue(f.args, "--permission-mode"); v != "acceptEdits" {
		t.Errorf("sandboxed claude permission mode %q", v)
	}
	ag := &Antigravity{Run: f.run}
	f.stdout = `{"status":"SUCCESS","response":"ok"}`
	if _, err := ag.Work(context.Background(), WorkRequest{Prompt: "x", NoExec: true, PermissionMode: "bypassPermissions"}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(f.args, "--dangerously-skip-permissions") {
		t.Errorf("sandboxed agy must not skip permissions: %v", f.args)
	}

	// Antigravity: the agent's own settings deny commands on every run,
	// even if a previous run allowed them again.
	base := t.TempDir()
	home := filepath.Join(base, "home")
	cli := filepath.Join(home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(cli, 0o700); err != nil {
		t.Fatal(err)
	}
	user := `{"model":"x","permissions":{"allow":["read_url(*)","command(git *)","command(rm *)"]}}`
	if err := os.WriteFile(filepath.Join(cli, "settings.json"), []byte(user), 0o600); err != nil {
		t.Fatal(err)
	}
	sb := &Sandbox{Bwrap: "bwrap", Home: home, Root: filepath.Join(base, "sandbox")}
	agentDir := filepath.Join(sb.Root, "bot")
	own := agentDir + ".config/gemini/antigravity-cli/settings.json"
	for round := 0; round < 2; round++ {
		if _, _, err := sb.Command("antigravity", agentDir, agentDir, nil, "agy", nil); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(own)
		var s struct {
			Model       string
			Permissions struct{ Allow, Deny []string }
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
		if len(s.Permissions.Allow) != 0 || !slices.Equal(s.Permissions.Deny, []string{"command(*)"}) ||
			(round == 0 && s.Model != "x") {
			t.Fatalf("round %d: %s", round, raw)
		}
		// A run tries to allow commands again in its copy.
		_ = os.WriteFile(own, []byte(`{"permissions":{"allow":["command(*)"]}}`), 0o600)
	}
	if raw, _ := os.ReadFile(filepath.Join(cli, "settings.json")); string(raw) != user {
		t.Errorf("the user's own agy settings must not change: %s", raw)
	}
}
