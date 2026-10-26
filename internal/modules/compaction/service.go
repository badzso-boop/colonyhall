package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

var (
	ErrForbidden         = errors.New("compaction: forbidden")
	ErrNotFound          = errors.New("compaction: not found")
	ErrInvalidThreshold  = errors.New("compaction: threshold must be between 20000 and 2000000")
	ErrNoScribe          = errors.New("compaction: no compactor agent configured in workspace settings")
	ErrPendingCompaction = errors.New("compaction: compaction already pending for this unit")
	ErrEmptyConversation = errors.New("compaction: no messages in unit to compact")
	ErrSummaryTooLong    = errors.New("compaction: summary markdown must be at most 40000 characters")
)

type FilesManager interface {
	Upload(ctx context.Context, by identity.Actor, channelID ids.ID, filename string, r io.Reader) (files.File, error)
	OpenContent(ctx context.Context, by identity.Actor, fileID ids.ID) (files.File, io.ReadCloser, error)
}

type Service struct {
	Store        Store
	Conversation conversation.Reader
	Directory    identity.Directory
	Files        FilesManager
	Events       eventlog.Store
	Bus          bus.Bus
	Clock        clock.Clock
	IDs          ids.Generator
	Tx           db.TxRunner
}

func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	switch e.Type {
	case conversation.EventMessageCreated:
		var p conversation.MessageCreated
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("compaction: bad %s payload: %w", e.Type, err)
		}
		return s.onMessageCreated(ctx, e, p)
	}
	return nil
}

func (s *Service) onMessageCreated(ctx context.Context, e eventlog.Event, p conversation.MessageCreated) error {
	if p.Kind == "system" {
		return nil
	}
	msgID, err := ids.Parse(p.MessageID)
	if err != nil {
		return err
	}
	chID, err := ids.Parse(p.ChannelID)
	if err != nil {
		return err
	}
	var threadRootID ids.ID
	if p.ThreadRootID != "" {
		if root, err := ids.Parse(p.ThreadRootID); err == nil {
			threadRootID = root
		}
	}

	// Approximate token count: body / 4 rounded up + attachments by name
	tokens := (len(p.Body) + 3) / 4
	for _, att := range p.Attachments {
		tokens += (len(att.Name) + 3) / 4
	}
	if tokens < 1 {
		tokens = 1
	}

	now := s.Clock.Now()
	unit, err := s.Store.AddTokensToUnit(ctx, e.WorkspaceID, chID, threadRootID, tokens, 1, now)
	if err != nil {
		return err
	}

	// Determine threshold: channel override or workspace default
	threshold := DefaultThresholdTokens
	chThresh, err := s.Store.GetChannelThreshold(ctx, chID)
	if err == nil && chThresh != nil {
		threshold = *chThresh
	} else {
		st, err := s.Store.GetSettings(ctx, e.WorkspaceID)
		if err == nil {
			threshold = st.CompactThresholdTokens
		}
	}

	if unit.ApproxTokensSince < threshold {
		return nil
	}

	st, err := s.Store.GetSettings(ctx, e.WorkspaceID)
	if err != nil || st.CompactorAgentID == ids.Nil {
		return nil // No scribe agent configured
	}

	// Check if there is an existing pending compaction
	pending, err := s.Store.PendingCompaction(ctx, chID, threadRootID)
	if err == nil {
		if now.Sub(pending.CreatedAt) > 15*time.Minute {
			_ = s.Store.FailPendingCompaction(ctx, pending.ID, "timeout", now)
			_ = s.appendAndPublish(ctx, e.WorkspaceID, EventFailed, pending.ID, CompactionFailed{
				CompactionID: pending.ID.String(),
				WorkspaceID:  e.WorkspaceID.String(),
				ChannelID:    chID.String(),
				ErrorCode:    "timeout",
			})
		} else {
			return nil // Already pending
		}
	}

	prev, _ := s.Store.LatestDoneCompaction(ctx, chID, threadRootID)
	comp := Compaction{
		ID:                   s.IDs.New(),
		WorkspaceID:          e.WorkspaceID,
		ChannelID:            chID,
		ThreadRootID:         threadRootID,
		UpToMessageID:        msgID,
		PreviousCompactionID: prev.ID,
		Trigger:              TriggerAuto,
		CompactorAgentID:     st.CompactorAgentID,
		State:                StatePending,
		CreatedAt:            now,
	}

	if err := s.Store.InsertCompaction(ctx, comp); err != nil {
		return err
	}

	return s.appendAndPublish(ctx, e.WorkspaceID, EventCreated, comp.ID, CompactionCreated{
		CompactionID:     comp.ID.String(),
		WorkspaceID:      e.WorkspaceID.String(),
		ChannelID:        chID.String(),
		ThreadRootID:     threadRootID.String(),
		CompactorAgentID: st.CompactorAgentID.String(),
		Trigger:          string(comp.Trigger),
	})
}

