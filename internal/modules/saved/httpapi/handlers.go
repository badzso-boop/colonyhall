// Package httpapi exposes saved items and reminders over REST (round 55, part E).
package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/saved"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(saved.ErrForbidden)
	httpx.RegisterValidation(saved.ErrNoteTooLong, saved.ErrBadRemindAt, saved.ErrEmpty, saved.ErrBadFilter)
	httpx.RegisterStatus(http.StatusConflict, saved.ErrTooMany)
}

func excerpt(s string) string {
	if utf8.RuneCountInString(s) <= saved.ExcerptLength {
		return s
	}
	r := []rune(s)
	return string(r[:saved.ExcerptLength]) + "…"
}

// ToAPI converts an item for the REST API.
func ToAPI(v saved.View) apispec.SavedItem {
	out := apispec.SavedItem{Id: v.ID, Note: v.Note, RemindAt: v.RemindAt, RemindedAt: v.RemindedAt, DoneAt: v.DoneAt,
		CreatedAt: v.CreatedAt, Available: v.Message != nil && v.Channel != nil}
	if v.MessageID != ids.Nil {
		out.MessageId = httpx.Ptr(v.MessageID)
		out.ChannelId = httpx.Ptr(v.ChannelID)
	}
	if out.Available {
		m, c := v.Message, v.Channel
		sm := apispec.SavedMessage{Id: m.ID, ChannelId: c.ID, ChannelName: c.Name, ChannelKind: apispec.ChannelKind(c.Kind),
			AuthorId: m.AuthorID, Body: excerpt(m.Body), CreatedAt: m.CreatedAt}
		if m.ThreadRootID != ids.Nil {
			sm.ThreadRootId = httpx.Ptr(m.ThreadRootID)
		}
		if m.ReplyCount > 0 {
			sm.ReplyCount = httpx.Ptr(m.ReplyCount)
		}
		if m.DeletedAt != nil {
			sm.Retracted = httpx.Ptr(true)
			sm.Body = ""
		}
		out.Message = &sm
	}
	return out
}

func parseCursor(s string) (*saved.Cursor, error) {
	at, id, ok := strings.Cut(s, "_")
	if !ok {
		return nil, fmt.Errorf("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, err
	}
	u, err := ids.Parse(id)
	if err != nil {
		return nil, err
	}
	return &saved.Cursor{At: t, ID: u}, nil
}

// Register mounts the saved-item endpoints.
func Register(mux *http.ServeMux, svc *saved.Service, conv *conversation.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }
	view := func(r *http.Request, it saved.Item) saved.View {
		v := saved.View{Item: it}
		if it.MessageID == ids.Nil {
			return v
		}
		by := httpx.Actor(r)
		if ok, err := conv.CanRead(r.Context(), by, it.ChannelID); err != nil || !ok {
			return v
		}
		if m, err := conv.MessageByID(r.Context(), it.MessageID); err == nil {
			v.Message = &m
		}
		if c, err := conv.ChannelByID(r.Context(), it.ChannelID); err == nil {
			v.Channel = &c
		}
		return v
	}

	mux.Handle("GET /api/v1/me/saved", wrap(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var after *saved.Cursor
		if c := q.Get("cursor"); c != "" {
			cur, err := parseCursor(c)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "cursor is not a next_cursor value"})
				return
			}
			after = cur
		}
		views, next, err := svc.List(r.Context(), httpx.Actor(r), saved.Filter(q.Get("filter")), after)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.SavedPage{Items: make([]apispec.SavedItem, len(views))}
		for i, v := range views {
			out.Items[i] = ToAPI(v)
		}
		if next != nil {
			out.NextCursor = httpx.Ptr(next.At.UTC().Format(time.RFC3339Nano) + "_" + next.ID.String())
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/me/saved", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.SaveItemRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		it, err := svc.Save(r.Context(), httpx.Actor(r), saved.SaveInput{MessageID: httpx.Deref(req.MessageId, ids.Nil),
			RemindAt: req.RemindAt, Note: httpx.Deref(req.Note, "")})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, ToAPI(view(r, it)))
	}))

	mux.Handle("PATCH /api/v1/me/saved/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "id must be a UUID"})
			return
		}
		var req apispec.UpdateSavedRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		in := saved.UpdateInput{Done: req.Done}
		if req.RemindAt != nil || httpx.Deref(req.ClearReminder, false) {
			in.SetRemind, in.RemindAt = true, req.RemindAt
		}
		it, err := svc.Update(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, ToAPI(view(r, it)))
	}))

	mux.Handle("DELETE /api/v1/me/saved/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "id must be a UUID"})
			return
		}
		if err := svc.Delete(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
