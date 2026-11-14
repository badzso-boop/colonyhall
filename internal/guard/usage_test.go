// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real outputs (2026-11-03, Claude Code 2.1.317 and agy 1.4.12), trimmed.
const claudeUsageFixture = `{"is_error":false,"num_turns":0,"total_cost_usd":0,"subtype":"success","result":"You are currently using your subscription to power your Claude Code usage\n\nCurrent session: 13% used · resets Nov 4, 3:39am (Europe/Budapest)\nCurrent week (all models): 29% used · resets Nov 8, 11:59pm (Europe/Budapest)\n\nWhat's contributing to your usage?\nLast 24h · 289 requests · 17 sessions\n"}`

const agyUsageFixture = `{"conversation_id":"","status":"SUCCESS","response":"…","num_turns":0,"usage":{"total_tokens":0},"command":{"name":"usage","data":{"description":"Within each group, models share a weekly limit and a 5-hour limit.","groups":[{"name":"Gemini Models","description":"Models within this group: Gemini Flash, Gemini Pro","buckets":[{"id":"gemini-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":1,"reset_time":"2026-11-10T21:50:31Z"},{"id":"gemini-5h","name":"Five Hour Limit Remaining","window":"5h","remaining_fraction":1,"reset_time":"2026-11-04T02:50:31Z"}]},{"name":"Claude and GPT models","description":"Models within this group: Claude Opus, Claude Sonnet, GPT-OSS","buckets":[{"id":"3p-weekly","name":"Weekly Limit Remaining","window":"weekly","remaining_fraction":0.49,"reset_time":"2026-11-05T13:43:16Z"},{"id":"3p-5h","name":"Five Hour Limit Remaining","window":"5h","remaining_fraction":1,"reset_time":"2026-11-04T02:50:31Z"}]}]}}}`

func TestParseClaudeUsage(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Budapest")
	now := time.Date(2026, 11, 3, 23, 0, 0, 0, loc)
	rl, ok := parseClaudeUsage([]byte(claudeUsageFixture), now)
	if !ok || rl.FiveHour == nil || rl.SevenDay == nil {
		t.Fatalf("parse: %+v %v", rl, ok)
	}
	if rl.FiveHour.Utilization != 0.13 || !rl.FiveHour.ResetsAt.Equal(time.Date(2026, 11, 4, 3, 39, 0, 0, loc)) {
		t.Fatalf("session: %+v", rl.FiveHour)
	}
	if rl.SevenDay.Utilization != 0.29 || !rl.SevenDay.ResetsAt.Equal(time.Date(2026, 11, 8, 23, 59, 0, 0, loc)) {
		t.Fatalf("week: %+v", rl.SevenDay)
	}
	if rl.FiveHour.Status != "allowed" {
		t.Fatal(rl.FiveHour.Status)
	}
	full := strings.Replace(claudeUsageFixture, "13% used", "100% used", 1)
	if rl, _ := parseClaudeUsage([]byte(full), now); rl.FiveHour.Status != "rejected" {
		t.Fatal("a full window is rejected")
	}
	if _, ok := parseClaudeUsage([]byte(`{"is_error":true,"result":"Current session: 10% used"}`), now); ok {
		t.Fatal("an error result must not count")
	}
	if _, ok := parseClaudeUsage([]byte(`{"result":"something else entirely"}`), now); ok {
		t.Fatal("unknown text must not count")
	}
}

func TestParseClaudeReset(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Budapest")
	dec := time.Date(2026, 12, 30, 22, 0, 0, 0, loc)
	if at, ok := parseClaudeReset("Jan 2, 4pm", "Europe/Budapest", dec); !ok || at.Year() != 2027 {
		t.Fatalf("year rollover: %v %v", at, ok)
	}
	if at, ok := parseClaudeReset("3:40am", "Europe/Budapest", dec); !ok || !at.Equal(time.Date(2026, 12, 31, 3, 40, 0, 0, loc)) {
		t.Fatalf("time only: %v %v", at, ok)
	}
	if _, ok := parseClaudeReset("Mar 3, 4pm", "Europe/Budapest", dec); ok {
		t.Fatal("months away is not a usage reset")
	}
}

func TestParseAgyUsage(t *testing.T) {
	groups, ok := parseAgyUsage([]byte(agyUsageFixture))
	if !ok || len(groups) != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	g := groups[1]
	if g.Name != "Claude and GPT models" || g.Models == nil || *g.Models != "Claude Opus, Claude Sonnet, GPT-OSS" {
		t.Fatalf("group: %+v", g)
	}
	if g.Weekly == nil || g.Weekly.Remaining != 0.49 || g.FiveHour == nil || g.FiveHour.Remaining != 1 || g.Weekly.ResetsAt == nil {
		t.Fatalf("windows: %+v %+v", g.Weekly, g.FiveHour)
	}
	if _, ok := parseAgyUsage([]byte(`{"status":"ERROR"}`)); ok {
		t.Fatal("a failed run must not count")
	}
}

func TestCheckUsageUpdatesTheLimiterAndTheReport(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Budapest")
	now := time.Date(2026, 11, 3, 23, 0, 0, 0, loc)
	var calls []string
	s := &Supervisor{
		Log: slog.Default(),
		Now: func() time.Time { return now },
		Run: func(_ context.Context, _, name string, args []string, _ []string) ([]byte, []byte, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			if name == "claude" {
				return []byte(claudeUsageFixture), nil, nil
			}
			return []byte(agyUsageFixture), nil, nil
		},
	}
	s.claudeLimiter = NewClaudeLimiter(filepath.Join(t.TempDir(), "u.json"), 90, 80, nil, s.Now)
	s.checkUsage(context.Background(), t.TempDir())
	if len(calls) != 2 || !strings.Contains(calls[0], "--no-session-persistence") || !strings.Contains(calls[0], "--strict-mcp-config") {
		t.Fatalf("calls: %v", calls)
	}
	lim := s.claudeLimiter.ReportLimits()
	if lim.FiveHour == nil || lim.FiveHour.Utilization != 0.13 || lim.SevenDay == nil || lim.SevenDay.Utilization != 0.29 {
		t.Fatalf("limiter: %+v", lim)
	}
	if len(s.agyUsage) != 2 || !s.usageCheckedAt.Equal(now) {
		t.Fatalf("agy usage %d, checked %v", len(s.agyUsage), s.usageCheckedAt)
	}

	// A reading above the reserve pauses Claude before any run fails.
	high := strings.Replace(claudeUsageFixture, "13% used", "93% used", 1)
	s.Run = func(_ context.Context, _, name string, _ []string, _ []string) ([]byte, []byte, error) {
		if name == "claude" {
			return []byte(high), nil, nil
		}
		return nil, nil, context.DeadlineExceeded
	}
	s.checkUsage(context.Background(), t.TempDir())
	if !s.claudeLimiter.IsPaused(now) {
		t.Fatal("93% must pause at the 90% reserve")
	}
	if len(s.agyUsage) != 2 {
		t.Fatal("a failed agy check must keep the last reading")
	}
}
