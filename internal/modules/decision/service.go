package decision

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// WikiAppender defines the wiki capabilities needed by decisions.
type WikiAppender interface {
	Append(ctx context.Context, by identity.Actor, slug string, text string) (wiki.Page, error)
	Create(ctx context.Context, by identity.Actor, in wiki.CreateInput) (wiki.Page, error)
}

// ConversationReader defines the conversation capabilities needed by decisions.
type ConversationReader interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
	ResolveChannel(ctx context.Context, by identity.Actor, nameOrID string) (conversation.Channel, error)
}

// Service manages decision logging and wiki synchronisation.
type Service struct {
	store Store
	wiki  WikiAppender
	conv  ConversationReader
	tx    db.TxRunner
	clock clock.Clock
	ids   ids.Generator
}

// NewService creates a new decision Service.
func NewService(store Store, w WikiAppender, conv ConversationReader, tx db.TxRunner, clk clock.Clock, gen ids.Generator) *Service {
	if clk == nil {
		clk = clock.System{}
	}
	if gen == nil {
		gen = ids.V7{}
	}
	return &Service{
		store: store,
		wiki:  w,
		conv:  conv,
		tx:    tx,
		clock: clk,
		ids:   gen,
	}
}

// AddDecisionInput specifies parameters to create a new decision.
type AddDecisionInput struct {
	Text        string
	Rationale   string
	Supersedes  *int
	ThreadMsgID *ids.ID
	ChannelID   *ids.ID
}

// AddDecision records a decision and appends it to the "Döntések" wiki page in the same transaction.
func (s *Service) AddDecision(ctx context.Context, actor identity.Actor, in AddDecisionInput) (Decision, error) {
	if actor.Halted {
		return Decision{}, identity.ErrHalted
	}

	trimmedText := strings.TrimSpace(in.Text)
	if trimmedText == "" {
		return Decision{}, ErrTextEmpty
	}
	if utf8.RuneCountInString(trimmedText) > 300 {
		return Decision{}, ErrTextTooLong
	}

	trimmedRationale := strings.TrimSpace(in.Rationale)
	if trimmedRationale == "" {
		return Decision{}, ErrRationaleEmpty
	}
	if utf8.RuneCountInString(trimmedRationale) > 500 {
		return Decision{}, ErrRationaleTooLong
	}

	if in.Supersedes != nil {
		if *in.Supersedes <= 0 {
			return Decision{}, ErrSupersededNotFound
		}
		_, err := s.store.GetByNumber(ctx, actor.WorkspaceID, *in.Supersedes)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return Decision{}, ErrSupersededNotFound
			}
			return Decision{}, err
		}
	}

	var channelID ids.ID
	var rootID *ids.ID

	if in.ThreadMsgID != nil && *in.ThreadMsgID != ids.Nil {
		msg, err := s.conv.MessageByID(ctx, *in.ThreadMsgID)
		if err != nil {
			return Decision{}, err
		}
		channelID = msg.ChannelID
		if msg.ThreadRootID != ids.Nil {
			r := msg.ThreadRootID
			rootID = &r
		} else {
			r := msg.ID
			rootID = &r
		}
	} else if in.ChannelID != nil && *in.ChannelID != ids.Nil {
		channelID = *in.ChannelID
	} else {
		ch, err := s.conv.ResolveChannel(ctx, actor, "")
		if err != nil {
			return Decision{}, err
		}
		channelID = ch.ID
	}

	canRead, err := s.conv.CanRead(ctx, actor, channelID)
	if err != nil {
		return Decision{}, err
	}
	if !canRead {
		return Decision{}, ErrForbidden
	}

	var d Decision
	err = s.tx.InTx(ctx, func(txCtx context.Context) error {
		nextNum, err := s.store.NextNumber(txCtx, actor.WorkspaceID)
		if err != nil {
			return err
		}

		d = Decision{
			ID:           s.ids.New(),
			Number:       nextNum,
			WorkspaceID:  actor.WorkspaceID,
			AuthorID:     actor.ID,
			ChannelID:    channelID,
			ThreadRootID: rootID,
			Text:         trimmedText,
			Rationale:    trimmedRationale,
			Supersedes:   in.Supersedes,
			CreatedAt:    s.clock.Now(),
		}

		if err := s.store.Insert(txCtx, d); err != nil {
			return err
		}

		// Wiki section format: D-12 — text. Why: rationale. (supersedes D-7) — @author, date, link to the thread
		var b strings.Builder
		fmt.Fprintf(&b, "D-%d — %s. Why: %s.", d.Number, d.Text, d.Rationale)
		if d.Supersedes != nil {
			fmt.Fprintf(&b, " (supersedes D-%d)", *d.Supersedes)
		}
		dateStr := d.CreatedAt.UTC().Format("2006-01-02 15:04")
		if d.ThreadRootID != nil && *d.ThreadRootID != ids.Nil {
			fmt.Fprintf(&b, " — @%s, %s, thread %s", actor.Handle, dateStr, d.ThreadRootID.String())
		} else {
			fmt.Fprintf(&b, " — @%s, %s", actor.Handle, dateStr)
		}
		sectionText := b.String()

		const wikiSlug = "dontesek"
		const wikiTitle = "Döntések"

		_, err = s.wiki.Append(txCtx, actor, wikiSlug, sectionText)
		if err != nil {
			if errors.Is(err, wiki.ErrNotFound) || errors.Is(err, db.ErrNotFound) {
				_, err = s.wiki.Create(txCtx, actor, wiki.CreateInput{
					Title: wikiTitle,
					Body:  "# " + wikiTitle + "\n\n" + sectionText,
				})
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return Decision{}, err
	}

	return d, nil
}

// ListDecisions lists decisions in the workspace.
func (s *Service) ListDecisions(ctx context.Context, actor identity.Actor, limit int, query string, validOnly bool) ([]Decision, error) {
	if actor.Halted {
		return nil, identity.ErrHalted
	}
	return s.store.List(ctx, actor.WorkspaceID, limit, query, validOnly)
}
