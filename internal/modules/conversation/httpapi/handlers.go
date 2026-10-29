// Package httpapi exposes the conversation module over REST.
// Request and response shapes come from api/openapi.yaml (internal/apispec).
package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(conversation.ErrInvalidName, conversation.ErrInvalidDescription, conversation.ErrEmptyBody, conversation.ErrBodyTooLong,
		conversation.ErrInvalidClientID, conversation.ErrBadThread, conversation.ErrBadEmoji, conversation.ErrBadDM,
		conversation.ErrAgentTooLong, conversation.ErrTooManyMentions, conversation.ErrGroupMention,
		conversation.ErrNothingToAck)
	httpx.RegisterForbidden(conversation.ErrForbidden)
	httpx.RegisterStatus(http.StatusTooManyRequests, conversation.ErrConversationPaused)
	httpx.RegisterStatus(http.StatusConflict, conversation.ErrLastAdmin)
	httpx.RegisterStatus(http.StatusNotFound, conversation.ErrMemberNotFound)
}

// MessageToAPI converts a message to its API shape.
func MessageToAPI(m conversation.Message) apispec.Message {
	out := apispec.Message{Id: m.ID, ChannelId: m.ChannelID, AuthorId: m.AuthorID,
		ClientMsgId: httpx.Ptr(m.ClientMsgID), Kind: apispec.MessageKind(m.Kind), Body: m.Body,
		ReplyCount: m.ReplyCount, LastReplyAt: m.LastReplyAt, CreatedAt: m.CreatedAt,
		ThreadRootId: httpx.Ptr(m.ThreadRootID), Mentions: m.Mentions,
		DeletedAt: m.DeletedAt, DeletedBy: m.DeletedBy}
	if m.DeletedAt != nil {
		out.Body = "🗑 Visszavonva"
	}
	if m.CompactionID != ids.Nil {
		out.CompactionId = httpx.Ptr(m.CompactionID)
	}
	if m.OriginChannelID != ids.Nil && m.OriginThreadID != ids.Nil {
		out.Origin = &apispec.MessageOrigin{
			ChannelId: m.OriginChannelID,
			Channel:   m.OriginChannel,
			ThreadId:  m.OriginThreadID,
		}
	}
	if m.AnswersMessageID != ids.Nil && m.AnswersChannelID != ids.Nil {
		out.Answers = &apispec.MessageAnswers{
			MessageId: m.AnswersMessageID,
			ChannelId: m.AnswersChannelID,
			AuthorId:  m.AnswersAuthorID,
		}
	}
	if out.Mentions == nil {
		out.Mentions = []ids.ID{}
	}
	if len(m.Reactions) > 0 && m.DeletedAt == nil {
		rs := make([]apispec.ReactionSummary, 0, len(m.Reactions))
		for _, r := range m.Reactions {
			rs = append(rs, apispec.ReactionSummary{Emoji: r.Emoji, Count: r.Count, ActorIds: r.ActorIDs})
		}
		out.Reactions = &rs
	}
	if len(m.Attachments) > 0 && m.DeletedAt == nil {
		atts := make([]apispec.FileAttachment, 0, len(m.Attachments))
		for _, a := range m.Attachments {
			atts = append(atts, apispec.FileAttachment{
				Id:   a.ID,
				Name: a.Name,
				Mime: a.Mime,
				Size: a.Size,
			})
		}
		out.Attachments = &atts
	}
	if len(m.LinkCards) > 0 && m.DeletedAt == nil {
		lcs := make([]apispec.LinkCard, 0, len(m.LinkCards))
		for _, lc := range m.LinkCards {
			lcs = append(lcs, apispec.LinkCard{
				Url:    lc.URL,
				Kind:   apispec.LinkCardKind(lc.Kind),
				Title:  lc.Title,
				State:  lc.State,
				Author: httpx.Ptr(lc.Author),
			})
		}
		out.LinkCards = &lcs
	}
	return out
}

func channelToAPI(c conversation.ChannelView) apispec.Channel {
	var desc *string
	if c.Description != "" {
		desc = &c.Description
	}
	out := apispec.Channel{
		Id:                     c.ID,
		Kind:                   apispec.ChannelKind(c.Kind),
		Name:                   c.Name,
		Description:            desc,
		CompactThresholdTokens: c.CompactThresholdTokens,
		IsMember:               c.IsMember,
		Unread:                 c.Unread,
		Mentions:               c.Mentions,
	}
	if c.MemberIDs != nil {
		out.MemberIds = &c.MemberIDs
	}
	return out
}

