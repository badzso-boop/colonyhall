package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// ErrForbidden is returned when a caller lacks permission.
var ErrForbidden = errors.New("delegations: forbidden")

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
	}
	return nil
}

func (s *Service) onMessageCreated(ctx context.Context, e eventlog.Event, p conversation.MessageCreated) error {
	if strings.HasPrefix(p.ClientMsgID, "q-") || p.Kind == "system" || p.NoThreadFollow || strings.HasPrefix(p.Body, "Q-") {
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

		if err := s.createDelegationAndQuestion(ctx, e, p, author, rec.Actor, ch, msgID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createDelegationAndQuestion(ctx context.Context, e eventlog.Event, p conversation.MessageCreated, fromAgent, toAgent identity.Actor, ch conversation.Channel, msgID ids.ID) error {
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

	threadRootID := ids.Nil
	if p.ThreadRootID != "" {
		if parsed, err := ids.Parse(p.ThreadRootID); err == nil {
			threadRootID = parsed
		}
	}
	if threadRootID == ids.Nil {
		threadRootID = msgID
	}

	deadlineMin := 24 * 60
	q, err := s.Question.Ask(ctx, fromAgent, question.AskInput{
		ChannelID:         ch.ID,
		ThreadRootID:      threadRootID,
		To:                "human",
		Text:              qText,
		Options:           []string{"Jóváhagyom", "Elutasítom"},
		DeadlineMinutes:   &deadlineMin,
		AllowMultipleOpen: true,
		AllowExpire:       true,
	})
	if err != nil {
		return fmt.Errorf("delegation: ask question: %w", err)
	}

	del := Delegation{
		ID:           s.IDs.New(),
		WorkspaceID:  e.WorkspaceID,
		MessageID:    msgID,
		ChannelID:    ch.ID,
		ThreadRootID: threadRootID,
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
		_, _ = s.Conversation.SendMessage(ctx, answerer, conversation.SendInput{
			ChannelID:      del.ChannelID,
			ThreadRootID:   del.ThreadRootID,
			Body:           "csak admin hagyhatja jóvá",
			NoThreadFollow: true,
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
		_, err = s.Delivery.Insert(ctx, delivery.Delivery{
			ID:          s.IDs.New(),
			WorkspaceID: del.WorkspaceID,
			RecipientID: del.ToAgentID,
			ItemKind:    delivery.ItemDelegated,
			ItemID:      del.MessageID,
			ChannelID:   del.ChannelID,
			FromActorID: del.FromAgentID,
			DelegatedBy: answerer.ID,
			Flags:       delivery.FlagUnread | delivery.FlagWake,
			EventSeq:    e.Seq,
			CreatedAt:   now,
		})
		if err != nil {
			return err
		}

		ansIDStr := answerer.ID.String()
		return s.recordEvent(ctx, del.WorkspaceID, answerer.ID, EventApproved, del.ID, EventPayload{
			DelegationID: del.ID.String(),
			MessageID:    del.MessageID.String(),
			ChannelID:    del.ChannelID.String(),
			ThreadRootID: del.ThreadRootID.String(),
			FromAgentID:  del.FromAgentID.String(),
			ToAgentID:    del.ToAgentID.String(),
			QuestionID:   del.QuestionID.String(),
			State:        StateApproved,
			DecidedBy:    &ansIDStr,
		})
	}

	// Admin denied
	affected, err := s.Store.UpdateStateConditional(ctx, del.ID, StatePending, StateDenied, &answerer.ID, now)
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}

	denyText := fmt.Sprintf("@%s elutasította", answerer.Handle)
	sendRes, err := s.Conversation.SendMessage(ctx, answerer, conversation.SendInput{
		ChannelID:      del.ChannelID,
		ThreadRootID:   del.ThreadRootID,
		Body:           denyText,
		NoThreadFollow: true,
	})
	if err != nil {
		return err
	}

	var answerMsgID ids.ID
	if sendRes.Message != nil {
		answerMsgID = sendRes.Message.ID
	}

	// Source agent gets an answer item
	_, err = s.Delivery.Insert(ctx, delivery.Delivery{
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

	sendRes, err := s.Conversation.SendMessage(ctx, fromAgent, conversation.SendInput{
		ChannelID:      del.ChannelID,
		ThreadRootID:   del.ThreadRootID,
		Body:           "lejárt",
		NoThreadFollow: true,
	})
	if err != nil {
		return err
	}

	var expiredMsgID ids.ID
	if sendRes.Message != nil {
		expiredMsgID = sendRes.Message.ID
	}

	// Source agent gets an answer item
	_, err = s.Delivery.Insert(ctx, delivery.Delivery{
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
		views = append(views, v)
	}
	return views, nil
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
