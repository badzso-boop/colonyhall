package guard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSandboxForReal runs a shell inside bwrap and checks what it can see
// and write. Skipped where bwrap does not work.
func TestSandboxForReal(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	ro, rw, other := filepath.Join(base, "docs"), filepath.Join(base, "app"), filepath.Join(base, "private")
	root := filepath.Join(home, ".local/share/colonyhall/sandbox")
	agentDir := filepath.Join(root, "bot")
	for _, d := range []string{filepath.Join(home, ".ssh"), filepath.Join(home, ".gemini"), filepath.Join(home, ".claude"),
		filepath.Join(root, "someone-else"), agentDir, ro, rw, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".ssh", "id_ed25519"), "KEY")
	write(filepath.Join(home, ".gemini", "token"), "GEMINI")
	write(filepath.Join(home, ".claude", ".credentials.json"), "CLAUDE-TOKEN")
	write(filepath.Join(home, ".claude", "settings.json"), `{"hooks":{}}`)
	write(filepath.Join(home, ".claude.json"), "STATE")
	write(filepath.Join(root, "someone-else", "notes"), "OTHER-AGENT")
	write(filepath.Join(rw, ".env"), "SECRET=1")
	write(filepath.Join(ro, "readme"), "DOCS")
	write(filepath.Join(other, "x"), "PRIVATE")

	sb := DetectSandbox(context.Background(), home, root)
	if sb == nil {
		t.Skip("bwrap not available")
	}
	script := `
see() { if cat "$1" >/dev/null 2>&1 && [ -s "$1" ]; then echo "see:$2"; else echo "hidden:$2"; fi; }
see ` + filepath.Join(home, ".ssh/id_ed25519") + ` ssh
see ` + filepath.Join(home, ".gemini/token") + ` gemini
see ` + filepath.Join(home, ".claude/settings.json") + ` realsettings
see ` + filepath.Join(home, ".claude.json") + ` realstate
see "$CLAUDE_CONFIG_DIR/.credentials.json" token
echo '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"evil"}]}]}}' > "$CLAUDE_CONFIG_DIR/settings.json" && echo "write:ownsettings" || echo "nowrite:ownsettings"
mkdir -p ` + filepath.Join(home, ".claude") + ` 2>/dev/null; echo hook > ` + filepath.Join(home, ".claude/settings.json") + ` 2>/dev/null; echo tried
see ` + filepath.Join(root, "someone-else/notes") + ` otheragent
see ` + filepath.Join(rw, ".env") + ` env
see ` + filepath.Join(ro, "readme") + ` docs
see ` + filepath.Join(other, "x") + ` private
touch ` + filepath.Join(ro, "new") + ` 2>/dev/null && echo "write:docs" || echo "nowrite:docs"
touch ` + filepath.Join(rw, "new") + ` 2>/dev/null && echo "write:app" || echo "nowrite:app"
touch ` + filepath.Join(agentDir, "new") + ` 2>/dev/null && echo "write:own" || echo "nowrite:own"
echo "home:$HOME"
`
	args, err := sb.Command("claude-code", agentDir, rw, []Grant{{Path: ro}, {Path: rw, Write: true}}, "/bin/sh", []string{"-c", script})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sb.Bwrap, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"hidden:ssh", "hidden:gemini", "hidden:realsettings", "hidden:realstate", "see:token", "write:ownsettings",
		"hidden:otheragent", "hidden:env",
		"see:docs", "hidden:private", "nowrite:docs", "write:app", "write:own", "home:" + home} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("want %q, got:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(ro, "new")); err == nil {
		t.Error("the read-only folder was written")
	}
	// The user's real Claude settings are untouched: no hook got in.
	if raw, _ := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); string(raw) != `{"hooks":{}}` {
		t.Errorf("real settings changed: %s", raw)
	}
}
