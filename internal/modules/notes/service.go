package notes

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ErrTextTooLong is returned when a note exceeds 2000 characters.
var ErrTextTooLong = errors.New("note text must be at most 2000 characters")

// ErrTextEmpty is returned when note text is empty.
var ErrTextEmpty = errors.New("note text cannot be empty")

// ErrThreadNotFound is returned when the specified message/thread cannot be found.
var ErrThreadNotFound = errors.New("thread message not found")

// ErrForbidden is returned when an agent tries to read another agent's notes.
var ErrForbidden = errors.New("forbidden: agents may not read other agents' notes")

// ConversationReader is the conversation module view needed by notes.
type ConversationReader interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
}

// Service manages agent notebook entries.
type Service struct {
	store  Store
	conv   ConversationReader
	actors identity.Directory
	clock  clock.Clock
}

// NewService creates a new notes Service.
func NewService(store Store, conv ConversationReader, actors identity.Directory, clk clock.Clock) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	return &Service{
		store:  store,
		conv:   conv,
		actors: actors,
		clock:  clk,
	}
}

func (s *Service) resolveThreadRoot(ctx context.Context, actor identity.Actor, threadMsgID ids.ID) (ids.ID, error) {
	msg, err := s.conv.MessageByID(ctx, threadMsgID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ids.Nil, ErrThreadNotFound
		}
		return ids.Nil, err
	}
	canRead, err := s.conv.CanRead(ctx, actor, msg.ChannelID)
	if err != nil {
		return ids.Nil, err
	}
	if !canRead {
		return ids.Nil, errors.New("permission denied: cannot read channel")
	}
	rootID := msg.ID
	if msg.ThreadRootID != ids.Nil {
		rootID = msg.ThreadRootID
	}
	return rootID, nil
}

// SetNote records a new note version for the caller and thread.
func (s *Service) SetNote(ctx context.Context, actor identity.Actor, threadMsgID ids.ID, text string) (Note, error) {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) == 0 {
		return Note{}, ErrTextEmpty
	}
	if utf8.RuneCountInString(text) > 2000 {
		return Note{}, ErrTextTooLong
	}

	rootID, err := s.resolveThreadRoot(ctx, actor, threadMsgID)
	if err != nil {
		return Note{}, err
	}

	n := Note{
		ID:           uuid.New(),
		WorkspaceID:  actor.WorkspaceID,
		AgentID:      actor.ID,
		ThreadRootID: rootID,
		Text:         text,
		CreatedAt:    s.clock.Now(),
	}
	return s.store.Create(ctx, n)
}

// GetLatestNote returns the newest note for the caller and thread.
func (s *Service) GetLatestNote(ctx context.Context, actor identity.Actor, threadMsgID ids.ID) (Note, error) {
	rootID, err := s.resolveThreadRoot(ctx, actor, threadMsgID)
	if err != nil {
		return Note{}, err
	}
	return s.store.GetLatest(ctx, actor.WorkspaceID, actor.ID, rootID)
}

// ListNotes returns notes for an agent, newest first. Humans may read any agent's notes,
// agents may only read their own.
func (s *Service) ListNotes(ctx context.Context, caller identity.Actor, targetAgentID ids.ID, threadMsgID *ids.ID) ([]Note, error) {
	if caller.Kind != identity.KindHuman && caller.ID != targetAgentID {
		return nil, ErrForbidden
	}

	targetAgent, err := s.actors.ActorByID(ctx, targetAgentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, err
	}
	if targetAgent.WorkspaceID != caller.WorkspaceID {
		return nil, db.ErrNotFound
	}

	var rootID *ids.ID
	if threadMsgID != nil {
		r, err := s.resolveThreadRoot(ctx, caller, *threadMsgID)
		if err != nil {
			return nil, err
		}
		rootID = &r
	}

	return s.store.ListByAgent(ctx, caller.WorkspaceID, targetAgentID, rootID)
}
