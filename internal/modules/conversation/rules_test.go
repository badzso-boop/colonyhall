package conversation

import (
	"errors"
	"strings"
	"testing"
)

func TestIsAcknowledgement(t *testing.T) {
	for _, s := range []string{"ok", "OK!", "Köszi.", "thanks", "@norbi ok", "👍", "Got it.", "  rendben  ", "OK, thanks!", "Köszi, rendben.", "@norbi got it, will do"} {
		if !IsAcknowledgement(s) {
			t.Errorf("%q should be an acknowledgement", s)
		}
	}
	for _, s := range []string{"ok, the PR is merged", "thanks, but the test fails", "done: #42", "pong", "ok?? why", "ok then I merge"} {
		if IsAcknowledgement(s) {
			t.Errorf("%q carries information and must be posted", s)
		}
	}
}

func TestAgentMessageRules(t *testing.T) {
	if err := checkAgentMessage("short", 1, false); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", AgentMaxBody+1)
	if err := checkAgentMessage(long, 0, false); !errors.Is(err, ErrAgentTooLong) {
		t.Fatalf("long message: %v", err)
	}
	if err := checkAgentMessage(long, 0, true); err != nil {
		t.Fatalf("elaborate allows long messages: %v", err)
	}
	if err := checkAgentMessage("hi", AgentMaxMentions+1, false); !errors.Is(err, ErrTooManyMentions) {
		t.Fatalf("mentions: %v", err)
	}
	if err := checkAgentMessage("@here deploy done", 0, false); !errors.Is(err, ErrGroupMention) {
		t.Fatalf("group mention: %v", err)
	}
	if err := checkAgentMessage("ping @allen", 0, false); err != nil {
		t.Fatalf("@allen is a handle, not a group mention: %v", err)
	}
}

func TestValidEmoji(t *testing.T) {
	for _, e := range []string{"👍", "👎", "🎉", "1️⃣", "🇭🇺", "👩‍💻", "✅"} {
		if !validEmoji(e) {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range []string{"", "<b>", "ok", "1", "a👍", "👍 ", "é", strings.Repeat("👍", MaxEmojiLen+1)} {
		if validEmoji(e) {
			t.Errorf("%q should be invalid", e)
		}
	}
}
