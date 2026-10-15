package guard

import (
	"os"
	"path/filepath"
	"slices"
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
	if err := os.WriteFile(filepath.Join(rw, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb := &Sandbox{Bwrap: "bwrap", Home: home, Root: filepath.Join(home, ".local/share/colonyhall/sandbox")}
	agentDir := filepath.Join(sb.Root, "bot")
	args := sb.Args("claude-code", agentDir, rw, []Grant{{Path: ro}, {Path: rw, Write: true}}, "claude", []string{"-p", "hi"})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--ro-bind " + ro + " " + ro,
		"--bind " + rw + " " + rw,
		"--bind " + filepath.Join(home, ".claude") + " ",
		"--tmpfs " + filepath.Join(home, ".gemini"), // another client's login stays hidden
		"--tmpfs " + filepath.Join(home, ".ssh"),
		"--ro-bind /dev/null " + filepath.Join(rw, ".env"),
		"--bind " + agentDir + " " + agentDir,
		"--clearenv",
		"--chdir " + rw + " -- claude -p hi",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	// The hiding must come after the grants, or a broad grant reveals it.
	if i, j := slices.Index(args, rw), strings.Index(joined, "--tmpfs "+filepath.Join(home, ".ssh")); i < 0 || j < strings.Index(joined, "--bind "+rw) {
		t.Error("deny list must be mounted after the grants")
	}
}
