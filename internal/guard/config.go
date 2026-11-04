package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the guard's YAML file. Two modes:
//
//   - host mode (host_token_file set): the server says which agents run
//     here and which folders they may use; this file only sets the
//     machine owner's limits (roots, sandbox). This is the normal mode.
//   - agents mode (agents listed): a fixed list with tokens, for a quick
//     single-machine setup without pairing.
type Config struct {
	Server string        `yaml:"server"`
	Agents []AgentConfig `yaml:"agents"`

	HostTokenFile    string        `yaml:"host_token_file"`
	Roots            []string      `yaml:"roots"`             // the only folders agents can ever get
	SandboxDir       string        `yaml:"sandbox_dir"`       // default ~/.local/share/colonyhall/sandbox
	SandboxNetwork   string        `yaml:"sandbox_network"`   // filtered (default), log, open
	SandboxAllow     []string      `yaml:"sandbox_allow"`     // additional machine-wide host:port allowlist entries
	WebReadAllow     []string      `yaml:"web_read_allow"`    // if set, only these hosts for web_read (*.domain or exact)
	WebReadDeny      []string      `yaml:"web_read_deny"`     // always refused hosts for web_read
	RequireSandbox   *bool         `yaml:"require_sandbox"`   // default true: no work runs without bwrap
	AllowUnsandboxed bool          `yaml:"allow_unsandboxed"` // default false: permit agents with sandbox: false
	ConfigPoll       time.Duration `yaml:"config_poll"`
	// UsagePoll is how often the guard asks the clients for their usage
	// (/usage, no model call); default 15m, negative turns it off.
	UsagePoll time.Duration     `yaml:"usage_poll"`
	Commands  map[string]string `yaml:"commands"` // client → binary, e.g. claude-code: claude
	// ClaudeTokenFile holds a token from `claude setup-token`; sandboxed
	// Claude runs use it instead of a copy of the user's login file.
	ClaudeTokenFile string      `yaml:"claude_token_file"`
	ExchangeDir     string      `yaml:"exchange_dir"`  // shared secrets exchange folder
	Defaults        AgentConfig `yaml:"defaults"`      // poll, debounce, batch, timeouts for host-mode agents
	PreviewPorts    string      `yaml:"preview_ports"` // port range for previews, e.g. 41000-41999
	PreviewHost     string      `yaml:"preview_host"`  // address where previews are published
	// TmuxSocket is the tmux socket name (tmux -L) of the agents'
	// terminals. tmux keeps sockets per user ($TMUX_TMPDIR/tmux-<uid>/), so
	// guards of different Linux users never share one; two guards of the
	// same user need different names. Default colonyhall.
	TmuxSocket string `yaml:"tmux_socket"`
	// MaxConcurrentRuns caps the client runs (decide, work, direct) of all
	// agents on this machine; the rest wait with their items pending.
	// 0 (default) means no limit.
	MaxConcurrentRuns int `yaml:"max_concurrent_runs"`

	RateLimitReserve int `yaml:"rate_limit_reserve"` // default 90 (%)
	RateLimitWarn    int `yaml:"rate_limit_warn"`    // default 80 (%)
}

