package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ClaudeCode runs `claude -p`. The decision run replaces the system prompt,
// turns off every tool, MCP server and settings file, and keeps no session:
// a wake-up costs about 1.3k tokens instead of ~15k with the defaults.
type ClaudeCode struct {
	Bin string // default "claude"
	Run Runner

	helpMu                 sync.Mutex
	hasAppendPromptChecked bool
	hasAppendPrompt        bool
}

type claudeOutput struct {
	Result     string          `json:"result"`
	Structured json.RawMessage `json:"structured_output"`
	SessionID  string          `json:"session_id"`
	IsError    bool            `json:"is_error"`
	Subtype    string          `json:"subtype"`
	NumTurns   int             `json:"num_turns"`
	Usage      struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func (c *ClaudeCode) bin() string {
	if c.Bin == "" {
		return "claude"
	}
	return c.Bin
}

// SupportsAppendSystemPrompt checks if the installed claude binary supports
// --append-system-prompt by inspecting claude --help output.
func (c *ClaudeCode) SupportsAppendSystemPrompt(ctx context.Context) bool {
	c.helpMu.Lock()
	defer c.helpMu.Unlock()
	if c.hasAppendPromptChecked {
		return c.hasAppendPrompt
	}
	c.hasAppendPromptChecked = true
	if c.Run == nil {
		return false
	}
	stdout, _, err := c.Run(ctx, "", c.bin(), []string{"--help"}, nil)
	if err == nil && strings.Contains(string(stdout), "--append-system-prompt") {
		c.hasAppendPrompt = true
	}
	return c.hasAppendPrompt
}

// Decide implements Adapter.
func (c *ClaudeCode) Decide(ctx context.Context, req DecideRequest) (RunResult, error) {
	prompt, refs := BuildPrompt(req.Handle, req.Items, false)
	args := []string{"-p", prompt,
		"--system-prompt", SystemPrompt(req.Handle),
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--no-session-persistence",
		"--output-format", "json",
		"--json-schema", DecisionSchema,
	}
	args = withModel(args, req.Model)
	res, out, err := c.execAs(ctx, req.Dir, req.Wrap, args, nil)
	if err != nil {
		return res, err
	}
	return finishDecision(res, structuredOrText(out.Structured, out.Result), refs)
}

// Summarize implements Adapter.
func (c *ClaudeCode) Summarize(ctx context.Context, req SummarizeRequest) (RunResult, error) {
	args := []string{
		"-p", req.Prompt,
		"--tools", "",
		"--strict-mcp-config",
		"--setting-sources", "",
		"--no-session-persistence",
		"--output-format", "json",
	}
	args = withModel(args, req.Model)
	res, out, err := c.execAs(ctx, req.Dir, req.Wrap, args, nil)
	if err != nil {
		return res, err
	}
	res.Text = out.Result
	return res, nil
}

// Work implements Adapter.
func (c *ClaudeCode) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	args := []string{"-p", req.Prompt, "--output-format", "json", "--no-session-persistence"}
	if req.Rules != "" && c.SupportsAppendSystemPrompt(ctx) {
		args = append(args, "--append-system-prompt", req.Rules)
	}
	mode := req.PermissionMode
	if req.NoExec && mode != "" && mode != "acceptEdits" && mode != "plan" {
		mode = "acceptEdits" // sandboxed: no mode that skips permission checks
	}
	if mode != "" {
		args = append(args, "--permission-mode", mode)
	}
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	if req.NoExec {
		args = append(args, "--tools", ClaudeFileTools, "--disallowedTools", "Bash")
	}
	if req.AllowWebRead {
		args = append(args, "--allowedTools", "mcp__colony-web__web_read")
		if req.MCPConfigPath != "" {
			args = append(args, "--mcp-config", req.MCPConfigPath, "--strict-mcp-config")
		}
	}
	args = withModel(args, req.Model)
	res, out, err := c.execAs(ctx, req.Dir, req.Wrap, args, nil)
	res.Text = out.Result
	return res, err
}

// Direct implements Adapter.
func (c *ClaudeCode) Direct(ctx context.Context, req DirectRequest) (RunResult, error) {
	args := []string{"-p", req.Prompt}
	if req.Rules != "" && c.SupportsAppendSystemPrompt(ctx) {
		args = append(args, "--append-system-prompt", req.Rules)
	}
	if req.Resume {
		args = append(args, "--resume", req.SessionID)
	} else {
		args = append(args, "--session-id", req.SessionID)
	}
	if req.PermissionMode != "" {
		args = append(args, "--permission-mode", req.PermissionMode)
	}
	if req.MCPConfigPath != "" {
		args = append(args, "--mcp-config", req.MCPConfigPath)
	}
	if req.PermissionPromptTool != "" {
		args = append(args, "--permission-prompt-tool", req.PermissionPromptTool)
	}
	args = append(args, "--output-format", "json")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	res, out, err := c.execAs(ctx, req.Dir, nil, args, req.Env)
	res.Text = out.Result
	if out.SessionID != "" {
		res.SessionID = out.SessionID
	}
	res.ContextTokens = claudeContextTokens(res.SessionID, res.Usage, out.NumTurns)
	return res, err
}

// claudeContextTokens is the context size at the run's last model call: the
// last assistant message's usage in the session transcript. Without the
// transcript it falls back to the run's usage averaged over its turns.
func claudeContextTokens(sessionID string, u Usage, turns int) int {
	if p := claudeSessionFile(sessionID); p != "" {
		if n, ok := lastTranscriptContext(p); ok {
			return n
		}
	}
	if turns < 1 {
		turns = 1
	}
	return (u.Input + u.CacheRead + u.CacheWrite) / turns
}

func (c *ClaudeCode) execAs(ctx context.Context, dir string, wrap Wrapper, args []string, extraEnv []string) (RunResult, claudeOutput, error) {
	start := time.Now()
	var (
		name string
		env  []string
		err  error
	)
	if wrap != nil {
		name, args, env, err = wrap.command(c.bin(), args)
		if err != nil {
			return RunResult{}, claudeOutput{}, fmt.Errorf("sandbox: %w", err)
		}
	} else {
		name = c.bin()
		if len(extraEnv) > 0 {
			env = os.Environ()
		}
	}
	if len(extraEnv) > 0 {
		env = append(env, extraEnv...)
	}
	stdout, _, err := c.Run(ctx, dir, name, args, env)
	res := RunResult{Duration: time.Since(start)}
	var out claudeOutput
	if jerr := json.Unmarshal(stdout, &out); jerr == nil {
		res.Usage = Usage{Input: out.Usage.Input, Output: out.Usage.Output, CacheRead: out.Usage.CacheRead, CacheWrite: out.Usage.CacheWrite}
		res.SessionID = out.SessionID
	} else if err == nil {
		return res, out, fmt.Errorf("claude: unreadable output: %w", jerr)
	}
	if err != nil {
		return res, out, err
	}
	if out.IsError {
		return res, out, fmt.Errorf("claude: run failed (%s): %s", out.Subtype, clip(out.Result, 300))
	}
	return res, out, nil
}

// ClaudeFileTools are the only tools of a sandboxed work run: files, no
// shell, no network.
const ClaudeFileTools = "Read,Write,Edit,Glob,Grep"

func withModel(args []string, model string) []string {
	if model != "" {
		return append(args, "--model", model)
	}
	return args
}
