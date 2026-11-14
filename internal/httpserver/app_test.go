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
	"testing/fstest"
)

func serveApp(t *testing.T, dist fstest.MapFS, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("GET /app/", appHandler(dist))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

var builtApp = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>Colonyhall</title>")},
	"assets/index-abc.js":  {Data: []byte("console.log(1)")},
	"brand/favicon.svg":    {Data: []byte("<svg/>")},
	"manifest.webmanifest": {Data: []byte(`{"name":"Colonyhall"}`)},
	".keep":                {Data: nil},
}

func TestAppServesIndexAndFallsBackForRoutes(t *testing.T) {
	for _, target := range []string{"/app/", "/app/c/123", "/app/wiki/telepites", "/app/index.html"} {
		rec := serveApp(t, builtApp, target)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>Colonyhall</title>") {
			t.Fatalf("%s: %d %q", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s: index Cache-Control = %q, want no-cache", target, got)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Fatalf("%s: Content-Type = %q", target, got)
		}
	}
}

func TestAppCachesHashedAssetsOnly(t *testing.T) {
	rec := serveApp(t, builtApp, "/app/assets/index-abc.js")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("asset: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("asset without nosniff")
	}
	rec = serveApp(t, builtApp, "/app/brand/favicon.svg")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("public file: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestAppServesTheManifestWithItsType(t *testing.T) {
	rec := serveApp(t, builtApp, "/app/manifest.webmanifest")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/manifest+json" {
		t.Fatalf("manifest: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestAppMissingFilesAre404NotTheIndex(t *testing.T) {
	for _, target := range []string{"/app/assets/gone.js", "/app/missing.png", "/app/.keep"} {
		if rec := serveApp(t, builtApp, target); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d, want 404", target, rec.Code)
		}
	}
}

func TestAppWithoutBuildAnswers503(t *testing.T) {
	rec := serveApp(t, fstest.MapFS{".keep": {Data: nil}}, "/app/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web") {
		t.Fatalf("unbuilt: %d %q", rec.Code, rec.Body.String())
	}
}

// TestNewRoutesAppToThePWA: /app/ goes to the embedded build (200 when
// `make web` ran, 503 otherwise), never to the dev page's file server.
func TestNewRoutesAppToThePWA(t *testing.T) {
	h := New(http.NewServeMux(), func(context.Context) error { return nil }, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/app/c/123", nil))
	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/app/c/123: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "Colonyhall (dev)") || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("/app/ was not served by the PWA handler: %q", rec.Body.String())
	}
}
