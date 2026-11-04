package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// ClaudeToken, when set (from `claude setup-token`), is given to Claude
	// instead of a copy of the user's login file.
	ClaudeToken string
	// TokenSource, when set, gives the current token instead (the login a
	// person sets from the PWA, round 38); it wins over ClaudeToken.
	TokenSource   func() string
	HasUnshareNet bool
	NetworkMode   string // "filtered", "log", "open"
	ColonyBin     string
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
	hasUnshareNet := exec.CommandContext(ctx, bin, "--unshare-net", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "true").Run() == nil
	return &Sandbox{Bwrap: bin, Version: strings.TrimSpace(string(ver)), Home: home, Root: root, HasUnshareNet: hasUnshareNet}
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

// colonyBinaryPath finds the path of the colony executable to mount inside the sandbox.
func colonyBinaryPath() (string, error) {
	if p, err := exec.LookPath("colony"); err == nil {
		return filepath.Abs(p)
	}
	self, err := os.Executable()
	if err == nil {
		if strings.HasSuffix(self, "/colony") {
			return filepath.EvalSymlinks(self)
		}
	}
	return "", errors.New("cannot find colony binary")
}

// SandboxOptions configures optional features for a sandboxed command.
type SandboxOptions struct {
	AllowWebRead bool
	AllowPlanner bool
}

// Command builds the bwrap command line around the client command, after
// preparing the agent's own config folder, and returns the argv for bwrap
// and the environment for the process.
func (s *Sandbox) Command(client, agentDir, workDir string, grants []Grant, name string, args []string) ([]string, []string, error) {
	return s.CommandWithOptions(client, agentDir, workDir, grants, name, args, SandboxOptions{})
}

