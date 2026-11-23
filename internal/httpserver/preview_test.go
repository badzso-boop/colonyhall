// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Round 57: the PWA may frame the preview listener, nothing else.
func TestAppFramesOnlyThePreviewListener(t *testing.T) {
	mux := http.NewServeMux()
	h := NewWithOptions(mux, func(context.Context) error { return nil }, slog.New(slog.DiscardHandler),
		Options{FrameSrc: []string{"http://*:8081", "https://*:8081"}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/app/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-src 'self' http://*:8081 https://*:8081") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("app CSP: %s", csp)
	}
	rec = httptest.NewRecorder()
	NewWithTimeouts(http.NewServeMux(), func(context.Context) error { return nil }, slog.New(slog.DiscardHandler), Timeouts{}).
		ServeHTTP(rec, httptest.NewRequest("GET", "/app/", nil))
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self';") {
		t.Fatalf("no preview listener, app CSP: %s", csp)
	}
}

func TestPreviewListenerHeaders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Security-Policy", "script-src 'self'")
	})
	h := PreviewHandler(mux, slog.New(slog.DiscardHandler))
	for host, want := range map[string]string{
		"colony.example.com:8081": "frame-ancestors http://colony.example.com:* https://colony.example.com:*",
		"[2001:db8::1]:8081":      "frame-ancestors http://[2001:db8::1]:* https://[2001:db8::1]:*",
		"evil.com;script-src *":   "frame-ancestors 'none'",
	} {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		got := rec.Header().Values("Content-Security-Policy")
		if len(got) != 2 || got[0] != want || got[1] != "script-src 'self'" {
			t.Errorf("%s: %q", host, got)
		}
		if rec.Header().Get("X-Frame-Options") != "" {
			t.Errorf("%s: X-Frame-Options set", host)
		}
	}
}
