// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes the Workshop over REST (round 56a): connectors,
// grants and the call log, for the owner and the admins.
package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/secrets"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(workshop.ErrForbidden, workshop.ErrOwnerOnly, secrets.ErrForbidden)
	httpx.RegisterValidation(workshop.ErrBadSlug, workshop.ErrBadTitle, workshop.ErrBadKind, workshop.ErrBadAuth,
		workshop.ErrBadHeader, workshop.ErrSecretNeeded, workshop.ErrSecretAgain, workshop.ErrBadRisk, workshop.ErrNotAgent,
		secrets.ErrNoKeys, secrets.ErrBadValue, secrets.ErrBadName, secrets.ErrBadKind)
	httpx.RegisterStatus(http.StatusNotFound, workshop.ErrNotFound, workshop.ErrUnknownTool, secrets.ErrNotFound)
	httpx.RegisterStatus(http.StatusConflict, workshop.ErrSlugTaken)
}

// ToAPI converts a connector for the REST API.
func ToAPI(v workshop.ConnectorView) apispec.Connector {
	out := apispec.Connector{Id: v.ID, Slug: v.Slug, Title: v.Title, Kind: apispec.ConnectorKind(v.Kind), BaseUrl: v.BaseURL,
		Auth: apispec.ConnectorAuth(v.Auth), HasSecret: v.HasSecret, ApprovalOff: v.ApprovalOff,
		State: apispec.ConnectorState(v.State), ToolsCachedAt: v.ToolsCachedAt, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		Tools: make([]apispec.ConnectorTool, 0, len(v.Tools))}
	if v.HeaderName != "" {
		out.HeaderName = httpx.Ptr(v.HeaderName)
	}
	if v.SecretHint != "" {
		out.SecretHint = httpx.Ptr(v.SecretHint)
	}
	if v.LastError != "" {
		out.LastError = httpx.Ptr(v.LastError)
	}
	for _, t := range v.Tools {
		ct := apispec.ConnectorTool{Name: t.Name, Description: t.Description, Risk: apispec.ToolRisk(t.Risk),
			EffectiveRisk: apispec.ToolRisk(v.EffectiveRisk(t))}
		if _, ok := v.RiskOverrides[t.Name]; ok {
			ct.Overridden = httpx.Ptr(true)
		}
		out.Tools = append(out.Tools, ct)
	}
	return out
}

func grantToAPI(g workshop.GrantView) apispec.ToolGrant {
	out := apispec.ToolGrant{Id: g.ID, AgentId: g.AgentID, TargetKind: apispec.ToolGrantTargetKind(g.TargetKind),
		GrantedBy: g.GrantedBy, CreatedAt: g.CreatedAt}
	if g.TargetID != ids.Nil {
		out.TargetId = httpx.Ptr(g.TargetID)
	}
	if g.AgentHandle != "" {
		out.AgentHandle = httpx.Ptr(g.AgentHandle)
	}
	if g.TargetName != "" {
		out.TargetName = httpx.Ptr(g.TargetName)
	}
	return out
}

func strPtr[T ~string](p *T) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}