func (s *Service) appendAndPublish(ctx context.Context, workspaceID ids.ID, typ string, compID ids.ID, payload any) error {
	ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, ids.Nil, ids.Nil, "compaction", compID, payload, s.Clock.Now())
	if err != nil {
		return err
	}
	if s.Events != nil {
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
	}
	if s.Bus != nil {
		s.Bus.Notify(ev.Seq)
	}
	return nil
}

func (s *Service) GetSettings(ctx context.Context, by identity.Actor) (Settings, error) {
	if by.Kind != identity.KindHuman {
		return Settings{}, ErrForbidden
	}
	st, err := s.Store.GetSettings(ctx, by.WorkspaceID)
	if err != nil {
		return Settings{}, err
	}
	if st.CompactorAgentID != ids.Nil && s.Directory != nil {
		if actor, err := s.Directory.ActorByID(ctx, st.CompactorAgentID); err == nil {
			st.CompactorHandle = actor.Handle
		}
	}
	return st, nil
}

func (s *Service) UpdateSettings(ctx context.Context, by identity.Actor, threshold int, compactorAgentID ids.ID) (Settings, error) {
	if by.Role != identity.RoleOwner && by.Role != identity.RoleAdmin {
		return Settings{}, ErrForbidden
	}
	if threshold < MinThresholdTokens || threshold > MaxThresholdTokens {
		return Settings{}, ErrInvalidThreshold
	}
	if compactorAgentID != ids.Nil && s.Directory != nil {
		actor, err := s.Directory.ActorByID(ctx, compactorAgentID)
		if err != nil || actor.Kind != identity.KindAgent {
			return Settings{}, fmt.Errorf("compactor actor must be an agent: %w", err)
		}
	}
	st := Settings{
		WorkspaceID:            by.WorkspaceID,
		CompactThresholdTokens: threshold,
		CompactorAgentID:       compactorAgentID,
		UpdatedAt:              s.Clock.Now(),
	}
	if err := s.Store.UpsertSettings(ctx, st); err != nil {
		return Settings{}, err
	}
	if st.CompactorAgentID != ids.Nil && s.Directory != nil {
		if actor, err := s.Directory.ActorByID(ctx, st.CompactorAgentID); err == nil {
			st.CompactorHandle = actor.Handle
		}
	}
	return st, nil
}

func (s *Service) GetChannelCompaction(ctx context.Context, by identity.Actor, channelID, threadRootID ids.ID) (CompactionUnit, int, bool, error) {
	canRead, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil || !canRead {
		return CompactionUnit{}, 0, false, ErrForbidden
	}

	threshold := DefaultThresholdTokens
	chThresh, err := s.Store.GetChannelThreshold(ctx, channelID)
	if err == nil && chThresh != nil {
		threshold = *chThresh
	} else {
		st, err := s.Store.GetSettings(ctx, by.WorkspaceID)
		if err == nil {
			threshold = st.CompactThresholdTokens
		}
	}

	unit, err := s.Store.GetUnit(ctx, channelID, threadRootID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			unit = CompactionUnit{
				ChannelID:    channelID,
				ThreadRootID: threadRootID,
			}
		} else {
			return CompactionUnit{}, 0, false, err
		}
	}

	hasPending := false
	pending, err := s.Store.PendingCompaction(ctx, channelID, threadRootID)
	if err == nil {
		if s.Clock.Now().Sub(pending.CreatedAt) > 15*time.Minute {
			_ = s.Store.FailPendingCompaction(ctx, pending.ID, "timeout", s.Clock.Now())
		} else {
			hasPending = true
		}
	}

	return unit, threshold, hasPending, nil
}

func (s *Service) UpdateChannelCompaction(ctx context.Context, by identity.Actor, channelID ids.ID, threshold *int) (conversation.Channel, error) {
	canWrite, err := s.Conversation.CanWrite(ctx, by, channelID)
	if err != nil || !canWrite {
		return conversation.Channel{}, ErrForbidden
	}
	if threshold != nil && (*threshold < MinThresholdTokens || *threshold > MaxThresholdTokens) {
		return conversation.Channel{}, ErrInvalidThreshold
	}
	if err := s.Store.SetChannelThreshold(ctx, channelID, threshold); err != nil {
		return conversation.Channel{}, err
	}
	ch, err := s.Conversation.ChannelByID(ctx, channelID)
	if err != nil {
		return conversation.Channel{}, err
	}
	ch.CompactThresholdTokens = threshold
	return ch, nil
}

