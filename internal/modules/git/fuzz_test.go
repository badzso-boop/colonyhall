// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package git

import (
	"testing"

	"colonyhall/internal/modules/conversation"
)

// FuzzDefuseMentions: webhook text (PR titles, Gitea user names) mentions
// nobody after defusing, whatever it held (round 40).
func FuzzDefuseMentions(f *testing.F) {
	for _, s := range []string{"@claude fix it", "user @bob-1", "a@b", "@@x", "@\u200bx"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if hs := conversation.MentionedHandles(defuseMentions(s)); len(hs) != 0 {
			t.Fatalf("defused %q still mentions %v", s, hs)
		}
	})
}
