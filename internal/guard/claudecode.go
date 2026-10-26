package guard

import (
	"bufio"
	"bytes"
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
	Bin       string // default "claude"
	Run       Runner
	StreamRun StreamRunner

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

type claudeStreamLine struct {
	Type      string               `json:"type"`
	Subtype   string               `json:"subtype"`
	Message   *claudeStreamMessage `json:"message"`
	Content   []claudeStreamBlock  `json:"content"`
	Result    string               `json:"result"`
	SessionID string               `json:"session_id"`
	IsError   bool                 `json:"is_error"`
	NumTurns  int                  `json:"num_turns"`
	Usage     struct {
		Input      int `json:"input_tokens"`
		Output     int `json:"output_tokens"`
		CacheRead  int `json:"cache_read_input_tokens"`
		CacheWrite int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
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
	if req.AllowWebRead {
		args = append(args, "--allowedTools", "mcp__colony-web__web_read")
		if req.MCPConfigPath != "" {
			args = append(args, "--mcp-config", req.MCPConfigPath, "--strict-mcp-config")
		}
	}
	args = withModel(args, req.Model)
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := c.execStream(ctx, req.Dir, req.Wrap, args, nil, baseDirs, req.OnStep)
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
	args = append(args, "--output-format", "stream-json", "--verbose")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := c.execStream(ctx, req.Dir, nil, args, req.Env, baseDirs, req.OnStep)
	res.Text = out.Result
	if out.SessionID != "" {
		res.SessionID = out.SessionID
	}
	return res, err
}

func (c *ClaudeCode) execStream(ctx context.Context, dir string, wrap Wrapper, args []string, extraEnv []string, baseDirs []string, onStep func(Step)) (RunResult, claudeOutput, error) {
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
		var sl claudeStreamLine
		if jerr := json.Unmarshal([]byte(line), &sl); jerr != nil {
			return
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