func (s *Service) TriggerCompaction(ctx context.Context, by identity.Actor, channelID, threadRootID ids.ID) (Compaction, error) {
	if by.Kind != identity.KindHuman {
		return Compaction{}, ErrForbidden
	}
	canWrite, err := s.Conversation.CanWrite(ctx, by, channelID)
	if err != nil || !canWrite {
		return Compaction{}, ErrForbidden
	}

	st, err := s.Store.GetSettings(ctx, by.WorkspaceID)
	if err != nil || st.CompactorAgentID == ids.Nil {
		return Compaction{}, ErrNoScribe
	}

	now := s.Clock.Now()
	pending, err := s.Store.PendingCompaction(ctx, channelID, threadRootID)
	if err == nil {
		if now.Sub(pending.CreatedAt) > 15*time.Minute {
			_ = s.Store.FailPendingCompaction(ctx, pending.ID, "timeout", now)
		} else {
			return Compaction{}, ErrPendingCompaction
		}
	}

	msgs, err := s.Conversation.MessagesInUnit(ctx, channelID, threadRootID)
	if err != nil {
		return Compaction{}, err
	}
	if len(msgs) == 0 {
		return Compaction{}, ErrEmptyConversation
	}
	upToMsgID := msgs[len(msgs)-1].ID

	prev, _ := s.Store.LatestDoneCompaction(ctx, channelID, threadRootID)
	comp := Compaction{
		ID:                   s.IDs.New(),
		WorkspaceID:          by.WorkspaceID,
		ChannelID:            channelID,
		ThreadRootID:         threadRootID,
		UpToMessageID:        upToMsgID,
		PreviousCompactionID: prev.ID,
		Trigger:              TriggerManual,
		RequestedBy:          by.ID,
		CompactorAgentID:     st.CompactorAgentID,
		State:                StatePending,
		CreatedAt:            now,
	}

	if err := s.Store.InsertCompaction(ctx, comp); err != nil {
		return Compaction{}, err
	}

	_ = s.appendAndPublish(ctx, by.WorkspaceID, EventCreated, comp.ID, CompactionCreated{
		CompactionID:     comp.ID.String(),
		WorkspaceID:      by.WorkspaceID.String(),
		ChannelID:        channelID.String(),
		ThreadRootID:     threadRootID.String(),
		CompactorAgentID: st.CompactorAgentID.String(),
		RequestedBy:      by.ID.String(),
		Trigger:          string(comp.Trigger),
	})

	return comp, nil
}

func (s *Service) GetScribeTask(ctx context.Context, by identity.Actor, compactionID ids.ID) (ScribeTask, error) {
	comp, err := s.Store.CompactionByID(ctx, compactionID)
	if err != nil {
		return ScribeTask{}, ErrNotFound
	}
	if comp.CompactorAgentID != by.ID {
		return ScribeTask{}, ErrNotFound
	}

	var prevSummary string
	if comp.PreviousCompactionID != ids.Nil {
		prev, err := s.Store.CompactionByID(ctx, comp.PreviousCompactionID)
		if err == nil && prev.SummaryFileID != ids.Nil && s.Files != nil {
			_, r, err := s.Files.OpenContent(ctx, by, prev.SummaryFileID)
			if err == nil {
				defer func() { _ = r.Close() }()
				data, _ := io.ReadAll(r)
				prevSummary = string(data)
			}
		}
	}

	msgs, err := s.Conversation.MessagesInUnit(ctx, comp.ChannelID, comp.ThreadRootID)
	if err != nil {
		return ScribeTask{}, err
	}

	// Filter messages: after previous compaction's up_to_message_id up to comp.UpToMessageID
	var filtered []conversation.Message
	passedPrev := (comp.PreviousCompactionID == ids.Nil)
	for _, m := range msgs {
		if !passedPrev {
			if m.ID == comp.UpToMessageID {
				passedPrev = true
			}
			continue
		}
		filtered = append(filtered, m)
		if m.ID == comp.UpToMessageID {
			break
		}
	}
	if len(filtered) == 0 {
		filtered = msgs
	}

	// Fetch handles
	authorIDs := make([]ids.ID, len(filtered))
	for i, m := range filtered {
		authorIDs[i] = m.AuthorID
	}
	handles := map[ids.ID]string{}
	kinds := map[ids.ID]string{}
	if s.Directory != nil {
		if actors, err := s.Directory.ActorsByIDs(ctx, authorIDs); err == nil {
			for _, a := range actors {
				handles[a.ID] = a.Handle
				kinds[a.ID] = string(a.Kind)
			}
		}
	}

	taskMsgs := make([]TaskMessage, len(filtered))
	for i, m := range filtered {
		var attNames []string
		for _, a := range m.Attachments {
			attNames = append(attNames, a.Name)
		}
		taskMsgs[i] = TaskMessage{
			ID:           m.ID,
			AuthorHandle: handles[m.AuthorID],
			AuthorKind:   kinds[m.AuthorID],
			Body:         m.Body,
			Attachments:  attNames,
			CreatedAt:    m.CreatedAt,
		}
	}

	return ScribeTask{
		ID:              comp.ID,
		ChannelID:       comp.ChannelID,
		ThreadRootID:    comp.ThreadRootID,
		PreviousSummary: prevSummary,
		Messages:        taskMsgs,
		MaxSummaryChars: DefaultMaxSummaryChars,
	}, nil
}