// CommandWithOptions builds the bwrap command line with additional sandbox options.
func (s *Sandbox) CommandWithOptions(client, agentDir, workDir string, grants []Grant, name string, args []string, opts SandboxOptions) ([]string, []string, error) {
	netIsolated := s.NetworkMode == "filtered" || s.NetworkMode == "log"
	// web_read reaches the guard only through the in-sandbox bridge; without
	// network isolation 127.0.0.1:3129 would be the machine's own loopback.
	webRead := opts.AllowWebRead && netIsolated
	planner := opts.AllowPlanner && netIsolated
	a := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--unshare-ipc", "--unshare-uts"}
	if netIsolated {
		a = append(a, "--unshare-net")
	}
	a = append(a, "--ro-bind", "/usr", "/usr", "--ro-bind", "/etc", "/etc",
		"--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp", "--tmpfs", "/run")
	for _, d := range []string{"/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/opt"} {
		a = append(a, systemDir(d)...)
	}
	var colonyBin string
	if netIsolated {
		colonyBin = s.ColonyBin
		if colonyBin == "" {
			colonyBin, _ = colonyBinaryPath()
		}
		if colonyBin != "" && exists(colonyBin) {
			a = append(a, "--ro-bind", colonyBin, colonyBin)
		}
		sockPath := filepath.Join(agentDir, "proxy.sock")
		if exists(sockPath) {
			a = append(a, "--bind", sockPath, sockPath)
		}
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
	// Git metadata is read-only in the sandbox: the guard runs git outside it
	// (the commit after a run), and a writable .git/config could name a
	// filter or other command that git would run there. A worktree's .git is
	// a pointer file (gitdir: ...); its repository's .git is protected too.
	for _, p := range gitMetadata(append([]string{workDir}, grantPaths(grants)...)) {
		a = append(a, "--ro-bind", p, p)
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
	mounts, clientEnv, err := s.clientMounts(client, agentDir, webRead, planner)
	if err != nil {
		return nil, nil, err
	}
	a = append(a, mounts...)
	var env []string
	env = append(env, "HOME="+s.Home)
	for _, k := range passEnv {
		if k == "CLAUDE_CODE_OAUTH_TOKEN" && s.claudeToken() != "" {
			continue
		}
		if netIsolated {
			switch strings.ToUpper(k) {
			case "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY":
				continue
			}
		}
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	if netIsolated {
		noProxy := ""
		if webRead || planner {
			noProxy = "127.0.0.1,localhost"
		}
		env = append(env,
			"HTTPS_PROXY=http://127.0.0.1:3128",
			"HTTP_PROXY=http://127.0.0.1:3128",
			"https_proxy=http://127.0.0.1:3128",
			"http_proxy=http://127.0.0.1:3128",
			"NO_PROXY="+noProxy,
			"no_proxy="+noProxy,
		)
	}
	if client == "claude-code" {
		env = append(env, "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	}
	env = append(env, clientEnv...)
	if netIsolated && colonyBin != "" {
		sockPath := filepath.Join(agentDir, "proxy.sock")
		bridgeArgs := []string{"sandbox-bridge", "-sock", sockPath, "-port", "3128"}
		if webRead {
			webreadSock := filepath.Join(agentDir, "webread.sock")
			bridgeArgs = append(bridgeArgs, "-webread-sock", webreadSock, "-webread-port", "3129")
		}
		if planner {
			plannerSock := filepath.Join(agentDir, "planner.sock")
			bridgeArgs = append(bridgeArgs, "-planner-sock", plannerSock, "-planner-port", "3130")
		}
		bridgeArgs = append(bridgeArgs, "--", name)
		bridgeArgs = append(bridgeArgs, args...)
		a = append(a, "--chdir", workDir, "--", colonyBin)
		return append(a, bridgeArgs...), env, nil
	}
	a = append(a, "--chdir", workDir, "--", name)
	return append(a, args...), env, nil
}

// clientMounts prepares the agent's own config folder for the client and
// returns the mounts and environment that point the client at it.
//
// The folder lives next to the agent's sandbox folder (<agent>.config), not
// inside it, so a run cannot swap it for a symlink that a later bwrap call
// would follow on the host. The guard writes into it only through os.Root,
// which refuses to follow links out of it. Login tokens are copied in fresh
// for every run and never written back: a run cannot plant its own
// credentials into the user's real login.
func (s *Sandbox) clientMounts(client, agentDir string, allowWebRead, allowPlanner bool) ([]string, []string, error) {
	cfgRoot := agentDir + ".config"
	if err := os.MkdirAll(cfgRoot, 0o700); err != nil {
		return nil, nil, err
	}
	switch client {
	case "claude-code":
		dir, err := s.ownConfig(cfgRoot, "claude", func(r *os.Root) error {
			if s.claudeToken() != "" {
				// passed as CLAUDE_CODE_OAUTH_TOKEN instead; no copy of the
				// guard user's login stays in the agent's config folder.
				_ = r.Remove(".credentials.json")
			} else {
				if err := copyInto(r, filepath.Join(s.Home, ".claude", ".credentials.json"), ".credentials.json", true); err != nil {
					return err
				}
			}
			if allowWebRead || allowPlanner {
				servers := map[string]any{}
				if allowWebRead {
					servers["colony-web"] = map[string]any{
						"type": "http",
						"url":  "http://127.0.0.1:3129/mcp",
					}
				}
				if allowPlanner {
					servers["colony-planner"] = map[string]any{
						"type": "http",
						"url":  "http://127.0.0.1:3130/mcp",
					}
				}
				mcpCfg, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
				if err != nil {
					return err
				}
				_ = r.Remove("mcp.json")
				if err := r.WriteFile("mcp.json", append(mcpCfg, '\n'), 0o600); err != nil {
					return err
				}
			} else {
				_ = r.Remove("mcp.json")
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		env := []string{"CLAUDE_CONFIG_DIR=" + dir}
		if tok := s.claudeToken(); tok != "" {
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+tok)
		}
		return []string{"--bind", dir, dir}, env, nil
	case "antigravity":
		realCLI := filepath.Join(s.Home, ".gemini", "antigravity-cli")
		dir, err := s.ownConfig(cfgRoot, "gemini", func(r *os.Root) error {
			for _, d := range []string{"antigravity-cli/bin", "antigravity-cli/builtin", "config"} {
				if err := r.MkdirAll(d, 0o700); err != nil {
					return err
				}
			}
			// Settings are copied once; the agent may change its copy, which
			// only ever affects its own sandboxed runs.
			for _, f := range []string{"antigravity-cli/settings.json", "antigravity-cli/installation_id", "google_accounts.json", "installation_id"} {
				if err := copyInto(r, filepath.Join(s.Home, ".gemini", f), f, false); err != nil {
					return err
				}
			}
			if err := denyCommands(r, "antigravity-cli/settings.json", allowWebRead, allowPlanner); err != nil {
				return err
			}
			if allowWebRead || allowPlanner {
				servers := map[string]any{}
				if allowWebRead {
					servers["colony-web"] = map[string]any{
						"disabled":  false,
						"url":       "http://127.0.0.1:3129/mcp",
						"serverUrl": "http://127.0.0.1:3129/mcp",
					}
				}
				if allowPlanner {
					servers["colony-planner"] = map[string]any{
						"disabled":  false,
						"url":       "http://127.0.0.1:3130/mcp",
						"serverUrl": "http://127.0.0.1:3130/mcp",
					}
				}
				mcpCfg, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
				if err != nil {
					return err
				}
				_ = r.Remove("config/mcp_config.json")
				if err := r.WriteFile("config/mcp_config.json", append(mcpCfg, '\n'), 0o600); err != nil {
					return err
				}
			} else {
				_ = r.Remove("config/mcp_config.json")
			}
			return copyInto(r, filepath.Join(realCLI, "antigravity-oauth-token"), "antigravity-cli/antigravity-oauth-token", true)
		})
		if err != nil {
			return nil, nil, err
		}
		gem := filepath.Join(s.Home, ".gemini")
		a := []string{"--bind", dir, gem}
		for _, d := range []string{"bin", "builtin"} { // helper programs, read-only
			if real := filepath.Join(realCLI, d); exists(real) {
				a = append(a, "--ro-bind", real, filepath.Join(gem, "antigravity-cli", d))
			}
		}
		return a, nil, nil
	}
	return nil, nil, nil
}

// ownConfig opens (creating) cfgRoot/name as a real folder and runs prep in
// it through os.Root.
func (s *Sandbox) ownConfig(cfgRoot, name string, prep func(*os.Root) error) (string, error) {
	r, err := os.OpenRoot(cfgRoot)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	if err := r.MkdirAll(name, 0o700); err != nil {
		return "", err
	}
	if fi, err := r.Lstat(name); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("sandbox: %s/%s is not a folder", cfgRoot, name)
	}
	sub, err := r.OpenRoot(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = sub.Close() }()
	if err := prep(sub); err != nil {
		return "", err
	}
	return filepath.Join(cfgRoot, name), nil
}

// copyInto copies a file from the real home into the agent's config.
// always: overwrite every run (login tokens); otherwise only once.
func copyInto(r *os.Root, src, dst string, always bool) error {
	if !always {
		if _, err := r.Lstat(dst); err == nil {
			return nil
		}
	}
	raw, err := os.ReadFile(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = r.Remove(dst) // a link the agent may have put there is replaced, never followed
	return r.WriteFile(dst, raw, 0o600)
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

// denyCommands rewrites the agent's own agy settings for every run: no
// allow rules at all (any inherited rule could permit running something),
// and command(*) denied. A headless run denies whatever is not allowed, so
// with accept-edits the agent can read and edit files and nothing else.
// When allowWebRead or allowPlanner is true, the corresponding MCP server is configured and allowed.
// Doing it every run also undoes any change a run made to its copy.
func denyCommands(r *os.Root, name string, allowWebRead, allowPlanner bool) error {
	settings := map[string]any{}
	if raw, err := r.ReadFile(name); err == nil {
		if err := json.Unmarshal(raw, &settings); err != nil {
			settings = map[string]any{}
		}
	}
	perms, _ := settings["permissions"].(map[string]any)
	if perms == nil {
		perms = map[string]any{}
	}
	var allowList []any
	if allowWebRead {
		allowList = append(allowList, "mcp(colony-web/*)")
	}
	if allowPlanner {
		allowList = append(allowList, "mcp(colony-planner/*)")
	}
	if allowList == nil {
		allowList = []any{}
	}
	perms["allow"] = allowList
	perms["deny"] = []any{"command(*)"}
	settings["permissions"] = perms
	if allowWebRead || allowPlanner {
		servers := map[string]any{}
		if allowWebRead {
			servers["colony-web"] = map[string]any{
				"disabled":  false,
				"url":       "http://127.0.0.1:3129/mcp",
				"serverUrl": "http://127.0.0.1:3129/mcp",
			}
		}
		if allowPlanner {
			servers["colony-planner"] = map[string]any{
				"disabled":  false,
				"url":       "http://127.0.0.1:3130/mcp",
				"serverUrl": "http://127.0.0.1:3130/mcp",
			}
		}
		settings["mcpServers"] = servers
	} else {
		delete(settings, "mcpServers")
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	_ = r.Remove(name)
	return r.WriteFile(name, raw, 0o600)
}

func grantPaths(grants []Grant) []string {
	out := make([]string, 0, len(grants))
	for _, g := range grants {
		out = append(out, g.Path)
	}
	return out
}

// gitMetadata lists what to mount read-only for the given folders: a .git
// pointer file and the repository's .git folder it points into, or a .git
// folder itself.
func gitMetadata(dirs []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] && exists(p) {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, d := range dirs {
		gf := filepath.Join(d, ".git")
		fi, err := os.Lstat(gf)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			add(gf)
			continue
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		add(gf)
		raw, err := os.ReadFile(gf)
		if err != nil {
			continue
		}
		gitdir, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
		if !ok {
			continue
		}
		if common := worktreeCommonDir(gf, strings.TrimSpace(gitdir)); common != "" {
			add(common)
		}
	}
	return out
}

// worktreeCommonDir returns the repository .git folder of a worktree whose
// pointer file is gf, or "" when the pointer is not a real worktree link.
// The pointer may have been written by an agent, so it is followed only when
// its target is a worktree admin folder (<repo>/.git/worktrees/<name>) whose
// own "gitdir" file points back at gf: git writes that back link, and an
// agent can fake it only in a folder it can already write — so a forged
// pointer never mounts a folder the agent could not see.
func worktreeCommonDir(gf, gitdir string) string {
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(gf), gitdir)
	}
	admin, err := filepath.EvalSymlinks(gitdir)
	if err != nil {
		return ""
	}
	parent := filepath.Dir(admin)
	if filepath.Base(parent) != "worktrees" {
		return ""
	}
	back, err := os.ReadFile(filepath.Join(admin, "gitdir"))
	if err != nil {
		return ""
	}
	realGF, err := filepath.EvalSymlinks(gf)
	if err != nil {
		return ""
	}
	realBack, err := filepath.EvalSymlinks(strings.TrimSpace(string(back)))
	if err != nil || realBack != realGF {
		return ""
	}
	common := filepath.Dir(parent)
	if filepath.Base(common) != ".git" {
		return ""
	}
	return common
}

// claudeToken is the Claude login token for sandboxed Claude runs ("" when
// none: the user's login file is copied instead).
func (s *Sandbox) claudeToken() string {
	if s.TokenSource != nil {
		if tok := s.TokenSource(); tok != "" {
			return tok
		}
	}
	return s.ClaudeToken
}