// tmuxSocketRe limits tmux_socket to a plain file name (tmux -L).
var tmuxSocketRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,63}$`)

// HostMode reports whether the server manages this machine's agents.
func (c *Config) HostMode() bool { return c.HostTokenFile != "" }

// Write scopes of an agent (round 38): docs is a docs-only planner.
const (
	WriteScopeAll  = "all"
	WriteScopeDocs = "docs"
)

// AgentConfig describes one agent the guard runs.
type AgentConfig struct {
	Name             string        `yaml:"name"`      // label for logs; defaults to the client name
	TokenEnv         string        `yaml:"token_env"` // env var holding the agent's API token
	TokenFile        string        `yaml:"token_file"`
	Client           string        `yaml:"client"`  // adapter: claude-code, antigravity
	Command          string        `yaml:"command"` // client binary; default per adapter
	Model            string        `yaml:"model"`   // model of the cheap decision run
	WorkModel        string        `yaml:"work_model"`
	Dir              string        `yaml:"dir"` // work directory (its own git worktree)
	PermissionMode   string        `yaml:"permission_mode"`
	AllowWork        bool          `yaml:"allow_work"`  // false: work requests are declined
	Mode             string        `yaml:"mode"`        // compact | direct
	RootDir          string        `yaml:"root_dir"`    // direct mode root directory
	Approvals        string        `yaml:"approvals"`   // none | chat
	WriteScope       string        `yaml:"write_scope"` // all (default) | docs: a docs-only planner (round 38)
	MaxSessionTokens int           `yaml:"max_session_tokens"`
	DailyTokenLimit  int64         `yaml:"daily_token_limit"`
	Poll             time.Duration `yaml:"poll"`
	FallbackPoll     time.Duration `yaml:"fallback_poll"`
	DisableWS        bool          `yaml:"disable_ws"`
	Debounce         time.Duration `yaml:"debounce"`
	Batch            int           `yaml:"batch"`
	DecideTimeout    time.Duration `yaml:"decide_timeout"`
	WorkTimeout      time.Duration `yaml:"work_timeout"`
	IdleTimeout      time.Duration `yaml:"idle_timeout"`
	MaxRun           time.Duration `yaml:"max_run"`
	HaltCheck        time.Duration `yaml:"halt_check"`
}

// LoadConfig reads and validates a config file.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("guard: %s: %w", path, err)
	}
	return c, c.Validate()
}

// Validate fills defaults and checks the config.
func (c *Config) Validate() error {
	if c.RateLimitReserve <= 0 {
		c.RateLimitReserve = 90
	}
	if c.RateLimitWarn <= 0 {
		c.RateLimitWarn = 80
	}
	if c.Server == "" {
		return fmt.Errorf("guard: server is required")
	}
	if c.MaxConcurrentRuns < 0 {
		return fmt.Errorf("guard: max_concurrent_runs must be 0 (no limit) or more")
	}
	if c.TmuxSocket == "" {
		c.TmuxSocket = DefaultTmuxSocket
	}
	if !tmuxSocketRe.MatchString(c.TmuxSocket) {
		return fmt.Errorf("guard: invalid tmux_socket %q (a file name: letters, digits, ., _ or -, at most 64, not starting with .)", c.TmuxSocket)
	}
	if c.HostMode() {
		return c.validateHost()
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("guard: no agents configured (or set host_token_file for host mode)")
	}
	for i := range c.Agents {
		a := &c.Agents[i]
		if a.TokenEnv == "" && a.TokenFile == "" {
			return fmt.Errorf("guard: agent %d: token_env or token_file is required", i+1)
		}
		if err := a.defaults(); err != nil {
			return fmt.Errorf("guard: agent %d: %w", i+1, err)
		}
	}
	return nil
}

func (c *Config) validateHost() error {
	if len(c.Agents) > 0 {
		return fmt.Errorf("guard: host mode: agents come from the server; remove the agents list")
	}
	if len(c.Roots) == 0 {
		return fmt.Errorf("guard: host mode: list at least one root folder (roots)")
	}
	home, _ := os.UserHomeDir()
	if c.SandboxDir == "" {
		c.SandboxDir = filepath.Join(home, ".local", "share", "colonyhall", "sandbox")
	}
	for _, p := range []*string{&c.HostTokenFile, &c.ClaudeTokenFile, &c.SandboxDir, &c.ExchangeDir} {
		if *p == "~" {
			*p = home
		} else if strings.HasPrefix(*p, "~/") {
			*p = filepath.Join(home, (*p)[2:])
		}
	}
	if c.RequireSandbox == nil {
		t := true
		c.RequireSandbox = &t
	}
	if c.ConfigPoll <= 0 {
		c.ConfigPoll = 10 * time.Second
	}
	if c.Commands == nil {
		c.Commands = map[string]string{}
	}
	if c.PreviewPorts == "" {
		c.PreviewPorts = "41000-41999"
	}
	if c.PreviewHost == "" {
		c.PreviewHost = "127.0.0.1"
	}
	if _, err := NewPortPool(c.PreviewPorts); err != nil {
		return fmt.Errorf("guard: preview_ports: %w", err)
	}
	if c.SandboxNetwork == "" {
		c.SandboxNetwork = "filtered"
	}
	if c.SandboxNetwork != "filtered" && c.SandboxNetwork != "log" && c.SandboxNetwork != "open" {
		return fmt.Errorf("guard: invalid sandbox_network %q (must be filtered, log, or open)", c.SandboxNetwork)
	}
	return nil
}

func (a *AgentConfig) defaults() error {
	if _, ok := Adapters[a.Client]; !ok {
		return fmt.Errorf("unknown client %q (have %s)", a.Client, strings.Join(AdapterNames(), ", "))
	}
	if a.Name == "" {
		a.Name = a.Client
	}
	if a.Dir == "" {
		a.Dir = "."
	}
	def := func(d *time.Duration, v time.Duration) {
		if *d <= 0 {
			*d = v
		}
	}
	def(&a.Poll, 5*time.Second)
	def(&a.FallbackPoll, 60*time.Second)
	def(&a.Debounce, 3*time.Second)
	def(&a.DecideTimeout, 3*time.Minute)
	def(&a.WorkTimeout, 15*time.Minute)
	if a.IdleTimeout <= 0 {
		if a.WorkTimeout > 0 && a.WorkTimeout != 15*time.Minute {
			a.IdleTimeout = a.WorkTimeout
		} else {
			a.IdleTimeout = 15 * time.Minute
		}
	}
	def(&a.MaxRun, 4*time.Hour)
	def(&a.HaltCheck, 2*time.Second)
	if a.Batch <= 0 {
		a.Batch = 10
	}
	if a.WorkModel == "" {
		a.WorkModel = a.Model
	}
	if a.Approvals == "" {
		a.Approvals = "none"
	}
	if a.Approvals != "none" && a.Approvals != "chat" {
		return fmt.Errorf("invalid approvals %q (must be none or chat)", a.Approvals)
	}
	if a.WriteScope == "" {
		a.WriteScope = WriteScopeAll
	}
	if a.WriteScope != WriteScopeAll && a.WriteScope != WriteScopeDocs {
		return fmt.Errorf("invalid write_scope %q (must be all or docs)", a.WriteScope)
	}
	if a.WriteScope == WriteScopeDocs && (a.Client != "claude-code" || a.Mode != "direct") {
		return fmt.Errorf("write_scope docs needs client claude-code in direct mode")
	}
	return nil
}

// Token reads the agent's token. It is never logged.
func (a AgentConfig) Token() (string, error) {
	if a.TokenEnv != "" {
		if t := strings.TrimSpace(os.Getenv(a.TokenEnv)); t != "" {
			return t, nil
		}
		if a.TokenFile == "" {
			return "", fmt.Errorf("guard: %s: environment variable %s is empty", a.Name, a.TokenEnv)
		}
	}
	raw, err := os.ReadFile(a.TokenFile)
	if err != nil {
		return "", fmt.Errorf("guard: %s: %w", a.Name, err)
	}
	return strings.TrimSpace(string(raw)), nil
}