type ScribeResultInput struct {
	Markdown  string
	ErrorCode string
}

func (s *Service) SubmitScribeResult(ctx context.Context, by identity.Actor, compactionID ids.ID, in ScribeResultInput) (Compaction, error) {
	comp, err := s.Store.CompactionByID(ctx, compactionID)
	if err != nil {
		return Compaction{}, ErrNotFound
	}
	if comp.CompactorAgentID != by.ID {
		return Compaction{}, ErrNotFound
	}
	if comp.State != StatePending {
		return comp, nil
	}

	now := s.Clock.Now()
	if in.ErrorCode != "" {
		if err := s.Store.FailPendingCompaction(ctx, comp.ID, in.ErrorCode, now); err != nil {
			return Compaction{}, err
		}
		_ = s.appendAndPublish(ctx, comp.WorkspaceID, EventFailed, comp.ID, CompactionFailed{
			CompactionID: comp.ID.String(),
			WorkspaceID:  comp.WorkspaceID.String(),
			ChannelID:    comp.ChannelID.String(),
			ErrorCode:    in.ErrorCode,
		})
		comp.State = StateFailed
		comp.ErrorCode = in.ErrorCode
		comp.CompletedAt = &now
		return comp, nil
	}

	if len(in.Markdown) > DefaultMaxSummaryChars {
		return Compaction{}, ErrSummaryTooLong
	}

	ch, err := s.Conversation.ChannelByID(ctx, comp.ChannelID)
	if err != nil {
		return Compaction{}, err
	}

	unitName := "main"
	if comp.ThreadRootID != ids.Nil {
		unitName = fmt.Sprintf("thread-%s", comp.ThreadRootID.String()[:8])
	}
	fileName := fmt.Sprintf("%s-%s-compaction-%d.md", ch.Name, unitName, now.Unix())

	file, err := s.Files.Upload(ctx, by, comp.ChannelID, fileName, strings.NewReader(in.Markdown))
	if err != nil {
		return Compaction{}, fmt.Errorf("upload summary file: %w", err)
	}

	msgs, _ := s.Conversation.MessagesInUnit(ctx, comp.ChannelID, comp.ThreadRootID)
	messagesCount := len(msgs)
	approxTokensIn := 0
	for _, m := range msgs {
		approxTokensIn += (len(m.Body) + 3) / 4
		for _, a := range m.Attachments {
			approxTokensIn += (len(a.Name) + 3) / 4
		}
	}
	summaryTokens := (len(in.Markdown) + 3) / 4

	var body string
	if comp.Trigger == TriggerAuto {
		body = fmt.Sprintf("🗜 Automatikus tömörítés — %d üzenet, ~%d token → ~%d tokenes kivonat", messagesCount, approxTokensIn, summaryTokens)
	} else {
		reqHandle := "admin"
		if comp.RequestedBy != ids.Nil && s.Directory != nil {
			if reqActor, err := s.Directory.ActorByID(ctx, comp.RequestedBy); err == nil {
				reqHandle = reqActor.Handle
			}
		}
		body = fmt.Sprintf("🗜 Tömörítés (kérte: @%s) — %d üzenet, ~%d token → ~%d tokenes kivonat", reqHandle, messagesCount, approxTokensIn, summaryTokens)
	}

	sysMsg, err := s.Conversation.PostSystemMessage(ctx, comp.WorkspaceID, comp.ChannelID, comp.ThreadRootID, by.ID, body, []ids.ID{file.ID})
	if err != nil {
		return Compaction{}, fmt.Errorf("post system message: %w", err)
	}

	if err := s.Store.CompleteCompaction(ctx, comp.ID, file.ID, sysMsg.ID, messagesCount, approxTokensIn, summaryTokens, now); err != nil {
		return Compaction{}, err
	}

	_ = s.Store.ResetUnit(ctx, comp.ChannelID, comp.ThreadRootID, comp.ID, 0, 0, now)

	_ = s.appendAndPublish(ctx, comp.WorkspaceID, EventDone, comp.ID, CompactionDone{
		CompactionID:    comp.ID.String(),
		WorkspaceID:     comp.WorkspaceID.String(),
		ChannelID:       comp.ChannelID.String(),
		ThreadRootID:    comp.ThreadRootID.String(),
		SummaryFileID:   file.ID.String(),
		SystemMessageID: sysMsg.ID.String(),
	})

	comp.State = StateDone
	comp.SummaryFileID = file.ID
	comp.SystemMessageID = sysMsg.ID
	comp.MessagesCount = messagesCount
	comp.ApproxTokensIn = approxTokensIn
	comp.SummaryTokens = summaryTokens
	comp.CompletedAt = &now
	return comp, nil
}

