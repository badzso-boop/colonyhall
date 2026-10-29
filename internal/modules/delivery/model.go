// Package delivery owns the per-recipient inbox: one row per recipient per
// item that concerns them, with flag bits (Zulip UserMessage pattern).
// Agents read this instead of whole channels, which keeps their context small.
package delivery

import (
	"time"

	"colonyhall/internal/platform/ids"
)

// Flags is a bit set describing why an item is in the inbox.
type Flags int16

// Flag bits. Stored as smallint; keep the values stable.
const (
	FlagUnread    Flags = 1 << 0
	FlagMentioned Flags = 1 << 1
	FlagWake      Flags = 1 << 2 // should wake an agent
	FlagDM        Flags = 1 << 3
	// FlagThread: a human replied, without mentioning anyone, in a thread
	// the recipient (an agent) had written in.
	FlagThread Flags = 1 << 4
)

// Has reports whether all bits in f2 are set.
func (f Flags) Has(f2 Flags) bool { return f&f2 == f2 }

// Item kinds.
const (
	ItemMessage      = "message"
	ItemReactionDown = "reaction_down" // a 👎 on the recipient's message; never wakes
	ItemAnswer       = "answer"        // answer to an agent's question; wakes
	ItemCompact      = "compact"       // compaction request to scribe agent; wakes
	ItemDelegated    = "delegated"     // approved task from another agent; wakes
	ItemTerminal     = "terminal"      // finished terminal wake; wakes
)

// EventItemCreated announces a waking inbox item to its recipient alone. The
// delivery consumer's own items follow the event that caused them, which the
// recipient already sees; an item written later by another module (an
// approved delegation) needs its own signal, or the recipient's guard would
// wait for its fallback poll. The gateway shows it only to the recipient.
const EventItemCreated = "delivery.item_created"

// ItemCreated is the payload of EventItemCreated: no content and no channel,
// only enough for the recipient's guard to check its inbox. A channel id
// would name a channel to a recipient that may not be a member of it.
type ItemCreated struct {
	RecipientID string `json:"recipient_id"`
	ItemKind    string `json:"item_kind"`
}

// Delivery is one inbox row.
type Delivery struct {
	ID          ids.ID
	WorkspaceID ids.ID
	RecipientID ids.ID
	ItemKind    string
	ItemID      ids.ID
	ChannelID   ids.ID
	FromActorID ids.ID // who caused it: the author, the reactor, …
	DelegatedBy ids.ID // admin human who approved it (for delegated items)
	ViaGrant    bool   // whether this delegated item was delivered via an active delegation grant
	Flags       Flags
	EventSeq    int64
	CreatedAt   time.Time
	AckedAt     *time.Time
}

// ReplyOrigin describes the origin thread where an agent's request originated.
type ReplyOrigin struct {
	ChannelID        ids.ID
	Channel          string
	ThreadRootID     ids.ID
	RequestMessageID ids.ID
}

// InboxItem is a delivery joined with what an agent needs to act on it.
type InboxItem struct {
	Delivery
	ChannelName  string
	AuthorHandle string
	// AuthorKind tells whether a human or an agent caused the item; the
	// guard lets full-access agents take tasks from humans only.
	AuthorKind   string
	Body         string
	ThreadRootID ids.ID
	MessageAt    time.Time
	DelegatedBy  string // handle of the approving admin (for delegated items)
	ReplyOrigin  *ReplyOrigin
}

// Limits.
const (
	DefaultInboxLimit = 20
	MaxInboxLimit     = 100
)
