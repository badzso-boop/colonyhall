package guard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// ClaudeCode runs `claude -p`. The decision run replaces the system prompt,
// turns off every tool, MCP server and settings file, and keeps no session:
// a wake-up costs about 1.3k tokens instead of ~15k with the defaults.
type ClaudeCode struct {
	Bin         string // default "claude"
	Run         Runner
	StreamRun   StreamRunner
	OnRateLimit func(RateLimit)
	// Token gives the guard's Claude login token (round 38); a run outside
	// the sandbox gets it as CLAUDE_CODE_OAUTH_TOKEN in its own environment
	// only. nil or "": the user's own login.
	Token func() string

	helpMu                 sync.Mutex
	hasAppendPromptChecked bool
	hasAppendPrompt        bool
	helpText               string
	helpChecked            bool
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

type claudeStreamLine struct {
	Type          string               `json:"type"`
	Subtype       string               `json:"subtype"`
	Message       *claudeStreamMessage `json:"message"`
	Content       []claudeStreamBlock  `json:"content"`
	Result        string               `json:"result"`
	SessionID     string               `json:"session_id"`
	IsError       bool                 `json:"is_error"`
	NumTurns      int                  `json:"num_turns"`
	RateLimitInfo *claudeRateLimitInfo `json:"rate_limit_info,omitempty"`
	Usage         struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

type claudeRateLimitWindowInfo struct {
	Utilization *float64 `json:"utilization,omitempty"`
	ResetsAt    *int64   `json:"resetsAt,omitempty"`
	Status      string   `json:"status,omitempty"`
}

type claudeRateLimitInfo struct {
	Status         string                               `json:"status,omitempty"`
	ResetsAt       *int64                               `json:"resetsAt,omitempty"`
	RateLimitType  string                               `json:"rateLimitType,omitempty"`
	UnifiedWindows map[string]claudeRateLimitWindowInfo `json:"unifiedWindows,omitempty"`
}

type claudeStreamMessage struct {
	Role    string              `json:"role"`
	Content []claudeStreamBlock `json:"content"`
	Usage   *struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

type claudeStreamBlock struct {
	Type      string                 `json:"type"`
	ID        string                 `json:"id"`
	ToolUseID string                 `json:"tool_use_id"`
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	Text      string                 `json:"text"`
}

func (c *ClaudeCode) bin() string {
	if c.Bin == "" {
		return "claude"
	}
	return c.Bin
}

// help returns `claude --help` (cached; "" when it cannot be run).
func (c *ClaudeCode) help(ctx context.Context) string {
	c.helpMu.Lock()
	defer c.helpMu.Unlock()
	if c.helpChecked {
		return c.helpText
	}
	c.helpChecked = true
	if c.Run == nil {
		return ""
	}
	stdout, _, err := c.Run(ctx, "", c.bin(), []string{"--help"}, nil)
	if err == nil {
		c.helpText = string(stdout)
	}
	return c.helpText
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
	args := []string{"-p", req.Prompt, "--output-format", "stream-json", "--verbose", "--no-session-persistence"}
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
	var allowedTools []string
	if req.AllowWebRead {
		allowedTools = append(allowedTools, "mcp__colony-web__web_read")
	}
	if req.AllowPlanner {
		allowedTools = append(allowedTools, "mcp__colony-planner__planner_read", "mcp__colony-planner__calendar_read",
			"mcp__colony-planner__channel_info")
	}
	if req.CodeGraph {
		allowedTools = append(allowedTools, CodeGraphTool)
	}
	if len(allowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(allowedTools, ","))
		if req.MCPConfigPath != "" {
			args = append(args, "--mcp-config", req.MCPConfigPath, "--strict-mcp-config")
		}
	}
	args = withModel(args, req.Model)
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := c.execStream(ctx, req.Dir, req.Wrap, args, nil, baseDirs, req.OnStep, req.OnActivity, req.OnRateLimit)
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
	if req.DocsOnly {
		// Never the agent's own permission mode or a permission prompt
		// tool: a refused action must stay refused, not become a question.
		docs, err := c.docsOnlyArgs(ctx, req.Dir, req.MCPConfigPath, req.CodeGraph)
		if err != nil {
			return RunResult{}, err
		}
		args = append(args, docs...)
	} else {
		if req.PermissionMode != "" {
			args = append(args, "--permission-mode", req.PermissionMode)
		}
		if req.MCPConfigPath != "" {
			args = append(args, "--mcp-config", req.MCPConfigPath)
		}
		if req.CodeGraph {
			// Read-only tools; without the rule the agent's own mode could
			// turn every call into an approval question in the chat.
			args = append(args, "--allowedTools", CodeGraphTool)
		}
		if req.PermissionPromptTool != "" {
			args = append(args, "--permission-prompt-tool", req.PermissionPromptTool)
		}
	}
	args = append(args, "--output-format", "stream-json", "--verbose")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := c.execStream(ctx, req.Dir, nil, args, req.Env, baseDirs, req.OnStep, req.OnActivity, req.OnRateLimit)
	res.Text = out.Result
	if out.SessionID != "" {
		res.SessionID = out.SessionID
	}
	return res, err
}

func (c *ClaudeCode) execStream(ctx context.Context, dir string, wrap Wrapper, args []string, extraEnv []string, baseDirs []string, onStep func(Step), onActivity func(), onRateLimit ...func(RateLimit)) (RunResult, claudeOutput, error) {
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
		if tok := c.token(); tok != "" {
			extraEnv = append(slices.Clip(extraEnv), "CLAUDE_CODE_OAUTH_TOKEN="+tok)
		}
		if len(extraEnv) > 0 {
			env = os.Environ()
		}
	}
	if len(extraEnv) > 0 {
		env = append(env, extraEnv...)
	}

	var (
		resultLineFound   bool
		out               claudeOutput
		res               RunResult
		lastContextTokens int
		activeTools       = make(map[string]*Step)
	)

	onLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if onActivity != nil {
			onActivity()
		}
		var sl claudeStreamLine
		if jerr := json.Unmarshal([]byte(line), &sl); jerr != nil {
			return
		}
		if sl.Type == "rate_limit_event" || sl.RateLimitInfo != nil {
			rl := parseClaudeRateLimit(sl, line)
			res.RateLimit = &rl
			if c.OnRateLimit != nil {
				c.OnRateLimit(rl)
			}
			for _, cb := range onRateLimit {
				if cb != nil {
					cb(rl)
				}
			}
			if sl.Type == "rate_limit_event" {
				return
			}
		}
		if sl.Type == "result" || sl.IsError || sl.Result != "" || (sl.Type == "" && sl.SessionID != "") {
			resultLineFound = true
			out.Result = sl.Result
			out.SessionID = sl.SessionID
			out.IsError = sl.IsError
			out.Subtype = sl.Subtype
			out.NumTurns = sl.NumTurns
			out.Usage = sl.Usage
			res.SessionID = sl.SessionID
			res.Usage = Usage{
				Input:      sl.Usage.Input,
				Output:     sl.Usage.Output,
				CacheRead:  sl.Usage.CacheRead,
				CacheWrite: sl.Usage.CacheWrite,
			}
			return
		}
		if sl.Type == "assistant" {
			blocks := sl.Content
			if sl.Message != nil {
				if sl.Message.Usage != nil {
					u := sl.Message.Usage
					lastContextTokens = u.Input + u.CacheRead + u.CacheWrite
				}
				if len(sl.Message.Content) > 0 {
					blocks = sl.Message.Content
				}
			}
			for _, b := range blocks {
				switch b.Type {
				case "thinking":
					if onStep != nil {
						onStep(Step{Kind: StepKindThink, Label: "Gondolkodik…", State: StepStateActive, StartedAt: time.Now()})
					}
				case "text":
					if strings.TrimSpace(b.Text) != "" && onStep != nil {
						onStep(Step{Kind: StepKindAnswer, Label: "Válaszol…", State: StepStateActive, StartedAt: time.Now()})
					}
				case "tool_use":
					st := mapClaudeTool(b, baseDirs)
					if b.ID != "" {
						activeTools[b.ID] = &st
					}
					if onStep != nil {
						onStep(st)
					}
				}
			}
			return
		}
		if sl.Type == "user" {
			blocks := sl.Content
			if sl.Message != nil && len(sl.Message.Content) > 0 {
				blocks = sl.Message.Content
			}
			for _, b := range blocks {
				if b.Type == "tool_result" {
					toolID := b.ToolUseID
					if toolID == "" {
						toolID = b.ID
					}
					if st, ok := activeTools[toolID]; ok {
						st.State = StepStateDone
						st.Duration = time.Since(st.StartedAt)
						st.DurationMs = st.Duration.Milliseconds()
						if onStep != nil {
							onStep(*st)
						}
						delete(activeTools, toolID)
					}
				}
			}
			return
		}
		if sl.Type == "system" && sl.Subtype == "permission_denied" {
			if onStep != nil {
				onStep(Step{Kind: StepKindDenied, Label: Redact(sl.Result), State: StepStateDone, StartedAt: time.Now()})
			}
			return
		}
	}

	streamer := c.StreamRun
	if streamer == nil && c.Run != nil {
		streamer = func(ctx context.Context, dir, name string, args, env []string, onStdoutLine func(string)) error {
			stdout, _, rerr := c.Run(ctx, dir, name, args, env)
			if len(stdout) > 0 {
				reader := bufio.NewReader(bytes.NewReader(stdout))
				for {
					l, err := reader.ReadString('\n')
					l = strings.TrimRight(l, "\r\n")
					if len(l) > 0 && onStdoutLine != nil {
						onStdoutLine(l)
					}
					if err != nil {
						break
					}
				}
			}
			return rerr
		}
	}
	if streamer == nil {
		streamer = ExecStreamRunner
	}

	runErr := streamer(ctx, dir, name, args, env, onLine)
	res.Duration = time.Since(start)

	if !resultLineFound {
		if runErr != nil {
			return res, out, runErr
		}
		return res, out, fmt.Errorf("claude: unreadable output: missing result line")
	}
	if out.IsError {
		return res, out, fmt.Errorf("claude: run failed (%s): %s", out.Subtype, clip(out.Result, 300))
	}
	if lastContextTokens > 0 {
		res.ContextTokens = lastContextTokens
	} else {
		res.ContextTokens = claudeContextTokens(res.SessionID, res.Usage, out.NumTurns)
	}
	return res, out, nil
}