func (s *Service) EditSummary(ctx context.Context, by identity.Actor, compactionID ids.ID, markdown string) (Compaction, error) {
	if len(markdown) > DefaultMaxSummaryChars {
		return Compaction{}, ErrSummaryTooLong
	}
	existing, err := s.Store.CompactionByID(ctx, compactionID)
	if err != nil {
		return Compaction{}, ErrNotFound
	}
	canWrite, err := s.Conversation.CanWrite(ctx, by, existing.ChannelID)
	if err != nil || !canWrite {
		return Compaction{}, ErrForbidden
	}

	now := s.Clock.Now()
	ch, _ := s.Conversation.ChannelByID(ctx, existing.ChannelID)
	unitName := "main"
	if existing.ThreadRootID != ids.Nil {
		unitName = fmt.Sprintf("thread-%s", existing.ThreadRootID.String()[:8])
	}
	fileName := fmt.Sprintf("%s-%s-compaction-%d-edit.md", ch.Name, unitName, now.Unix())

	file, err := s.Files.Upload(ctx, by, existing.ChannelID, fileName, strings.NewReader(markdown))
	if err != nil {
		return Compaction{}, fmt.Errorf("upload edited summary file: %w", err)
	}

	summaryTokens := (len(markdown) + 3) / 4
	newComp := Compaction{
		ID:                   s.IDs.New(),
		WorkspaceID:          existing.WorkspaceID,
		ChannelID:            existing.ChannelID,
		ThreadRootID:         existing.ThreadRootID,
		UpToMessageID:        existing.UpToMessageID,
		PreviousCompactionID: existing.PreviousCompactionID,
		Trigger:              TriggerEdit,
		RequestedBy:          by.ID,
		CompactorAgentID:     existing.CompactorAgentID,
		State:                StateDone,
		SummaryFileID:        file.ID,
		SystemMessageID:      existing.SystemMessageID,
		MessagesCount:        existing.MessagesCount,
		ApproxTokensIn:       existing.ApproxTokensIn,
		SummaryTokens:        summaryTokens,
		CreatedAt:            now,
		CompletedAt:          &now,
	}

	if err := s.Store.InsertCompaction(ctx, newComp); err != nil {
		return Compaction{}, err
	}

	_ = s.Store.ResetUnit(ctx, existing.ChannelID, existing.ThreadRootID, newComp.ID, 0, 0, now)

	_ = s.appendAndPublish(ctx, by.WorkspaceID, EventDone, newComp.ID, CompactionDone{
		CompactionID:    newComp.ID.String(),
		WorkspaceID:     newComp.WorkspaceID.String(),
		ChannelID:       newComp.ChannelID.String(),
		ThreadRootID:    newComp.ThreadRootID.String(),
		SummaryFileID:   file.ID.String(),
		SystemMessageID: existing.SystemMessageID.String(),
	})

	return newComp, nil
}

