// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(delegation.ErrForbidden)
	httpx.RegisterStatus(http.StatusNotFound, delegation.ErrGrantNotFound)
	httpx.RegisterStatus(http.StatusConflict, delegation.ErrGrantNotActive)
	httpx.RegisterValidation(delegation.ErrInvalidSettings)
}

// ToAPI converts a DelegationView to its API shape.
func ToAPI(v delegation.DelegationView) apispec.Delegation {
	var threadRoot *ids.ID
	if v.ThreadRootID != ids.Nil {
		threadRoot = &v.ThreadRootID
	}
	var fromHandle *string
	if v.FromAgentHandle != "" {
		fromHandle = &v.FromAgentHandle
	}
	var toHandle *string
	if v.ToAgentHandle != "" {
		toHandle = &v.ToAgentHandle
	}
	var appHandle *string
	if v.ApproverHandle != "" {
		appHandle = &v.ApproverHandle
	}
	var approvedByGrant *bool
	if v.ApprovedByGrant {
		approvedByGrant = httpx.Ptr(true)
	}
	return apispec.Delegation{
		Id:               v.ID,
		WorkspaceId:      v.WorkspaceID,
		MessageId:        v.MessageID,
		ChannelId:        v.ChannelID,
		ThreadRootId:     threadRoot,
		FromAgentId:      v.FromAgentID,
		FromAgentHandle:  fromHandle,
		ToAgentId:        v.ToAgentID,
		ToAgentHandle:    toHandle,
		QuestionId:       v.QuestionID,
		State:            apispec.DelegationState(v.State),
		DecidedBy:        v.DecidedBy,
		DecidedByHandle:  v.DecidedByHandle,
		ApproverHandle:   appHandle,
		DecidedAt:        v.DecidedAt,
		CreatedAt:        v.CreatedAt,
		ApprovedByGrant:  approvedByGrant,
		GrantIdleMinutes: v.GrantIdleMinutes,
		GrantMaxHours:    v.GrantMaxHours,
	}
}

// GrantToAPI converts a GrantView to its API shape.
func GrantToAPI(v delegation.GrantView) apispec.DelegationGrant {
	var threadRoot *ids.ID
	if v.ThreadRootID != ids.Nil {
		threadRoot = &v.ThreadRootID
	}
	var revBy *ids.ID
	if v.RevokedBy != nil && *v.RevokedBy != ids.Nil {
		revBy = v.RevokedBy
	}
	return apispec.DelegationGrant{
		Id:                 v.ID,
		WorkspaceId:        v.WorkspaceID,
		ChannelId:          v.ChannelID,
		ThreadRootId:       threadRoot,
		AgentAId:           v.AgentAID,
		AgentAHandle:       v.AgentAHandle,
		AgentBId:           v.AgentBID,
		AgentBHandle:       v.AgentBHandle,
		DelegationId:       v.DelegationID,
		ApprovedBy:         v.ApprovedBy,
		ApproverHandle:     v.ApproverHandle,
		ApprovedAt:         v.ApprovedAt,
		LastUsedAt:         v.LastUsedAt,
		ExpiresAt:          v.ExpiresAt,
		EffectiveExpiresAt: v.EffectiveExpiresAt(v.IdleMinutes),
		RevokedBy:          revBy,
		RevokedByHandle:    v.RevokedByHandle,
		RevokedAt:          v.RevokedAt,
	}
}

// SettingsToAPI converts delegation.Settings to apispec.DelegationSettings.
func SettingsToAPI(st delegation.Settings) apispec.DelegationSettings {
	return apispec.DelegationSettings{
		GrantIdleMinutes: st.GrantIdleMinutes,
		GrantMaxHours:    st.GrantMaxHours,
		UpdatedAt:        st.UpdatedAt,
	}
}

// Register mounts the delegation routes on mux.
func Register(mux *http.ServeMux, svc *delegation.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/delegations", wrap(func(w http.ResponseWriter, r *http.Request) {
		var state *string
		if s := r.URL.Query().Get("state"); s != "" {
			state = &s
		}
		var msgID *ids.ID
		if m := r.URL.Query().Get("message_id"); m != "" {
			if parsed, err := ids.Parse(m); err == nil {
				msgID = &parsed
			}
		}
		items, err := svc.List(r.Context(), httpx.Actor(r), state, msgID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.DelegationList{
			Delegations: make([]apispec.Delegation, 0, len(items)),
		}
		for _, it := range items {
			out.Delegations = append(out.Delegations, ToAPI(it))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("GET /api/v1/delegation-settings", wrap(func(w http.ResponseWriter, r *http.Request) {
		st, err := svc.GetSettings(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, SettingsToAPI(st))
	}))

	mux.Handle("PUT /api/v1/delegation-settings", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.UpdateDelegationSettingsRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		st, err := svc.UpdateSettings(r.Context(), httpx.Actor(r), req.GrantIdleMinutes, req.GrantMaxHours)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, SettingsToAPI(st))
	}))

	mux.Handle("GET /api/v1/delegation-grants", wrap(func(w http.ResponseWriter, r *http.Request) {
		activeOnly := r.URL.Query().Get("active") == "true"
		grants, err := svc.ListGrants(r.Context(), httpx.Actor(r), activeOnly)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.DelegationGrantList{
			Grants: make([]apispec.DelegationGrant, 0, len(grants)),
		}
		for _, g := range grants {
			out.Grants = append(out.Grants, GrantToAPI(g))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/delegation-grants/{id}/revoke", wrap(func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, err := ids.Parse(idStr)
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid grant id"})
			return
		}
		v, err := svc.RevokeGrant(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, GrantToAPI(v))
	}))
}
