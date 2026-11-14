// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/server"
)

type schedOut struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Cron      string `json:"cron"`
	NextRunAt string `json:"next_run_at"`
	LastError string `json:"last_error"`
	RunCount  int    `json:"run_count"`
}

func (e *env) runDue() int {
	e.t.Helper()
	n, err := e.app.Schedule.RunDue(context.Background())
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// TestScheduledTaskReachesAgentAsHumans: at the given local time the
// message is posted in the creator's name, once, and the agent's inbox
// shows it as coming from a human (so a full-access agent may act on it).
func TestScheduledTaskReachesAgentAsHumans(t *testing.T) {
	e := newEnv(t) // clock: 2026-10-15 09:00 UTC = 11:00 Budapest
	general := e.generalID()
	if s := e.do("POST", "/api/v1/schedules", server.DevAgentToken,
		map[string]any{"channel_id": general, "body": "x", "at": "2026-10-16 01:00"}, nil); s != http.StatusForbidden {
		t.Fatalf("an agent schedules: %d", s)
	}
	var sc schedOut
	if s := e.do("POST", "/api/v1/schedules", server.DevAdminToken, map[string]any{"channel_id": general,
		"body": "@fake-agent read and analyse the logs, then write a detailed report", "at": "2026-10-16 01:00"}, &sc); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	if sc.NextRunAt != "2026-10-15T23:00:00Z" { // 01:00 Budapest (CEST) = 23:00 UTC
		t.Fatalf("local time not interpreted in Europe/Budapest: %s", sc.NextRunAt)
	}
	if n := e.runDue(); n != 0 {
		t.Fatal("ran too early")
	}
	e.clock.Advance(14*time.Hour + time.Minute)
	if n := e.runDue(); n != 1 {
		t.Fatalf("due run: %d", n)
	}
	if n := e.runDue(); n != 0 {
		t.Fatal("a one-off ran twice")
	}
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if !strings.HasPrefix(last.Body, "⏰ @fake-agent read") {
		t.Fatalf("posted: %+v", last)
	}
	var in struct {
		Items []struct {
			Author     string
			AuthorKind string `json:"author_kind"`
		}
	}
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if len(in.Items) != 1 || in.Items[0].Author != "norbi" || in.Items[0].AuthorKind != "human" {
		t.Fatalf("inbox: %+v", in.Items)
	}
	var list struct{ Schedules []schedOut }
	e.do("GET", "/api/v1/schedules", server.DevAdminToken, nil, &list)
	if len(list.Schedules) != 1 || list.Schedules[0].State != "done" || list.Schedules[0].RunCount != 1 {
		t.Fatalf("after the run: %+v", list.Schedules)
	}
}

// TestRepeatingAndMissedRuns: a daily schedule runs once per day, missed
// runs collapse into one (made up if not too late), and canceling stops it.
func TestRepeatingAndMissedRuns(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	var sc schedOut
	if s := e.do("POST", "/api/v1/schedules", server.DevAdminToken,
		map[string]any{"channel_id": general, "body": "daily check", "cron": "0 1 * * *"}, &sc); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	for _, bad := range []map[string]any{
		{"channel_id": general, "body": "x", "cron": "every day"},
		{"channel_id": general, "body": "x"},
		{"channel_id": general, "body": "x", "at": "2026-10-16 01:00", "cron": "0 1 * * *"},
		{"channel_id": general, "body": "x", "at": "2020-01-01 01:00"},
		{"channel_id": general, "body": "x", "at": "2026-10-16 01:00", "timezone": "Mars/Base"},
	} {
		if s := e.do("POST", "/api/v1/schedules", server.DevAdminToken, bad, nil); s != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, s)
		}
	}
	e.clock.Advance(15 * time.Hour) // past the first 01:00
	if n := e.runDue(); n != 1 {
		t.Fatalf("first day: %d", n)
	}
	e.clock.Advance(3 * 24 * time.Hour) // server "down" for three days
	if n := e.runDue(); n != 0 {
		t.Fatal("a run 3 days late must not be made up")
	}
	var list struct{ Schedules []schedOut }
	e.do("GET", "/api/v1/schedules", server.DevAdminToken, nil, &list)
	if !strings.Contains(list.Schedules[0].LastError, "missed") || list.Schedules[0].State != "active" {
		t.Fatalf("missed run: %+v", list.Schedules[0])
	}
	e.clock.Advance(24 * time.Hour)
	if n := e.runDue(); n != 1 {
		t.Fatalf("next day after the gap: %d", n)
	}
	if s := e.do("DELETE", "/api/v1/schedules/"+sc.ID, server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("an agent cancels: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/schedules/"+sc.ID, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("cancel: %d", s)
	}
	e.clock.Advance(24 * time.Hour)
	if n := e.runDue(); n != 0 {
		t.Fatal("a canceled schedule ran")
	}
}

