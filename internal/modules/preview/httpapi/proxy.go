// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/preview"
	"colonyhall/internal/platform/db"
)

// Links builds the address the browser opens a preview on (round 57). The
// proxy runs on its own listener, so a preview's scripts get their own
// origin: they cannot read the app's pages or its CSRF token, and the
// app's API refuses their state-changing calls.
type Links struct {
	// Origin is COLONY_PREVIEW_ORIGIN (scheme://host:port). Empty: the
	// request's own scheme and host name with Port.
	Origin string
	// Port is the preview listener's port (from COLONY_PREVIEW_ADDR).
	// Empty and no Origin: there is no proxy, previews have no proxy_url.
	Port string
}

// Enabled reports whether the server runs a preview listener.
func (l Links) Enabled() bool { return l.Origin != "" || l.Port != "" }

// URL is the proxy address of preview id for a request to the app.
func (l Links) URL(r *http.Request, id string) string {
	if !l.Enabled() {
		return ""
	}
	origin := strings.TrimRight(l.Origin, "/")
	if origin == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		origin = scheme + "://" + net.JoinHostPort(strings.Trim(host, "[]"), l.Port)
	}
	return origin + "/api/v1/previews/" + id + "/proxy/"
}

// Headers a preview never receives: the person's Colonyhall credentials.
var strippedRequestHeaders = []string{"Authorization", httpx.CSRFHeader}

// RegisterProxy mounts the reverse proxy on the preview listener's mux.
// Only there: on the app's own origin a preview's scripts would act with
// the person's session.
func RegisterProxy(mux *http.ServeMux, svc *preview.Service, auth identity.RequestAuthenticator, logger *slog.Logger) {
	h := &proxyHandler{svc: svc, auth: auth, log: logger}
	mux.Handle("/api/v1/previews/{previewId}/proxy", h)
	mux.Handle("/api/v1/previews/{previewId}/proxy/{path...}", h)
}

type proxyHandler struct {
	svc  *preview.Service
	auth identity.RequestAuthenticator
	log  *slog.Logger
}

// authenticate is RequireAuth without the CSRF check: the preview's own
// forms post to the preview, not to Colonyhall, and never carry the
// app's token. A break-glass session is refused (AuthenticateSession does
// it outside RequireAuth).
func (h *proxyHandler) authenticate(r *http.Request) (identity.Actor, error) {
	if tok := httpx.BearerToken(r); tok != "" {
		return h.auth.Authenticate(r.Context(), tok)
	}
	c, err := r.Cookie(httpx.SessionCookieName(r))
	if err != nil {
		return identity.Actor{}, identity.ErrUnauthorized
	}
	a, _, err := h.auth.AuthenticateSession(r.Context(), c.Value)
	return a, err
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "previewId")
	if !ok {
		return
	}
	a, err := h.authenticate(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	ctx := httpx.WithActor(r.Context(), a)
	r = r.WithContext(ctx)
	p, err := h.svc.ProxyTarget(ctx, a, id)
	switch {
	case errors.Is(err, preview.ErrOffline):
		offline(w, r, p.State)
		return
	case err != nil:
		if errors.Is(err, preview.ErrForbidden) || errors.Is(err, db.ErrNotFound) {
			h.log.Warn("preview proxy: access refused", "preview", id, "actor", a.ID, "err", err)
		}
		httpx.Error(w, r, err)
		return
	}
	target, err := url.Parse(p.URL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" {
		offline(w, r, p.State)
		return
	}
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// The path stays as it is: the preview runs with this base
			// (COLONY_PREVIEW_BASE).
			pr.Out.URL.Scheme = target.Scheme
			pr.Out.URL.Host = target.Host
			pr.Out.Host = target.Host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
			pr.Out.Header.Set("X-Forwarded-Prefix", "/api/v1/previews/"+id.String()+"/proxy")
			for _, k := range strippedRequestHeaders {
				pr.Out.Header.Del(k)
			}
			stripSessionCookies(pr.Out)
		},
		ModifyResponse: func(res *http.Response) error {
			dropSessionSetCookies(res.Header)
			// Our frame-ancestors rule decides who may embed it.
			res.Header.Del("X-Frame-Options")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			h.log.Info("preview proxy: backend unreachable", "preview", id, "err", err)
			offline(w, r, p.State)
		},
	}
	rp.ServeHTTP(w, r)
}

// stripSessionCookies drops the Colonyhall session from the Cookie header
// and keeps the preview's own cookies.
func stripSessionCookies(r *http.Request) {
	cookies := r.Cookies()
	r.Header.Del("Cookie")
	for _, c := range cookies {
		if c.Name == httpx.SessionCookie || c.Name == httpx.HostSessionCookie {
			continue
		}
		r.AddCookie(c)
	}
}

// dropSessionSetCookies: cookies ignore the port, so a preview could
// otherwise overwrite the person's Colonyhall session on the app's origin.
func dropSessionSetCookies(h http.Header) {
	vals := h.Values("Set-Cookie")
	h.Del("Set-Cookie")
	for _, v := range vals {
		name, _, _ := strings.Cut(strings.TrimSpace(v), "=")
		if strings.EqualFold(strings.TrimSpace(name), httpx.SessionCookie) || strings.EqualFold(strings.TrimSpace(name), httpx.HostSessionCookie) {
			continue
		}
		h.Add("Set-Cookie", v)
	}
}

var offlinePage = template.Must(template.New("offline").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="5"><title>Preview offline</title>
<style>body{font:15px system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;color:#555;background:#fafafa}
@media(prefers-color-scheme:dark){body{color:#bbb;background:#1b1b1b}}main{max-width:28rem;text-align:center}</style></head>
<body><main><h1>Preview offline</h1><p>The preview container is not running or is still starting up (state: {{.}}). This page retries every 5 seconds.</p></main></body></html>
`))

// offline answers 502: an HTML page for a browser (it retries by itself),
// a problem document for anything else.
func offline(w http.ResponseWriter, r *http.Request, state string) {
	if state == "" {
		state = "unknown"
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusBadGateway)
		_ = offlinePage.Execute(w, state)
		return
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "preview_offline", "title": "Bad Gateway", "status": http.StatusBadGateway,
		"detail": "the preview container is offline or starting up (state: " + state + ")"})
}
