package guard

import (
	"colonyhall/internal/apispec"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
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
	// StreamRun overrides the stream runner for adapters (tests).
	StreamRun StreamRunner
	Log       *slog.Logger
	// NewAdapter overrides the adapter registry (tests).
	NewAdapter func(ac AgentConfig) Adapter
	// DockerRunner overrides the docker compose runner (tests).
	DockerRunner DockerRunner
	// Now overrides time.Now (tests).
	Now func() time.Time

	host          *Client
	claudeLimiter *ClaudeLimiter
	agyLimiter    *AgyLimiter
	roots         []string
	home          string
	sandbox       *Sandbox
	previews      *PreviewSupervisor
	terminals     *TerminalManager
	runSlots      *RunSlots // max_concurrent_runs, shared by all agents

	mu               sync.Mutex
	lastOK           time.Time
	version          string
	agents           map[string]*managed
	rejected         []Rejected
	reported         time.Time
	exchangeOK       bool
	worktrees        []Worktree
	fileFetchLimiter chan struct{}
	agyUsage         []apispec.AgyUsageGroup
	usageCheckedAt   time.Time
}

// stateDir is the guard's own state folder.
func (s *Supervisor) stateDir() string { return filepath.Dir(s.Cfg.SandboxDir) }

func (s *Supervisor) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type managed struct {
	agent       *Agent
	cancel      context.CancelFunc
	done        chan struct{}
	key         string
	grants      []Grant // checked against this machine
	conf        HostAgent
	worktree    *Worktree
	activeProxy *SandboxProxy
	proxyMu     sync.Mutex
}

