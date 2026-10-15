package guard

import (
	"context"
	"encoding/json"
	"fmt"
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
	Response   string          `json:"response"`
	Structured json.RawMessage `json:"structured_output"`
	Status     string          `json:"status"`
	Denied     []struct {
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
	res, out, err := a.exec(ctx, req.Dir, args)
	if err != nil {
		return res, err
	}
	return finishDecision(res, structuredOrText(out.Structured, out.Response), refs)
}

// Work implements Adapter.
func (a *Antigravity) Work(ctx context.Context, req WorkRequest) (RunResult, error) {
	args := []string{"-p", req.Prompt, "--output-format", "json"}
	if req.PermissionMode != "" {
		args = append(args, "--mode", req.PermissionMode)
	}
	args = withModel(args, req.Model)
	res, out, err := a.exec(ctx, req.Dir, args)
	res.Text = out.Response
	return res, err
}

func (a *Antigravity) exec(ctx context.Context, dir string, args []string) (RunResult, agyOutput, error) {
	start := time.Now()
	stdout, _, err := a.Run(ctx, dir, a.bin(), args)
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
		return res, out, fmt.Errorf("agy: run status %q", out.Status)
	}
	return res, out, nil
}
