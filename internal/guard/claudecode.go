package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ClaudeCode runs `claude -p`. The decision run replaces the system prompt,
// turns off every tool, MCP server and settings file, and keeps no session:
// a wake-up costs about 1.3k tokens instead of ~15k with the defaults.
type ClaudeCode struct {
	Bin string // default "claude"
	Run Runner
}

type claudeOutput struct {
	Result     string          `json:"result"`
	Structured json.RawMessage `json:"structured_output"`
	SessionID  string          `json:"session_id"`
	IsError    bool            `json:"is_error"`
	Subtype    string          `json:"subtype"`
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
	res, out, err := c.execAs(ctx, req.Dir, req.Wrap, args)
	if err != nil {
		return res, err
	}
	return finishDecision(res, structuredOrText(out.Structured, out.Result), refs)
}

// Work implements Adapter.
func (c *ClaudeCode) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	args := []string{"-p", req.Prompt, "--output-format", "json", "--no-session-persistence"}
	if req.PermissionMode != "" {
		args = append(args, "--permission-mode", req.PermissionMode)
	}
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = withModel(args, req.Model)
	res, out, err := c.execAs(ctx, req.Dir, req.Wrap, args)
	res.Text = out.Result
	return res, err
}

// Direct implements Adapter.
func (c *ClaudeCode) Direct(ctx context.Context, req DirectRequest) (RunResult, error) {
	args := []string{"-p", req.Prompt}
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
	args = append(args, "--output-format", "json")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	res, out, err := c.execAs(ctx, req.Dir, nil, args)
	res.Text = out.Result
	if out.SessionID != "" {
		res.SessionID = out.SessionID
	} else {
		res.SessionID = req.SessionID
	}
	return res, err
}

func (c *ClaudeCode) execAs(ctx context.Context, dir string, wrap Wrapper, args []string) (RunResult, claudeOutput, error) {
	start := time.Now()
	name, args, env, err := wrap.command(c.bin(), args)
	if err != nil {
		return RunResult{}, claudeOutput{}, fmt.Errorf("sandbox: %w", err)
	}
	stdout, _, err := c.Run(ctx, dir, name, args, env)
	res := RunResult{Duration: time.Since(start)}
	var out claudeOutput
	if jerr := json.Unmarshal(stdout, &out); jerr == nil {
		res.Usage = Usage{Input: out.Usage.Input, Output: out.Usage.Output, CacheRead: out.Usage.CacheRead, CacheWrite: out.Usage.CacheWrite}
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

func withModel(args []string, model string) []string {
	if model != "" {
		return append(args, "--model", model)
	}
	return args
}
