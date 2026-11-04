package guard

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// twoGuardHome starts a host-mode supervisor whose config only names what
// the runbook asks of a second guard; everything else comes from defaults
// derived from its HOME.
func twoGuardHome(t *testing.T, ctx context.Context, home string, extra func(*Config)) *Supervisor {
	t.Helper()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "colonyhall"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "colonyhall", "host.token"), []byte("col_host\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := filepath.Join(home, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Server: "http://127.0.0.1:1", HostTokenFile: "~/.config/colonyhall/host.token", Roots: []string{projects}}
	if extra != nil {
		extra(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	s := &Supervisor{Cfg: cfg, Log: discardLog()}
	if err := s.start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	return s
}

// shortHome keeps unix socket paths under the 108-byte limit.
func shortHome(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "gh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	d, _ = filepath.EvalSymlinks(d)
	return d
}

// Two guards on one machine under different Linux users (different HOME
// and guard.yaml) must not share a folder, socket or port.
func TestTwoGuardsOnOneMachineShareNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	homeA, homeB := shortHome(t), shortHome(t)
	a := twoGuardHome(t, ctx, homeA, nil)
	// The second guard of the same Linux user is the hard case; a guard of
	// another user gets its own tmux folder from tmux itself.
	b := twoGuardHome(t, ctx, homeB, func(c *Config) { c.TmuxSocket = "colonyhall-b" })

	// Same handle on both machines: the server allows that across hosts.
	ha := HostAgent{AgentID: "11111111-1111-1111-1111-111111111111", Handle: "coder", Client: "claude-code", Sandbox: true}
	footprint := func(s *Supervisor) map[string]string {
		dir, err := s.agentDir(ha)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]string{
			"sandbox_dir":   s.Cfg.SandboxDir,
			"state":         s.stateDir(),
			"usage check":   filepath.Join(s.stateDir(), "usage"),
			"claude usage":  filepath.Join(s.stateDir(), "claude_usage.json"),
			"agy quota":     filepath.Join(s.stateDir(), "agy_quota.json"),
			"terminals":     s.terminals.terminalsDir(),
			"previews":      s.previews.dir(),
			"agent folder":  dir,
			"proxy socket":  filepath.Join(dir, "proxy.sock"),
			"webread sock":  filepath.Join(dir, "webread.sock"),
			"planner sock":  filepath.Join(dir, "planner.sock"),
			"client config": dir + ".config",
		}
	}
	fa, fb := footprint(a), footprint(b)
	for k, pa := range fa {
		pb := fb[k]
		if !strings.HasPrefix(pa, homeA+"/") || !strings.HasPrefix(pb, homeB+"/") {
			t.Errorf("%s is not under its guard's HOME: %s / %s", k, pa, pb)
		}
	}
	if a.terminals.SocketName() == b.terminals.SocketName() {
		t.Errorf("both guards use tmux socket %q", a.terminals.SocketName())
	}

	// The sandbox proxies of the same handle both listen; a shared path
	// would have made the second one remove the first one's socket.
	ma, mb := &managed{conf: ha}, &managed{conf: ha}
	for _, p := range []struct {
		s *Supervisor
		m *managed
	}{{a, ma}, {b, mb}} {
		dir, _ := p.s.agentDir(ha)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := p.s.startProxyForRun(p.m, dir); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = p.m.activeProxy.Close() })
	}
	for _, sock := range []string{fa["proxy socket"], fb["proxy socket"]} {
		c, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatalf("proxy %s: %v", sock, err)
		}
		_ = c.Close()
	}

	// Both guards default to the same preview range: a port the first one
	// published (docker holds it) is skipped by the second.
	pa, err := a.previews.Ports.Allocate(ha.AgentID, "prev-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	held, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(pa[0])))
	if err != nil {
		t.Skipf("preview port %d is busy on this machine: %v", pa[0], err)
	}
	defer func() { _ = held.Close() }()
	pb, err := b.previews.Ports.Allocate(ha.AgentID, "prev-b", 1)
	if err != nil {
		t.Fatal(err)
	}
	if pb[0] == pa[0] {
		t.Fatalf("both guards got preview port %d", pa[0])
	}
}

func TestPreviewPortsHeldByAnotherProgram(t *testing.T) {
	pool, err := NewPortPool("41000-41009")
	if err != nil {
		t.Fatal(err)
	}
	pool.Bindable = func(port int) bool { return port > 41008 }
	if _, err := pool.Allocate("a", "p1", 2); err == nil || !strings.Contains(err.Error(), "its own preview_ports range") {
		t.Fatalf("want a clear error, got %v", err)
	}
	got, err := pool.Allocate("a", "p1", 1)
	if err != nil || got[0] != 41009 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPreviewOverridesStayInTheGuardsFolder(t *testing.T) {
	ps := &PreviewSupervisor{}
	if d := ps.dir(); d == filepath.Join(os.TempDir(), "colony-previews") {
		t.Fatalf("default override folder is shared between users: %s", d)
	}
	ps.Dir = t.TempDir()
	if got := ps.recoveredOverride("p1"); got != filepath.Join(ps.Dir, "p1", "docker-compose.override.yaml") {
		t.Fatalf("override %s", got)
	}
}

func TestConfigTmuxSocketAndConcurrency(t *testing.T) {
	base := func() Config {
		return Config{Server: "http://x", HostTokenFile: "/t", Roots: []string{"/"}}
	}
	c := base()
	if err := c.Validate(); err != nil || c.TmuxSocket != DefaultTmuxSocket || c.MaxConcurrentRuns != 0 {
		t.Fatalf("defaults: %v %q %d", err, c.TmuxSocket, c.MaxConcurrentRuns)
	}
	for _, bad := range []string{"../x", "a/b", ".hidden", "a b", strings.Repeat("x", 65)} {
		c := base()
		c.TmuxSocket = bad
		if err := c.Validate(); err == nil {
			t.Errorf("tmux_socket %q accepted", bad)
		}
	}
	c = base()
	c.MaxConcurrentRuns = -1
	if err := c.Validate(); err == nil {
		t.Error("negative max_concurrent_runs accepted")
	}
	c = base()
	c.PreviewPorts = "9-3"
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "preview_ports") {
		t.Errorf("bad preview_ports: %v", err)
	}
}

// The colony-terminal MCP server of an agent uses its guard's tmux socket.
func TestTerminalMCPUsesTheGuardsTmuxSocket(t *testing.T) {
	tm := NewTerminalManager(t.TempDir(), nil, t.TempDir())
	tm.SetSocketName("colonyhall-colleague")
	a := &Agent{Cfg: AgentConfig{Name: "coder"}, Terminals: tm}
	args := a.terminalMCPArgs("/state", "ch", "th")
	if i := indexOf(args, "-tmux-socket"); i < 0 || args[i+1] != "colonyhall-colleague" {
		t.Fatalf("args %v", args)
	}
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}
