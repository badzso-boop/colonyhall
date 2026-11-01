package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

var (
	// ErrForbidden is returned when a caller lacks permission.
	ErrForbidden = errors.New("delegations: forbidden")
	// ErrGrantNotFound is returned when a requested grant does not exist.
	ErrGrantNotFound = errors.New("delegations: grant not found")
	// ErrGrantNotActive is returned when trying to revoke an inactive grant.
	ErrGrantNotActive = errors.New("delegations: grant is not active")
	// ErrInvalidSettings is returned when settings validation fails.
	ErrInvalidSettings = errors.New("delegations: invalid settings")
)

// Conversation defines the conversation module methods needed by delegation.
type Conversation interface {
	conversation.Reader
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
}

// Directory defines the identity lookup methods needed by delegation.
type Directory interface {
	identity.Directory
}

// Hosts defines the host assignment methods needed by delegation.
type Hosts interface {
	IsFullAccess(ctx context.Context, agentID ids.ID) (bool, error)
}

// QuestionService defines the question methods needed by delegation.
type QuestionService interface {
	Ask(ctx context.Context, by identity.Actor, in question.AskInput) (question.Question, error)
	Reopen(ctx context.Context, id ids.ID) error
	CloseByGrant(ctx context.Context, questionID ids.ID, closedBy ids.ID, answerText string) error
	WithdrawSilently(ctx context.Context, questionID ids.ID) error
}

// DeliveryStore defines the delivery insert method needed by delegation.
type DeliveryStore interface {
	Insert(ctx context.Context, d delivery.Delivery) (bool, error)
}

// Service provides delegation use cases.
type Service struct {
	Store        Store
	Conversation Conversation
	Directory    Directory
	Hosts        Hosts
	Question     QuestionService
	Delivery     DeliveryStore
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// NeedsApproval reports whether a task message from fromActorID to toActorID requires delegation approval.
func (s *Service) NeedsApproval(ctx context.Context, fromActorID, toActorID ids.ID) (bool, error) {
	if s.Hosts == nil {
		return false, nil
	}
	return s.Hosts.IsFullAccess(ctx, toActorID)
}

// IsDelegationQuestion reports whether questionID belongs to an existing delegation.
func (s *Service) IsDelegationQuestion(ctx context.Context, questionID ids.ID) (bool, error) {
	_, err := s.Store.GetByQuestionID(ctx, questionID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// HandleEvent processes incoming events.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	switch e.Type {
	case conversation.EventMessageCreated:
		var p conversation.MessageCreated
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delegation: bad %s payload: %w", e.Type, err)
		}
		return s.onMessageCreated(ctx, e, p)
	case question.EventAnswered:
		var p question.Event
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delegation: bad %s payload: %w", e.Type, err)
		}
		return s.onQuestionAnswered(ctx, e, p)
	case question.EventExpired:
		var p question.Event
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delegation: bad %s payload: %w", e.Type, err)
		}
		return s.onQuestionExpired(ctx, e, p)
	case conversation.EventMessageRetracted:
		var p conversation.MessageRetracted
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delegation: bad %s payload: %w", e.Type, err)
		}
		return s.onMessageRetracted(ctx, e, p)
	}
	return nil
}

func (s *Service) getSettings(ctx context.Context, workspaceID ids.ID) (Settings, error) {
	st, err := s.Store.GetSettings(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return DefaultSettings(workspaceID), nil
		}
		return Settings{}, err
	}
	return st, nil
}

