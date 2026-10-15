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
	for _, d := range req.AddDirs {
		args = append(args, "--add-dir", d)
	}
	args = withModel(args, req.Model)
	res, out, err := a.execAs(ctx, req.Dir, req, args)
	res.Text = out.Response
	return res, err
}

func (a *Antigravity) exec(ctx context.Context, dir string, args []string) (RunResult, agyOutput, error) {
	return a.execAs(ctx, dir, WorkRequest{}, args)
}

func (a *Antigravity) execAs(ctx context.Context, dir string, req WorkRequest, args []string) (RunResult, agyOutput, error) {
	start := time.Now()
	name, args := req.command(a.bin(), args)
	stdout, _, err := a.Run(ctx, dir, name, args)
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