// TestScheduleDelayedAndPausedCreator: a run a few hours late is made up
// and marked; a paused creator's schedule does not post.
func TestScheduleDelayedAndPausedCreator(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	e.do("POST", "/api/v1/schedules", server.DevAdminToken, map[string]any{"channel_id": general, "body": "late one", "at": "2026-10-15 12:00"}, nil)
	e.clock.Advance(4 * time.Hour) // due 10:00 UTC, now 13:00 UTC
	if n := e.runDue(); n != 1 {
		t.Fatalf("made up: %d", n)
	}
	msgs := e.messages(general)
	if last := msgs[len(msgs)-1]; last.Body != "⏰ (delayed) late one" {
		t.Fatalf("delayed: %q", last.Body)
	}

	_, humanToken := e.createActor("human", "anna", "member")
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", humanToken, nil, &me)
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": me.ID}, nil)
	if s := e.do("POST", "/api/v1/schedules", humanToken, map[string]any{"channel_id": general, "body": "from anna", "at": "2026-10-15 16:00"}, nil); s != http.StatusCreated {
		t.Fatalf("member schedules: %d", s)
	}
	e.do("PUT", "/api/v1/actors/"+me.ID+"/state", server.DevAdminToken, map[string]any{"state": "paused"}, nil)
	e.clock.Advance(2 * time.Hour)
	if n := e.runDue(); n != 0 {
		t.Fatal("a paused creator's schedule posted")
	}
	var list struct{ Schedules []schedOut }
	e.do("GET", "/api/v1/schedules", server.DevAdminToken, nil, &list)
	found := false
	for _, s := range list.Schedules {
		if s.LastError != "" && s.State == "done" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the failed run must be recorded: %+v", list.Schedules)
	}
}

type schedPreviewOut struct {
	Cron     string   `json:"cron"`
	Timezone string   `json:"timezone"`
	NextRuns []string `json:"next_runs"`
}

// TestSchedulePreview: the builder's preview uses the same rules as create,
// shows five runs in the schedule's time zone (DST included), refuses bad
// input, writes nothing, and is for humans only.
func TestSchedulePreview(t *testing.T) {
	e := newEnv(t)
	preview := func(token string, body map[string]any) (schedPreviewOut, int) {
		var out schedPreviewOut
		s := e.do("POST", "/api/v1/schedules/preview", token, body, &out)
		return out, s
	}

	// The clock is 2026-10-15 09:00 UTC (11:00 in Budapest): today's 09:00 is past.
	daily, s := preview(server.DevAdminToken, map[string]any{"cron": "0 9 * * *"})
	if s != http.StatusOK || daily.Timezone != "Europe/Budapest" || daily.Cron != "0 9 * * *" {
		t.Fatalf("daily: %d %+v", s, daily)
	}
	want := []string{"2026-10-16T07:00:00Z", "2026-10-17T07:00:00Z", "2026-10-18T07:00:00Z", "2026-10-19T07:00:00Z", "2026-10-20T07:00:00Z"}
	if strings.Join(daily.NextRuns, ",") != strings.Join(want, ",") {
		t.Fatalf("daily runs: %v", daily.NextRuns)
	}

	weekdays, _ := preview(server.DevAdminToken, map[string]any{"cron": "30 8 * * 1-5"})
	// Thu 15 (08:30 local already past), Fri 16, Mon 19, Tue 20, Wed 21, Thu 22.
	if weekdays.NextRuns[0] != "2026-10-16T06:30:00Z" || weekdays.NextRuns[1] != "2026-10-19T06:30:00Z" || len(weekdays.NextRuns) != 5 {
		t.Fatalf("weekdays: %v", weekdays.NextRuns)
	}

	// Across the end of summer time (Sunday 25 October): 09:00 local stays 09:00.
	e.clock.Advance(8 * 24 * time.Hour) // 2026-10-23 09:00 UTC
	dst, _ := preview(server.DevAdminToken, map[string]any{"cron": "0 9 * * *"})
	if dst.NextRuns[0] != "2026-10-24T07:00:00Z" || dst.NextRuns[1] != "2026-10-25T08:00:00Z" {
		t.Fatalf("DST: %v", dst.NextRuns)
	}

	oneOff, s := preview(server.DevAdminToken, map[string]any{"at": "2026-11-02 09:00"})
	if s != http.StatusOK || oneOff.Cron != "" || len(oneOff.NextRuns) != 1 || oneOff.NextRuns[0] != "2026-11-02T08:00:00Z" {
		t.Fatalf("one-off: %d %+v", s, oneOff)
	}

	for _, bad := range []map[string]any{
		{"cron": "61 * * * *"},
		{"cron": "0 9 * * *", "at": "2026-11-02 09:00"},
		{},
		{"cron": "0 9 * * *", "timezone": "Mars/Olympus"},
		{"at": "2020-01-01 09:00"},
	} {
		if _, s := preview(server.DevAdminToken, bad); s != http.StatusBadRequest {
			t.Fatalf("bad input %v: status %d, want 400", bad, s)
		}
	}
	if _, s := preview(server.DevAgentToken, map[string]any{"cron": "0 9 * * *"}); s != http.StatusForbidden {
		t.Fatalf("agent: status %d, want 403", s)
	}

	var list struct{ Schedules []schedOut }
	e.do("GET", "/api/v1/schedules", server.DevAdminToken, nil, &list)
	if len(list.Schedules) != 0 {
		t.Fatalf("preview stored a schedule: %+v", list.Schedules)
	}
}
