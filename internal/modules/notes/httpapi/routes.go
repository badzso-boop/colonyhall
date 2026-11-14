// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/notes"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(notes.ErrForbidden, notes.ErrChannelForbidden)
	httpx.RegisterValidation(notes.ErrTextTooLong)
	httpx.RegisterValidation(notes.ErrTextEmpty)
	httpx.RegisterStatus(http.StatusNotFound, notes.ErrThreadNotFound)
}

// Register mounts the notes HTTP endpoints on mux.
func Register(mux *http.ServeMux, svc *notes.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/agents/{id}/notes", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)

		targetAgentID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid agent id"})
			return
		}

		var threadID *ids.ID
		if q := strings.TrimSpace(r.URL.Query().Get("thread")); q != "" {
			tid, err := ids.Parse(q)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid thread id"})
				return
			}
			threadID = &tid
		}

		notesList, err := svc.ListNotes(r.Context(), actor, targetAgentID, threadID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		out := apispec.AgentNotesResponse{
			Notes: make([]apispec.AgentNote, len(notesList)),
		}
		for i, n := range notesList {
			out.Notes[i] = apispec.AgentNote{
				Id:           uuid.UUID(n.ID),
				AgentId:      uuid.UUID(n.AgentID),
				ThreadRootId: uuid.UUID(n.ThreadRootID),
				Text:         n.Text,
				CreatedAt:    n.CreatedAt,
			}
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
