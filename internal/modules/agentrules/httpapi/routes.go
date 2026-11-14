// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/agentrules"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterForbidden(agentrules.ErrForbidden)
	httpx.RegisterValidation(agentrules.ErrRulesTooLong)
	httpx.RegisterStatus(http.StatusNotFound, agentrules.ErrNotFound)
}

// Register mounts the agent rules HTTP endpoints on mux.
func Register(mux *http.ServeMux, svc *agentrules.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/agent-rules", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindHuman {
			httpx.Error(w, r, agentrules.ErrForbidden)
			return
		}

		latest, err := svc.GetLatest(r.Context(), actor.WorkspaceID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		history, err := svc.ListHistory(r.Context(), actor.WorkspaceID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		var createdByPtr *uuid.UUID
		var createdAtPtr *time.Time
		if latest.Version > 0 {
			uid := uuid.UUID(latest.CreatedBy)
			createdByPtr = &uid
			t := latest.CreatedAt
			createdAtPtr = &t
		}

		historyEntries := make([]apispec.AgentRulesHistoryEntry, len(history))
		for i, h := range history {
			uid := uuid.UUID(h.CreatedBy)
			t := h.CreatedAt
			historyEntries[i] = apispec.AgentRulesHistoryEntry{
				Version:   h.Version,
				CreatedBy: &uid,
				CreatedAt: &t,
			}
		}

		out := apispec.AgentRulesResponse{
			Version:   latest.Version,
			Text:      latest.Text,
			CreatedBy: createdByPtr,
			CreatedAt: createdAtPtr,
			History:   historyEntries,
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("PUT /api/v1/agent-rules", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindHuman || !actor.IsAdmin() {
			httpx.Error(w, r, agentrules.ErrForbidden)
			return
		}

		var in apispec.UpdateAgentRulesRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}

		updated, err := svc.Update(r.Context(), actor, in.Text)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		uid := uuid.UUID(updated.CreatedBy)
		t := updated.CreatedAt
		httpx.JSON(w, http.StatusOK, apispec.AgentRules{
			Version:   updated.Version,
			Text:      updated.Text,
			CreatedBy: &uid,
			CreatedAt: &t,
		})
	}))

	mux.Handle("GET /api/v1/agent-rules/{version}", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		if actor.Kind != identity.KindHuman {
			httpx.Error(w, r, agentrules.ErrForbidden)
			return
		}

		vStr := r.PathValue("version")
		v, err := strconv.Atoi(vStr)
		if err != nil || v < 0 {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "version must be a non-negative integer"})
			return
		}

		rules, err := svc.GetVersion(r.Context(), actor.WorkspaceID, v)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		var createdByPtr *uuid.UUID
		var createdAtPtr *time.Time
		if rules.Version > 0 {
			uid := uuid.UUID(rules.CreatedBy)
			createdByPtr = &uid
			t := rules.CreatedAt
			createdAtPtr = &t
		}

		httpx.JSON(w, http.StatusOK, apispec.AgentRules{
			Version:   rules.Version,
			Text:      rules.Text,
			CreatedBy: createdByPtr,
			CreatedAt: createdAtPtr,
		})
	}))

	mux.Handle("GET /api/v1/agent/instructions", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		version, text, err := svc.RenderInstructions(r.Context(), actor)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		httpx.JSON(w, http.StatusOK, apispec.AgentInstructions{
			Version: version,
			Text:    text,
		})
	}))
}