func (s *Service) onMessageCreated(ctx context.Context, e eventlog.Event, p conversation.MessageCreated) error {
	if p.Silent || strings.HasPrefix(p.ClientMsgID, "q-") || p.Kind == "system" || p.NoThreadFollow || strings.HasPrefix(p.Body, "Q-") {
		return nil
	}

	authorID, err := ids.Parse(p.AuthorID)
	if err != nil {
		return err
	}
	author, err := s.Directory.ActorByID(ctx, authorID)
	if err != nil {
		return err
	}
	if author.Kind != identity.KindAgent {
		return nil
	}

	msgID, err := ids.Parse(p.MessageID)
	if err != nil {
		return err
	}
	ch, err := s.Conversation.ChannelByID(ctx, e.ChannelID)
	if err != nil {
		return err
	}

	recipients, err := delivery.FindRecipients(ctx, s.Conversation, s.Directory, ch, p)
	if err != nil {
		return err
	}

	for _, rec := range recipients {
		if rec.Actor.Kind != identity.KindAgent {
			continue
		}
		needsApproval, err := s.NeedsApproval(ctx, author.ID, rec.Actor.ID)
		if err != nil {
			return err
		}
		if !needsApproval {
			continue
		}

		var origThreadRootID ids.ID
		if p.ThreadRootID != "" {
			if parsed, err := ids.Parse(p.ThreadRootID); err == nil {
				origThreadRootID = parsed
			}
		}
		conv := MessageConversation(ch.Kind == conversation.KindDM, ch.ID, msgID, origThreadRootID)

		st, err := s.getSettings(ctx, e.WorkspaceID)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		activeGrant, err := s.Store.GetActiveGrant(ctx, e.WorkspaceID, author.ID, rec.Actor.ID, conv.ChannelID, conv.ThreadRootID, now)
		if err == nil && activeGrant.IsActive(now, st.GrantIdleMinutes) {
			// Covered by active grant! Deliver directly without creating delegation or question.
			if err := s.Store.UpdateGrantLastUsed(ctx, activeGrant.ID, now); err != nil {
				return err
			}
			err = s.deliver(ctx, delivery.Delivery{
				ID:          s.IDs.New(),
				WorkspaceID: e.WorkspaceID,
				RecipientID: rec.Actor.ID,
				ItemKind:    delivery.ItemDelegated,
				ItemID:      msgID,
				ChannelID:   ch.ID,
				FromActorID: author.ID,
				DelegatedBy: activeGrant.ApprovedBy,
				ViaGrant:    true,
				Flags:       delivery.FlagUnread | delivery.FlagWake,
				EventSeq:    e.Seq,
				CreatedAt:   now,
			})
			if err != nil {
				return err
			}
			continue
		}

		// A reply is not a task: the full-access agent asked the author here
		// first (Norbi's decision, 2026-11-01), so the answer goes through.
		if replied, err := s.answersRequest(ctx, ch, msgID, author.ID, rec.Actor.ID, st, now); err != nil {
			return err
		} else if replied {
			err = s.deliver(ctx, delivery.Delivery{
				ID:          s.IDs.New(),
				WorkspaceID: e.WorkspaceID,
				RecipientID: rec.Actor.ID,
				ItemKind:    delivery.ItemMessage,
				ItemID:      msgID,
				ChannelID:   ch.ID,
				FromActorID: author.ID,
				Flags:       rec.Flags,
				EventSeq:    e.Seq,
				CreatedAt:   now,
			})
			if err != nil {
				return err
			}
			continue
		}

		if err := s.createDelegationAndQuestion(ctx, e, p, author, rec.Actor, ch, msgID, conv); err != nil {
			return err
		}
	}
	return nil
}

