// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes tool approval requests over REST.
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(approval.ErrForbidden)
	httpx.RegisterValidation(approval.ErrBadDecision, approval.ErrBadToolName)
	httpx.RegisterStatus(http.StatusConflict, approval.ErrAlreadyDecided)
}

// Register mounts the approval routes on mux.
func Register(mux *http.ServeMux, svc *approval.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/approvals/{approvalId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("approvalId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "approvalId must be a UUID"})
			return
		}
		req, err := svc.Get(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSpec(req))
	}))

	mux.Handle("POST /api/v1/approvals/{approvalId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("approvalId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "approvalId must be a UUID"})
			return
		}
		var in apispec.DecideApprovalRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		req, err := svc.Decide(r.Context(), httpx.Actor(r), id, string(in.Decision), httpx.Deref(in.Message, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toSpec(req))
	}))
}

func toSpec(r approval.ApprovalRequest) apispec.ApprovalRequest {
	out := apispec.ApprovalRequest{
		Id:        r.ID,
		AgentId:   r.AgentID,
		ChannelId: r.ChannelID,
		ToolName:  r.ToolName,
		Input:     r.Input,
		State:     apispec.ApprovalState(r.State),
		CreatedAt: r.CreatedAt,
		ExpiresAt: r.ExpiresAt,
	}
	if r.ThreadRootID != ids.Nil {
		out.ThreadRootId = &r.ThreadRootID
	}
	if r.DecidedBy != ids.Nil {
		out.DecidedBy = &r.DecidedBy
	}
	if r.DecisionMessage != "" {
		out.DecisionMessage = &r.DecisionMessage
	}
	if r.DecidedAt != nil {
		out.DecidedAt = r.DecidedAt
	}
	return out
}
