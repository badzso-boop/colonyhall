// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"encoding/json"
	"html"
	"io"
	"net"
	"net/http"
	"net/netip"
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
//
// Round 51: admin.<base domain> is the platform operator's (admin, its
// own routes only). A browser asking a suspended workspace for a page gets
// a "paused" page; every API, WebSocket, MCP and guard call still gets the
// same 404 as an unknown slug.
func tenantHandler(base string, secureCookies bool, workspaces identity.Store, next, admin http.Handler, adminAllow []netip.Prefix) http.Handler {
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
		if host == "admin."+base && admin != nil {
			// Only from the operator's addresses (COLONY_OPERATOR_ALLOWLIST):
			// elsewhere the host does not exist, and nobody can lock the
			// operator out with failed logins.
			if len(adminAllow) > 0 && !clientip.Contains(adminAllow, clientip.From(r.Context())) {
				noTenant(w)
				return
			}
			// The operator's API, its app and the health checks; anything
			// else (the customers' API, sockets, MCP) is not here.
			p := r.URL.Path
			if p == "/" || p == "/healthz" || p == "/readyz" || strings.HasPrefix(p, "/agent/") || strings.HasPrefix(p, "/api/v1/operator/") {
				admin.ServeHTTP(w, r)
			} else {
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
		if err == nil && ws.State == identity.WorkspaceSuspended && wantsPage(r) {
			suspendedPage(w, r)
			return
		}
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

// wantsPage: a browser asking for a page (not the API, a socket or MCP).
func wantsPage(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	for _, p := range []string{"/api/", "/ws/", "/mcp"} {
		if strings.HasPrefix(r.URL.Path, p) {
			return false
		}
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// suspendedTexts are the paused page's texts (round 51), Hungarian first.
var suspendedTexts = map[string][3]string{
	"hu": {"A munkaterület szünetel", "A munkaterület szünetel", "Az adatok megmaradtak. Keresd az üzemeltetőt, ha újra használni szeretnéd."},
	"en": {"This workspace is paused", "This workspace is paused", "Your data is kept. Contact the operator to use it again."},
}

// pageLanguage picks hu or en from Accept-Language (Hungarian by default).
func pageLanguage(r *http.Request) string {
	for part := range strings.SplitSeq(r.Header.Get("Accept-Language"), ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		switch {
		case strings.HasPrefix(tag, "hu"):
			return "hu"
		case strings.HasPrefix(tag, "en"):
			return "en"
		}
	}
	return "hu"
}

// suspendedPage is the browser's answer for a suspended workspace: no
// reason, no names, no script.
func suspendedPage(w http.ResponseWriter, r *http.Request) {
	lang := pageLanguage(r)
	t := suspendedTexts[lang]
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Language", lang)
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, `<!doctype html><html lang="`+lang+`"><head><meta charset="utf-8">`+
		`<meta name="viewport" content="width=device-width, initial-scale=1"><title>`+html.EscapeString(t[0])+`</title>`+
		`<style>body{margin:0;min-height:100vh;display:grid;place-items:center;font:16px/1.5 system-ui,sans-serif;background:#f6f6f4;color:#1d1d1b}`+
		`main{max-width:28rem;padding:2rem 1rem;text-align:center}h1{font-size:1.4rem;margin:0 0 .5rem}`+
		`@media (prefers-color-scheme:dark){body{background:#161615;color:#ececea}}</style></head>`+
		`<body><main><h1>`+html.EscapeString(t[1])+`</h1><p>`+html.EscapeString(t[2])+`</p></main></body></html>`)
}
