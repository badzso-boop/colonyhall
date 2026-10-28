package question

import (
	"context"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Conversation defines the conversation module methods needed by question service.
type Conversation interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	SendMessage(ctx context.Context, by identity.Actor, in conversation.SendInput) (conversation.SendResult, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
}

// Directory defines the identity lookup methods needed by question service.
type Directory interface {
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
	ActorByHandle(ctx context.Context, workspaceID ids.ID, handle string) (identity.Actor, error)
	Admins(ctx context.Context, workspaceID ids.ID) ([]identity.Actor, error)
}

// Hosts defines the host assignment methods needed by question service.
type Hosts interface {
	IsFullAccess(ctx context.Context, agentID ids.ID) (bool, error)
}

// DelegationChecker checks if a question is part of a delegation.
type DelegationChecker interface {
	IsDelegationQuestion(ctx context.Context, questionID ids.ID) (bool, error)
}

// Service provides question use cases.
type Service struct {
	Store        Store
	Conversation Conversation
	Directory    Directory
	Hosts        Hosts
	Delegation   DelegationChecker
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// AskInput contains parameters for asking a question.
type AskInput struct {
	ChannelID         ids.ID
	ThreadRootID      ids.ID
	To                string
	Text              string
	Options           []string
	DefaultOption     *string
	DeadlineMinutes   *int
	AllowMultipleOpen bool
	AllowExpire       bool
	NoMessage         bool
}

// Ask asks a new question in a conversation thread.
func (s *Service) Ask(ctx context.Context, by identity.Actor, in AskInput) (Question, error) {
	if by.Halted {
		return Question{}, ErrForbidden
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return Question{}, ErrEmptyText
	}

	// Clean and validate options
	var cleanedOptions []string
	for _, opt := range in.Options {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" {
			cleanedOptions = append(cleanedOptions, trimmed)
		}
	}
	if cleanedOptions == nil {
		cleanedOptions = []string{}
	}

	var defOption *string
	if in.DefaultOption != nil {
		d := strings.TrimSpace(*in.DefaultOption)
		if d != "" {
			if len(cleanedOptions) > 0 {
				found := false
				for _, o := range cleanedOptions {
					if o == d {
						found = true
						break
					}
				}
				if !found {
					return Question{}, ErrInvalidOption
				}
			}
			defOption = &d
		}
	}

	var deadline *time.Time
	if in.DeadlineMinutes != nil && *in.DeadlineMinutes > 0 {
		dl := s.Clock.Now().Add(time.Duration(*in.DeadlineMinutes) * time.Minute)
		deadline = &dl
	}

	// Check channel read access
	ok, err := s.Conversation.CanRead(ctx, by, in.ChannelID)
	if err != nil || !ok {
		return Question{}, ErrForbidden
	}

	// Validate thread root if provided
	if in.ThreadRootID != ids.Nil {
		root, err := s.Conversation.MessageByID(ctx, in.ThreadRootID)
		if err != nil {
			return Question{}, err
		}
		if root.ChannelID != in.ChannelID {
			return Question{}, ErrForbidden
		}
		if !in.AllowMultipleOpen {
			// One open question per thread per asker
			count, err := s.Store.CountOpenByThreadAsker(ctx, in.ThreadRootID, by.ID)
			if err != nil {
				return Question{}, err
			}
			if count > 0 {
				return Question{}, ErrQuestionAlreadyOpen
			}
		}
	}

	// Resolve target
	var toHuman bool
	var toActorID *ids.ID
	var toActorHandle string
	var redirected bool

	trimmedTo := strings.TrimSpace(in.To)
	if trimmedTo == "" || strings.EqualFold(trimmedTo, "human") {
		toHuman = true
	} else {
		handle := strings.TrimPrefix(trimmedTo, "@")
		target, err := s.Directory.ActorByHandle(ctx, by.WorkspaceID, handle)
		if err != nil {
			if targetID, parseErr := ids.Parse(handle); parseErr == nil {
				target, err = s.Directory.ActorByID(ctx, targetID)
			}
		}
		if err != nil {
			return Question{}, fmt.Errorf("unknown target actor %q: %w", in.To, err)
		}
		if target.WorkspaceID != by.WorkspaceID {
			return Question{}, ErrForbidden
		}
		if target.Kind == identity.KindHuman {
			toHuman = true
			toActorID = &target.ID
			toActorHandle = target.Handle
		} else {
			// Target is an agent: check if full-access agent
			if by.Kind == identity.KindAgent {
				isFullAccess, err := s.Hosts.IsFullAccess(ctx, target.ID)
				if err != nil {
					return Question{}, err
				}
				if isFullAccess {
					// Redirect to humans!
					toHuman = true
					toActorID = nil
					redirected = true
				} else {
					toHuman = false
					toActorID = &target.ID
					toActorHandle = target.Handle
				}
			} else {
				toHuman = false
				toActorID = &target.ID
				toActorHandle = target.Handle
			}
		}
	}

	num, err := s.Store.NextNumber(ctx, by.WorkspaceID)
	if err != nil {
		return Question{}, err
	}
	formattedNum := FormatNumber(num)

	// Format message body
	var msgBody string
	if !toHuman && toActorHandle != "" {
		msgBody = fmt.Sprintf("@%s %s: %s", toActorHandle, formattedNum, text)
	} else if toHuman && toActorHandle != "" {
		msgBody = fmt.Sprintf("@%s %s: %s", toActorHandle, formattedNum, text)
	} else if redirected {
		msgBody = fmt.Sprintf("%s (to humans, redirected: full-access agent): %s", formattedNum, text)
	} else {
		msgBody = fmt.Sprintf("%s (to humans): %s", formattedNum, text)
	}
	if len(cleanedOptions) > 0 {
		msgBody += "\nOptions: " + strings.Join(cleanedOptions, ", ")
	}
	if defOption != nil {
		msgBody += fmt.Sprintf("\nDefault: %s", *defOption)
	}
	if deadline != nil {
		msgBody += fmt.Sprintf("\nDeadline: %s", deadline.Format(time.RFC3339))
	}

	qID := s.IDs.New()

	threadRootID := in.ThreadRootID
	if !in.NoMessage {
		// Post message to thread
		sendRes, err := s.Conversation.SendMessage(ctx, by, conversation.SendInput{
			ChannelID:    in.ChannelID,
			ThreadRootID: in.ThreadRootID,
			Body:         msgBody,
			ClientMsgID:  fmt.Sprintf("q-%s", qID),
			Elaborate:    true,
		})
		if err != nil {
			return Question{}, err
		}
		if threadRootID == ids.Nil && sendRes.Message != nil {
			threadRootID = sendRes.Message.ID
		}
	}

	now := s.Clock.Now()
	q := Question{
		ID:            qID,
		Number:        num,
		WorkspaceID:   by.WorkspaceID,
		AskedBy:       by.ID,
		ToActorID:     toActorID,
		ToHuman:       toHuman,
		AllowExpire:   in.AllowExpire,
		ChannelID:     in.ChannelID,
		ThreadRootID:  threadRootID,
		Text:          text,
		Options:       cleanedOptions,
		DefaultOption: defOption,
		Deadline:      deadline,
		State:         StateOpen,
		CreatedAt:     now,
	}

	var toActorIDStr string
	if toActorID != nil {
		toActorIDStr = toActorID.String()
	}
	var defOptStr string
	if defOption != nil {
		defOptStr = *defOption
	}
	var deadlineStr string
	if deadline != nil {
		deadlineStr = deadline.Format(time.RFC3339)
	}

	evPayload := Event{
		QuestionID:      q.ID.String(),
		Number:          q.Number,
		FormattedNumber: formattedNum,
		AskedBy:         q.AskedBy.String(),
		ToActorID:       toActorIDStr,
		ToHuman:         toHuman,
		ChannelID:       q.ChannelID.String(),
		ThreadRootID:    threadRootID.String(),
		Text:            text,
		Options:         cleanedOptions,
		DefaultOption:   defOptStr,
		Deadline:        deadlineStr,
		State:           StateOpen,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventAsked, q.ID, evPayload, func(ctx context.Context) error {
		return s.Store.Insert(ctx, q)
	})
	if err != nil {
		return Question{}, err
	}
	return q, nil
}

// Answer answers an open or escalated question.
func (s *Service) Answer(ctx context.Context, by identity.Actor, questionRef string, answerText string) (Question, error) {
	if by.Halted {
		return Question{}, ErrForbidden
	}
	q, err := s.resolve(ctx, by.WorkspaceID, questionRef)
	if err != nil {
		return Question{}, err
	}
	if q.WorkspaceID != by.WorkspaceID {
		return Question{}, ErrForbidden
	}
	if q.State != StateOpen && q.State != StateEscalated {
		return Question{}, ErrQuestionNotOpen
	}

	// Permission check:
	// Only target (or any human of the workspace for human, or for escalated questions) can answer.
	// Admin humans can always answer.
	if by.IsAdmin() && by.Kind == identity.KindHuman {
		// Admin human always allowed
	} else if q.State == StateEscalated {
		if by.Kind != identity.KindHuman {
			return Question{}, ErrForbidden
		}
	} else if q.ToHuman {
		if by.Kind != identity.KindHuman {
			return Question{}, ErrForbidden
		}
	} else {
		// Question to specific agent
		if q.ToActorID == nil || by.ID != *q.ToActorID {
			return Question{}, ErrForbidden
		}
	}

	ans := strings.TrimSpace(answerText)
	if ans == "" {
		return Question{}, ErrEmptyAnswer
	}

	isDel := false
	if s.Delegation != nil {
		isDel, _ = s.Delegation.IsDelegationQuestion(ctx, q.ID)
	}

	var answerMsgID string
	if !isDel {
		// Post answer message to thread
		msgBody := fmt.Sprintf("%s answer: %s", q.FormattedNumber(), ans)
		sendRes, err := s.Conversation.SendMessage(ctx, by, conversation.SendInput{
			ChannelID:    q.ChannelID,
			ThreadRootID: q.ThreadRootID,
			Body:         msgBody,
			Elaborate:    true,
			// The asker gets the answer as its own inbox item.
			NoThreadFollow: true,
		})
		if err != nil {
			return Question{}, err
		}
		if sendRes.Message != nil {
			answerMsgID = sendRes.Message.ID.String()
		}
	}

	now := s.Clock.Now()
	var updated Question
	evPayload := Event{
		QuestionID:      q.ID.String(),
		Number:          q.Number,
		FormattedNumber: q.FormattedNumber(),
		AskedBy:         q.AskedBy.String(),
		ChannelID:       q.ChannelID.String(),
		ThreadRootID:    q.ThreadRootID.String(),
		State:           StateAnswered,
		Answer:          ans,
		AnsweredBy:      by.ID.String(),
		AnswerMessageID: answerMsgID,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventAnswered, q.ID, evPayload, func(ctx context.Context) error {
		u, ok, err := s.Store.Answer(ctx, q.ID, ans, &by.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			return ErrQuestionNotOpen
		}
		updated = u
		return nil
	})
	if err != nil {
		return Question{}, err
	}
	return updated, nil
}

// CloseByGrant answers/resolves an open question via an active delegation grant, without posting a chat message.
func (s *Service) CloseByGrant(ctx context.Context, questionID ids.ID, closedBy ids.ID, answerText string) error {
	q, err := s.Store.GetByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q.State != StateOpen && q.State != StateEscalated {
		return nil // already answered/closed
	}
	ans := strings.TrimSpace(answerText)
	if ans == "" {
		ans = "az engedély alapján"
	}

	now := s.Clock.Now()
	evPayload := Event{
		QuestionID:      q.ID.String(),
		Number:          q.Number,
		FormattedNumber: q.FormattedNumber(),
		AskedBy:         q.AskedBy.String(),
		ChannelID:       q.ChannelID.String(),
		ThreadRootID:    q.ThreadRootID.String(),
		State:           StateAnswered,
		Answer:          ans,
		AnsweredBy:      closedBy.String(),
	}

	return s.record(ctx, q.WorkspaceID, closedBy, EventAnswered, q.ID, evPayload, func(ctx context.Context) error {
		_, ok, err := s.Store.Answer(ctx, q.ID, ans, &closedBy, now)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		return nil
	})
}

// Withdraw withdraws an open or escalated question. Only the asker may withdraw.
func (s *Service) Withdraw(ctx context.Context, by identity.Actor, questionRef string) (Question, error) {
	if by.Halted {
		return Question{}, ErrForbidden
	}
	q, err := s.resolve(ctx, by.WorkspaceID, questionRef)
	if err != nil {
		return Question{}, err
	}
	if q.WorkspaceID != by.WorkspaceID {
		return Question{}, ErrForbidden
	}
	if by.ID != q.AskedBy {
		return Question{}, ErrForbidden
	}
	if q.State != StateOpen && q.State != StateEscalated {
		return Question{}, ErrQuestionNotOpen
	}

	// Post withdraw message to thread
	msgBody := fmt.Sprintf("%s withdrawn by asker.", q.FormattedNumber())
	_, _ = s.Conversation.SendMessage(ctx, by, conversation.SendInput{
		ChannelID:      q.ChannelID,
		ThreadRootID:   q.ThreadRootID,
		Body:           msgBody,
		Elaborate:      true,
		NoThreadFollow: true,
	})

	now := s.Clock.Now()
	var updated Question
	evPayload := Event{
		QuestionID:      q.ID.String(),
		Number:          q.Number,
		FormattedNumber: q.FormattedNumber(),
		AskedBy:         q.AskedBy.String(),
		ChannelID:       q.ChannelID.String(),
		ThreadRootID:    q.ThreadRootID.String(),
		State:           StateWithdrawn,
	}

	err = s.record(ctx, by.WorkspaceID, by.ID, EventWithdrawn, q.ID, evPayload, func(ctx context.Context) error {
		u, ok, err := s.Store.Withdraw(ctx, q.ID, now)
		if err != nil {
			return err
		}
		if !ok {
			return ErrQuestionNotOpen
		}
		updated = u
		return nil
	})
	if err != nil {
		return Question{}, err
	}
	return updated, nil
}

// WithdrawSilently withdraws an open question without posting any chat messages.
func (s *Service) WithdrawSilently(ctx context.Context, questionID ids.ID) error {
	q, err := s.Store.GetByID(ctx, questionID)
	if err != nil {
		return err
	}
	if q.State != StateOpen && q.State != StateEscalated {
		return nil
	}
	now := s.Clock.Now()
	evPayload := Event{
		QuestionID:      q.ID.String(),
		Number:          q.Number,
		FormattedNumber: q.FormattedNumber(),
		AskedBy:         q.AskedBy.String(),
		ChannelID:       q.ChannelID.String(),
		ThreadRootID:    q.ThreadRootID.String(),
		State:           StateWithdrawn,
	}
	return s.record(ctx, q.WorkspaceID, q.AskedBy, EventWithdrawn, q.ID, evPayload, func(ctx context.Context) error {
		_, _, err := s.Store.Withdraw(ctx, q.ID, now)
		return err
	})
}

// Get loads a question by its reference.
func (s *Service) Get(ctx context.Context, by identity.Actor, questionRef string) (Question, error) {
	q, err := s.resolve(ctx, by.WorkspaceID, questionRef)
	if err != nil {
		return Question{}, err
	}
	if q.WorkspaceID != by.WorkspaceID {
		return Question{}, ErrForbidden
	}
	return q, nil
}

// List lists questions in the workspace, optionally filtered by state.
func (s *Service) List(ctx context.Context, by identity.Actor, state *string) ([]Question, error) {
	return s.Store.List(ctx, by.WorkspaceID, state)
}

// RunDue processes open questions to agents that reached their deadline or 30-min escalation window.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	escalateBefore := now.Add(-30 * time.Minute)
	due, err := s.Store.Due(ctx, escalateBefore, now, 50)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, q := range due {
		// Check deadline first
		if q.Deadline != nil && !now.Before(*q.Deadline) {
			if q.DefaultOption != nil && *q.DefaultOption != "" {
				// Fallback to default
				answer := *q.DefaultOption
				var updated Question
				var claimed bool
				err := s.Tx.InTx(ctx, func(ctx context.Context) error {
					u, ok, err := s.Store.Answer(ctx, q.ID, answer, nil, now)
					if err != nil {
						return err
					}
					claimed = ok
					updated = u
					return nil
				})
				if err != nil || !claimed {
					continue
				}

				asker, err := s.Directory.ActorByID(ctx, q.AskedBy)
				var msgID string
				if err == nil {
					body := fmt.Sprintf("%s deadline reached; falling back to default: %s", q.FormattedNumber(), answer)
					res, err := s.Conversation.SendMessage(ctx, asker, conversation.SendInput{
						ChannelID:    q.ChannelID,
						ThreadRootID: q.ThreadRootID,
						Body:         body,
						ClientMsgID:  fmt.Sprintf("q-fb-%s", q.ID),
						Elaborate:    true,
					})
					if err == nil && res.Message != nil {
						msgID = res.Message.ID.String()
					}
				}

				evPayload := Event{
					QuestionID:      q.ID.String(),
					Number:          q.Number,
					FormattedNumber: q.FormattedNumber(),
					AskedBy:         q.AskedBy.String(),
					ChannelID:       q.ChannelID.String(),
					ThreadRootID:    q.ThreadRootID.String(),
					State:           StateAnswered,
					Answer:          answer,
					AnswerMessageID: msgID,
				}
				_ = s.recordEvent(ctx, q.WorkspaceID, q.AskedBy, EventAnswered, q.ID, evPayload)
				processed++
				_ = updated
				continue
			}

			// Expire
			var claimed bool
			err := s.Tx.InTx(ctx, func(ctx context.Context) error {
				_, ok, err := s.Store.Expire(ctx, q.ID, now)
				if err != nil {
					return err
				}
				claimed = ok
				return nil
			})
			if err != nil || !claimed {
				continue
			}

			asker, err := s.Directory.ActorByID(ctx, q.AskedBy)
			if err == nil {
				body := fmt.Sprintf("%s deadline reached without answer (expired).", q.FormattedNumber())
				_, _ = s.Conversation.SendMessage(ctx, asker, conversation.SendInput{
					ChannelID:    q.ChannelID,
					ThreadRootID: q.ThreadRootID,
					Body:         body,
					ClientMsgID:  fmt.Sprintf("q-exp-%s", q.ID),
					Elaborate:    true,
				})
			}

			evPayload := Event{
				QuestionID:      q.ID.String(),
				Number:          q.Number,
				FormattedNumber: q.FormattedNumber(),
				AskedBy:         q.AskedBy.String(),
				ChannelID:       q.ChannelID.String(),
				ThreadRootID:    q.ThreadRootID.String(),
				State:           StateExpired,
			}
			_ = s.recordEvent(ctx, q.WorkspaceID, q.AskedBy, EventExpired, q.ID, evPayload)
			processed++
			continue
		}

		// Check 30-min escalation
		if !now.Before(q.CreatedAt.Add(30 * time.Minute)) {
			var claimed bool
			err := s.Tx.InTx(ctx, func(ctx context.Context) error {
				_, ok, err := s.Store.Escalate(ctx, q.ID, now)
				if err != nil {
					return err
				}
				claimed = ok
				return nil
			})
			if err != nil || !claimed {
				continue
			}

			asker, err := s.Directory.ActorByID(ctx, q.AskedBy)
			if err == nil {
				admins, _ := s.Directory.Admins(ctx, q.WorkspaceID)
				mentions := make([]string, 0, len(admins))
				for _, a := range admins {
					mentions = append(mentions, "@"+a.Handle)
				}
				mStr := strings.Join(mentions, " ")
				if mStr != "" {
					mStr += " "
				}
				body := fmt.Sprintf("%s%s was unanswered for 30 minutes and has been escalated to humans.", mStr, q.FormattedNumber())
				_, _ = s.Conversation.SendMessage(ctx, asker, conversation.SendInput{
					ChannelID:    q.ChannelID,
					ThreadRootID: q.ThreadRootID,
					Body:         body,
					ClientMsgID:  fmt.Sprintf("q-esc-%s", q.ID),
					Elaborate:    true,
				})
			}

			evPayload := Event{
				QuestionID:      q.ID.String(),
				Number:          q.Number,
				FormattedNumber: q.FormattedNumber(),
				AskedBy:         q.AskedBy.String(),
				ChannelID:       q.ChannelID.String(),
				ThreadRootID:    q.ThreadRootID.String(),
				State:           StateEscalated,
			}
			_ = s.recordEvent(ctx, q.WorkspaceID, q.AskedBy, EventEscalated, q.ID, evPayload)
			processed++
			continue
		}
	}
	return processed, nil
}

// Loop runs RunDue periodically until ctx ends.
func (s *Service) Loop(ctx context.Context, interval time.Duration, logf func(msg string, args ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.RunDue(ctx); err != nil && ctx.Err() == nil {
			logf("questions: run failed", "err", err)
		} else if n > 0 {
			logf("questions: processed", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) resolve(ctx context.Context, workspaceID ids.ID, ref string) (Question, error) {
	id, num, isNum, err := ParseReference(ref)
	if err != nil {
		return Question{}, err
	}
	if isNum {
		return s.Store.GetByNumber(ctx, workspaceID, num)
	}
	return s.Store.GetByID(ctx, id)
}

func (s *Service) record(ctx context.Context, workspaceID, actorID ids.ID, typ string, aggID ids.ID, payload any, change func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "question", aggID, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil {
		s.Bus.Notify(seq)
	}
	return err
}

// Reopen sets an answered question back to open state.
func (s *Service) Reopen(ctx context.Context, id ids.ID) error {
	return s.Store.Reopen(ctx, id)
}

func (s *Service) recordEvent(ctx context.Context, workspaceID, actorID ids.ID, typ string, aggID ids.ID, payload any) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "question", aggID, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil {
		s.Bus.Notify(seq)
	}
	return err
}
