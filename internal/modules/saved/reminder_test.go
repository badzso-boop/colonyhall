// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package saved

import (
	"testing"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/platform/ids"
)

// The reminder's system message carries no fixed word in any language:
// the ⏰ mark, the note and the link.
func TestReminderBodyIsLanguageNeutral(t *testing.T) {
	gen := &ids.Sequential{}
	m := conversation.Message{ID: gen.New(), ChannelID: gen.New()}
	cases := []struct {
		note string
		m    *conversation.Message
		want string
	}{
		{"", nil, "⏰"},
		{"call Anna", nil, "⏰ call Anna"},
		{"call Anna", &m, "⏰ call Anna\n" + MessageLink(m)},
		{"", &m, "⏰\n" + MessageLink(m)},
	}
	for _, c := range cases {
		if got := reminderBody(c.note, c.m); got != c.want {
			t.Errorf("reminderBody(%q, %v) = %q, want %q", c.note, c.m != nil, got, c.want)
		}
	}
}
