// Package agents is an agent's lifecycle (round 33, docs/01-terv.md
// "Agent életciklus"): an agent is configured once, as a whole, and never
// edited; deleting it archives it. The module ties together the actor
// (identity), the machine assignment and grants (hosts), the channel
// memberships (conversation) and its own instructions, which it stores.
package agents

import (
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
)

// MaxInstructions is the longest own instruction text, in characters.
const MaxInstructions = 8000

// Instructions are an agent's own extra system prompt, set at creation.
// Version is 1; it is there for the later version picker.
type Instructions struct {
	AgentID     ids.ID
	WorkspaceID ids.ID
	Version     int
	Text        string
	CreatedBy   ids.ID
	CreatedAt   time.Time
}

var (
	ErrForbidden           = errors.New("forbidden")
	ErrInstructionsTooLong = errors.New("the instructions are too long (at most 8000 characters)")
	ErrInstructionsSecret  = errors.New("the instructions look like they contain a secret (a token, key or password); secrets never go to an agent's prompt, use the colonyhall-exchange folder")
	ErrHostRequired        = errors.New("choose a machine, or create an API-key agent")
	ErrAPIKeyWithHost      = errors.New("an API-key agent has no machine, folders or grants")
	ErrHandleTaken         = errors.New("the handle belongs to an agent or person that is not deleted")
	ErrEffectiveTooLong    = errors.New("the shared rules and these instructions together are too long for the agent's prompt")
	ErrChannelNotFound     = errors.New("a chosen channel does not exist")
)

// MaxEffective caps the shared rules plus the own instructions: the text
// travels as --append-system-prompt, MCP instructions or a prompt head.
const MaxEffective = 16000

// secret matches token-like strings the server already redacts elsewhere
// (runs.Redact), a PEM private key, and NAME=value assignments of secrets.
var secret = regexp.MustCompile(`(?i)(col_|colh_|sk-ant-[a-z0-9]*-|sk-|AIza|ya29\.|ghp_|github_pat_|glpat-|xox[abp]-|AKIA)[A-Za-z0-9_\-\.]{8,}` +
	`|-----BEGIN [A-Z ]*PRIVATE KEY-----` +
	`|\b[A-Z0-9_]*(TOKEN|SECRET|PASSWORD|API_KEY)\s*[=:]\s*\S{6,}`)

// LooksSecret reports whether text seems to hold a secret.
func LooksSecret(text string) bool { return secret.MatchString(text) }

// ValidateInstructions checks an own instruction text.
func ValidateInstructions(text string) error {
	if utf8.RuneCountInString(text) > MaxInstructions {
		return ErrInstructionsTooLong
	}
	if LooksSecret(text) {
		return ErrInstructionsSecret
	}
	return nil
}

// Effective is what the agent receives: the shared rules, then its own
// instructions under a clear heading.
func Effective(rules, own, handle string) string {
	if own == "" {
		return rules
	}
	return rules + "\n\n## Instructions for @" + handle + "\n\n" + own
}
