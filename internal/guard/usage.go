package guard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
)

// The clients' own /usage commands answer in print mode without a model call
// (0 tokens, 0 turns), so asking them costs nothing from the limits they
// report. The guard asks every UsagePoll and puts the answer into its report.

const usageTimeout = 90 * time.Second

// claudeUsageArgs asks Claude Code for its usage without keeping a session
// and without starting the user's MCP servers.
var claudeUsageArgs = []string{"-p", "/usage", "--output-format", "json", "--no-session-persistence", "--strict-mcp-config"}

var agyUsageArgs = []string{"-p", "/usage", "--output-format", "json"}

var (
	claudeSessionRe = regexp.MustCompile(`(?m)^Current session:\s*(\d{1,3})% used(?:\s*·\s*resets\s+([^(\n]+?)\s*(?:\(([^)\n]+)\))?)?\s*$`)
	claudeWeekRe    = regexp.MustCompile(`(?m)^Current week \(all models\):\s*(\d{1,3})% used(?:\s*·\s*resets\s+([^(\n]+?)\s*(?:\(([^)\n]+)\))?)?\s*$`)
)

// parseClaudeUsage reads the two windows from the text Claude Code's /usage
// prints. A window it cannot read is left out.
func parseClaudeUsage(stdout []byte, now time.Time) (RateLimit, bool) {
	var out struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if json.Unmarshal(stdout, &out) != nil || out.IsError {
		return RateLimit{}, false
	}
	window := func(re *regexp.Regexp) *RateLimitWindow {
		m := re.FindStringSubmatch(out.Result)
		if m == nil {
			return nil
		}
		pct, err := strconv.Atoi(m[1])
		if err != nil || pct > 100 {
			return nil
		}
		w := &RateLimitWindow{Utilization: float64(pct) / 100, Status: "allowed"}
		if pct >= 100 {
			w.Status = "rejected"
		}
		if at, ok := parseClaudeReset(m[2], m[3], now); ok {
			w.ResetsAt = at
		}
		return w
	}
	rl := RateLimit{FiveHour: window(claudeSessionRe), SevenDay: window(claudeWeekRe)}
	return rl, rl.FiveHour != nil || rl.SevenDay != nil
}

// parseClaudeReset reads "Nov 4, 3:39am" or "3:39am" in the named zone: the
// year (or the day) is the next one that puts it within the coming 8 days.
func parseClaudeReset(text, zone string, now time.Time) (time.Time, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	loc := time.Local
	if zone != "" {
		if l, err := time.LoadLocation(strings.TrimSpace(zone)); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	for _, layout := range []string{"Jan 2, 3:04pm", "Jan 2, 3pm", "Jan 2 3:04pm", "Jan 2 3pm"} {
		t, err := time.ParseInLocation(layout, text, loc)
		if err != nil {
			continue
		}
		for _, y := range []int{local.Year(), local.Year() + 1} {
			c := time.Date(y, t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
			if !c.Before(now.Add(-time.Hour)) && c.Before(now.Add(8*24*time.Hour)) {
				return c, true
			}
		}
		return time.Time{}, false
	}
	for _, layout := range []string{"3:04pm", "3pm"} {
		t, err := time.ParseInLocation(layout, text, loc)
		if err != nil {
			continue
		}
		c := time.Date(local.Year(), local.Month(), local.Day(), t.Hour(), t.Minute(), 0, 0, loc)
		if c.Before(now) {
			c = c.AddDate(0, 0, 1)
		}
		return c, true
	}
	return time.Time{}, false
}

// parseAgyUsage reads the groups agy's /usage command returns.
func parseAgyUsage(stdout []byte) ([]apispec.AgyUsageGroup, bool) {
	var out struct {
		Status  string `json:"status"`
		Command struct {
			Name string `json:"name"`
			Data struct {
				Groups []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					Buckets     []struct {
						Window            string     `json:"window"`
						RemainingFraction *float64   `json:"remaining_fraction"`
						ResetTime         *time.Time `json:"reset_time"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"data"`
		} `json:"command"`
	}
	if json.Unmarshal(stdout, &out) != nil || out.Status != "SUCCESS" || out.Command.Name != "usage" {
		return nil, false
	}
	groups := []apispec.AgyUsageGroup{}
	for _, g := range out.Command.Data.Groups {
		if len(groups) == 10 || strings.TrimSpace(g.Name) == "" {
			continue
		}
		row := apispec.AgyUsageGroup{Name: clip(g.Name, 80)}
		if models := strings.TrimSpace(strings.TrimPrefix(g.Description, "Models within this group:")); models != "" {
			m := clip(models, 200)
			row.Models = &m
		}
		for _, b := range g.Buckets {
			if b.RemainingFraction == nil {
				continue
			}
			w := &apispec.AgyUsageWindow{Remaining: float32(min(max(*b.RemainingFraction, 0), 1)), ResetsAt: b.ResetTime}
			switch b.Window {
			case "5h":
				row.FiveHour = w
			case "weekly":
				row.Weekly = w
			}
		}
		groups = append(groups, row)
	}
	return groups, len(groups) > 0
}

// pollUsage asks the installed clients for their usage every UsagePoll.
func (s *Supervisor) pollUsage(ctx context.Context) {
	if s.Cfg.UsagePoll < 0 {
		return
	}
	every := s.Cfg.UsagePoll
	if every == 0 {
		every = 15 * time.Minute
	}
	// An empty folder of its own: the clients look for project settings there.
	dir := filepath.Join(s.stateDir(), "usage")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.Log.Warn("guard: usage folder", "err", err)
		return
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.checkUsage(ctx, dir)
		timer.Reset(every)
	}
}

func (s *Supervisor) clientBin(name string) string {
	if bin := s.Cfg.Commands[name]; bin != "" {
		return bin
	}
	return defaultCommands[name]
}

func (s *Supervisor) checkUsage(ctx context.Context, dir string) {
	run := s.Run
	if run == nil {
		run = ExecRunner
	}
	ask := func(name string, args []string) []byte {
		cctx, cancel := context.WithTimeout(ctx, usageTimeout)
		defer cancel()
		var env []string
		if tok := s.login.Token(); tok != "" && name == "claude-code" {
			env = append(os.Environ(), "CLAUDE_CODE_OAUTH_TOKEN="+tok)
		}
		stdout, _, err := run(cctx, dir, s.clientBin(name), args, env)
		if err != nil {
			s.Log.Debug("guard: usage check failed", "client", name, "err", err)
			return nil
		}
		return stdout
	}
	now := s.now()
	changed := false
	if out := ask("claude-code", claudeUsageArgs); out != nil {
		if rl, ok := parseClaudeUsage(out, now); ok && s.claudeLimiter != nil {
			s.claudeLimiter.Update(rl)
			changed = true
		}
	}
	if out := ask("antigravity", agyUsageArgs); out != nil {
		if groups, ok := parseAgyUsage(out); ok {
			s.mu.Lock()
			s.agyUsage = groups
			s.mu.Unlock()
			changed = true
		}
	}
	if changed {
		s.mu.Lock()
		s.usageCheckedAt = now
		s.mu.Unlock()
		s.report(ctx)
	}
}
