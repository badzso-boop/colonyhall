package guard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Sandbox runs work in bubblewrap (bwrap): the client sees the system
// read-only, its own login files, the agent's sandbox folder and the
// granted folders, and nothing else of the machine. It is not a container:
// the machine's installed, logged-in CLI runs, with a narrowed view.
type Sandbox struct {
	Bwrap   string // path of bwrap
	Version string
	Home    string // the user's real home
	Root    string // the guard's sandbox folder (all agents)
}

// DetectSandbox finds a working bwrap, or returns nil.
func DetectSandbox(ctx context.Context, home, root string) *Sandbox {
	bin, err := exec.LookPath("bwrap")
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, bin, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "true").Run(); err != nil {
		return nil
	}
	ver, _ := exec.CommandContext(ctx, bin, "--version").Output()
	return &Sandbox{Bwrap: bin, Version: strings.TrimSpace(string(ver)), Home: home, Root: root}
}

// clientFiles are each client's own login and settings, mounted writable
// so token refreshes keep working. Other clients' files stay hidden.
var clientFiles = map[string][]string{
	"claude-code": {".claude", ".claude.json"},
	"antigravity": {".gemini"},
}

// passEnv is the environment a sandboxed client keeps; everything else
// (including the guard's own tokens) is dropped.
var passEnv = []string{"PATH", "LANG", "LC_ALL", "TERM", "USER", "LOGNAME", "TZ",
	"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "GEMINI_API_KEY", "GOOGLE_API_KEY",
	"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "SSL_CERT_FILE"}

// Args builds the bwrap command line around the client command.
func (s *Sandbox) Args(client, agentDir, workDir string, grants []Grant, name string, args []string) []string {
	a := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--ro-bind", "/usr", "/usr", "--ro-bind", "/etc", "/etc",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--tmpfs", "/run"}
	for _, d := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/opt"} {
		a = append(a, systemDir(d)...)
	}
	// Machine secrets readable by root stay hidden.
	for _, f := range []string{"/etc/shadow", "/etc/gshadow", "/etc/sudoers"} {
		if _, err := os.Stat(f); err == nil {
			a = append(a, "--ro-bind", "/dev/null", f)
		}
	}
	if _, err := os.Stat("/etc/ssh"); err == nil {
		a = append(a, "--tmpfs", "/etc/ssh")
	}
	// DNS: /etc/resolv.conf is often a symlink (WSL: /mnt/wsl/resolv.conf).
	if real, err := filepath.EvalSymlinks("/etc/resolv.conf"); err == nil && !strings.HasPrefix(real, "/etc/") {
		a = append(a, "--ro-bind", real, real)
	}
	// An empty home, then only what the client needs from it.
	a = append(a, "--tmpfs", s.Home)
	if local := filepath.Join(s.Home, ".local"); exists(local) {
		a = append(a, "--ro-bind", local, local) // CLIs installed per user (~/.local/bin, ~/.local/share/…)
	}
	for _, g := range grants {
		if g.Write {
			a = append(a, "--bind", g.Path, g.Path)
		} else {
			a = append(a, "--ro-bind", g.Path, g.Path)
		}
		for _, d := range deniedInside(g.Path, s.Home) {
			a = append(a, hide(d)...)
		}
	}
	// Mounted after the grants, so a broad grant (e.g. the whole home)
	// cannot reveal them: the deny list, other clients' logins, other
	// agents' sandboxes.
	for _, d := range deniedPaths(s.Home) {
		if exists(d) {
			a = append(a, hide(d)...)
		}
	}
	for other, files := range clientFiles {
		if other == client {
			continue
		}
		for _, f := range files {
			if p := filepath.Join(s.Home, f); exists(p) {
				a = append(a, hide(p)...)
			}
		}
	}
	if exists(s.Root) {
		a = append(a, "--tmpfs", s.Root)
	}
	for _, f := range clientFiles[client] {
		if p := filepath.Join(s.Home, f); exists(p) {
			a = append(a, "--bind", p, p)
		}
	}
	a = append(a, "--bind", agentDir, agentDir)
	a = append(a, "--clearenv", "--setenv", "HOME", s.Home)
	for _, k := range passEnv {
		if v, ok := os.LookupEnv(k); ok {
			a = append(a, "--setenv", k, v)
		}
	}
	a = append(a, "--chdir", workDir, "--", name)
	return append(a, args...)
}

// systemDir mounts a top-level system folder read-only, or recreates it
// as the symlink it is on merged-/usr systems.
func systemDir(d string) []string {
	fi, err := os.Lstat(d)
	if err != nil {
		return nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		if target, err := os.Readlink(d); err == nil {
			return []string{"--symlink", target, d}
		}
		return nil
	}
	return []string{"--ro-bind", d, d}
}

// hide masks a folder with an empty tmpfs, or a file with /dev/null.
func hide(p string) []string {
	if fi, err := os.Stat(p); err == nil && fi.IsDir() {
		return []string{"--tmpfs", p}
	}
	return []string{"--ro-bind", "/dev/null", p}
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