// answersRequest reports whether the message answers the recipient: in the
// same unit (the thread, else the DM's or channel's main stream), within
// the grant's idle window, the recipient wrote to the author before it (a
// DM between them, or a message mentioning the author).
func (s *Service) answersRequest(ctx context.Context, ch conversation.Channel, msgID, authorID, recipientID ids.ID, st Settings, now time.Time) (bool, error) {
	msg, err := s.Conversation.MessageByID(ctx, msgID)
	if err != nil {
		return false, err
	}
	units := []ids.ID{msg.ThreadRootID}
	if msg.ThreadRootID != ids.Nil && ch.Kind == conversation.KindDM {
		units = append(units, ids.Nil) // a DM is one conversation, threads included
	}
	window := time.Duration(st.GrantIdleMinutes) * time.Minute
	for _, unit := range units {
		list, err := s.Conversation.MessagesInUnit(ctx, ch.ID, unit)
		if err != nil {
			return false, err
		}
		for _, m := range list {
			if m.AuthorID != recipientID || m.DeletedAt != nil || m.ID == msgID || !m.CreatedAt.Before(msg.CreatedAt) || now.Sub(m.CreatedAt) > window {
				continue
			}
			if ch.Kind == conversation.KindDM || slices.Contains(m.Mentions, authorID) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Service) createDelegationAndQuestion(ctx context.Context, e eventlog.Event, p conversation.MessageCreated, fromAgent, toAgent identity.Actor, ch conversation.Channel, msgID ids.ID, conv ConversationKey) error {
	// Idempotency: skip if already created for this message and target
	if _, err := s.Store.GetByMessageAndTarget(ctx, msgID, toAgent.ID); err == nil {
		return nil
	}

	bodyPreview := strings.TrimSpace(p.Body)
	runes := []rune(bodyPreview)
	if len(runes) > 500 {
		bodyPreview = string(runes[:497]) + "..."
	}
	cleanBody := strings.ReplaceAll(bodyPreview, "@", "")
	qText := fmt.Sprintf("@%s feladatot adna @%s-nak: »%s« Jóváhagyod?", fromAgent.Handle, toAgent.Handle, cleanBody)

	questionThreadRoot := conv.ThreadRootID
	if questionThreadRoot == ids.Nil {
		questionThreadRoot = msgID
	}

	deadlineMin := 24 * 60
	q, err := s.Question.Ask(ctx, fromAgent, question.AskInput{
		ChannelID:         ch.ID,
		ThreadRootID:      questionThreadRoot,
		To:                "human",
		Text:              qText,
		Options:           []string{"Jóváhagyom", "Elutasítom"},
		DeadlineMinutes:   &deadlineMin,
		AllowMultipleOpen: true,
		AllowExpire:       true,
		NoMessage:         true,
	})
	if err != nil {
		return fmt.Errorf("delegation: ask question: %w", err)
	}

	del := Delegation{
		ID:           s.IDs.New(),
		WorkspaceID:  e.WorkspaceID,
		MessageID:    msgID,
		ChannelID:    ch.ID,
		ThreadRootID: conv.ThreadRootID,
		FromAgentID:  fromAgent.ID,
		ToAgentID:    toAgent.ID,
		QuestionID:   q.ID,
		State:        StatePending,
		CreatedAt:    s.Clock.Now(),
	}

	if err := s.Store.Insert(ctx, del); err != nil {
		return fmt.Errorf("delegation: insert: %w", err)
	}

	return s.recordEvent(ctx, del.WorkspaceID, fromAgent.ID, EventCreated, del.ID, EventPayload{
		DelegationID: del.ID.String(),
		MessageID:    del.MessageID.String(),
		ChannelID:    del.ChannelID.String(),
		ThreadRootID: del.ThreadRootID.String(),
		FromAgentID:  del.FromAgentID.String(),
		ToAgentID:    del.ToAgentID.String(),
		QuestionID:   del.QuestionID.String(),
		State:        del.State,
	})
}

func (s *Service) onQuestionAnswered(ctx context.Context, e eventlog.Event, p question.Event) error {
	if p.QuestionID == "" {
		return nil
	}
	qID, err := ids.Parse(p.QuestionID)
	if err != nil {
		return nil
	}

	del, err := s.Store.GetByQuestionID(ctx, qID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if del.State != StatePending {
		return nil
	}

	answererID, err := ids.Parse(p.AnsweredBy)
	if err != nil {
		return nil
	}
	answerer, err := s.Directory.ActorByID(ctx, answererID)
	if err != nil {
		return err
	}

	isAdminHuman := answerer.Kind == identity.KindHuman && answerer.IsAdmin()
	if !isAdminHuman {
		// Non-admin answer: question remains pending, inform the answerer
		_ = s.Question.Reopen(ctx, qID)
		sendThreadRoot := del.ThreadRootID
		if sendThreadRoot == ids.Nil {
			sendThreadRoot = del.MessageID
		}
		_, _ = s.Conversation.SendMessage(ctx, answerer, conversation.SendInput{
			ChannelID:      del.ChannelID,
			ThreadRootID:   sendThreadRoot,
			Body:           "csak admin hagyhatja jóvá",
			NoThreadFollow: true,
			Silent:         true,
		})
		return nil
	}

	ans := strings.TrimSpace(strings.ToLower(p.Answer))
	isApprove := strings.HasPrefix(ans, "jóváhagyom") || strings.HasPrefix(ans, "jovahagyom") || strings.HasPrefix(ans, "approve")

	now := s.Clock.Now()
	if isApprove {
		affected, err := s.Store.UpdateStateConditional(ctx, del.ID, StatePending, StateApproved, &answerer.ID, now)
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}

		// Target agent gets exactly one 'delegated' inbox item carrying the approver
		err = s.deliver(ctx, delivery.Delivery{
			ID:          s.IDs.New(),
			WorkspaceID: del.WorkspaceID,
			RecipientID: del.ToAgentID,
			ItemKind:    delivery.ItemDelegated,
			ItemID:      del.MessageID,
			ChannelID:   del.ChannelID,
			FromActorID: del.FromAgentID,
			DelegatedBy: answerer.ID,
			ViaGrant:    false,
			Flags:       delivery.FlagUnread | delivery.FlagWake,
			EventSeq:    e.Seq,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}

		ansIDStr := answerer.ID.String()
		if err := s.recordEvent(ctx, del.WorkspaceID, answerer.ID, EventApproved, del.ID, EventPayload{
			DelegationID: del.ID.String(),
			MessageID:    del.MessageID.String(),
			ChannelID:    del.ChannelID.String(),
			ThreadRootID: del.ThreadRootID.String(),
			FromAgentID:  del.FromAgentID.String(),
			ToAgentID:    del.ToAgentID.String(),
			QuestionID:   del.QuestionID.String(),
			State:        StateApproved,
			DecidedBy:    &ansIDStr,
		}); err != nil {
			return err
		}

		// Create active grant if none exists for pair & conversation
		st, err := s.getSettings(ctx, del.WorkspaceID)
		if err != nil {
			return err
		}
		canonA, canonB := CanonicalPair(del.FromAgentID, del.ToAgentID)
		existingGrant, err := s.Store.GetActiveGrant(ctx, del.WorkspaceID, canonA, canonB, del.ChannelID, del.ThreadRootID, now)
		if err != nil || !existingGrant.IsActive(now, st.GrantIdleMinutes) {
			grantID := s.IDs.New()
			expiresAt := now.Add(time.Duration(st.GrantMaxHours) * time.Hour)
			grant := Grant{
				ID:           grantID,
				WorkspaceID:  del.WorkspaceID,
				ChannelID:    del.ChannelID,
				ThreadRootID: del.ThreadRootID,
				AgentAID:     canonA,
				AgentBID:     canonB,
				DelegationID: del.ID,
				ApprovedBy:   answerer.ID,
				ApprovedAt:   now,
				LastUsedAt:   now,
				ExpiresAt:    expiresAt,
			}
			if err := s.Store.InsertGrant(ctx, grant); err != nil {
				return err
			}
			if err := s.recordEvent(ctx, del.WorkspaceID, answerer.ID, EventGrantCreated, grantID, GrantEventPayload{
				GrantID:            grantID.String(),
				WorkspaceID:        grant.WorkspaceID.String(),
				ChannelID:          grant.ChannelID.String(),
				ThreadRootID:       grant.ThreadRootID.String(),
				AgentAID:           grant.AgentAID.String(),
				AgentBID:           grant.AgentBID.String(),
				DelegationID:       grant.DelegationID.String(),
				ApprovedBy:         grant.ApprovedBy.String(),
				ApprovedAt:         grant.ApprovedAt.Format(time.RFC3339),
				ExpiresAt:          grant.ExpiresAt.Format(time.RFC3339),
				EffectiveExpiresAt: grant.EffectiveExpiresAt(st.GrantIdleMinutes).Format(time.RFC3339),
			}); err != nil {
				return err
			}
		} else {
			if err := s.Store.UpdateGrantLastUsed(ctx, existingGrant.ID, now); err != nil {
				return err
			}
		}

		// Approve other pending delegations of the same pair in the same conversation
		otherPending, err := s.Store.ListPendingByPairAndConversation(ctx, del.WorkspaceID, canonA, canonB, del.ChannelID, del.ThreadRootID)
		if err != nil {
			return err
		}
		{
			for _, otherDel := range otherPending {
				if otherDel.ID == del.ID {
					continue
				}
				aff, err := s.Store.ApproveViaGrant(ctx, otherDel.ID, answerer.ID, now)
				if err != nil {
					return err
				}
				if aff == 0 {
					continue
				}
				if err := s.deliver(ctx, delivery.Delivery{
					ID:          s.IDs.New(),
					WorkspaceID: otherDel.WorkspaceID,
					RecipientID: otherDel.ToAgentID,
					ItemKind:    delivery.ItemDelegated,
					ItemID:      otherDel.MessageID,
					ChannelID:   otherDel.ChannelID,
					FromActorID: otherDel.FromAgentID,
					DelegatedBy: answerer.ID,
					ViaGrant:    true,
					Flags:       delivery.FlagUnread | delivery.FlagWake,
					EventSeq:    e.Seq,
					CreatedAt:   now,
				}); err != nil {
					return err
				}
				if otherDel.QuestionID != ids.Nil && s.Question != nil {
					if err := s.Question.CloseByGrant(ctx, otherDel.QuestionID, answerer.ID, "az engedély alapján"); err != nil {
						return err
					}
				}
				if err := s.recordEvent(ctx, otherDel.WorkspaceID, answerer.ID, EventApproved, otherDel.ID, EventPayload{
					DelegationID: otherDel.ID.String(),
					MessageID:    otherDel.MessageID.String(),
					ChannelID:    otherDel.ChannelID.String(),
					ThreadRootID: otherDel.ThreadRootID.String(),
					FromAgentID:  otherDel.FromAgentID.String(),
					ToAgentID:    otherDel.ToAgentID.String(),
					QuestionID:   otherDel.QuestionID.String(),
					State:        StateApproved,
					DecidedBy:    &ansIDStr,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	}

	// Admin denied
	affected, err := s.Store.UpdateStateConditional(ctx, del.ID, StatePending, StateDenied, &answerer.ID, now)
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}

	sendThreadRoot := del.ThreadRootID
	if sendThreadRoot == ids.Nil {
		sendThreadRoot = del.MessageID
	}

	denyText := fmt.Sprintf("@%s elutasította", answerer.Handle)
	sendRes, err := s.Conversation.SendMessage(ctx, answerer, conversation.SendInput{
		ChannelID:      del.ChannelID,
		ThreadRootID:   sendThreadRoot,
		Body:           denyText,
		NoThreadFollow: true,
		Silent:         true,
	})
	if err != nil {
		return err
	}

	var answerMsgID ids.ID
	if sendRes.Message != nil {
		answerMsgID = sendRes.Message.ID
	}

	// Source agent gets an answer item
	err = s.deliver(ctx, delivery.Delivery{
		ID:          s.IDs.New(),
		WorkspaceID: del.WorkspaceID,
		RecipientID: del.FromAgentID,
		ItemKind:    delivery.ItemAnswer,
		ItemID:      answerMsgID,
		ChannelID:   del.ChannelID,
		FromActorID: answerer.ID,
		Flags:       delivery.FlagUnread | delivery.FlagWake,
		EventSeq:    e.Seq,
		CreatedAt:   now,
	})
	if err != nil {
		return err
	}

	ansIDStr := answerer.ID.String()
	return s.recordEvent(ctx, del.WorkspaceID, answerer.ID, EventDenied, del.ID, EventPayload{
		DelegationID: del.ID.String(),
		MessageID:    del.MessageID.String(),
		ChannelID:    del.ChannelID.String(),
		ThreadRootID: del.ThreadRootID.String(),
		FromAgentID:  del.FromAgentID.String(),
		ToAgentID:    del.ToAgentID.String(),
		QuestionID:   del.QuestionID.String(),
		State:        StateDenied,
		DecidedBy:    &ansIDStr,
	})
}

func (s *Service) onQuestionExpired(ctx context.Context, e eventlog.Event, p question.Event) error {
	if p.QuestionID == "" {
		return nil
	}
	qID, err := ids.Parse(p.QuestionID)
	if err != nil {
		return nil
	}

	del, err := s.Store.GetByQuestionID(ctx, qID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	if del.State != StatePending {
		return nil
	}

	now := s.Clock.Now()
	affected, err := s.Store.UpdateStateConditional(ctx, del.ID, StatePending, StateExpired, nil, now)
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}

	fromAgent, err := s.Directory.ActorByID(ctx, del.FromAgentID)
	if err != nil {
		return err
	}

	sendThreadRoot := del.ThreadRootID
	if sendThreadRoot == ids.Nil {
		sendThreadRoot = del.MessageID
	}

	sendRes, err := s.Conversation.SendMessage(ctx, fromAgent, conversation.SendInput{
		ChannelID:      del.ChannelID,
		ThreadRootID:   sendThreadRoot,
		Body:           "lejárt",
		NoThreadFollow: true,
		Silent:         true,
	})
	if err != nil {
		return err
	}

	var expiredMsgID ids.ID
	if sendRes.Message != nil {
		expiredMsgID = sendRes.Message.ID
	}

	// Source agent gets an answer item
	err = s.deliver(ctx, delivery.Delivery{
		ID:          s.IDs.New(),
		WorkspaceID: del.WorkspaceID,
		RecipientID: del.FromAgentID,
		ItemKind:    delivery.ItemAnswer,
		ItemID:      expiredMsgID,
		ChannelID:   del.ChannelID,
		FromActorID: del.FromAgentID,
		Flags:       delivery.FlagUnread | delivery.FlagWake,
		EventSeq:    e.Seq,
		CreatedAt:   now,
	})
	if err != nil {
		return err
	}

	return s.recordEvent(ctx, del.WorkspaceID, del.FromAgentID, EventExpired, del.ID, EventPayload{
		DelegationID: del.ID.String(),
		MessageID:    del.MessageID.String(),
		ChannelID:    del.ChannelID.String(),
		ThreadRootID: del.ThreadRootID.String(),
		FromAgentID:  del.FromAgentID.String(),
		ToAgentID:    del.ToAgentID.String(),
		QuestionID:   del.QuestionID.String(),
		State:        StateExpired,
	})
}

func (s *Service) onMessageRetracted(ctx context.Context, e eventlog.Event, p conversation.MessageRetracted) error {
	msgID, err := ids.Parse(p.MessageID)
	if err != nil {
		return nil
	}
	dels, err := s.Store.ListByMessageID(ctx, msgID)
	if err != nil {
		return err
	}
	now := s.Clock.Now()
	for _, del := range dels {
		if del.State != StatePending {
			continue
		}
		retractorID, _ := ids.Parse(p.RetractedBy)
		affected, err := s.Store.UpdateStateConditional(ctx, del.ID, StatePending, StateCancelled, &retractorID, now)
		if err != nil {
			return err
		}
		if affected > 0 && del.QuestionID != ids.Nil && s.Question != nil {
			_ = s.Question.WithdrawSilently(ctx, del.QuestionID)
		}
	}
	return nil
}

// List returns delegations in the workspace, optionally filtered by state and message ID.
func (s *Service) List(ctx context.Context, by identity.Actor, state *string, messageID *ids.ID) ([]DelegationView, error) {
	if by.Kind == identity.KindAgent && messageID == nil {
		return nil, ErrForbidden
	}

	var items []Delegation
	var err error
	if messageID != nil {
		if by.Kind == identity.KindAgent {
			del, err := s.Store.GetByMessageAndTarget(ctx, *messageID, by.ID)
			if err == nil {
				if state == nil || *state == del.State {
					items = []Delegation{del}
				}
			} else if !errors.Is(err, db.ErrNotFound) {
				return nil, err
			}
		} else {
			all, err := s.Store.ListByWorkspace(ctx, by.WorkspaceID, state)
			if err != nil {
				return nil, err
			}
			for _, d := range all {
				if d.MessageID == *messageID {
					items = append(items, d)
				}
			}
		}
	} else {
		if by.Kind != identity.KindHuman {
			return nil, ErrForbidden
		}
		items, err = s.Store.ListByWorkspace(ctx, by.WorkspaceID, state)
		if err != nil {
			return nil, err
		}
	}

	actorIDs := make([]ids.ID, 0, len(items)*3)
	for _, it := range items {
		actorIDs = append(actorIDs, it.FromAgentID, it.ToAgentID)
		if it.DecidedBy != nil {
			actorIDs = append(actorIDs, *it.DecidedBy)
		}
	}

	actors, err := s.Directory.ActorsByIDs(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	handles := make(map[ids.ID]string, len(actors))
	for _, a := range actors {
		handles[a.ID] = a.Handle
	}

	var defaultApprover string
	if len(items) > 0 {
		admins, err := s.Directory.Admins(ctx, by.WorkspaceID)
		if err == nil && len(admins) > 0 {
			defaultApprover = admins[0].Handle
		}
	}

	st, _ := s.getSettings(ctx, by.WorkspaceID)
	grants, _ := s.Store.ListGrants(ctx, by.WorkspaceID, false, s.Clock.Now())
	grantByDelID := make(map[ids.ID]Grant, len(grants))
	for _, g := range grants {
		grantByDelID[g.DelegationID] = g
	}

	views := make([]DelegationView, 0, len(items))
	for _, it := range items {
		v := DelegationView{
			Delegation:      it,
			FromAgentHandle: handles[it.FromAgentID],
			ToAgentHandle:   handles[it.ToAgentID],
			ApproverHandle:  defaultApprover,
		}
		if it.DecidedBy != nil {
			h := handles[*it.DecidedBy]
			v.DecidedByHandle = &h
			v.ApproverHandle = h
		}
		if g, ok := grantByDelID[it.ID]; ok {
			idle := st.GrantIdleMinutes
			maxH := int((g.ExpiresAt.Sub(g.ApprovedAt) + time.Minute) / time.Hour)
			v.GrantIdleMinutes = &idle
			v.GrantMaxHours = &maxH
		}
		v.ApprovedByGrant = it.ViaGrant
		views = append(views, v)
	}
	return views, nil
}

// GetSettings returns delegation settings for the caller's workspace (humans only).
func (s *Service) GetSettings(ctx context.Context, by identity.Actor) (Settings, error) {
	if by.Kind != identity.KindHuman {
		return Settings{}, ErrForbidden
	}
	return s.getSettings(ctx, by.WorkspaceID)
}

// UpdateSettings updates delegation settings for the caller's workspace (admin humans only).
func (s *Service) UpdateSettings(ctx context.Context, by identity.Actor, idleMinutes, maxHours int) (Settings, error) {
	if by.Kind != identity.KindHuman || !by.IsAdmin() {
		return Settings{}, ErrForbidden
	}
	if idleMinutes < MinGrantIdleMinutes || idleMinutes > MaxGrantIdleMinutes {
		return Settings{}, ErrInvalidSettings
	}
	if maxHours < MinGrantMaxHours || maxHours > MaxGrantMaxHours {
		return Settings{}, ErrInvalidSettings
	}
	st := Settings{
		WorkspaceID:      by.WorkspaceID,
		GrantIdleMinutes: idleMinutes,
		GrantMaxHours:    maxHours,
		UpdatedAt:        s.Clock.Now(),
	}
	err := s.inTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpsertSettings(ctx, st); err != nil {
			return err
		}
		return s.recordEvent(ctx, by.WorkspaceID, by.ID, EventSettingsUpdated, by.WorkspaceID, SettingsEventPayload{
			WorkspaceID:      by.WorkspaceID.String(),
			GrantIdleMinutes: idleMinutes,
			GrantMaxHours:    maxHours,
		})
	})
	if err != nil {
		return Settings{}, err
	}
	return st, nil
}

// ListGrants returns delegation grants for the caller's workspace (humans only).
func (s *Service) ListGrants(ctx context.Context, by identity.Actor, activeOnly bool) ([]GrantView, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	now := s.Clock.Now()
	st, err := s.getSettings(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	grants, err := s.Store.ListGrants(ctx, by.WorkspaceID, activeOnly, now)
	if err != nil {
		return nil, err
	}

	actorIDs := make([]ids.ID, 0, len(grants)*4)
	for _, g := range grants {
		actorIDs = append(actorIDs, g.AgentAID, g.AgentBID, g.ApprovedBy)
		if g.RevokedBy != nil {
			actorIDs = append(actorIDs, *g.RevokedBy)
		}
	}
	actors, err := s.Directory.ActorsByIDs(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	handles := make(map[ids.ID]string, len(actors))
	for _, a := range actors {
		handles[a.ID] = a.Handle
	}

	views := make([]GrantView, 0, len(grants))
	for _, g := range grants {
		if activeOnly && !g.IsActive(now, st.GrantIdleMinutes) {
			continue
		}
		v := GrantView{
			Grant:          g,
			AgentAHandle:   handles[g.AgentAID],
			AgentBHandle:   handles[g.AgentBID],
			ApproverHandle: handles[g.ApprovedBy],
			IdleMinutes:    st.GrantIdleMinutes,
		}
		if g.RevokedBy != nil {
			h := handles[*g.RevokedBy]
			v.RevokedByHandle = &h
		}
		views = append(views, v)
	}
	return views, nil
}

// RevokeGrant revokes an active delegation grant (admin humans only).
func (s *Service) RevokeGrant(ctx context.Context, by identity.Actor, id ids.ID) (GrantView, error) {
	if by.Kind != identity.KindHuman || !by.IsAdmin() {
		return GrantView{}, ErrForbidden
	}
	g, err := s.Store.GetGrantByID(ctx, id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return GrantView{}, ErrGrantNotFound
		}
		return GrantView{}, err
	}
	if g.WorkspaceID != by.WorkspaceID {
		return GrantView{}, ErrForbidden
	}
	now := s.Clock.Now()
	st, err := s.getSettings(ctx, by.WorkspaceID)
	if err != nil {
		return GrantView{}, err
	}
	if !g.IsActive(now, st.GrantIdleMinutes) {
		return GrantView{}, ErrGrantNotActive
	}
	var updated Grant
	err = s.inTx(ctx, func(ctx context.Context) error {
		aff, err := s.Store.RevokeGrantConditional(ctx, id, by.ID, now)
		if err != nil {
			return err
		}
		if aff == 0 {
			return ErrGrantNotActive
		}
		updated, err = s.Store.GetGrantByID(ctx, id)
		if err != nil {
			return err
		}
		rByStr := by.ID.String()
		rAtStr := now.Format(time.RFC3339)
		return s.recordEvent(ctx, by.WorkspaceID, by.ID, EventGrantRevoked, id, GrantEventPayload{
			GrantID:            id.String(),
			WorkspaceID:        updated.WorkspaceID.String(),
			ChannelID:          updated.ChannelID.String(),
			ThreadRootID:       updated.ThreadRootID.String(),
			AgentAID:           updated.AgentAID.String(),
			AgentBID:           updated.AgentBID.String(),
			DelegationID:       updated.DelegationID.String(),
			ApprovedBy:         updated.ApprovedBy.String(),
			ApprovedAt:         updated.ApprovedAt.Format(time.RFC3339),
			ExpiresAt:          updated.ExpiresAt.Format(time.RFC3339),
			EffectiveExpiresAt: updated.EffectiveExpiresAt(st.GrantIdleMinutes).Format(time.RFC3339),
			RevokedBy:          &rByStr,
			RevokedAt:          &rAtStr,
		})
	})
	if err != nil {
		return GrantView{}, err
	}

	views, err := s.ListGrants(ctx, by, false)
	if err == nil {
		for _, v := range views {
			if v.ID == id {
				return v, nil
			}
		}
	}
	return GrantView{
		Grant:          updated,
		ApproverHandle: by.Handle,
		IdleMinutes:    st.GrantIdleMinutes,
	}, nil
}

// inTx runs fn in one transaction when a TxRunner is set.
func (s *Service) inTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.Tx == nil {
		return fn(ctx)
	}
	return s.Tx.InTx(ctx, fn)
}

// deliver writes an inbox item and, when it is new and should wake, tells
// the recipient's guard right away (delivery.EventItemCreated). The item is
// written by this consumer after the event that caused it (an answered
// question, an expiry), so a guard woken by that event found the inbox still
// empty and waited for its fallback poll, up to a minute.
func (s *Service) deliver(ctx context.Context, d delivery.Delivery) error {
	inserted, err := s.Delivery.Insert(ctx, d)
	if err != nil || !inserted || !d.Flags.Has(delivery.FlagWake) {
		return err
	}
	by := d.DelegatedBy
	if by == ids.Nil {
		by = d.FromActorID
	}
	return s.recordEvent(ctx, d.WorkspaceID, by, delivery.EventItemCreated, d.ID, delivery.ItemCreated{
		RecipientID: d.RecipientID.String(),
		ItemKind:    d.ItemKind,
	})
}

func (s *Service) recordEvent(ctx context.Context, workspaceID, actorID ids.ID, typ string, aggID ids.ID, payload any) error {
	if s.Events == nil {
		return nil
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "delegation", aggID, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	return err
}
