// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clientip"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/tenant"
)

// tenantHandler picks the workspace from the subdomain in multi-tenant
// mode (round 45, docs/23-elkulonites.md): <slug>.<base domain> serves
// that workspace only, for every request (REST, WebSockets, MCP, files,
// the apps) and before authentication. An unknown slug, a suspended
// workspace, the bare base domain and any other host get the same answer,
// which does not tell whether a slug exists. The bare base domain serves
// only the health checks and the shared Gitea webhook.
func tenantHandler(base string, secureCookies bool, workspaces identity.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.TrimSuffix(host, ".")
		if host == base {
			switch r.URL.Path {
			case "/healthz", "/readyz", "/api/v1/hooks/gitea":
				next.ServeHTTP(w, r)
			default:
				noTenant(w)
			}
			return
		}
		slug, ok := strings.CutSuffix(host, "."+base)
		if !ok || !identity.ValidSlug(slug) {
			noTenant(w)
			return
		}
		ws, err := workspaces.WorkspaceBySlug(r.Context(), slug)
		if err != nil || ws.State != identity.WorkspaceActive {
			noTenant(w)
			return
		}
		ctx := tenant.With(r.Context(), tenant.Request{Workspace: ws.ID, Slug: ws.Slug, HostCookie: secureCookies,
			RemoteAddr: clientip.String(clientip.From(r.Context())), UserAgent: r.UserAgent()})
		next.ServeHTTP(w, r.WithContext(db.WithWorkspace(ctx, ws.ID)))
	})
}

// noTenant is the one answer for every host that is not an active
// workspace's subdomain.
func noTenant(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": "Not Found", "status": http.StatusNotFound})
}
