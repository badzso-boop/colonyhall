package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// Version is reported to the server.
const Version = "0.2.0"

// configMaxAge is how long work may run on the last known grants when the
// server cannot be reached; after that work is refused (fail closed).
const configMaxAge = 3 * time.Minute

// Enroll pairs this machine with a code from the admin and stores the host
// token in tokenFile (mode 0600).
func Enroll(ctx context.Context, server, code, name, tokenFile string) error {
	c := &Client{BaseURL: server, HTTP: &http.Client{Timeout: 30 * time.Second}}
	token, err := c.Enroll(ctx, code, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(tokenFile, []byte(token+"\n"), 0o600)
}

// Supervisor runs a machine in host mode: it reports the machine, follows
// the server's config and runs one Agent loop per assigned agent.
type Supervisor struct {
	Cfg Config
	Run Runner
	Log *slog.Logger
	// NewAdapter overrides the adapter registry (tests).
	NewAdapter func(ac AgentConfig) Adapter

	host    *Client
	roots   []string
	home    string
	sandbox *Sandbox

	mu         sync.Mutex
	lastOK     time.Time
	version    string
	agents     map[string]*managed
	rejected   []Rejected
	reported   time.Time
	exchangeOK bool
}

type managed struct {
	agent  *Agent
	cancel context.CancelFunc
	done   chan struct{}
	key    string
	grants []Grant // checked against this machine
	conf   HostAgent
}

// Start prepares the machine: token, roots, sandbox.
func (s *Supervisor) start(ctx context.Context) error {
	raw, err := os.ReadFile(s.Cfg.HostTokenFile)
	if err != nil {
		return fmt.Errorf("guard: host token: %w (run `colony guard enroll` first)", err)
	}
	s.host = &Client{BaseURL: s.Cfg.Server, Token: strings.TrimSpace(string(raw)), HTTP: &http.Client{Timeout: 30 * time.Second}}
	if s.roots, err = ResolveRoots(s.Cfg.Roots); err != nil {
		return err
	}
	s.home, _ = os.UserHomeDir()
	if err := os.MkdirAll(s.Cfg.SandboxDir, 0o700); err != nil {
		return err
	}
	s.sandbox = DetectSandbox(ctx, s.home, s.Cfg.SandboxDir)
	if s.sandbox != nil && s.Cfg.ClaudeTokenFile != "" {
		raw, err := os.ReadFile(s.Cfg.ClaudeTokenFile)
		if err != nil {
			return fmt.Errorf("guard: claude_token_file: %w", err)
		}
		s.sandbox.ClaudeToken = strings.TrimSpace(string(raw))
	}
	if s.sandbox == nil && *s.Cfg.RequireSandbox {
		s.Log.Warn("guard: no working bwrap; work runs are disabled (install bubblewrap or set require_sandbox: false)")
	}
	if s.Cfg.ExchangeDir != "" {
		if err := CheckExchangeDir(s.Cfg.ExchangeDir); err != nil {
			s.exchangeOK = false
			s.Log.Warn("guard: secrets exchange folder not usable (e.g. Z: drive is not mounted: mount -t drvfs Z: /mnt/z)", "dir", s.Cfg.ExchangeDir, "err", err)
		} else {
			s.exchangeOK = true
		}
	}
	s.agents = map[string]*managed{}
	return nil
}

// RunHost runs the supervisor until ctx ends.
func (s *Supervisor) RunHost(ctx context.Context) error {
	if err := s.start(ctx); err != nil {
		return err
	}
	sandbox := "none"
	if s.sandbox != nil {
		sandbox = s.sandbox.Version
	}
	s.Log.Info("guard: host mode", "server", s.Cfg.Server, "roots", strings.Join(s.roots, ","), "sandbox", sandbox)
	s.report(ctx)
	t := time.NewTicker(s.Cfg.ConfigPoll)
	defer t.Stop()
	for {
		s.sync(ctx)
		select {
		case <-ctx.Done():
			s.stopAll()
			return nil
		case <-t.C:
		}
	}
}

func (s *Supervisor) sync(ctx context.Context) {
	s.mu.Lock()
	version := s.version
	s.mu.Unlock()
	cfg, changed, err := s.host.HostConfig(ctx, version)
	var ae *APIError
	switch {
	case errors.As(err, &ae) && ae.Status == http.StatusUnauthorized:
		s.Log.Error("guard: the server rejected the host token (revoked?); stopping all agents")
		s.stopAll()
		return
	case err != nil:
		if ctx.Err() == nil {
			s.Log.Warn("guard: cannot fetch config", "err", err)
		}
		return
	}
	s.mu.Lock()
	s.lastOK = time.Now()
	s.mu.Unlock()
	if changed {
		s.reconcile(ctx, cfg)
	}
	s.mu.Lock()
	stale := time.Since(s.reported) > 10*time.Minute
	s.mu.Unlock()
	if stale {
		s.report(ctx)
	}
}

// reconcile starts, updates and stops agent loops to match the config.
func (s *Supervisor) reconcile(ctx context.Context, cfg HostConfig) {
	var rejected []Rejected
	want := map[string]bool{}
	for _, ha := range cfg.Agents {
		want[ha.AgentID] = true
		grants := s.checkGrants(ha, &rejected)
		s.mu.Lock()
		m := s.agents[ha.AgentID]
		s.mu.Unlock()
		if !ha.Sandbox && !s.Cfg.AllowUnsandboxed {
			reason := "unsandboxed agents are not allowed on this machine (guard.yaml allow_unsandboxed)"
			rejected = append(rejected, Rejected{AgentID: ha.AgentID, Path: ha.RootDir, Reason: reason})
			s.Log.Warn("guard: unsandboxed agent refused", "agent", "@"+ha.Handle, "reason", reason)
			if m != nil {
				s.stop(ha.AgentID)
			}
			continue
		}
		if !ha.Sandbox || ha.Mode == "direct" || ha.RootDir != "" {
			realRoot, reason := CheckGrant(ha.RootDir, s.roots, s.home)
			if reason != "" {
				rejected = append(rejected, Rejected{AgentID: ha.AgentID, Path: ha.RootDir, Reason: reason})
				s.Log.Warn("guard: root_dir refused", "agent", "@"+ha.Handle, "path", ha.RootDir, "reason", reason)
				if m != nil {
					s.stop(ha.AgentID)
				}
				continue
			}
			ha.RootDir = realRoot
		}
		key := strings.Join([]string{ha.Client, ha.Model, ha.WorkModel, fmt.Sprint(ha.AllowWork), ha.PermissionMode, ha.Handle, ha.Mode, ha.RootDir, fmt.Sprint(ha.Sandbox), ha.Approvals, fmt.Sprint(ha.MaxSessionTokens), fmt.Sprint(ha.DailyTokenLimit)}, "|")
		switch {
		case m != nil && m.key == key:
			s.mu.Lock()
			changedGrants := !slices.Equal(m.grants, grants)
			m.grants, m.conf = grants, ha
			s.mu.Unlock()
			if changedGrants {
				s.Log.Info("guard: folder access changed", "agent", "@"+ha.Handle, "grants", len(grants))
				m.agent.StopWork()
			}
		default:
			if m != nil {
				s.stop(ha.AgentID)
			}
			s.startAgent(ctx, ha, key, grants)
		}
	}
	s.mu.Lock()
	var gone []string
	for id := range s.agents {
		if !want[id] {
			gone = append(gone, id)
		}
	}
	s.version = cfg.Version
	changed := !slices.Equal(s.rejected, rejected)
	s.rejected = rejected
	s.mu.Unlock()
	for _, id := range gone {
		s.stop(id)
	}
	if changed {
		s.report(ctx)
	}
}

// checkGrants keeps the grants that are valid on this machine.
func (s *Supervisor) checkGrants(ha HostAgent, rejected *[]Rejected) []Grant {
	var out []Grant
	for _, g := range ha.Grants {
		real, reason := CheckGrant(g.Path, s.roots, s.home)
		if reason != "" {
			*rejected = append(*rejected, Rejected{AgentID: ha.AgentID, Path: g.Path, Reason: reason})
			s.Log.Warn("guard: grant refused", "agent", "@"+ha.Handle, "path", g.Path, "reason", reason)
			continue
		}
		out = append(out, Grant{Path: real, Mode: g.Mode, Write: g.Mode == "write"})
	}
	return out
}

// The server is not trusted with this machine's file system: a handle
// becomes a folder name and an agent id goes into a URL, so both must have
// exactly the server's documented shape.
var (
	handleRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
	agentIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// agentDir returns the agent's folder inside the sandbox root, or an error
// if the handle could escape it.
func (s *Supervisor) agentDir(ha HostAgent) (string, error) {
	if !handleRe.MatchString(ha.Handle) || !agentIDRe.MatchString(ha.AgentID) {
		return "", fmt.Errorf("guard: refusing agent with handle %q / id %q from the server", ha.Handle, ha.AgentID)
	}
	root := filepath.Clean(s.Cfg.SandboxDir)
	dir := filepath.Join(root, ha.Handle)
	if filepath.Dir(dir) != root {
		return "", fmt.Errorf("guard: agent folder %q escapes the sandbox", dir)
	}
	return dir, nil
}

func cleanStaleMCPConfigs(dir string) {
	matches, err := filepath.Glob(filepath.Join(dir, "mcp-*.json"))
	if err != nil {
		return
	}
	for _, f := range matches {
		_ = os.Remove(f)
	}
}

func (s *Supervisor) startAgent(ctx context.Context, ha HostAgent, key string, grants []Grant) {
	if !ha.Sandbox && !s.Cfg.AllowUnsandboxed {
		s.Log.Error("guard: agent not started: unsandboxed agents are not allowed on this machine (guard.yaml allow_unsandboxed)", "agent", "@"+ha.Handle)
		return
	}
	newAdapter := s.NewAdapter
	if newAdapter == nil {
		factory, ok := Adapters[ha.Client]
		if !ok {
			s.Log.Error("guard: unknown client", "agent", "@"+ha.Handle, "client", ha.Client)
			return
		}
		newAdapter = func(ac AgentConfig) Adapter { return factory(ac, s.Run) }
	}
	if ha.Sandbox && s.sandbox == nil && *s.Cfg.RequireSandbox && ha.Client != "claude-code" {
		// Only Claude Code can run a decision without any tools; other
		// clients would read untrusted messages with tools, unsandboxed.
		s.Log.Error("guard: agent not started: its client needs the sandbox even to decide", "agent", "@"+ha.Handle, "client", ha.Client)
		return
	}
	dir, err := s.agentDir(ha)
	if err != nil {
		s.Log.Error("guard: agent refused", "err", err)
		return
	}
	for _, d := range []string{dir, filepath.Join(dir, "work"), filepath.Join(dir, "decide")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			s.Log.Error("guard: sandbox folder", "err", err)
			return
		}
	}
	cleanStaleMCPConfigs(dir)
	_ = os.Remove(filepath.Join(dir, "agy-workspace", agyMCPFile)) // left by a crash
	ac := s.Cfg.Defaults
	ac.Name, ac.Client, ac.Model, ac.WorkModel = ha.Handle, ha.Client, ha.Model, ha.WorkModel
	ac.AllowWork, ac.PermissionMode, ac.Command = ha.AllowWork, ha.PermissionMode, s.Cfg.Commands[ha.Client]
	ac.Mode, ac.RootDir = ha.Mode, ha.RootDir
	ac.Approvals = ha.Approvals
	ac.MaxSessionTokens, ac.DailyTokenLimit = ha.MaxSessionTokens, ha.DailyTokenLimit
	ac.Dir = filepath.Join(dir, "decide") // decision runs: an empty folder, no tools
	_ = ac.defaults()

	m := &managed{key: key, grants: grants, conf: ha, done: make(chan struct{})}
	var httpClient *http.Client
	if s.host != nil {
		httpClient = s.host.HTTP
	}
	agent := &Agent{
		Cfg:         ac,
		Client:      &Client{BaseURL: s.Cfg.Server, Tokens: &hostTokens{host: s.host, agentID: ha.AgentID}, HTTP: httpClient},
		Adapter:     newAdapter(ac),
		Log:         s.Log,
		Mode:        ha.Mode,
		RootDir:     ha.RootDir,
		StateDir:    dir,
		ExchangeDir: s.Cfg.ExchangeDir,
		ExchangeOK:  s.exchangeOK,
	}
	agent.Policy = func() WorkPolicy { return s.policy(m, dir) }
	if ha.Sandbox && s.sandbox != nil {
		sb := s.sandbox
		decideDir, client := ac.Dir, ha.Client
		agent.DecideWrap = func(name string, args []string) (string, []string, []string, error) {
			a, env, err := sb.Command(client, dir, decideDir, nil, name, args)
			return sb.Bwrap, a, env, err
		}
	}
	m.agent = agent
	actx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	s.mu.Lock()
	s.agents[ha.AgentID] = m
	s.mu.Unlock()
	go func() {
		defer close(m.done)
		for actx.Err() == nil {
			if err := agent.Run(actx); err != nil {
				s.Log.Warn("guard: agent loop", "agent", "@"+ha.Handle, "err", err)
			}
			_ = sleep(actx, 30*time.Second)
		}
	}()
}

// policy decides a work run's folders from the agent's checked grants.
func (s *Supervisor) policy(m *managed, agentDir string) WorkPolicy {
	s.mu.Lock()
	grants := slices.Clone(m.grants)
	conf := m.conf
	fresh := time.Since(s.lastOK) < configMaxAge
	s.mu.Unlock()
	switch {
	case !conf.AllowWork:
		return WorkPolicy{Reason: "I can only chat here; ask a human to run this task."}
	case !fresh:
		return WorkPolicy{Reason: "I cannot confirm my folder access with the server right now; try again later."}
	case conf.Sandbox && s.sandbox == nil && *s.Cfg.RequireSandbox:
		return WorkPolicy{Reason: "Work is disabled on my machine: no sandbox (bwrap) is available there."}
	}
	// Re-check every grant now: a run with write access could have swapped a
	// granted subfolder for a symlink since the config arrived.
	var ok []Grant
	for _, g := range grants {
		if real, reason := CheckGrant(g.Path, s.roots, s.home); reason == "" && real == g.Path {
			ok = append(ok, g)
		} else {
			s.Log.Warn("guard: grant dropped for this run", "agent", "@"+conf.Handle, "path", g.Path, "reason", reason)
		}
	}
	grants = ok
	work := filepath.Join(agentDir, "work")
	for _, g := range grants { // prefer a writable folder, then a readable one
		if g.Write {
			work = g.Path
			break
		}
	}
	if work == filepath.Join(agentDir, "work") && len(grants) > 0 {
		work = grants[0].Path
	}
	var add []string
	for _, g := range grants {
		if g.Path != work {
			add = append(add, g.Path)
		}
	}
	p := WorkPolicy{Allowed: true, Dir: work, AddDirs: add}
	// Sandboxed agents may read and write their folders but not run
	// commands (decision, 2026-10-15); full-access agents run as in a terminal.
	p.NoExec = conf.Sandbox
	if conf.Sandbox && s.sandbox != nil {
		sb, client := s.sandbox, conf.Client
		p.Wrap = func(name string, args []string) (string, []string, []string, error) {
			a, env, err := sb.Command(client, agentDir, work, grants, name, args)
			return sb.Bwrap, a, env, err
		}
	}
	return p
}

func (s *Supervisor) stop(agentID string) {
	s.mu.Lock()
	m := s.agents[agentID]
	delete(s.agents, agentID)
	s.mu.Unlock()
	if m == nil {
		return
	}
	m.cancel()
	<-m.done
	s.Log.Info("guard: agent stopped", "agent", "@"+m.conf.Handle)
}

func (s *Supervisor) stopAll() {
	s.mu.Lock()
	var ids []string
	for id := range s.agents {
		ids = append(ids, id)
	}
	s.version = ""
	s.mu.Unlock()
	for _, id := range ids {
		s.stop(id)
	}
}

// report tells the server what this machine has.
func (s *Supervisor) report(ctx context.Context) {
	r := HostReport{GuardVersion: Version, OS: runtime.GOOS + "/" + runtime.GOARCH, Roots: s.roots, Clients: []ReportClient{}}
	for _, name := range AdapterNames() {
		bin := s.Cfg.Commands[name]
		if bin == "" {
			bin = defaultCommands[name]
		}
		if v, ok := clientVersion(ctx, bin); ok {
			r.Clients = append(r.Clients, ReportClient{Name: name, Version: v})
		}
	}
	if s.sandbox != nil {
		r.Sandbox = s.sandbox.Version
	}
	r.AllowUnsandboxed = s.Cfg.AllowUnsandboxed
	r.ExchangeDir = s.Cfg.ExchangeDir
	r.ExchangeOK = s.exchangeOK
	s.mu.Lock()
	r.Rejected = append([]Rejected{}, s.rejected...)
	s.mu.Unlock()
	if s.host == nil {
		return
	}
	if err := s.host.Report(ctx, r); err != nil {
		if ctx.Err() == nil {
			s.Log.Warn("guard: report failed", "err", err)
		}
		return
	}
	s.mu.Lock()
	s.reported = time.Now()
	s.mu.Unlock()
}

var defaultCommands = map[string]string{"claude-code": "claude", "antigravity": "agy"}

func clientVersion(ctx context.Context, bin string) (string, bool) {
	path, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", true
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return clip(line, 60), true
}

// hostTokens supplies an assigned agent's short-lived token, renewing it
// well before it expires.
type hostTokens struct {
	host    *Client
	agentID string

	mu    sync.Mutex
	token string
	exp   time.Time
}

func (t *hostTokens) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Until(t.exp) > 10*time.Minute {
		return t.token, nil
	}
	tok, exp, err := t.host.AgentToken(ctx, t.agentID)
	if err != nil {
		return "", err
	}
	t.token, t.exp = tok, exp
	return tok, nil
}

func (t *hostTokens) Invalidate() {
	t.mu.Lock()
	t.token = ""
	t.mu.Unlock()
}
