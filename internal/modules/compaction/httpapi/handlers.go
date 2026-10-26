package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(compaction.ErrForbidden)
	httpx.RegisterValidation(compaction.ErrInvalidThreshold)
	httpx.RegisterValidation(compaction.ErrNoScribe)
	httpx.RegisterValidation(compaction.ErrEmptyConversation)
	httpx.RegisterValidation(compaction.ErrSummaryTooLong)
	httpx.RegisterStatus(http.StatusConflict, compaction.ErrPendingCompaction)
	httpx.RegisterStatus(http.StatusNotFound, compaction.ErrNotFound)
}

func CompactionSettingsToAPI(st compaction.Settings) apispec.CompactionSettings {
	var compactorID *ids.ID
	if st.CompactorAgentID != ids.Nil {
		compactorID = &st.CompactorAgentID
	}
	var compactorHandle *string
	if st.CompactorHandle != "" {
		compactorHandle = &st.CompactorHandle
	}
	return apispec.CompactionSettings{
		CompactThresholdTokens: st.CompactThresholdTokens,
		CompactorAgentId:       compactorID,
		CompactorHandle:        compactorHandle,
		UpdatedAt:              st.UpdatedAt,
	}
}

func CompactionUnitStatusToAPI(u compaction.CompactionUnit, threshold int, hasPending bool) apispec.CompactionUnitStatus {
	var threadRootID *ids.ID
	if u.ThreadRootID != ids.Nil {
		threadRootID = &u.ThreadRootID
	}
	var lastCompID *ids.ID
	if u.LastCompactionID != ids.Nil {
		lastCompID = &u.LastCompactionID
	}
	return apispec.CompactionUnitStatus{
		ChannelId:            u.ChannelID,
		ThreadRootId:         threadRootID,
		ApproxTokensSince:    u.ApproxTokensSince,
		MessagesSince:        u.MessagesSince,
		ThresholdTokens:      threshold,
		HasPendingCompaction: hasPending,
		LastCompactionId:     lastCompID,
	}
}

func channelToAPI(c conversation.Channel) apispec.Channel {
	var desc *string
	if c.Description != "" {
		desc = &c.Description
	}
	return apispec.Channel{
		Id:                     c.ID,
		Kind:                   apispec.ChannelKind(c.Kind),
		Name:                   c.Name,
		Description:            desc,
		CompactThresholdTokens: c.CompactThresholdTokens,
	}
}

func CompactionToAPI(c compaction.Compaction) apispec.Compaction {
	var threadRootID *ids.ID
	if c.ThreadRootID != ids.Nil {
		threadRootID = &c.ThreadRootID
	}
	var upToMsgID *ids.ID
	if c.UpToMessageID != ids.Nil {
		upToMsgID = &c.UpToMessageID
	}
	var prevCompID *ids.ID
	if c.PreviousCompactionID != ids.Nil {
		prevCompID = &c.PreviousCompactionID
	}
	var reqBy *ids.ID
	if c.RequestedBy != ids.Nil {
		reqBy = &c.RequestedBy
	}
	var compAgentID *ids.ID
	if c.CompactorAgentID != ids.Nil {
		compAgentID = &c.CompactorAgentID
	}
	var summaryFileID *ids.ID
	if c.SummaryFileID != ids.Nil {
		summaryFileID = &c.SummaryFileID
	}
	var sysMsgID *ids.ID
	if c.SystemMessageID != ids.Nil {
		sysMsgID = &c.SystemMessageID
	}
	var errCode *string
	if c.ErrorCode != "" {
		errCode = &c.ErrorCode
	}

	return apispec.Compaction{
		Id:                   c.ID,
		WorkspaceId:          c.WorkspaceID,
		ChannelId:            c.ChannelID,
		ThreadRootId:         threadRootID,
		UpToMessageId:        upToMsgID,
		PreviousCompactionId: prevCompID,
		Trigger:              apispec.CompactionTrigger(c.Trigger),
		RequestedBy:          reqBy,
		CompactorAgentId:     compAgentID,
		State:                apispec.CompactionState(c.State),
		SummaryFileId:        summaryFileID,
		SystemMessageId:      sysMsgID,
		Stats: &apispec.CompactionStats{
			Messages:       &c.MessagesCount,
			ApproxTokensIn: &c.ApproxTokensIn,
			SummaryTokens:  &c.SummaryTokens,
		},
		ErrorCode:   errCode,
		CreatedAt:   c.CreatedAt,
		CompletedAt: c.CompletedAt,
	}
}

func ScribeTaskToAPI(t compaction.ScribeTask) apispec.ScribeCompactionTask {
	var threadRootID *ids.ID
	if t.ThreadRootID != ids.Nil {
		threadRootID = &t.ThreadRootID
	}
	var prevSummary *string
	if t.PreviousSummary != "" {
		prevSummary = &t.PreviousSummary
	}
	msgs := make([]apispec.CompactionTaskMessage, len(t.Messages))
	for i, m := range t.Messages {
		atts := m.Attachments
		if atts == nil {
			atts = []string{}
		}
		msgs[i] = apispec.CompactionTaskMessage{
			Id:           m.ID,
			AuthorHandle: m.AuthorHandle,
			AuthorKind:   m.AuthorKind,
			Body:         m.Body,
			Attachments:  &atts,
			CreatedAt:    m.CreatedAt,
		}
	}
	maxChars := t.MaxSummaryChars
	return apispec.ScribeCompactionTask{
		Id:              t.ID,
		ChannelId:       t.ChannelID,
		ThreadRootId:    threadRootID,
		PreviousSummary: prevSummary,
		Messages:        msgs,
		MaxSummaryChars: &maxChars,
	}
}

