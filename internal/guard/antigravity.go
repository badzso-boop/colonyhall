// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Antigravity runs Google's Antigravity CLI (`agy -p`). Its built-in system
// prompt (~12k tokens) cannot be replaced, so a wake-up costs about 13k
// tokens; the rules go into the prompt and the answer is one JSON object.
type Antigravity struct {
	Bin       string // default "agy"
	Run       Runner
	StreamRun StreamRunner
}

type agyOutput struct {
	Response       string          `json:"response"`
	Structured     json.RawMessage `json:"structured_output"`
	Status         string          `json:"status"`
	Error          string          `json:"error"`
	ConversationID string          `json:"conversation_id"`
	Denied         []struct {
		Action string `json:"action"`
	} `json:"denied_actions"`
	Usage struct {
		Input     int `json:"input_tokens"`
		Output    int `json:"output_tokens"`
		Thinking  int `json:"thinking_tokens"`
		CacheRead int `json:"cache_read_tokens"`
	} `json:"usage"`
}

type agyStreamLine struct {
	Event           string          `json:"event"`
	ConversationID  string          `json:"conversation_id"`
	Model           string          `json:"model"`
	Cwd             string          `json:"cwd"`
	StepIndex       int             `json:"step_index"`
	State           string          `json:"state"`
	StepType        string          `json:"step_type"`
	ToolName        string          `json:"tool_name"`
	ToolInfo        *agyToolInfo    `json:"tool_info"`
	TextDelta       string          `json:"text_delta"`
	DurationSeconds float64         `json:"duration_seconds"`
	Usage           *agyStreamUsage `json:"usage"`
	Status          string          `json:"status"`
	Response        string          `json:"response"`
	Error           string          `json:"error"`
	DeniedActions   []struct {
		Action string `json:"action"`
	} `json:"denied_actions"`
	// agy (1.4.x) nests each event's fields under a key named after the
	// event: {"event":"result","result":{"status":...}}.
	StepUpdate *agyStreamLine `json:"step_update"`
	Result     *agyStreamLine `json:"result"`
}

type agyToolInfo struct {
	Parameters map[string]interface{} `json:"parameters"`
}

type agyStreamUsage struct {
	Input     int `json:"input_tokens"`
	Output    int `json:"output_tokens"`
	Thinking  int `json:"thinking_tokens"`
	CacheRead int `json:"cache_read_tokens"`
}

func (a *Antigravity) bin() string {
	if a.Bin == "" {
		return "agy"
	}
	return a.Bin
}

// Decide implements Adapter.
func (a *Antigravity) Decide(ctx context.Context, req DecideRequest) (RunResult, error) {
	prompt, refs := BuildPrompt(req.Handle, req.Items, true)
	prompt += "\nDo not call any tools; answer only with the JSON."
	args := []string{"-p", prompt, "--output-format", "json", "--json-schema", DecisionSchema, "--disable-slash-commands"}
	args = withModel(args, req.Model)
	res, out, err := a.execAs(ctx, req.Dir, req.Wrap, args)
	if err != nil {
		return res, err
	}
	return finishDecision(res, structuredOrText(out.Structured, out.Response), refs)
}

// Summarize implements Adapter.
func (a *Antigravity) Summarize(ctx context.Context, req SummarizeRequest) (RunResult, error) {
	prompt := req.Prompt + "\nDo not call any tools; answer only with the Markdown summary."
	args := []string{"-p", prompt, "--output-format", "json", "--disable-slash-commands"}
	args = withModel(args, req.Model)
	res, out, err := a.execAs(ctx, req.Dir, req.Wrap, args)
	if err != nil {
		return res, err
	}
	res.Text = out.Response
	return res, nil
}

// Work implements Adapter.
func (a *Antigravity) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	prompt := req.Prompt
	if req.Rules != "" {
		prompt = fmt.Sprintf("<colonyhall_rules version=\"%d\">\n%s\n</colonyhall_rules>\n\n", req.RulesVersion, req.Rules) + prompt
	}
	args := []string{"-p", prompt, "--output-format", "stream-json"}
	mode := req.PermissionMode
	if req.NoExec {
		mode = "accept-edits" // sandboxed: file edits yes, nothing that bypasses permissions
	}
	args = append(args, agyPermissionArgs(mode)...)
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = withModel(args, req.Model)
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := a.execStream(ctx, req.Dir, req.Wrap, args, baseDirs, req.OnStep, req.OnActivity)
	res.Text = out.Response
	return res, err
}