func (s *Service) Regenerate(ctx context.Context, by identity.Actor, compactionID ids.ID) (Compaction, error) {
	existing, err := s.Store.CompactionByID(ctx, compactionID)
	if err != nil {
		return Compaction{}, ErrNotFound
	}
	canWrite, err := s.Conversation.CanWrite(ctx, by, existing.ChannelID)
	if err != nil || !canWrite {
		return Compaction{}, ErrForbidden
	}

	st, err := s.Store.GetSettings(ctx, by.WorkspaceID)
	if err != nil || st.CompactorAgentID == ids.Nil {
		return Compaction{}, ErrNoScribe
	}

	now := s.Clock.Now()
	newComp := Compaction{
		ID:                   s.IDs.New(),
		WorkspaceID:          existing.WorkspaceID,
		ChannelID:            existing.ChannelID,
		ThreadRootID:         existing.ThreadRootID,
		UpToMessageID:        existing.UpToMessageID,
		PreviousCompactionID: existing.ID,
		Trigger:              TriggerRegenerate,
		RequestedBy:          by.ID,
		CompactorAgentID:     st.CompactorAgentID,
		State:                StatePending,
		CreatedAt:            now,
	}

	if err := s.Store.InsertCompaction(ctx, newComp); err != nil {
		return Compaction{}, err
	}

	_ = s.appendAndPublish(ctx, by.WorkspaceID, EventCreated, newComp.ID, CompactionCreated{
		CompactionID:     newComp.ID.String(),
		WorkspaceID:      by.WorkspaceID.String(),
		ChannelID:        existing.ChannelID.String(),
		ThreadRootID:     existing.ThreadRootID.String(),
		CompactorAgentID: st.CompactorAgentID.String(),
		RequestedBy:      by.ID.String(),
		Trigger:          string(newComp.Trigger),
	})

	return newComp, nil
}

func (s *Service) GetAgentContext(ctx context.Context, by identity.Actor, channelID, threadRootID ids.ID) (AgentContext, error) {
	if by.Kind != identity.KindAgent {
		return AgentContext{}, ErrForbidden
	}
	canRead, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil || !canRead {
		return AgentContext{}, ErrForbidden
	}

	latest, err := s.Store.LatestDoneCompaction(ctx, channelID, threadRootID)
	var summaryText string
	var upToMsgID ids.ID
	if err == nil && latest.SummaryFileID != ids.Nil && s.Files != nil {
		_, r, err := s.Files.OpenContent(ctx, by, latest.SummaryFileID)
		if err == nil {
			defer func() { _ = r.Close() }()
			bytes, _ := io.ReadAll(r)
			summaryText = string(bytes)
			upToMsgID = latest.UpToMessageID
		}
	}

	msgs, err := s.Conversation.MessagesInUnit(ctx, channelID, threadRootID)
	if err != nil {
		return AgentContext{}, err
	}

	// Filter messages after upToMsgID
	var after []conversation.Message
	passed := (upToMsgID == ids.Nil)
	for _, m := range msgs {
		if !passed {
			if m.ID == upToMsgID {
				passed = true
			}
			continue
		}
		if latest.SystemMessageID != ids.Nil && m.ID == latest.SystemMessageID {
			continue
		}
		after = append(after, m)
	}

	hasMore := false
	if len(after) > 200 {
		hasMore = true
		after = after[len(after)-200:]
	}

	authorIDs := make([]ids.ID, len(after))
	for i, m := range after {
		authorIDs[i] = m.AuthorID
	}
	handles := map[ids.ID]string{}
	kinds := map[ids.ID]string{}
	if s.Directory != nil {
		if actors, err := s.Directory.ActorsByIDs(ctx, authorIDs); err == nil {
			for _, a := range actors {
				handles[a.ID] = a.Handle
				kinds[a.ID] = string(a.Kind)
			}
		}
	}

	ctxMsgs := make([]ContextMessage, len(after))
	for i, m := range after {
		var attNames []string
		for _, a := range m.Attachments {
			attNames = append(attNames, a.Name)
		}
		ctxMsgs[i] = ContextMessage{
			ID:           m.ID,
			AuthorHandle: handles[m.AuthorID],
			AuthorKind:   kinds[m.AuthorID],
			Body:         m.Body,
			Attachments:  attNames,
			CreatedAt:    m.CreatedAt,
		}
	}

	var compID ids.ID
	if latest.ID != ids.Nil {
		compID = latest.ID
	}

	return AgentContext{
		CompactionID:    compID,
		SummaryMarkdown: summaryText,
		MessagesAfter:   ctxMsgs,
		HasMore:         hasMore,
	}, nil
}
