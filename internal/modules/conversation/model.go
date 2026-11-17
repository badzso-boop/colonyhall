// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package conversation owns channels, memberships and messages.
package conversation

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

// ChannelKind is the type of a channel.
type ChannelKind string

// Channel kinds.
const (
	KindPublic  ChannelKind = "public"
	KindPrivate ChannelKind = "private"
	KindDM      ChannelKind = "dm"
)

// Channel is a conversation space.
type Channel struct {
	ID                     ids.ID
	WorkspaceID            ids.ID
	Kind                   ChannelKind
	Name                   string
	Description            string
	CompactThresholdTokens *int
	MsgCount               int64 // total messages, for unread badges
	CreatedBy              ids.ID
	CreatedAt              time.Time
	// ArchivedAt marks a read-only channel; ArchivedBy is the admin who did it.
	ArchivedAt *time.Time
	ArchivedBy *ids.ID
	// AgentLockedAt marks a channel where agents cannot write and are not
	// woken; humans keep talking. AgentLockedBy is the admin who did it.
	AgentLockedAt *time.Time
	AgentLockedBy *ids.ID
}

// Archived reports whether the channel is read-only.
func (c Channel) Archived() bool { return c.ArchivedAt != nil }

// AgentLocked reports whether the channel is locked against agents.
func (c Channel) AgentLocked() bool { return c.AgentLockedAt != nil }

// Member is an actor's membership in a channel. MsgCount is how many of the
// channel's messages the member has seen; unread = Channel.MsgCount - MsgCount
// (Mattermost ChannelMember pattern).
type Member struct {
	ChannelID    ids.ID
	ActorID      ids.ID
	CanWrite     bool
	MsgCount     int64
	MentionCount int64
	JoinedAt     time.Time
	LeftAt       *time.Time
}

// ChannelView is a channel as seen by one member, with unread counts.
type ChannelView struct {
	Channel
	Unread   int64
	Mentions int64
	IsMember bool
	// MemberIDs is filled for DM channels, so clients can show who is in it.
	MemberIDs []ids.ID
	// Muted is the viewer's own mute; MutedUntil nil while muted means forever.
	Muted      bool
	MutedUntil *time.Time
	// NotifyLevel is the viewer's effective notification level (round 55):
	// their choice, NotifyNone while muted, or the default for the kind.
	NotifyLevel NotifyLevel
}

// NotifyLevel decides what of a conversation counts for one person: the
// unread badge, the activity view and (round 43) push.
type NotifyLevel string

// Notification levels.
const (
	NotifyAll      NotifyLevel = "all"
	NotifyMentions NotifyLevel = "mentions"
	NotifyNone     NotifyLevel = "none" // muted
)

// DefaultNotifyLevel: every message of a DM, only mentions in a channel.
func DefaultNotifyLevel(kind ChannelKind) NotifyLevel {
	if kind == KindDM {
		return NotifyAll
	}
	return NotifyMentions
}

// ValidNotifyLevel reports whether l is a known level.
func ValidNotifyLevel(l NotifyLevel) bool {
	return l == NotifyAll || l == NotifyMentions || l == NotifyNone
}

// Mute is one viewer's notification setting for a channel (round 34's mute,
// round 55's level). Level NotifyNone (or empty) is a mute: Until nil means
// forever, and an expired mute counts as unmuted (the default level). Other
// levels have no Until.
type Mute struct {
	ActorID   ids.ID
	ChannelID ids.ID
	Until     *time.Time
	CreatedAt time.Time
	Level     NotifyLevel
}

// Active reports whether the mute is in force at now.
func (m Mute) Active(now time.Time) bool {
	if m.Level != "" && m.Level != NotifyNone {
		return false
	}
	return m.Until == nil || m.Until.After(now)
}

// LevelAt is the level this setting gives at now, or "" for the default.
func (m Mute) LevelAt(now time.Time) NotifyLevel {
	if m.Level == NotifyAll || m.Level == NotifyMentions {
		return m.Level
	}
	if m.Active(now) {
		return NotifyNone
	}
	return ""
}