func AgentContextToAPI(c compaction.AgentContext) apispec.AgentContext {
	var compID *ids.ID
	if c.CompactionID != ids.Nil {
		compID = &c.CompactionID
	}
	var summaryText *string
	if c.SummaryMarkdown != "" {
		summaryText = &c.SummaryMarkdown
	}
	msgs := make([]apispec.ContextMessage, len(c.MessagesAfter))
	for i, m := range c.MessagesAfter {
		atts := m.Attachments
		if atts == nil {
			atts = []string{}
		}
		msgs[i] = apispec.ContextMessage{
			Id:           m.ID,
			AuthorHandle: m.AuthorHandle,
			AuthorKind:   m.AuthorKind,
			Body:         m.Body,
			Attachments:  &atts,
			CreatedAt:    m.CreatedAt,
		}
	}
	return apispec.AgentContext{
		CompactionId:    compID,
		SummaryMarkdown: summaryText,
		MessagesAfter:   msgs,
		HasMore:         c.HasMore,
	}
}

func Register(mux *http.ServeMux, svc *compaction.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/settings/compaction", wrap(func(w http.ResponseWriter, r *http.Request) {
		st, err := svc.GetSettings(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionSettingsToAPI(st))
	}))

	mux.Handle("PUT /api/v1/settings/compaction", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.UpdateCompactionSettingsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var compactorAgentID ids.ID
		if in.CompactorAgentId != nil {
			compactorAgentID = *in.CompactorAgentId
		}
		st, err := svc.UpdateSettings(r.Context(), httpx.Actor(r), in.CompactThresholdTokens, compactorAgentID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionSettingsToAPI(st))
	}))

	mux.Handle("GET /api/v1/channels/{channelId}/compaction", wrap(func(w http.ResponseWriter, r *http.Request) {
		chID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var threadRootID ids.ID
		if q := r.URL.Query().Get("thread_root_id"); q != "" {
			if id, err := ids.Parse(q); err == nil {
				threadRootID = id
			}
		}
		unit, thresh, hasPending, err := svc.GetChannelCompaction(r.Context(), httpx.Actor(r), chID, threadRootID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionUnitStatusToAPI(unit, thresh, hasPending))
	}))

	mux.Handle("PATCH /api/v1/channels/{channelId}/compaction", wrap(func(w http.ResponseWriter, r *http.Request) {
		chID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var in apispec.UpdateChannelCompactionRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		ch, err := svc.UpdateChannelCompaction(r.Context(), httpx.Actor(r), chID, in.CompactThresholdTokens)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, channelToAPI(ch))
	}))

	mux.Handle("POST /api/v1/channels/{channelId}/compactions", wrap(func(w http.ResponseWriter, r *http.Request) {
		chID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var threadRootID ids.ID
		var body struct {
			ThreadRootID *ids.ID `json:"thread_root_id"`
		}
		_ = httpx.Decode(r, &body)
		if body.ThreadRootID != nil {
			threadRootID = *body.ThreadRootID
		}
		comp, err := svc.TriggerCompaction(r.Context(), httpx.Actor(r), chID, threadRootID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, CompactionToAPI(comp))
	}))

	mux.Handle("GET /api/v1/agent/compactions/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		compID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		task, err := svc.GetScribeTask(r.Context(), httpx.Actor(r), compID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, ScribeTaskToAPI(task))
	}))

	mux.Handle("POST /api/v1/agent/compactions/{id}/result", wrap(func(w http.ResponseWriter, r *http.Request) {
		compID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var in apispec.CompactionResultRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var markdown string
		if in.Markdown != nil {
			markdown = *in.Markdown
		}
		var errorCode string
		if in.ErrorCode != nil {
			errorCode = *in.ErrorCode
		}
		comp, err := svc.SubmitScribeResult(r.Context(), httpx.Actor(r), compID, compaction.ScribeResultInput{
			Markdown:  markdown,
			ErrorCode: errorCode,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionToAPI(comp))
	}))

	mux.Handle("PUT /api/v1/compactions/{id}/summary", wrap(func(w http.ResponseWriter, r *http.Request) {
		compID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var in apispec.UpdateCompactionSummaryRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		comp, err := svc.EditSummary(r.Context(), httpx.Actor(r), compID, in.Markdown)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionToAPI(comp))
	}))

	mux.Handle("POST /api/v1/compactions/{id}/regenerate", wrap(func(w http.ResponseWriter, r *http.Request) {
		compID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		comp, err := svc.Regenerate(r.Context(), httpx.Actor(r), compID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CompactionToAPI(comp))
	}))

	mux.Handle("GET /api/v1/agent/context", wrap(func(w http.ResponseWriter, r *http.Request) {
		chID, err := ids.Parse(r.URL.Query().Get("channel_id"))
		if err != nil {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}
		var threadRootID ids.ID
		if q := r.URL.Query().Get("thread_root_id"); q != "" {
			if id, err := ids.Parse(q); err == nil {
				threadRootID = id
			}
		}
		agentCtx, err := svc.GetAgentContext(r.Context(), httpx.Actor(r), chID, threadRootID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, AgentContextToAPI(agentCtx))
	}))
}
