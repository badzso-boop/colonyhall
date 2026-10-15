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

// clientConfig lists every AI client's real config and login files. None
// of them is ever visible in the sandbox: a work run that could write the
// real settings could add a hook or an allow rule that later runs outside
// the sandbox (in a decision run or in the user's own CLI). Each agent gets
// its own config folder instead, with only the login token from the real
// one (see clientMounts).
var clientConfig = []string{".claude", ".claude.json", ".gemini", ".config/antigravity"}

// passEnv is the environment a sandboxed client keeps; everything else
// (including the guard's own tokens) is dropped.
var passEnv = []string{"PATH", "LANG", "LC_ALL", "TERM", "USER", "LOGNAME", "TZ",
	"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "GEMINI_API_KEY", "GOOGLE_API_KEY",
	"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "https_proxy", "http_proxy", "no_proxy", "SSL_CERT_FILE"}

// Command builds the bwrap command line around the client command, after
// preparing the agent's own config folder.
func (s *Sandbox) Command(client, agentDir, workDir string, grants []Grant, name string, args []string) ([]string, error) {
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
	// cannot reveal them: the deny list, every client's real config, the
	// other agents' sandboxes.
	for _, d := range deniedPaths(s.Home) {
		if exists(d) {
			a = append(a, hide(d)...)
		}
	}
	for _, f := range clientConfig {
		if p := filepath.Join(s.Home, f); exists(p) {
			a = append(a, hide(p)...)
		}
	}
	if exists(s.Root) {
		a = append(a, "--tmpfs", s.Root)
	}
	a = append(a, "--bind", agentDir, agentDir)
	mounts, env, err := s.clientMounts(client, agentDir)
	if err != nil {
		return nil, err
	}
	a = append(a, mounts...)
	a = append(a, "--clearenv", "--setenv", "HOME", s.Home)
	for _, k := range passEnv {
		if v, ok := os.LookupEnv(k); ok {
			a = append(a, "--setenv", k, v)
		}
	}
	a = append(a, env...)
	a = append(a, "--chdir", workDir, "--", name)
	return append(a, args...), nil
}

// clientMounts prepares the agent's own config folder for the client and
// returns the mounts and environment that point the client at it. Only the
// login token comes from the user's real config, writable so a refreshed
// token is kept; a token cannot run code.
func (s *Sandbox) clientMounts(client, agentDir string) ([]string, []string, error) {
	switch client {
	case "claude-code":
		cfg := filepath.Join(agentDir, "config", "claude")
		if err := os.MkdirAll(cfg, 0o700); err != nil {
			return nil, nil, err
		}
		var a []string
		if real := filepath.Join(s.Home, ".claude", ".credentials.json"); exists(real) {
			dst := filepath.Join(cfg, ".credentials.json")
			if err := touch(dst); err != nil {
				return nil, nil, err
			}
			a = append(a, "--bind", real, dst)
		}
		return a, []string{"--setenv", "CLAUDE_CONFIG_DIR", cfg}, nil
	case "antigravity":
		own := filepath.Join(agentDir, "config", "gemini")
		cli := filepath.Join(own, "antigravity-cli")
		if err := os.MkdirAll(cli, 0o700); err != nil {
			return nil, nil, err
		}
		realCLI := filepath.Join(s.Home, ".gemini", "antigravity-cli")
		// Settings are copied once; the agent may change its copy, which
		// only ever affects its own sandboxed runs.
		for _, f := range []string{"antigravity-cli/settings.json", "antigravity-cli/installation_id", "google_accounts.json", "installation_id"} {
			if err := copyOnce(filepath.Join(s.Home, ".gemini", f), filepath.Join(own, f)); err != nil {
				return nil, nil, err
			}
		}
		gem := filepath.Join(s.Home, ".gemini")
		a := []string{"--bind", own, gem}
		for _, d := range []string{"bin", "builtin"} { // helper programs, read-only
			if real := filepath.Join(realCLI, d); exists(real) {
				if err := os.MkdirAll(filepath.Join(cli, d), 0o700); err != nil {
					return nil, nil, err
				}
				a = append(a, "--ro-bind", real, filepath.Join(gem, "antigravity-cli", d))
			}
		}
		if real := filepath.Join(realCLI, "antigravity-oauth-token"); exists(real) {
			if err := touch(filepath.Join(cli, "antigravity-oauth-token")); err != nil {
				return nil, nil, err
			}
			a = append(a, "--bind", real, filepath.Join(gem, "antigravity-cli", "antigravity-oauth-token"))
		}
		return a, nil, nil
	}
	return nil, nil, nil
}

// touch makes sure a mount target file exists.
func touch(p string) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// copyOnce copies src to dst unless dst exists or src does not.
func copyOnce(src, dst string) error {
	if exists(dst) || !exists(src) {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o600)
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
