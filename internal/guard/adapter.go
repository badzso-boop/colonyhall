package guard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Adapter runs one AI client. Adding a client = a new Adapter registered in
// Adapters; the guard loop does not change.
type Adapter interface {
	// Decide is the cheap wake-up run: no tools, JSON answer.
	Decide(ctx context.Context, req DecideRequest) (RunResult, error)
	// Work is a full agent run with the client's tools, in the work dir.
	Work(ctx context.Context, req WorkRequest) (RunResult, error)
	// Direct is a full CLI session per thread, run in the root dir.
	Direct(ctx context.Context, req DirectRequest) (RunResult, error)
}

// DecideRequest is the input of a decision run.
type DecideRequest struct {
	Handle string
	Items  []PromptItem
	Model  string
	Dir    string
	Wrap   Wrapper
}

// WorkRequest is the input of a work run.
type WorkRequest struct {
	Prompt         string
	Model          string
	Dir            string
	PermissionMode string // client-specific, e.g. Claude's acceptEdits; empty = client default
	AddDirs        []string
	// NoExec limits the run to file tools: sandboxed agents may read and
	// write their granted folders but not run commands.
	NoExec bool
	// Wrap, when set, wraps the client command (the bwrap sandbox).
	Wrap         Wrapper
	Rules        string
	RulesVersion int
}

// DirectRequest is the input of a direct mode run: a full interactive session
// in the agent's root directory.
type DirectRequest struct {
	Prompt               string
	Model                string
	Dir                  string
	SessionID            string
	Resume               bool
	ConversationID       string
	PermissionMode       string
	MCPConfigPath        string
	PermissionPromptTool string
	Env                  []string
	AddDirs              []string
	Rules                string
	RulesVersion         int
}

// Wrapper wraps a client command, e.g. in the bwrap sandbox.
type Wrapper func(name string, args []string) (string, []string, []string, error)

func (w Wrapper) command(name string, args []string) (string, []string, []string, error) {
	if w == nil {
		return name, args, nil, nil
	}
	return w(name, args)
}

// Usage counts tokens of one run.
type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cache_read"`
	CacheWrite int `json:"cache_write"`
}

// Total is every token the run consumed.
func (u Usage) Total() int { return u.Input + u.Output + u.CacheRead + u.CacheWrite }

// RunResult is what a client run produced.
type RunResult struct {
	Text           string   // final text (work runs) or raw JSON (decision runs)
	Decision       Decision // parsed decision (decision runs)
	Refs           map[string]PromptItem
	Usage          Usage
	Duration       time.Duration
	SessionID      string
	ConversationID string
	// Denied lists actions the client refused because a headless run
	// cannot ask for permission (Antigravity reports them).
	Denied []string
}

// Runner executes a command; tests replace it.
type Runner func(ctx context.Context, dir, name string, args []string, env []string) (stdout, stderr []byte, err error)

// ExecRunner runs a real process with stdin closed, killing its whole
// process group when ctx ends (emergency stop, pause, timeout).
func ExecRunner(ctx context.Context, dir, name string, args []string, env []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = env
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	setProcessGroup(cmd)
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.Bytes(), errb.Bytes(), ctx.Err()
	}
	if err != nil {
		return out.Bytes(), errb.Bytes(), fmt.Errorf("%s: %w: %s", name, err, clip(strings.TrimSpace(errb.String()), 300))
	}
	return out.Bytes(), errb.Bytes(), nil
}

// Factory builds an adapter for an agent's config.
type Factory func(cfg AgentConfig, run Runner) Adapter

// Adapters is the registry of client adapters by name.
var Adapters = map[string]Factory{
	"claude-code": func(cfg AgentConfig, run Runner) Adapter { return &ClaudeCode{Bin: cfg.Command, Run: run} },
	"antigravity": func(cfg AgentConfig, run Runner) Adapter { return &Antigravity{Bin: cfg.Command, Run: run} },
}

// AdapterNames lists the registered adapters.
func AdapterNames() []string {
	names := make([]string, 0, len(Adapters))
	for n := range Adapters {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// finishDecision parses the decision and keeps only actions on known refs.
func finishDecision(res RunResult, raw string, refs map[string]PromptItem) (RunResult, error) {
	d, err := ParseDecision(raw)
	if err != nil {
		return res, err
	}
	res.Refs = refs
	res.Text = raw
	for _, a := range d.Actions {
		if _, ok := refs[a.Ref]; ok {
			res.Decision.Actions = append(res.Decision.Actions, a)
		}
	}
	return res, nil
}

func structuredOrText(structured json.RawMessage, text string) string {
	if len(structured) > 0 && string(structured) != "null" {
		return string(structured)
	}
	return text
}
