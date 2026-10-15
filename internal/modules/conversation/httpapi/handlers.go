// Package httpapi exposes the conversation module over REST.
package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(conversation.ErrInvalidName, conversation.ErrEmptyBody, conversation.ErrBodyTooLong,
		conversation.ErrInvalidClientID, conversation.ErrBadThread)
	httpx.RegisterForbidden(conversation.ErrForbidden)
}

// ChannelDTO is the JSON form of a channel as seen by the caller.
type ChannelDTO struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	IsMember bool   `json:"is_member"`
	Unread   int64  `json:"unread"`
	Mentions int64  `json:"mentions"`
}

// MessageDTO is the JSON form of a message.
type MessageDTO struct {
	ID           string     `json:"id"`
	ChannelID    string     `json:"channel_id"`
	ThreadRootID string     `json:"thread_root_id,omitempty"`
	AuthorID     string     `json:"author_id"`
	ClientMsgID  string     `json:"client_msg_id,omitempty"`
	Kind         string     `json:"kind"`
	Body         string     `json:"body"`
	Mentions     []string   `json:"mentions"`
	ReplyCount   int        `json:"reply_count"`
	LastReplyAt  *time.Time `json:"last_reply_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// MessageToDTO converts a message.
func MessageToDTO(m conversation.Message) MessageDTO {
	d := MessageDTO{ID: m.ID.String(), ChannelID: m.ChannelID.String(), AuthorID: m.AuthorID.String(),
		ClientMsgID: m.ClientMsgID, Kind: m.Kind, Body: m.Body, ReplyCount: m.ReplyCount,
		LastReplyAt: m.LastReplyAt, CreatedAt: m.CreatedAt, Mentions: []string{}}
	if m.ThreadRootID != ids.Nil {
		d.ThreadRootID = m.ThreadRootID.String()
	}
	for _, id := range m.Mentions {
		d.Mentions = append(d.Mentions, id.String())
	}
	return d
}

// Register mounts the conversation routes on mux. Every route requires auth.
func Register(mux *http.ServeMux, svc *conversation.Service, auth identity.Authenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/channels", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListChannels(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]ChannelDTO, 0, len(list))
		for _, c := range list {
			out = append(out, ChannelDTO{ID: c.ID.String(), Kind: string(c.Kind), Name: c.Name,
				IsMember: c.IsMember, Unread: c.Unread, Mentions: c.Mentions})
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"channels": out})
	}))

	mux.Handle("POST /api/v1/channels", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.CreateChannel(r.Context(), httpx.Actor(r), in.Name, conversation.ChannelKind(in.Kind))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, ChannelDTO{ID: c.ID.String(), Kind: string(c.Kind), Name: c.Name, IsMember: true})
	}))

	mux.Handle("POST /api/v1/channels/{id}/members", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			ActorID  string `json:"actor_id"`
			CanWrite *bool  `json:"can_write"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		actorID, err := ids.Parse(in.ActorID)
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "actor_id must be a UUID"})
			return
		}
		canWrite := in.CanWrite == nil || *in.CanWrite
		if err := svc.AddMember(r.Context(), httpx.Actor(r), channelID, actorID, canWrite); err != nil {
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
		out := make([]MessageDTO, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, MessageToDTO(m))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"messages": out})
	}))

	mux.Handle("POST /api/v1/channels/{id}/messages", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Body         string `json:"body"`
			ThreadRootID string `json:"thread_root_id"`
			ClientMsgID  string `json:"client_msg_id"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		root := ids.Nil
		if in.ThreadRootID != "" {
			var err error
			if root, err = ids.Parse(in.ThreadRootID); err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "thread_root_id must be a message ID"})
				return
			}
		}
		m, err := svc.SendMessage(r.Context(), httpx.Actor(r), conversation.SendInput{
			ChannelID: channelID, ThreadRootID: root, ClientMsgID: in.ClientMsgID, Body: in.Body,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, MessageToDTO(m))
	}))

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
