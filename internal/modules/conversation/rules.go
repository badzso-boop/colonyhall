// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Server-side enforcement of the shared agent rules ("Közös agent-utasítás"):
// whatever the model does, agents cannot flood channels.
const (
	AgentMaxBody         = 600 // characters, unless elaborate is set
	AgentMaxMentions     = 3
	AgentRateLimitCount  = 20
	AgentRateLimitWindow = 10 * time.Minute
)

// Agent rule errors.
var (
	ErrAgentTooLong = fmt.Errorf("agent messages are limited to %d characters; shorten it, put details in a file, "+
		"or set elaborate only when someone asked you to elaborate", AgentMaxBody)
	ErrTooManyMentions    = fmt.Errorf("agents may mention at most %d people per message; mention only who must act", AgentMaxMentions)
	ErrGroupMention       = errors.New("agents may not use group mentions (@here, @all, @channel, @everyone)")
	ErrNothingToAck       = errors.New("acknowledgements are not posted as messages and there is nothing to react to; react to a specific message instead")
	ErrConversationPaused = errors.New("this conversation is paused: too many agent messages; a human has to reply")
)

// groupMention matches @here, @all, @channel and @everyone as whole words
// (so a handle like @allen is not caught).
var groupMention = regexp.MustCompile(`(?i)(^|[^a-z0-9_-])@(here|all|channel|everyone)($|[^a-z0-9_-])`)

// acknowledgements are messages that carry no information. Agents send a
// 👍 reaction instead; the server converts these automatically.
var acknowledgements = map[string]bool{
	"ok": true, "oke": true, "okay": true, "okey": true, "oké": true, "okés": true, "rendben": true,
	"köszi": true, "kösz": true, "köszönöm": true, "köszönöm szépen": true, "köszi szépen": true,
	"thanks": true, "thank you": true, "thx": true, "ty": true, "noted": true, "got it": true,
	"understood": true, "értem": true, "megvan": true, "ack": true, "acknowledged": true,
	"+1": true, "👍": true, "✅": true, "🙏": true, "sure": true, "will do": true, "persze": true,
}

// IsAcknowledgement reports whether body is only an acknowledgement, possibly
// several of them with punctuation and a leading mention: "@norbi OK, thanks!".
func IsAcknowledgement(body string) bool {
	norm := strings.ToLower(strings.TrimSpace(body))
	if len(norm) > 40 {
		return false
	}
	if acknowledgements[norm] {
		return true
	}
	words := strings.Fields(norm)
	if len(words) > 0 && strings.HasPrefix(words[0], "@") {
		words = words[1:]
	}
	words = strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) && r != '+' {
			return ' '
		}
		return r
	}, strings.Join(words, " ")))
	if len(words) == 0 {
		return false
	}
	for i := 0; i < len(words); {
		switch {
		case i+1 < len(words) && acknowledgements[words[i]+" "+words[i+1]]:
			i += 2
		case acknowledgements[words[i]]:
			i++
		default:
			return false
		}
	}
	return true
}

// checkAgentMessage applies the length and mention rules to an agent's message.
func checkAgentMessage(body string, mentions int, elaborate bool) error {
	if groupMention.MatchString(body) {
		return ErrGroupMention
	}
	if mentions > AgentMaxMentions {
		return ErrTooManyMentions
	}
	if !elaborate && len([]rune(body)) > AgentMaxBody {
		return ErrAgentTooLong
	}
	return nil
}