// Start prepares the machine: token, roots, sandbox.
func (s *Supervisor) start(ctx context.Context) error {
	s.fileFetchLimiter = make(chan struct{}, 2)
	s.runSlots = NewRunSlots(s.Cfg.MaxConcurrentRuns)
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
	if s.sandbox != nil {
		s.sandbox.NetworkMode = s.Cfg.SandboxNetwork
		if s.Cfg.ClaudeTokenFile != "" {
			raw, err := os.ReadFile(s.Cfg.ClaudeTokenFile)
			if err != nil {
				return fmt.Errorf("guard: claude_token_file: %w", err)
			}
			s.sandbox.ClaudeToken = strings.TrimSpace(string(raw))
		}
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
	runner := s.DockerRunner
	if runner == nil {
		runner = &ExecDockerRunner{}
	}
	ps, err := NewPreviewSupervisor(s.host, runner, s.roots, s.Cfg.PreviewHost, s.Cfg.PreviewPorts, s.Log)
	if err != nil {
		return fmt.Errorf("guard: preview supervisor: %w", err)
	}
	stateDir := filepath.Dir(s.Cfg.SandboxDir)
	ps.Dir = filepath.Join(stateDir, "previews")
	ps.Ports.Bindable = bindable(ps.PreviewHost)
	s.previews = ps

	s.terminals = NewTerminalManager(stateDir, s.roots, s.home)
	if s.Cfg.TmuxSocket != "" {
		s.terminals.SetSocketName(s.Cfg.TmuxSocket)
	}
	s.terminals.SetOnStart(s.handleTerminalStart)
	s.terminals.SetOnFinish(s.handleTerminalFinish)
	_ = s.terminals.AdoptExisting()
	s.terminals.StartWatcher(ctx, 5*time.Second)

	usageFile := filepath.Join(stateDir, "claude_usage.json")
	nowFn := time.Now
	if s.Now != nil {
		nowFn = s.Now
	}
	if s.agyLimiter == nil {
		s.agyLimiter = NewAgyLimiter(filepath.Join(stateDir, "agy_quota.json"), s.Log, nowFn)
		s.agyLimiter.SetOnResume(func() {
			s.report(context.Background())
			s.WakeAll()
		})
	}
	if s.claudeLimiter == nil {
		s.claudeLimiter = NewClaudeLimiter(usageFile, s.Cfg.RateLimitReserve, s.Cfg.RateLimitWarn, s.Log, nowFn)
		s.claudeLimiter.SetOnPauseChange(func(paused bool) {
			s.report(context.Background())
			if !paused {
				s.WakeAll()
			}
		})
	}
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
	s.Log.Info("guard: host mode", "server", s.Cfg.Server, "roots", strings.Join(s.roots, ","), "sandbox", sandbox, "sandbox_network", s.Cfg.SandboxNetwork)
	s.report(ctx)
	go s.hostWS().Run(ctx)
	go s.pollUsage(ctx)
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

// hostWS is the guard's own WebSocket (host token) for host-level folder
// listings; the deny rules are the same as for the agents of this machine.
func (s *Supervisor) hostWS() *HostWS {
	return &HostWS{Client: s.host, Log: s.Log, Scope: HostFsScope{Roots: s.roots, Home: s.home,
		DenyFiles: []string{s.Cfg.HostTokenFile, s.Cfg.ClaudeTokenFile}, ExchangeDir: s.Cfg.ExchangeDir}}
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
	} else {
		if s.previews != nil {
			s.previews.CheckExpired(ctx)
		}
		halted := make(map[string]bool)
		s.mu.Lock()
		for id, m := range s.agents {
			if m.agent != nil && m.agent.IsHalted() {
				halted[id] = true
			}
		}
		s.mu.Unlock()
		if s.previews != nil {
			s.previews.CheckHalted(ctx, halted)
		}
		if s.terminals != nil {
			s.terminals.CheckHalted(halted)
		}
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
				s.stop(ha.AgentID, "halt")
			}
			continue
		}
		var wt *Worktree
		if ha.WorktreeRepo != "" {
			realRepo, reason := CheckGrant(ha.WorktreeRepo, s.roots, s.home)
			if reason != "" {
				rejected = append(rejected, Rejected{AgentID: ha.AgentID, Path: ha.WorktreeRepo, Reason: reason})
				s.Log.Warn("guard: worktree_repo refused", "agent", "@"+ha.Handle, "path", ha.WorktreeRepo, "reason", reason)
				if m != nil {
					s.stop(ha.AgentID, "halt")
				}
				continue
			}
			ha.WorktreeRepo = realRepo
			wtPath, branch, err := EnsureWorktree(ctx, ha.WorktreeRepo, ha.Handle)
			if err != nil {
				rejected = append(rejected, Rejected{AgentID: ha.AgentID, Path: ha.WorktreeRepo, Reason: err.Error()})
				s.Log.Warn("guard: worktree setup failed", "agent", "@"+ha.Handle, "path", ha.WorktreeRepo, "err", err)
				if m != nil {
					s.stop(ha.AgentID, "halt")
				}
				continue
			}
			wt = &Worktree{AgentID: ha.AgentID, Path: wtPath, Branch: branch}
			ha.RootDir = wtPath
		} else if !ha.Sandbox || ha.Mode == "direct" || ha.RootDir != "" {
			realRoot, reason := CheckGrant(ha.RootDir, s.roots, s.home)
			if reason != "" {
				rejected = append(rejected, Rejected{AgentID: ha.AgentID, Path: ha.RootDir, Reason: reason})
				s.Log.Warn("guard: root_dir refused", "agent", "@"+ha.Handle, "path", ha.RootDir, "reason", reason)
				if m != nil {
					s.stop(ha.AgentID, "halt")
				}
				continue
			}
			ha.RootDir = realRoot
		}
		// The limits are not in the key: an admin may change them (round
		// 33), and the agent takes them over without a restart.
		key := strings.Join([]string{ha.Client, ha.Model, ha.WorkModel, fmt.Sprint(ha.AllowWork), ha.PermissionMode, ha.Handle, ha.Mode, ha.RootDir, ha.WorktreeRepo, fmt.Sprint(ha.Sandbox), ha.Approvals}, "|")
		switch {
		case m != nil && m.key == key:
			s.mu.Lock()
			changedGrants := !slices.Equal(m.grants, grants)
			if m.conf.MaxSessionTokens != ha.MaxSessionTokens && m.agent != nil {
				m.agent.SetSessionLimit(ha.MaxSessionTokens)
			}
			m.grants, m.conf = grants, ha
			m.worktree = wt
			s.mu.Unlock()
			if changedGrants {
				s.Log.Info("guard: folder access changed", "agent", "@"+ha.Handle, "grants", len(grants))
				m.agent.StopWork()
			}
		default:
			if m != nil {
				// Restarting with a new configuration: its terminals keep running.
				s.stop(ha.AgentID, "")
			}
			s.startAgent(ctx, ha, key, grants, wt)
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
	var curWT []Worktree
	for _, m := range s.agents {
		if m.worktree != nil {
			curWT = append(curWT, *m.worktree)
		}
	}
	sort.Slice(curWT, func(i, j int) bool { return curWT[i].AgentID < curWT[j].AgentID })
	changed := !slices.Equal(s.rejected, rejected) || !slices.Equal(s.worktrees, curWT)
	s.rejected = rejected
	s.worktrees = curWT
	s.mu.Unlock()
	// Gone from the config: the agent was deleted (archived, round 33).
	for _, id := range gone {
		s.stop(id, KilledArchived)
	}
	if changed || len(gone) > 0 {
		s.report(ctx)
	}
	halted := make(map[string]bool)
	s.mu.Lock()
	for id, m := range s.agents {
		if m.agent != nil && m.agent.IsHalted() {
			halted[id] = true
		}
	}
	s.mu.Unlock()
	if s.previews != nil {
		s.previews.Reconcile(ctx, cfg.Previews, cfg.Agents, halted)
	}
	if s.terminals != nil {
		s.terminals.CheckHalted(halted)
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

func (s *Supervisor) startProxyForRun(m *managed, agentDir string) error {
	if s.Cfg.SandboxNetwork == "open" {
		return nil
	}
	m.proxyMu.Lock()
	defer m.proxyMu.Unlock()
	if m.activeProxy != nil {
		_ = m.activeProxy.Close()
		m.activeProxy = nil
	}
	allow := append([]string{}, ClientDefaultAllow[m.conf.Client]...)
	allow = append(allow, s.Cfg.SandboxAllow...)
	sockPath := filepath.Join(agentDir, "proxy.sock")
	pr, err := StartSandboxProxy(ProxyConfig{
		Mode:      s.Cfg.SandboxNetwork,
		Allow:     allow,
		Server:    s.Cfg.Server,
		Client:    m.conf.Client,
		AgentName: m.conf.Handle,
		SockPath:  sockPath,
		Log:       s.Log,
	})
	if err != nil {
		return fmt.Errorf("guard: start sandbox proxy: %w", err)
	}
	m.activeProxy = pr
	return nil
}

func (s *Supervisor) startAgent(ctx context.Context, ha HostAgent, key string, grants []Grant, wt *Worktree) {
	if !ha.Sandbox && !s.Cfg.AllowUnsandboxed {
		s.Log.Error("guard: agent not started: unsandboxed agents are not allowed on this machine (guard.yaml allow_unsandboxed)", "agent", "@"+ha.Handle)
		return
	}
	m := &managed{key: key, grants: grants, conf: ha, worktree: wt, done: make(chan struct{})}
	runner := s.Run
	agentRunner := func(ctx context.Context, dir, name string, args []string, env []string) ([]byte, []byte, error) {
		m.proxyMu.Lock()
		pr := m.activeProxy
		m.proxyMu.Unlock()
		if pr != nil {
			defer func() {
				_ = pr.Close()
				m.proxyMu.Lock()
				m.activeProxy = nil
				m.proxyMu.Unlock()
			}()
		}
		return runner(ctx, dir, name, args, env)
	}
	newAdapter := s.NewAdapter
	if newAdapter == nil {
		factory, ok := Adapters[ha.Client]
		if !ok {
			s.Log.Error("guard: unknown client", "agent", "@"+ha.Handle, "client", ha.Client)
			return
		}
		newAdapter = func(ac AgentConfig) Adapter {
			ad := factory(ac, agentRunner)
			if s.StreamRun != nil {
				switch a := ad.(type) {
				case *ClaudeCode:
					a.StreamRun = s.StreamRun
				case *Antigravity:
					a.StreamRun = s.StreamRun
				}
			}
			return ad
		}
	}
	if ha.Sandbox && s.sandbox == nil && *s.Cfg.RequireSandbox && ha.Client != "claude-code" {
		// Only Claude Code can run a decision without any tools; other
		// clients would read untrusted messages with tools, unsandboxed.
		s.Log.Error("guard: agent not started: its client needs the sandbox even to decide", "agent", "@"+ha.Handle, "client", ha.Client)
		return
	}
	if ha.Sandbox && s.sandbox != nil && !s.sandbox.HasUnshareNet && s.Cfg.SandboxNetwork != "open" && ha.Client != "claude-code" {
		s.Log.Error("guard: agent not started: sandbox network isolation (--unshare-net) is not available", "agent", "@"+ha.Handle, "client", ha.Client)
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

	var httpClient *http.Client
	if s.host != nil {
		httpClient = s.host.HTTP
	}
	agent := &Agent{
		Cfg:          ac,
		Client:       &Client{BaseURL: s.Cfg.Server, Tokens: &hostTokens{host: s.host, agentID: ha.AgentID}, HTTP: httpClient},
		Adapter:      newAdapter(ac),
		Log:          s.Log,
		Mode:         ha.Mode,
		RootDir:      ha.RootDir,
		StateDir:     dir,
		ExchangeDir:  s.Cfg.ExchangeDir,
		ExchangeOK:   s.exchangeOK,
		WorktreeRepo: ha.WorktreeRepo,
		HostConfigVersion: func() string {
			s.mu.Lock()
			defer s.mu.Unlock()
			return s.version
		},
		Sandbox:       ha.Sandbox,
		AllowWebRead:  ha.AllowWebRead,
		WebReadAllow:  s.Cfg.WebReadAllow,
		WebReadDeny:   s.Cfg.WebReadDeny,
		Roots:         s.roots,
		ClaudeLimiter: s.claudeLimiter,
		AgyLimiter:    s.agyLimiter,
		Now:           s.Now,
		GrantsFunc: func() []Grant {
			s.mu.Lock()
			defer s.mu.Unlock()
			return slices.Clone(m.grants)
		},
		DenyFiles:        []string{s.Cfg.HostTokenFile, s.Cfg.ClaudeTokenFile},
		FileFetchLimiter: s.fileFetchLimiter,
		Terminals:        s.terminals,
		RunSlots:         s.runSlots,
	}
	if wt != nil {
		agent.WorktreeBranch = wt.Branch
		agent.WorktreePath = wt.Path
	}
	agent.Policy = func() WorkPolicy { return s.policy(m, dir) }
	// agy logs to ~/.gemini/antigravity-cli/log; a sandboxed agent's ~/.gemini
	// is its own config copy (see clientMounts).
	if home, err := os.UserHomeDir(); err == nil {
		agent.AgyConfigDir = filepath.Join(home, ".gemini")
	}
	if ha.Sandbox && s.sandbox != nil {
		agent.AgyConfigDir = filepath.Join(dir+".config", "gemini")
		agent.AgyScope = "@" + ha.Handle
	}
	if ha.Sandbox && s.sandbox != nil {
		sb := s.sandbox
		decideDir, client := ac.Dir, ha.Client
		agent.DecideWrap = func(name string, args []string) (string, []string, []string, error) {
			if err := s.startProxyForRun(m, dir); err != nil {
				return "", nil, nil, err
			}
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
	case conf.Sandbox && s.sandbox != nil && !s.sandbox.HasUnshareNet && s.Cfg.SandboxNetwork != "open":
		return WorkPolicy{Reason: "Work is disabled on my machine: sandbox network isolation (--unshare-net) is not available (update bubblewrap or set sandbox_network: open)."}
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
	if m.worktree != nil {
		work = m.worktree.Path
		grants = append([]Grant{{Path: m.worktree.Path, Mode: "write", Write: true}}, grants...)
	} else {
		for _, g := range grants { // prefer a writable folder, then a readable one
			if g.Write {
				work = g.Path
				break
			}
		}
		if work == filepath.Join(agentDir, "work") && len(grants) > 0 {
			work = grants[0].Path
		}
	}
	var add []string
	for _, g := range grants {
		if g.Path != work {
			add = append(add, g.Path)
		}
	}
	p := WorkPolicy{
		Allowed:      true,
		Dir:          work,
		AddDirs:      add,
		Grants:       grants,
		AgentDir:     agentDir,
		AllowWebRead: conf.Sandbox && conf.AllowWebRead && s.Cfg.SandboxNetwork != "open",
		AllowPlanner: conf.Sandbox && s.Cfg.SandboxNetwork != "open",
	}
	// Sandboxed agents may read and write their folders but not run
	// commands (decision, 2026-10-15); full-access agents run as in a terminal.
	p.NoExec = conf.Sandbox
	if conf.Sandbox && s.sandbox != nil {
		sb, client := s.sandbox, conf.Client
		allowWebRead := conf.AllowWebRead
		p.Wrap = func(name string, args []string) (string, []string, []string, error) {
			if err := s.startProxyForRun(m, agentDir); err != nil {
				return "", nil, nil, err
			}
			a, env, err := sb.CommandWithOptions(client, agentDir, work, grants, name, args, SandboxOptions{
				AllowWebRead: allowWebRead,
				AllowPlanner: true,
			})
			return sb.Bwrap, a, env, err
		}
	}
	return p
}

// KilledArchived marks terminals ended because their agent was deleted.
const KilledArchived = "archived"

// stop ends an agent's runs. killedBy, if set, also kills its terminals
// with that reason ("halt", "archived"); empty keeps them (a restart).
func (s *Supervisor) stop(agentID, killedBy string) {
	s.mu.Lock()
	m := s.agents[agentID]
	delete(s.agents, agentID)
	s.mu.Unlock()
	if m == nil {
		return
	}
	m.proxyMu.Lock()
	if m.activeProxy != nil {
		_ = m.activeProxy.Close()
		m.activeProxy = nil
	}
	m.proxyMu.Unlock()
	m.cancel()
	<-m.done
	if s.terminals != nil && killedBy != "" {
		s.terminals.KillAllForHandle(m.conf.Handle, killedBy)
	}
	s.Log.Info("guard: agent stopped", "agent", "@"+m.conf.Handle, "reason", killedBy)
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
		s.stop(id, "halt")
	}
	if s.previews != nil {
		s.previews.StopAll(context.Background())
	}
	if s.terminals != nil {
		s.terminals.KillAll("halt")
	}
}

// WakeAll wakes all managed agents immediately.
func (s *Supervisor) WakeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.agents {
		if m.agent != nil {
			m.agent.Wake()
		}
	}
}

func (s *Supervisor) handleTerminalStart(meta *TerminalMeta) {
	if meta == nil {
		return
	}
	s.Log.Info("guard: terminal started", "agent", "@"+meta.Handle, "name", meta.Name, "command", Redact(clip(meta.Command, 120)), "cwd", meta.Cwd)

	s.mu.Lock()
	var client *Client
	for _, m := range s.agents {
		if m.conf.AgentID == meta.AgentID || cleanHandle(m.conf.Handle) == meta.Handle {
			if m.agent != nil {
				client = m.agent.Client
			}
			break
		}
	}
	s.mu.Unlock()

	if client == nil {
		s.Log.Warn("guard: terminal started but agent client not found", "agent_id", meta.AgentID, "name", meta.Name)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := HandleTerminalStart(ctx, client, meta); err != nil {
		s.Log.Warn("guard: handle terminal start failed", "agent_id", meta.AgentID, "name", meta.Name, "err", err)
	}
}

func (s *Supervisor) handleTerminalFinish(meta *TerminalMeta) {
	if meta == nil {
		return
	}
	dur := meta.Duration(time.Now())
	if meta.KilledBy != "" {
		s.Log.Info("guard: terminal killed", "agent", "@"+meta.Handle, "name", meta.Name, "killed_by", meta.KilledBy, "duration", dur)
	} else {
		exitCode := -1
		if meta.ExitCode != nil {
			exitCode = *meta.ExitCode
		}
		s.Log.Info("guard: terminal finished", "agent", "@"+meta.Handle, "name", meta.Name, "exit_code", exitCode, "duration", dur)
	}

	s.mu.Lock()
	var client *Client
	for _, m := range s.agents {
		if m.conf.AgentID == meta.AgentID || cleanHandle(m.conf.Handle) == meta.Handle {
			if m.agent != nil {
				client = m.agent.Client
			}
			break
		}
	}
	s.mu.Unlock()

	if client == nil {
		s.Log.Warn("guard: terminal finished but agent client not found", "agent_id", meta.AgentID, "name", meta.Name)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := HandleTerminalFinish(ctx, client, s.terminals, meta); err != nil {
		s.Log.Warn("guard: handle terminal finish failed", "agent_id", meta.AgentID, "name", meta.Name, "err", err)
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
	if s.claudeLimiter != nil {
		r.ClaudeLimits = s.claudeLimiter.ReportLimits()
	}
	if s.agyLimiter != nil {
		r.AgyLimits = s.agyLimiter.Report()
	}
	s.mu.Lock()
	r.AgyUsage = s.agyUsage
	if !s.usageCheckedAt.IsZero() {
		at := s.usageCheckedAt
		r.UsageCheckedAt = &at
	}
	s.mu.Unlock()
	r.AllowUnsandboxed = s.Cfg.AllowUnsandboxed
	r.ExchangeDir = s.Cfg.ExchangeDir
	r.ExchangeOK = s.exchangeOK
	s.mu.Lock()
	r.Rejected = append([]Rejected{}, s.rejected...)
	r.Worktrees = append([]Worktree{}, s.worktrees...)
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

// bindable reports whether a preview port is still free on host: a port
// another program holds (another guard on this machine, a developer's
// server) is skipped instead of failing in docker compose.
func bindable(host string) func(port int) bool {
	return func(port int) bool {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return false
		}
		_ = l.Close()
		return true
	}
}