func mapClaudeTool(block claudeStreamBlock, baseDirs []string) Step {
	now := time.Now()
	name := block.Name
	input := block.Input
	getString := func(k string) string {
		if input == nil {
			return ""
		}
		if v, ok := input[k].(string); ok {
			return v
		}
		return ""
	}

	switch name {
	case "Read":
		p := getString("file_path")
		return Step{Kind: StepKindRead, Label: RelPath(p, baseDirs...), Path: p, State: StepStateActive, StartedAt: now}
	case "Write":
		p := getString("file_path")
		return Step{Kind: StepKindWrite, Label: RelPath(p, baseDirs...), Path: p, State: StepStateActive, StartedAt: now}
	case "Edit":
		p := getString("file_path")
		return Step{Kind: StepKindEdit, Label: RelPath(p, baseDirs...), Path: p, State: StepStateActive, StartedAt: now}
	case "Bash":
		cmd := getString("command")
		return Step{Kind: StepKindCommand, Label: Redact(cmd), State: StepStateActive, StartedAt: now}
	case "Glob", "Grep":
		pattern := getString("pattern")
		if pattern == "" {
			pattern = getString("path")
		}
		return Step{Kind: StepKindSearch, Label: Redact(pattern), State: StepStateActive, StartedAt: now}
	case "WebFetch":
		url := getString("url")
		return Step{Kind: StepKindWeb, Label: Redact(url), State: StepStateActive, StartedAt: now}
	default:
		if name == "mcp__colony-web__web_read" {
			url := getString("url")
			return Step{Kind: StepKindWeb, Label: Redact(url), State: StepStateActive, StartedAt: now}
		}
		if strings.HasPrefix(name, "mcp__") {
			parts := strings.Split(name, "__")
			tool := name
			if len(parts) >= 3 {
				tool = parts[1] + "." + parts[2]
			}
			return Step{Kind: StepKindMCP, Label: tool, State: StepStateActive, StartedAt: now}
		}
		return Step{Kind: StepKindOther, Label: Redact(name), State: StepStateActive, StartedAt: now}
	}
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
		if tok := c.token(); tok != "" {
			extraEnv = append(slices.Clip(extraEnv), "CLAUDE_CODE_OAUTH_TOKEN="+tok)
		}
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

func parseClaudeRateLimit(sl claudeStreamLine, raw string) RateLimit {
	rl := RateLimit{
		Raw: raw,
	}
	if sl.RateLimitInfo == nil {
		return rl
	}
	info := sl.RateLimitInfo
	rl.Status = info.Status
	if info.ResetsAt != nil && *info.ResetsAt > 0 {
		rl.ResetsAt = time.Unix(*info.ResetsAt, 0)
	}
	if info.UnifiedWindows != nil {
		if fh, ok := info.UnifiedWindows["five_hour"]; ok {
			w := &RateLimitWindow{
				Status: fh.Status,
			}
			if fh.Utilization != nil {
				w.Utilization = *fh.Utilization
			}
			if fh.ResetsAt != nil && *fh.ResetsAt > 0 {
				w.ResetsAt = time.Unix(*fh.ResetsAt, 0)
			}
			rl.FiveHour = w
		}
		if sd, ok := info.UnifiedWindows["seven_day"]; ok {
			w := &RateLimitWindow{
				Status: sd.Status,
			}
			if sd.Utilization != nil {
				w.Utilization = *sd.Utilization
			}
			if sd.ResetsAt != nil && *sd.ResetsAt > 0 {
				w.ResetsAt = time.Unix(*sd.ResetsAt, 0)
			}
			rl.SevenDay = w
		}
	}
	if rl.ResetsAt.IsZero() && rl.FiveHour != nil && !rl.FiveHour.ResetsAt.IsZero() {
		rl.ResetsAt = rl.FiveHour.ResetsAt
	}
	return rl
}

// DocsOnlyTools are the only built-in tools of a docs-only planner: read
// and search, and the two file-writing tools, which its permission rules
// confine to Markdown. No Bash, NotebookEdit, Agent or web tools.
const DocsOnlyTools = "Read,Write,Edit,Glob,Grep"

// docsOnlyDenied are denied by name as well, although --tools and
// --restricted already leave them out: a deny rule given as a flag cannot
// be dropped like a settings file that fails validation.
const docsOnlyDenied = "Bash,NotebookEdit,PowerShell,Agent,WebFetch,WebSearch"

// docsOnlyFlags must all be in `claude --help`: older clients do not know
// them, and a docs-only planner never runs without them.
var docsOnlyFlags = []string{"--restricted", "--permission-prompts", "dontAsk", "--settings", "--strict-mcp-config"}

// ErrDocsOnlyUnsupported: the installed claude lacks a flag the docs-only
// restriction needs; the run is refused rather than started unrestricted.
var ErrDocsOnlyUnsupported = errors.New("claude: this Claude Code version cannot enforce a docs-only agent (needs --restricted, --permission-prompts and --permission-mode dontAsk; update Claude Code)")

// ErrDocsOnlyRoot: the root folder cannot be written into a permission
// rule exactly (a glob or separator character would widen or break it).
var ErrDocsOnlyRoot = errors.New("claude: a docs-only agent's root folder must be an absolute path without spaces, commas, quotes, brackets, parentheses or glob characters")

// docsOnlyArgs are the Claude Code arguments of a docs-only planner
// (round 38). Every layer fails closed on its own:
//
//   - --restricted removes the command-running tools, ignores the user,
//     project and local settings files (so no allow rule or hook written
//     there by anyone, e.g. the sandboxed partner into the project's
//     .claude folder, applies) and confines the file tools to the working
//     directory; --tools leaves only DocsOnlyTools; --disallowedTools
//     denies the command, notebook, sub-agent and web tools by name.
//   - --permission-mode dontAsk denies whatever is not pre-approved by an
//     allow rule, and --permission-prompts none denies anything that
//     would still prompt. No --permission-prompt-tool is ever passed: a
//     refused action is a refusal, not an approval question in the chat.
//     The agent's configured mode (auto by default) is not used.
//   - The allow rules (in --settings, a session-only JSON string, so a
//     path is never split like a flag list) pre-approve only
//     Edit/Write of **/*.md under the root, the Colonyhall MCP tools and,
//     when the run has it, CodeGraph's read-only tools (round 39); the MCP
//     rules are also given as --allowedTools.
//
// Precedence relied on: Claude Code evaluates deny rules first, then ask,
// then allow (deny > ask > allow, across all sources). There is no deny
// or ask rule on Edit or Write here, so nothing overrides the .md allow;
// a write to any other file matches no allow rule and, in dontAsk mode
// with prompts off, is denied. Should --settings be ignored (Claude Code
// silently skips settings that fail validation in -p mode), the .md
// writes are refused too, and every other limit still holds.
func (c *ClaudeCode) docsOnlyArgs(ctx context.Context, dir, mcpConfig string, codeGraph bool) ([]string, error) {
	help := c.help(ctx)
	for _, f := range docsOnlyFlags {
		if !strings.Contains(help, f) {
			return nil, ErrDocsOnlyUnsupported
		}
	}
	mcpAllow := []string{"mcp__colonyhall"}
	if codeGraph {
		mcpAllow = append(mcpAllow, CodeGraphTool)
	}
	settings, err := DocsOnlySettings(dir, mcpAllow[1:]...)
	if err != nil {
		return nil, err
	}
	args := []string{
		"--restricted",
		"--permission-mode", "dontAsk",
		"--permission-prompts", "none",
		"--tools", DocsOnlyTools,
		"--disallowedTools", docsOnlyDenied,
		"--allowedTools", strings.Join(mcpAllow, ","),
		"--settings", settings,
		"--strict-mcp-config",
	}
	if mcpConfig != "" {
		args = append(args, "--mcp-config", mcpConfig)
	}
	return args, nil
}

// DocsOnlySettings is the session-only settings JSON of a docs-only
// planner whose root folder is dir (see docsOnlyArgs).
func DocsOnlySettings(dir string, extraAllow ...string) (string, error) {
	roots := []string{dir}
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		roots = append(roots, real) // the rule must match the resolved path too
	}
	allow := []string{}
	for _, r := range roots {
		if !filepath.IsAbs(r) || filepath.Clean(r) != r || r == "/" || strings.ContainsAny(r, " \t\r\n,()[]{}*?!\"'\\`$") {
			return "", ErrDocsOnlyRoot
		}
		// "//" starts an absolute path in a Claude Code permission rule.
		allow = append(allow, "Edit(/"+r+"/**/*.md)", "Write(/"+r+"/**/*.md)")
	}
	allow = append(allow, "mcp__colonyhall")
	allow = append(allow, extraAllow...)
	doc := map[string]any{
		"permissions": map[string]any{
			"defaultMode":                  "dontAsk",
			"allow":                        allow,
			"deny":                         strings.Split(docsOnlyDenied, ","),
			"disableBypassPermissionsMode": "disable",
		},
		"disableAllHooks": true,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *ClaudeCode) token() string {
	if c.Token == nil {
		return ""
	}
	return c.Token()
}