// ChannelToAPI converts a domain Channel to an OpenAPI Channel.
func ChannelToAPI(c conversation.Channel) apispec.Channel {
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

// SendResultToAPI converts a send outcome.
func SendResultToAPI(r conversation.SendResult) apispec.SendResult {
	var out apispec.SendResult
	if r.Message != nil {
		m := MessageToAPI(*r.Message)
		out.Message = &m
	}
	if c := r.ConvertedToReaction; c != nil {
		out.ConvertedToReaction = &apispec.Reaction{MessageId: c.MessageID, ActorId: c.ActorID, Emoji: c.Emoji}
	}
	return out
}

// Register mounts the conversation routes on mux. Every route requires auth.
func Register(mux *http.ServeMux, svc *conversation.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/channels", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListChannels(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ChannelList{Channels: make([]apispec.Channel, 0, len(list))}
		for _, c := range list {
			out.Channels = append(out.Channels, channelToAPI(c))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/channels", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateChannelRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		kind := conversation.ChannelKind(httpx.Deref(in.Kind, apispec.Public))
		var desc string
		if in.Description != nil {
			desc = *in.Description
		}
		c, err := svc.CreateChannel(r.Context(), httpx.Actor(r), in.Name, desc, kind)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, channelToAPI(conversation.ChannelView{Channel: c, IsMember: true}))
	}))

	mux.Handle("GET /api/v1/channels/{id}/members", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		members, err := svc.ListMembers(r.Context(), httpx.Actor(r), channelID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ChannelMemberList{
			Members: make([]apispec.ChannelMember, len(members)),
		}
		for i, m := range members {
			out.Members[i] = apispec.ChannelMember{
				ActorId:  m.ActorID,
				Handle:   m.Handle,
				Kind:     apispec.ActorKind(m.Kind),
				CanWrite: m.CanWrite,
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/channels/{id}/members", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var in apispec.AddMemberRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := svc.AddMember(r.Context(), httpx.Actor(r), channelID, in.ActorId, httpx.Deref(in.CanWrite, true)); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("DELETE /api/v1/channels/{id}/members/{actorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		actorID, ok := pathID(w, r, "actorId")
		if !ok {
			return
		}
		if err := svc.RemoveMember(r.Context(), httpx.Actor(r), channelID, actorID); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/channels/{id}/messages", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		before := ids.Nil
		if b := r.URL.Query().Get("before"); b != "" {
			var err error
			if before, err = ids.Parse(b); err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "before must be a message ID"})
				return
			}
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		msgs, err := svc.ListMessages(r.Context(), httpx.Actor(r), channelID, before, limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.MessageList{Messages: make([]apispec.Message, 0, len(msgs))}
		for _, m := range msgs {
			out.Messages = append(out.Messages, MessageToAPI(m))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/channels/{id}/messages", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			apispec.SendMessageRequest
			Silent *bool `json:"silent,omitempty"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		actor := httpx.Actor(r)
		if in.Silent != nil && *in.Silent && actor.Kind == identity.KindAgent {
			httpx.Error(w, r, identity.ErrForbidden)
			return
		}
		var fileIDs []ids.ID
		if in.FileIds != nil {
			fileIDs = *in.FileIds
		}
		silent := in.Silent != nil && *in.Silent && actor.Kind != identity.KindAgent
		res, err := svc.SendMessage(r.Context(), actor, conversation.SendInput{
			ChannelID: channelID, ThreadRootID: httpx.Deref(in.ThreadRootId, ids.Nil),
			ClientMsgID: httpx.Deref(in.ClientMsgId, ""), Body: in.Body, Elaborate: httpx.Deref(in.Elaborate, false),
			FileIDs: fileIDs, Silent: silent,
			AnswersMessageID: httpx.Deref(in.AnswersMessageId, ids.Nil),
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		status := http.StatusCreated
		if res.Message == nil {
			status = http.StatusOK
		}
		httpx.JSON(w, status, SendResultToAPI(res))
	}))

	mux.Handle("POST /api/v1/agent/terminal-start", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.TerminalStartRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindAgent {
			httpx.Error(w, r, identity.ErrForbidden)
			return
		}
		if !terminalName.MatchString(in.Name) {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid terminal name"})
			return
		}
		chID := in.ChannelId
		threadRootID := httpx.Deref(in.ThreadRootId, ids.Nil)
		// The server builds the text: a silent message must not carry free text
		// from an agent.
		body := fmt.Sprintf("🖥 %s elindult", in.Name)
		if cmd := firstLine(httpx.Deref(in.Command, "")); cmd != "" {
			body += ": `" + inertCode(cmd) + "`"
		}
		_, err := svc.SendMessage(r.Context(), actor, conversation.SendInput{
			ChannelID:    chID,
			ThreadRootID: threadRootID,
			Body:         body,
			Silent:       true,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/agent/retract-note", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.RetractNoteRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindAgent {
			httpx.Error(w, r, identity.ErrForbidden)
			return
		}
		threadRootID := httpx.Deref(in.ThreadRootId, ids.Nil)
		_, err := svc.SendMessage(r.Context(), actor, conversation.SendInput{
			ChannelID:    in.ChannelId,
			ThreadRootID: threadRootID,
			Body:         RetractNoteText,
			Silent:       true,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/agent/ratelimit-note", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.RateLimitNoteRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindAgent {
			httpx.Error(w, r, identity.ErrForbidden)
			return
		}
		loc, err := time.LoadLocation("Europe/Budapest")
		if err != nil {
			loc = time.UTC
		}
		resetTime := in.ResetsAt.In(loc).Format("15:04")
		var body string
		switch in.Window {
		case apispec.FiveHour:
			body = fmt.Sprintf("⏸ A Claude 5 órás kerete %d%%-on, %s-kor folytatom.", in.Utilization, resetTime)
		case apispec.SevenDay:
			dayNames := map[time.Weekday]string{
				time.Monday:    "hétfőn",
				time.Tuesday:   "kedden",
				time.Wednesday: "szerdán",
				time.Thursday:  "csütörtökön",
				time.Friday:    "pénteken",
				time.Saturday:  "szombaton",
				time.Sunday:    "vasárnap",
			}
			dayName := dayNames[in.ResetsAt.In(loc).Weekday()]
			body = fmt.Sprintf("⏸ A Claude heti kerete %d%%-on, %s %s-kor folytatom.", in.Utilization, dayName, resetTime)
		default:
			body = fmt.Sprintf("⏸ A Claude kerete %d%%-on, %s-kor folytatom.", in.Utilization, resetTime)
		}
		threadRootID := httpx.Deref(in.ThreadRootId, ids.Nil)
		_, err = svc.SendMessage(r.Context(), actor, conversation.SendInput{
			ChannelID:    in.ChannelId,
			ThreadRootID: threadRootID,
			Body:         body,
			Silent:       true,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/dms", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.OpenDMRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		by := httpx.Actor(r)
		c, err := svc.OpenDM(r.Context(), by, in.ActorIds)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		members, err := svc.MemberIDs(r.Context(), c.ID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, channelToAPI(conversation.ChannelView{Channel: c, IsMember: true, MemberIDs: members}))
	}))

	mux.Handle("GET /api/v1/messages/{id}/thread", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		root, replies, err := svc.Thread(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.Thread{Root: MessageToAPI(root), Replies: make([]apispec.Message, 0, len(replies))}
		for _, m := range replies {
			out.Replies = append(out.Replies, MessageToAPI(m))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	reaction := func(add bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, ok := pathID(w, r, "id")
			if !ok {
				return
			}
			emoji := r.PathValue("emoji")
			var err error
			if add {
				err = svc.React(r.Context(), httpx.Actor(r), id, emoji)
			} else {
				err = svc.Unreact(r.Context(), httpx.Actor(r), id, emoji)
			}
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}
	mux.Handle("PUT /api/v1/messages/{id}/reactions/{emoji}", wrap(reaction(true)))
	mux.Handle("DELETE /api/v1/messages/{id}/reactions/{emoji}", wrap(reaction(false)))

	mux.Handle("POST /api/v1/channels/{id}/read", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if err := svc.MarkRead(r.Context(), httpx.Actor(r), channelID); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/messages/{id}/retract", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		if err := svc.RetractMessage(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}

// RetractNoteText is the only text of the guard's note after a retracted
// message stopped a run.
const RetractNoteText = "⏹ Leállítottam: az üzenetet visszavonták."

var terminalName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// firstLine keeps the first line of s, at most 200 characters.
func firstLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 200 {
		s = string(r[:199]) + "…"
	}
	return s
}

// inertCode makes agent-supplied text safe inside an inline code span: no
// backtick can close the span, and no @ can become a mention.
func inertCode(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	return strings.ReplaceAll(s, "@", "@\u200b")
}
