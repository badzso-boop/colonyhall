// Package httpapi exposes the delivery module (the agent inbox) over REST.
package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// ItemDTO is the JSON form of an inbox item.
type ItemDTO struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	ItemID       string    `json:"item_id"`
	Channel      string    `json:"channel"`
	ChannelID    string    `json:"channel_id"`
	ThreadRootID string    `json:"thread_root_id,omitempty"`
	Author       string    `json:"author"`
	Body         string    `json:"body"`
	Mentioned    bool      `json:"mentioned"`
	DM           bool      `json:"dm"`
	At           time.Time `json:"at"`
}

// ToDTO converts an inbox item.
func ToDTO(it delivery.InboxItem) ItemDTO {
	d := ItemDTO{ID: it.ID.String(), Kind: it.ItemKind, ItemID: it.ItemID.String(), Channel: it.ChannelName,
		ChannelID: it.ChannelID.String(), Author: it.AuthorHandle, Body: it.Body,
		Mentioned: it.Flags.Has(delivery.FlagMentioned), DM: it.Flags.Has(delivery.FlagDM), At: it.MessageAt}
	if it.ThreadRootID != ids.Nil {
		d.ThreadRootID = it.ThreadRootID.String()
	}
	return d
}

// Register mounts the inbox routes on mux. Every route requires auth.
func Register(mux *http.ServeMux, svc *delivery.Service, auth identity.Authenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/agent/inbox", wrap(func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, total, err := svc.Inbox(r.Context(), httpx.Actor(r), limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]ItemDTO, 0, len(items))
		for _, it := range items {
			out = append(out, ToDTO(it))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "total": total})
	}))

	mux.Handle("POST /api/v1/agent/inbox/ack", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			IDs []string `json:"ids"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		list := make([]ids.ID, 0, len(in.IDs))
		for _, s := range in.IDs {
			id, err := ids.Parse(s)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "ids must be UUIDs"})
				return
			}
			list = append(list, id)
		}
		n, err := svc.Ack(r.Context(), httpx.Actor(r), list)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"acked": n})
	}))
}
