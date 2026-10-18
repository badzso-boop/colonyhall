package guard

import (
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
	Bin string // default "agy"
	Run Runner
}

type agyOutput struct {
	Response       string          `json:"response"`
	Structured     json.RawMessage `json:"structured_output"`
	Status         string          `json:"status"`
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

// Work implements Adapter.
func (a *Antigravity) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	prompt := req.Prompt
	if req.Rules != "" {
		prompt = fmt.Sprintf("<colonyhall_rules version=\"%d\">\n%s\n</colonyhall_rules>\n\n", req.RulesVersion, req.Rules) + prompt
	}
	args := []string{"-p", prompt, "--output-format", "json"}
	mode := req.PermissionMode
	if req.NoExec {
		mode = "accept-edits" // sandboxed: file edits yes, nothing that bypasses permissions
	}
	args = append(args, agyPermissionArgs(mode)...)
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = withModel(args, req.Model)
	res, out, err := a.execAs(ctx, req.Dir, req.Wrap, args)
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
	args = append(args, "--output-format", "json")
	if req.Model != "" {
		args = withModel(args, req.Model)
	}
	res, out, err := a.execAs(ctx, req.Dir, nil, args)
	res.Text = out.Response
	res.ConversationID = out.ConversationID
	for _, d := range out.Denied {
		res.Denied = append(res.Denied, d.Action)
	}
	return res, err
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
		// A run can end in ERROR after it already produced its decision.
		if len(out.Structured) > 0 && string(out.Structured) != "null" {
			return res, out, nil
		}
		return res, out, fmt.Errorf("agy: run status %q: %s", out.Status, clip(strings.TrimSpace(out.Response), 200))
	}
	return res, out, nil
}
