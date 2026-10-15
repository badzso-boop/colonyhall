// Package httpapi exposes the conversation module over REST.
// Request and response shapes come from api/openapi.yaml (internal/apispec).
package httpapi

import (
	"net/http"
	"strconv"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(conversation.ErrInvalidName, conversation.ErrEmptyBody, conversation.ErrBodyTooLong,
		conversation.ErrInvalidClientID, conversation.ErrBadThread, conversation.ErrBadEmoji, conversation.ErrBadDM,
		conversation.ErrAgentTooLong, conversation.ErrTooManyMentions, conversation.ErrGroupMention,
		conversation.ErrNothingToAck)
	httpx.RegisterForbidden(conversation.ErrForbidden)
}

// MessageToAPI converts a message to its API shape.
func MessageToAPI(m conversation.Message) apispec.Message {
	out := apispec.Message{Id: m.ID, ChannelId: m.ChannelID, AuthorId: m.AuthorID,
		ClientMsgId: httpx.Ptr(m.ClientMsgID), Kind: apispec.MessageKind(m.Kind), Body: m.Body,
		ReplyCount: m.ReplyCount, LastReplyAt: m.LastReplyAt, CreatedAt: m.CreatedAt,
		ThreadRootId: httpx.Ptr(m.ThreadRootID), Mentions: m.Mentions}
	if out.Mentions == nil {
		out.Mentions = []ids.ID{}
	}
	if len(m.Reactions) > 0 {
		rs := make([]apispec.ReactionSummary, 0, len(m.Reactions))
		for _, r := range m.Reactions {
			rs = append(rs, apispec.ReactionSummary{Emoji: r.Emoji, Count: r.Count, ActorIds: r.ActorIDs})
		}
		out.Reactions = &rs
	}
	return out
}

func channelToAPI(c conversation.ChannelView) apispec.Channel {
	out := apispec.Channel{Id: c.ID, Kind: apispec.ChannelKind(c.Kind), Name: c.Name,
		IsMember: c.IsMember, Unread: c.Unread, Mentions: c.Mentions}
	if c.MemberIDs != nil {
		out.MemberIds = &c.MemberIDs
	}
	return out
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
		c, err := svc.CreateChannel(r.Context(), httpx.Actor(r), in.Name, kind)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, channelToAPI(conversation.ChannelView{Channel: c, IsMember: true}))
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
		var in apispec.SendMessageRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		res, err := svc.SendMessage(r.Context(), httpx.Actor(r), conversation.SendInput{
			ChannelID: channelID, ThreadRootID: httpx.Deref(in.ThreadRootId, ids.Nil),
			ClientMsgID: httpx.Deref(in.ClientMsgId, ""), Body: in.Body, Elaborate: httpx.Deref(in.Elaborate, false),
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
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}
