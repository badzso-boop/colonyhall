package mcptools

import (
	"strings"
	"testing"

	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/platform/ids"
)

// TestFormatInboxThumbsDown: the reactor must not look like the author of
// the agent's own message (a live Antigravity run got this wrong).
func TestFormatInboxThumbsDown(t *testing.T) {
	gen := &ids.Sequential{}
	msg := gen.New()
	out := FormatInbox("bot", []delivery.InboxItem{{
		Delivery:     delivery.Delivery{ID: gen.New(), ItemKind: delivery.ItemReactionDown, ItemID: msg},
		ChannelName:  "general",
		AuthorHandle: "norbi",
		Body:         "17 * 23 = 391",
	}}, 1)
	for _, want := range []string{"@norbi reacted 👎", "your message=" + msg.String(), `your message: "17 * 23 = 391"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
