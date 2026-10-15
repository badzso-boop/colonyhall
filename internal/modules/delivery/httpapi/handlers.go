// Package httpapi exposes the delivery module (the agent inbox) over REST.
// Request and response shapes come from api/openapi.yaml (internal/apispec).
package httpapi

import (
	"net/http"
	"strconv"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/identity"
)

// ToAPI converts an inbox item to its API shape.
func ToAPI(it delivery.InboxItem) apispec.InboxItem {
	return apispec.InboxItem{Id: it.ID, Kind: it.ItemKind, ItemId: it.ItemID, Channel: it.ChannelName,
		ChannelId: it.ChannelID, ThreadRootId: httpx.Ptr(it.ThreadRootID), Author: it.AuthorHandle, Body: it.Body,
		Mentioned: it.Flags.Has(delivery.FlagMentioned), Dm: it.Flags.Has(delivery.FlagDM), At: it.MessageAt}
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
		out := apispec.Inbox{Total: total, Items: make([]apispec.InboxItem, 0, len(items))}
		for _, it := range items {
			out.Items = append(out.Items, ToAPI(it))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/agent/inbox/ack", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.AckRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		n, err := svc.Ack(r.Context(), httpx.Actor(r), in.Ids)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.AckResponse{Acked: n})
	}))
}
