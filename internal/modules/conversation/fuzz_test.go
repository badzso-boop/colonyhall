// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"strings"
	"testing"
)

// FuzzMentionedHandles: every handle found is a valid handle, written in
// the body after an @, and listed once (round 40).
func FuzzMentionedHandles(f *testing.F) {
	for _, s := range []string{"@bob hi", "mail@bob.hu", "@a", "@" + strings.Repeat("x", 40), "x @bob-1 @bob-1 @Bob", "@\u200bbob", "(@ab)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		seen := map[string]bool{}
		for _, h := range MentionedHandles(body) {
			if seen[h] {
				t.Fatalf("%q listed twice for %q", h, body)
			}
			seen[h] = true
			if len(h) < 2 || len(h) > 32 || h[0] == '-' || strings.Trim(h, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
				t.Fatalf("invalid handle %q from %q", h, body)
			}
			if !strings.Contains(body, "@"+h) {
				t.Fatalf("handle %q not in %q", h, body)
			}
		}
	})
}

// FuzzDefuseMentions: quoted text (a forward, round 55) never mentions
// anyone and never carries a chip reference, whatever it contains.
func FuzzDefuseMentions(f *testing.F) {
	for _, s := range []string{"@claude fix it", "a@b", "@@x", "[approval:x]", "[message:a/b] @all", "\u200b@x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := DefuseMentions(s)
		if got := MentionedHandles(out); len(got) != 0 {
			t.Fatalf("DefuseMentions(%q) still mentions %v", s, got)
		}
		for _, chip := range []string{"[approval:", "[error:", "[message:"} {
			if strings.Contains(out, chip) {
				t.Fatalf("DefuseMentions(%q) keeps %s", s, chip)
			}
		}
	})
}
