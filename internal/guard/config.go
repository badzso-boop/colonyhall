package guard

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the guard's YAML file.
type Config struct {
	Server string        `yaml:"server"`
	Agents []AgentConfig `yaml:"agents"`
}

// AgentConfig describes one agent the guard runs.
type AgentConfig struct {
	Name           string        `yaml:"name"`      // label for logs; defaults to the client name
	TokenEnv       string        `yaml:"token_env"` // env var holding the agent's API token
	TokenFile      string        `yaml:"token_file"`
	Client         string        `yaml:"client"`  // adapter: claude-code, antigravity
	Command        string        `yaml:"command"` // client binary; default per adapter
	Model          string        `yaml:"model"`   // model of the cheap decision run
	WorkModel      string        `yaml:"work_model"`
	Dir            string        `yaml:"dir"` // work directory (its own git worktree)
	PermissionMode string        `yaml:"permission_mode"`
	AllowWork      bool          `yaml:"allow_work"` // false: work requests are declined
	Poll           time.Duration `yaml:"poll"`
	Debounce       time.Duration `yaml:"debounce"`
	Batch          int           `yaml:"batch"`
	DecideTimeout  time.Duration `yaml:"decide_timeout"`
	WorkTimeout    time.Duration `yaml:"work_timeout"`
	HaltCheck      time.Duration `yaml:"halt_check"`
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
	if c.Server == "" {
		return fmt.Errorf("guard: server is required")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("guard: no agents configured")
	}
	for i := range c.Agents {
		a := &c.Agents[i]
		if _, ok := Adapters[a.Client]; !ok {
			return fmt.Errorf("guard: agent %d: unknown client %q (have %s)", i+1, a.Client, strings.Join(AdapterNames(), ", "))
		}
		if a.TokenEnv == "" && a.TokenFile == "" {
			return fmt.Errorf("guard: agent %d: token_env or token_file is required", i+1)
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
		def(&a.Debounce, 3*time.Second)
		def(&a.DecideTimeout, 3*time.Minute)
		def(&a.WorkTimeout, 15*time.Minute)
		def(&a.HaltCheck, 2*time.Second)
		if a.Batch <= 0 {
			a.Batch = 10
		}
		if a.WorkModel == "" {
			a.WorkModel = a.Model
		}
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
