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
	ID          ids.ID
	WorkspaceID ids.ID
	Kind        ChannelKind
	Name        string
	MsgCount    int64 // total messages, for unread badges
	CreatedBy   ids.ID
	CreatedAt   time.Time
}

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
}

// ChannelView is a channel as seen by one member, with unread counts.
type ChannelView struct {
	Channel
	Unread   int64
	Mentions int64
	IsMember bool
	// MemberIDs is filled for DM channels, so clients can show who is in it.
	MemberIDs []ids.ID
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
	Mentions     []ids.ID
	ReplyCount   int
	LastReplyAt  *time.Time
	CreatedAt    time.Time
	Reactions    []ReactionSummary // filled on reads
	LinkCards    []LinkCard        // filled on reads
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

// Limits.
const (
	MaxBodyLen      = 20000
	MaxClientMsgID  = 64
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// Errors returned by the conversation service.
var (
	ErrForbidden       = errors.New("not allowed in this channel")
	ErrInvalidName     = errors.New("channel name must be 2-32 characters: lowercase letters, digits and '-'")
	ErrEmptyBody       = errors.New("message is empty")
	ErrBodyTooLong     = errors.New("message is too long")
	ErrInvalidClientID = errors.New("client_msg_id is too long")
	ErrBadThread       = errors.New("thread root must be a top-level message in the same channel")
	ErrBadEmoji        = errors.New("emoji must be a short emoji sequence (1-32 characters, no text)")
	ErrBadDM           = errors.New("a DM needs 1-8 other people from this workspace")
)

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