// Direct implements Adapter.
func (a *Antigravity) Direct(ctx context.Context, req DirectRequest) (RunResult, error) {
	prompt := req.Prompt
	if req.Rules != "" {
		prompt = fmt.Sprintf("<colonyhall_rules version=\"%d\">\n%s\n</colonyhall_rules>\n\n", req.RulesVersion, req.Rules) + prompt
	}
	args := []string{"-p", prompt}
	if req.ConversationID != "" {
		args = append(args, "--conversation", req.ConversationID)
	}
	args = append(args, agyPermissionArgs(req.PermissionMode)...)
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = append(args, "--output-format", "stream-json")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	baseDirs := append([]string{req.Dir}, req.AddDirs...)
	res, out, err := a.execStream(ctx, req.Dir, nil, args, baseDirs, req.OnStep, req.OnActivity)
	res.Text = out.Response
	res.ConversationID = out.ConversationID
	return res, err
}

func (a *Antigravity) execStream(ctx context.Context, dir string, wrap Wrapper, args []string, baseDirs []string, onStep func(Step), onActivity func()) (RunResult, agyOutput, error) {
	start := time.Now()
	var (
		name string
		env  []string
		err  error
	)
	if wrap != nil {
		name, args, env, err = wrap.command(a.bin(), args)
		if err != nil {
			return RunResult{}, agyOutput{}, fmt.Errorf("sandbox: %w", err)
		}
	} else {
		name = a.bin()
	}

	var (
		resultFound    bool
		out            agyOutput
		res            RunResult
		lastAgyContext int
	)

	onLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if onActivity != nil {
			onActivity()
		}
		var sl agyStreamLine
		if jerr := json.Unmarshal([]byte(line), &sl); jerr != nil {
			return
		}
		if inner := sl.StepUpdate; inner != nil && sl.Event == "step_update" {
			inner.Event = sl.Event
			sl = *inner
		} else if inner := sl.Result; inner != nil && sl.Event == "result" {
			inner.Event = sl.Event
			sl = *inner
		}
		if sl.Event == "" && (sl.Status != "" || sl.Response != "" || sl.ConversationID != "") {
			sl.Event = "result"
		}
		switch sl.Event {
		case "step_update":
			switch sl.StepType {
			case "tool":
				st := mapAgyTool(sl, baseDirs)
				if onStep != nil {
					onStep(st)
				}
			case "agent_response":
				if strings.ToUpper(sl.State) == "ACTIVE" {
					if onStep != nil {
						if sl.TextDelta != "" {
							onStep(Step{Kind: StepKindAnswer, Label: "Válaszol…", State: StepStateActive, StartedAt: time.Now()})
						} else {
							onStep(Step{Kind: StepKindThink, Label: "Gondolkodik…", State: StepStateActive, StartedAt: time.Now()})
						}
					}
				} else if strings.ToUpper(sl.State) == "DONE" {
					if sl.Usage != nil {
						lastAgyContext = sl.Usage.Input + sl.Usage.CacheRead
					}
					if onStep != nil {
						d := time.Duration(sl.DurationSeconds * float64(time.Second))
						onStep(Step{
							Kind:       StepKindAnswer,
							Label:      "Válaszol…",
							State:      StepStateDone,
							StartedAt:  time.Now(),
							Duration:   d,
							DurationMs: d.Milliseconds(),
						})
					}
				}
			}
		case "result":
			resultFound = true
			out.Status = sl.Status
			out.Response = sl.Response
			out.Error = sl.Error
			out.ConversationID = sl.ConversationID
			if sl.Usage != nil {
				out.Usage.Input = sl.Usage.Input
				out.Usage.Output = sl.Usage.Output
				out.Usage.Thinking = sl.Usage.Thinking
				out.Usage.CacheRead = sl.Usage.CacheRead
				res.Usage = Usage{
					Input:     sl.Usage.Input,
					Output:    sl.Usage.Output + sl.Usage.Thinking,
					CacheRead: sl.Usage.CacheRead,
				}
			}
			for _, d := range sl.DeniedActions {
				out.Denied = append(out.Denied, struct {
					Action string `json:"action"`
				}{Action: d.Action})
				res.Denied = append(res.Denied, d.Action)
			}
		}
	}

	streamer := a.StreamRun
	if streamer == nil && a.Run != nil {
		streamer = func(ctx context.Context, dir, name string, args, env []string, onStdoutLine func(string)) error {
			stdout, _, rerr := a.Run(ctx, dir, name, args, env)
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

	if !resultFound {
		if runErr != nil {
			return res, out, runErr
		}
		return res, out, fmt.Errorf("agy: unreadable output: missing result event")
	}

	if lastAgyContext > 0 {
		res.ContextTokens = lastAgyContext
	}

	if out.Status != "SUCCESS" {
		if len(out.Structured) > 0 && string(out.Structured) != "null" {
			return res, out, nil
		}
		if strings.TrimSpace(out.Response) != "" && out.Usage.Output > 0 {
			res.ClientWarning = fmt.Sprintf("agy: run status %q after an answer", out.Status)
			return res, out, nil
		}
		return res, out, &AgyRunError{Status: out.Status, Reason: out.Error, Response: out.Response}
	}
	return res, out, nil
}

func mapAgyTool(sl agyStreamLine, baseDirs []string) Step {
	now := time.Now()
	name := sl.ToolName
	params := make(map[string]interface{})
	if sl.ToolInfo != nil && sl.ToolInfo.Parameters != nil {
		params = sl.ToolInfo.Parameters
	}
	getString := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := params[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}

	state := StepStateActive
	if strings.ToUpper(sl.State) == "DONE" {
		state = StepStateDone
	}

	var d time.Duration
	var dms int64
	if sl.DurationSeconds > 0 {
		d = time.Duration(sl.DurationSeconds * float64(time.Second))
		dms = d.Milliseconds()
	}

	switch name {
	case "read_file":
		p := getString("Path", "path", "file_path")
		return Step{Kind: StepKindRead, Label: stepLabel(RelPath(p, baseDirs...)), Path: p, State: state, StartedAt: now, Duration: d, DurationMs: dms}
	case "write_file", "replace", "edit_file", "edit":
		p := getString("Path", "path", "file_path")
		kind := StepKindWrite
		if strings.Contains(name, "edit") || name == "replace" {
			kind = StepKindEdit
		}
		return Step{Kind: kind, Label: stepLabel(RelPath(p, baseDirs...)), Path: p, State: state, StartedAt: now, Duration: d, DurationMs: dms}
	case "run_command", "shell":
		cmd := getString("CommandLine", "command", "cmd")
		return Step{Kind: StepKindCommand, Label: Redact(cmd), State: state, StartedAt: now, Duration: d, DurationMs: dms}
	case "search_web", "web_search":
		q := getString("query", "q")
		return Step{Kind: StepKindSearch, Label: Redact(q), State: state, StartedAt: now, Duration: d, DurationMs: dms}
	case "search", "grep", "list_directory", "list", "find":
		q := getString("query", "pattern", "Path", "path")
		return Step{Kind: StepKindSearch, Label: Redact(q), State: state, StartedAt: now, Duration: d, DurationMs: dms}
	case "web_read":
		url := getString("url")
		return Step{Kind: StepKindWeb, Label: Redact(url), State: state, StartedAt: now, Duration: d, DurationMs: dms}
	default:
		return Step{Kind: StepKindOther, Label: Redact(name), State: state, StartedAt: now, Duration: d, DurationMs: dms}
	}
}

// agyPermissionArgs maps the agent's permission mode to agy flags. A
// headless agy run denies anything its allow list does not cover (even
// writing a file), so direct agents need at least accept-edits.
func agyPermissionArgs(mode string) []string {
	switch mode {
	case "auto", "accept-edits", "acceptEdits":
		return []string{"--mode", "accept-edits"}
	case "plan":
		return []string{"--mode", "plan"}
	case "bypassPermissions", "bypass":
		return []string{"--dangerously-skip-permissions"} // only when the admin chose it for this agent
	}
	return nil
}

func (a *Antigravity) execAs(ctx context.Context, dir string, wrap Wrapper, args []string) (RunResult, agyOutput, error) {
	start := time.Now()
	name, args, env, err := wrap.command(a.bin(), args)
	if err != nil {
		return RunResult{}, agyOutput{}, fmt.Errorf("sandbox: %w", err)
	}
	stdout, _, err := a.Run(ctx, dir, name, args, env)
	res := RunResult{Duration: time.Since(start)}
	var out agyOutput
	if jerr := json.Unmarshal(stdout, &out); jerr == nil {
		res.Usage = Usage{Input: out.Usage.Input, Output: out.Usage.Output + out.Usage.Thinking, CacheRead: out.Usage.CacheRead}
	} else if err == nil {
		return res, out, fmt.Errorf("agy: unreadable output: %w", jerr)
	}
	if err != nil {
		return res, out, err
	}
	if out.Status != "SUCCESS" {
		// A run can end in ERROR after it already produced its decision or
		// its answer (seen on long conversations: a full answer, then
		// status ERROR). The answer is kept; treating it as a failure threw
		// it away and retried the run, so the chat only saw "working".
		if len(out.Structured) > 0 && string(out.Structured) != "null" {
			return res, out, nil
		}
		// Only with evidence that the model wrote it: an error text in the
		// response field (paths, internals) must not reach the chat, where
		// error details never go.
		if strings.TrimSpace(out.Response) != "" && out.Usage.Output > 0 {
			res.ClientWarning = fmt.Sprintf("agy: run status %q after an answer", out.Status)
			return res, out, nil
		}
		return res, out, &AgyRunError{Status: out.Status, Reason: out.Error, Response: out.Response}
	}
	return res, out, nil
}
