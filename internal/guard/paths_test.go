package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	args, err := sb.Command("claude-code", agentDir, rw, []Grant{{Path: ro}, {Path: rw, Write: true}}, "claude", []string{"-p", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	cfg := filepath.Join(agentDir, "config", "claude")
	for _, want := range []string{
		"--ro-bind " + ro + " " + ro,
		"--bind " + rw + " " + rw,
		"--tmpfs " + filepath.Join(home, ".claude"), // the real config is never visible
		"--tmpfs " + filepath.Join(home, ".gemini"),
		"--tmpfs " + filepath.Join(home, ".ssh"),
		"--ro-bind /dev/null " + filepath.Join(rw, ".env"),
		"--bind " + agentDir + " " + agentDir,
		"--bind " + filepath.Join(home, ".claude", ".credentials.json") + " " + filepath.Join(cfg, ".credentials.json"),
		"--setenv CLAUDE_CONFIG_DIR " + cfg,
		"--clearenv",
		"--chdir " + rw + " -- claude -p hi",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "--bind "+filepath.Join(home, ".claude")+" ") {
		t.Error("the real ~/.claude must never be bound")
	}
	// The hiding must come after the grants, or a broad grant reveals it.
	if strings.Index(joined, "--tmpfs "+filepath.Join(home, ".ssh")) < strings.Index(joined, "--bind "+rw) {
		t.Error("deny list must be mounted after the grants")
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
