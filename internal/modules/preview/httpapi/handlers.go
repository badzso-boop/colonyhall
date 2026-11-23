// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi serves the preview HTTP endpoints.
package httpapi

import (
	"context"
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(preview.ErrForbidden)
	httpx.RegisterValidation(
		preview.ErrBadName,
		preview.ErrBadPath,
		preview.ErrBadTTL,
		preview.ErrNotAllowed,
		preview.ErrAlreadyRunning,
	)
}

// HostAuthenticator verifies a guard's bearer token.
type HostAuthenticator interface {
	AuthenticateHost(ctx context.Context, token string) (hosts.Host, error)
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}

func previewOut(r *http.Request, links Links, p preview.Preview) apispec.Preview {
	var urlPtr, errPtr *string
	if p.URL != "" {
		urlPtr = &p.URL
	}
	if p.Error != "" {
		errPtr = &p.Error
	}
	return apispec.Preview{
		Id:           p.ID,
		WorkspaceId:  p.WorkspaceID,
		AgentId:      p.AgentID,
		HostId:       p.HostID,
		ChannelId:    p.ChannelID,
		ThreadRootId: p.ThreadRootID,
		Name:         p.Name,
		ComposePath:  p.ComposePath,
		State:        apispec.PreviewState(p.State),
		Url:          urlPtr,
		Error:        errPtr,
		ProxyUrl:     proxyURL(r, links, p),
		ExpiresAt:    p.ExpiresAt,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	}
}

// proxyURL is set for people only: agents do not use the proxy.
func proxyURL(r *http.Request, links Links, p preview.Preview) *string {
	if httpx.Actor(r).Kind != identity.KindHuman {
		return nil
	}
	u := links.URL(r, p.ID.String())
	if u == "" {
		return nil
	}
	return &u
}

// Register mounts the preview routes on mux.
func Register(mux *http.ServeMux, svc *preview.Service, auth identity.RequestAuthenticator, hostAuth HostAuthenticator, links Links) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/previews", wrap(func(w http.ResponseWriter, r *http.Request) {
		var agentID ids.ID
		if aStr := r.URL.Query().Get("agent_id"); aStr != "" {
			var err error
			agentID, err = ids.Parse(aStr)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "agent_id must be a UUID"})
				return
			}
		}
		ps, err := svc.List(r.Context(), httpx.Actor(r), agentID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.PreviewList{Previews: make([]apispec.Preview, 0, len(ps))}
		for _, p := range ps {
			out.Previews = append(out.Previews, previewOut(r, links, p))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("GET /api/v1/previews/{previewId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "previewId")
		if !ok {
			return
		}
		p, err := svc.Get(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, previewOut(r, links, p))
	}))

	mux.Handle("POST /api/v1/previews/{previewId}/stop", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "previewId")
		if !ok {
			return
		}
		p, err := svc.StopByID(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, previewOut(r, links, p))
	}))

	// Round 57: the app's own origin never proxies a preview (its scripts
	// would act with the person's session); the proxy is on the preview
	// listener (RegisterProxy). Behind RequireAuth, so a break-glass
	// session gets its usual refusal.
	notHere := wrap(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, db.ErrNotFound)
	})
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		mux.Handle(m+" /api/v1/previews/{previewId}/proxy", notHere)
		mux.Handle(m+" /api/v1/previews/{previewId}/proxy/{path...}", notHere)
	}

	// Guard route: report preview state.
	mux.HandleFunc("POST /api/v1/host/previews/{previewId}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "previewId")
		if !ok {
			return
		}
		h, err := hostAuth.AuthenticateHost(r.Context(), httpx.BearerToken(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		r = r.WithContext(db.WithWorkspace(r.Context(), h.WorkspaceID))
		var in apispec.HostPreviewReport
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		url := ""
		if in.Url != nil {
			url = *in.Url
		}
		errStr := ""
		if in.Error != nil {
			errStr = *in.Error
		}
		if err := svc.ReportStatus(r.Context(), h.ID, id, string(in.State), url, errStr); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
