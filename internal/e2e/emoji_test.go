// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) emojiUsage(token string) apispec.EmojiUsage {
	e.t.Helper()
	var out apispec.EmojiUsage
	if s := e.do("GET", "/api/v1/me/emoji", token, nil, &out); s != http.StatusOK {
		e.t.Fatalf("emoji usage: %d", s)
	}
	return out
}

// Round 55, part C: the emoji picker's recent and frequent are per person,
// only for people, and only emoji are recorded.
func TestEmojiUsageIsPerPerson(t *testing.T) {
	e := newEnv(t)
	_, otherTok := e.createUser("mari", "Mari")

	if got := e.emojiUsage(server.DevAdminToken); len(got.Recent) != 0 || len(got.Frequent) != 0 {
		t.Fatalf("empty at first: %+v", got)
	}
	record := func(token, emoji string) int {
		return e.do("POST", "/api/v1/me/emoji", token, map[string]any{"emoji": emoji}, nil)
	}
	for _, em := range []string{"👍", "🎉", "👍", "🚀"} {
		if s := record(server.DevAdminToken, em); s != http.StatusNoContent {
			t.Fatalf("record %s: %d", em, s)
		}
		e.clock.Advance(time.Minute)
	}
	got := e.emojiUsage(server.DevAdminToken)
	if !slices.Equal(got.Recent, []string{"🚀", "👍", "🎉"}) || !slices.Equal(got.Frequent, []string{"👍", "🚀", "🎉"}) {
		t.Fatalf("usage: %+v", got)
	}

	// Another person never sees mine; an agent has none and no way in.
	if got := e.emojiUsage(otherTok); len(got.Recent) != 0 || len(got.Frequent) != 0 {
		t.Fatalf("another person sees my emoji: %+v", got)
	}
	if s := e.do("GET", "/api/v1/me/emoji", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent read: %d", s)
	}
	if s := record(server.DevAgentToken, "👍"); s != http.StatusForbidden {
		t.Fatalf("agent record: %d", s)
	}

	// Only emoji, as for reactions.
	for _, bad := range []string{"", "abc", "<b>", "👍 x", "😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀"} {
		if s := record(server.DevAdminToken, bad); s != http.StatusBadRequest {
			t.Fatalf("record %q: %d, want 400", bad, s)
		}
	}
	if got := e.emojiUsage(server.DevAdminToken); len(got.Recent) != 3 {
		t.Fatalf("a refused emoji was recorded: %+v", got)
	}
}