// ThreadSummary is one thread the viewer takes part in, for "Szálaim".
type ThreadSummary struct {
	Root          Message
	Channel       Channel // filled by the service
	LastReadAt    *time.Time
	UnreadReplies int64
	// Mentioned: the viewer is mentioned in the root or a reply.
	Mentioned     bool
	LatestReplies []Message
}

// ThreadCursor pages "Szálaim" by (last reply time, root id), newest first.
type ThreadCursor struct {
	LastReplyAt time.Time
	RootID      ids.ID
}

// Reaction is one actor's emoji on a message. Removal is a soft mark.
type Reaction struct {
	MessageID ids.ID
	ActorID   ids.ID
	Emoji     string
	CreatedAt time.Time
}

// ReactionSummary groups a message's reactions by emoji.
type ReactionSummary struct {
	Emoji    string
	Count    int
	ActorIDs []ids.ID
}

// Attachment represents file metadata attached to a chat message.
type Attachment struct {
	ID   ids.ID `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
	// Deleted: the uploader deleted the file (round 54); the name, type and
	// size are blank then, so the message shows only "File deleted".
	Deleted bool `json:"deleted,omitempty"`
}

// Message is one chat message. Rows are append-only.
type Message struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID // ids.Nil for a top-level message
	AuthorID     ids.ID
	ClientMsgID  string // set by the client for idempotent sends; may be empty
	Kind         string // "text" or "system"
	Body         string
	CompactionID ids.ID // set for compaction system messages
	Mentions     []ids.ID
	ReplyCount   int
	LastReplyAt  *time.Time
	// ReplyParticipantIDs: up to MaxReplyParticipants reply authors, most
	// recent first. Filled on read for thread roots, never stored.
	ReplyParticipantIDs []ids.ID
	CreatedAt           time.Time
	DeletedAt           *time.Time
	DeletedBy           *ids.ID
	OriginChannelID     ids.ID // origin channel if sent by agent with an origin
	OriginChannel       string // channel name, filled on reads
	OriginThreadID      ids.ID // origin thread root if sent by agent with an origin
	// AnswersMessageID is the DM reply an agent's answer in its origin
	// thread responds to (set by the guard, checked on send).
	AnswersMessageID ids.ID
	// Important: an agent marked it (MCP send important: true); such a
	// message is pinned when sent (round 55, part D).
	Important bool
	// AlsoInChannel: a thread reply also shown in the channel's main stream
	// (round 55, part G). Delivery is unchanged: channel agents wake only
	// when mentioned.
	AlsoInChannel bool
	// Blocks make it an interactive message (round 56c); nil: plain.
	Blocks []Block
	// ForwardedFromID is the message a forward quotes (round 55, part F);
	// cleared on reads for viewers who cannot read the source.
	ForwardedFromID  ids.ID
	Pinned           bool              // filled on reads
	AnswersChannelID ids.ID            // filled on reads
	AnswersAuthorID  ids.ID            // filled on reads
	Reactions        []ReactionSummary // filled on reads
	Attachments      []Attachment      // filled on reads
	LinkCards        []LinkCard        // filled on reads
}

// LinkCard is unfurled metadata for a link in a message.
type LinkCard struct {
	URL    string
	Kind   string
	Title  string
	State  string
	Author string
}

// LinkCardProvider provides link cards for messages.
type LinkCardProvider interface {
	LinkCardsForMessages(ctx context.Context, messages []Message) (map[ids.ID][]LinkCard, error)
}

// Pin is a message pinned in its conversation. Unpinning marks the row.
type Pin struct {
	ID          ids.ID
	WorkspaceID ids.ID
	ChannelID   ids.ID
	MessageID   ids.ID
	PinnedBy    ids.ID
	PinnedAt    time.Time
	UnpinnedBy  *ids.ID
	UnpinnedAt  *time.Time
}

// PinnedMessage is a pin with its message, for the conversation's list.
type PinnedMessage struct {
	Pin     Pin
	Message Message
}

// Limits.
const (
	// MaxPins bounds the pinned messages of one conversation.
	MaxPins = 50
	// AgentPinsPerDay bounds how many pins one agent makes in one
	// conversation in 24 hours, so an agent cannot flood the list.
	AgentPinsPerDay = 5

	MaxBodyLen      = 20000
	MaxClientMsgID  = 64
	DefaultPageSize = 50
	MaxPageSize     = 200
	// MaxReplyParticipants: reply authors shown on a thread's summary row.
	MaxReplyParticipants = 3
)

// Errors returned by the conversation service.
var (
	ErrForbidden   = errors.New("not allowed in this channel")
	ErrInvalidName = errors.New("channel name must be 2-32 characters: lowercase letters, digits and '-'")
	// ErrBadChannelKind: only public and private channels are created by
	// name; DMs come from the DM endpoint (round 40).
	ErrBadChannelKind     = errors.New("kind must be public or private")
	ErrInvalidDescription = errors.New("channel description must be at most 250 characters")
	ErrLastAdmin          = errors.New("cannot remove the last human admin of a private channel")
	ErrMemberNotFound     = errors.New("member not found")
	ErrEmptyBody          = errors.New("message is empty")
	ErrBodyTooLong        = errors.New("message is too long")
	ErrInvalidClientID    = errors.New("client_msg_id is too long")
	ErrBadThread          = errors.New("thread root must be a top-level message in the same channel")
	ErrBadEmoji           = errors.New("emoji must be a short emoji sequence (1-32 characters, no text)")
	ErrBadDM              = errors.New("a DM needs 1-8 other people from this workspace")
	ErrChannelArchived    = errors.New("the channel is archived and read-only")
	ErrChannelLocked      = errors.New("the channel is locked against agents")
	ErrDMUnchangeable     = errors.New("a DM cannot be renamed, archived or have its members changed")
	ErrNothingToUpdate    = errors.New("give a name or a description to change")
	ErrBadMuteUntil       = errors.New("until must be in the future")
	ErrBadNotifyLevel     = errors.New("level must be all, mentions or none")
	ErrAlsoNeedsThread    = errors.New("also_in_channel is for thread replies in a channel")
	ErrForwardRetracted   = errors.New("a retracted message cannot be forwarded")
	ErrPinLimit           = errors.New("a conversation has at most 50 pinned messages; unpin one first")
	ErrAgentPinLimit      = errors.New("an agent pins at most 5 messages per conversation per day")
	ErrPinOwnOnly         = errors.New("an agent can pin only its own messages")
	ErrPinRetracted       = errors.New("a retracted message cannot be pinned")
	ErrImportantHumans    = errors.New("only agents mark a message important; people pin it")
)

// MaxDescriptionLen is the maximum allowed length of a channel description.
const MaxDescriptionLen = 250

// ValidateDescription checks channel description length.
func ValidateDescription(desc string) error {
	if len([]rune(desc)) > MaxDescriptionLen {
		return ErrInvalidDescription
	}
	return nil
}

var (
	namePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
	mentionPattern = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])@([a-z0-9][a-z0-9-]{1,31})`)
)

// dmPrefix is reserved for DM channels (see dmName).
const dmPrefix = "dm-"

// ValidateName checks a channel name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) || strings.HasPrefix(name, dmPrefix) {
		return ErrInvalidName
	}
	return nil
}

// ValidateBody trims and checks a message body.
func ValidateBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ErrEmptyBody
	}
	if len(body) > MaxBodyLen {
		return "", ErrBodyTooLong
	}
	return body, nil
}

// MentionedHandles returns the distinct @handles in body, in order of appearance.
func MentionedHandles(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mentionPattern.FindAllStringSubmatch(body, -1) {
		h := m[1]
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}