// Register mounts the Workshop endpoints.
func Register(mux *http.ServeMux, svc *workshop.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }
	one := func(w http.ResponseWriter, r *http.Request, v workshop.ConnectorView, err error, status int) {
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, status, ToAPI(v))
	}
	view := func(r *http.Request, c workshop.Connector, err error) (workshop.ConnectorView, error) {
		if err != nil {
			return workshop.ConnectorView{}, err
		}
		return svc.GetConnector(r.Context(), httpx.Actor(r), c.ID)
	}

	registerExtensions(mux, svc, wrap)
	registerRoutines(mux, svc, wrap)
	registerBundles(mux, svc, wrap)

	mux.Handle("GET /api/v1/workshop/connectors", wrap(func(w http.ResponseWriter, r *http.Request) {
		vs, err := svc.ListConnectors(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ConnectorList{Connectors: make([]apispec.Connector, len(vs)), SecretsAvailable: svc.Secrets.Available()}
		for i, v := range vs {
			out.Connectors[i] = ToAPI(v)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/workshop/connectors", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.CreateConnectorRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		in := workshop.ConnectorInput{Slug: req.Slug, Title: &req.Title, Kind: string(req.Kind), BaseURL: &req.BaseUrl,
			Auth: strPtr(req.Auth), HeaderName: req.HeaderName, Secret: req.Secret}
		c, err := svc.CreateConnector(r.Context(), httpx.Actor(r), in)
		v, err := view(r, c, err)
		one(w, r, v, err, http.StatusCreated)
	}))

	mux.Handle("GET /api/v1/workshop/connectors/{connectorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		v, err := svc.GetConnector(r.Context(), httpx.Actor(r), id)
		one(w, r, v, err, http.StatusOK)
	}))

	mux.Handle("PATCH /api/v1/workshop/connectors/{connectorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		var req apispec.UpdateConnectorRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.UpdateConnector(r.Context(), httpx.Actor(r), id, workshop.ConnectorInput{Title: req.Title,
			BaseURL: req.BaseUrl, Auth: strPtr(req.Auth), HeaderName: req.HeaderName, Secret: req.Secret})
		v, err := view(r, c, err)
		one(w, r, v, err, http.StatusOK)
	}))

	mux.Handle("DELETE /api/v1/workshop/connectors/{connectorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		if err := svc.DeleteConnector(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/workshop/connectors/{connectorId}/refresh", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		v, err := svc.RefreshTools(r.Context(), httpx.Actor(r), id)
		one(w, r, v, err, http.StatusOK)
	}))

	mux.Handle("PUT /api/v1/workshop/connectors/{connectorId}/approval", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		var req apispec.ConnectorApprovalRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.SetApprovalOff(r.Context(), httpx.Actor(r), id, req.ApprovalOff)
		v, err := view(r, c, err)
		one(w, r, v, err, http.StatusOK)
	}))

	mux.Handle("PUT /api/v1/workshop/connectors/{connectorId}/tools/{tool}/risk", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "connectorId")
		if !ok {
			return
		}
		var req apispec.ToolRiskRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.SetRisk(r.Context(), httpx.Actor(r), id, r.PathValue("tool"), string(req.Risk))
		v, err := view(r, c, err)
		one(w, r, v, err, http.StatusOK)
	}))

	mux.Handle("GET /api/v1/workshop/grants", wrap(func(w http.ResponseWriter, r *http.Request) {
		gs, err := svc.ListGrants(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ToolGrantList{Grants: make([]apispec.ToolGrant, len(gs))}
		for i, g := range gs {
			out.Grants[i] = grantToAPI(g)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/workshop/grants", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.AddToolGrantRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		g, err := svc.AddGrant(r.Context(), httpx.Actor(r), req.AgentId, string(req.TargetKind), httpx.Deref(req.TargetId, ids.Nil))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		gv := workshop.GrantView{Grant: g}
		if gs, err := svc.ListGrants(r.Context(), httpx.Actor(r)); err == nil {
			for _, x := range gs {
				if x.ID == g.ID {
					gv = x
				}
			}
		}
		httpx.JSON(w, http.StatusOK, grantToAPI(gv))
	}))

	mux.Handle("DELETE /api/v1/workshop/grants/{grantId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "grantId")
		if !ok {
			return
		}
		if err := svc.RevokeGrant(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/workshop/calls", wrap(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var target *ids.ID
		if v := q.Get("target_id"); v != "" {
			id, err := ids.Parse(v)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "target_id must be a UUID"})
				return
			}
			target = &id
		}
		var before *time.Time
		if v := q.Get("before"); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "before must be a date-time"})
				return
			}
			before = &t
		}
		limit := 50
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "limit must be 1–200"})
				return
			}
			limit = n
		}
		cs, err := svc.ListCalls(r.Context(), httpx.Actor(r), target, before, limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ToolCallList{Calls: make([]apispec.ToolCallEntry, len(cs))}
		for i, c := range cs {
			e := apispec.ToolCallEntry{Id: c.ID, ActorId: c.ActorID, TargetKind: apispec.ToolCallEntryTargetKind(c.TargetKind),
				TargetId: c.TargetID, Tool: c.Tool, Risk: apispec.ToolRisk(c.Risk), StartedAt: c.StartedAt,
				DurationMs: c.DurationMS, InBytes: c.InBytes, OutBytes: c.OutBytes, Outcome: apispec.ToolCallEntryOutcome(c.Outcome)}
			if c.ApprovalID != ids.Nil {
				e.ApprovalId = httpx.Ptr(c.ApprovalID)
			}
			if c.Error != "" {
				e.Error = httpx.Ptr(c.Error)
			}
			out.Calls[i] = e
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}

func errorsIs(err, target error) bool { return errors.Is(err, target) }
