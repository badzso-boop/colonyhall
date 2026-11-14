// Package httpapi exposes the activity feed over REST (round 55, part H).
package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/activity"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(activity.ErrForbidden)
	httpx.RegisterValidation(activity.ErrBadKind)
}

func excerpt(s string) string {
	if utf8.RuneCountInString(s) <= activity.ExcerptLength {
		return s
	}
	return string([]rune(s)[:activity.ExcerptLength]) + "…"
}

func ptrID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

// ToAPI converts an item for the REST API. What it is about is shown only
// while the recipient can read its conversation.
func ToAPI(v activity.View) apispec.ActivityItem {
	out := apispec.ActivityItem{Id: v.ID, Kind: apispec.ActivityKind(v.Kind), CreatedAt: v.CreatedAt, Read: v.ReadAt != nil,
		Available: v.Channel != nil, RefId: ptrID(v.RefID), ActorId: ptrID(v.ActorID)}
	if v.Channel == nil {
		return out
	}
	out.ChannelId, out.ChannelName, out.ChannelKind = &v.Channel.ID, &v.Channel.Name, httpx.Ptr(apispec.ChannelKind(v.Channel.Kind))
	out.MessageId, out.ThreadRootId = ptrID(v.MessageID), ptrID(v.ThreadRootID)
	if v.Emoji != "" {
		out.Emoji = &v.Emoji
	}
	if v.Message != nil && v.Message.DeletedAt == nil {
		out.Excerpt = httpx.Ptr(excerpt(v.Message.Body))
	}
	return out
}

func parseCursor(s string) (*activity.Cursor, error) {
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
	return &activity.Cursor{At: t, ID: u}, nil
}

// Register mounts the activity endpoints.
func Register(mux *http.ServeMux, svc *activity.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/me/activity", wrap(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query()
		q := activity.Query{Kind: activity.Kind(v.Get("kind")), UnreadOnly: v.Get("unread") == "true"}
		if c := v.Get("cursor"); c != "" {
			cur, err := parseCursor(c)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "cursor is not a next_cursor value"})
				return
			}
			q.After = cur
		}
		if l := v.Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 1 || n > activity.MaxPageSize {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "limit must be 1-100"})
				return
			}
			q.Limit = n
		}
		views, unread, next, err := svc.List(r.Context(), httpx.Actor(r), q)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ActivityPage{Items: make([]apispec.ActivityItem, len(views)), UnreadCount: unread}
		for i, it := range views {
			out.Items[i] = ToAPI(it)
		}
		if next != nil {
			out.NextCursor = httpx.Ptr(next.At.UTC().Format(time.RFC3339Nano) + "_" + next.ID.String())
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/me/activity/read", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.MarkActivityReadRequest
		if r.ContentLength != 0 {
			if err := httpx.Decode(r, &in); err != nil {
				httpx.Error(w, r, err)
				return
			}
		}
		var list []ids.ID
		if in.ItemIds != nil {
			if len(*in.ItemIds) > 200 {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "at most 200 item ids"})
				return
			}
			list = append(list, *in.ItemIds...)
		}
		n, err := svc.MarkRead(r.Context(), httpx.Actor(r), list, httpx.Deref(in.ChannelId, ids.Nil))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.MarkedCount{Marked: n})
	}))
}
