// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpserver adds the cross-cutting HTTP parts around the module
// routes: health checks, request logging, panic recovery, the development
// web page at / and the PWA at /app/.
package httpserver

import (
	"bufio"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"time"

	"colonyhall/web"
)

//go:embed web
var webFS embed.FS

// New wraps the module routes with health endpoints, the dev page, the PWA and middleware.
func New(api *http.ServeMux, ready func(context.Context) error, logger *slog.Logger) http.Handler {
	return NewWithTimeouts(api, ready, logger, Timeouts{})
}

// NewWithTimeouts is New with per-request read and write deadlines.
func NewWithTimeouts(api *http.ServeMux, ready func(context.Context) error, logger *slog.Logger, t Timeouts) http.Handler {
	return NewWithOptions(api, ready, logger, Options{Timeouts: t})
}

// Options are the handler's settings.
type Options struct {
	Timeouts
	// FrameSrc are the sources the PWA may embed in a frame besides its
	// own origin: the preview listener (round 57).
	FrameSrc []string
}

// NewWithOptions is New with deadlines and the PWA's frame sources.
func NewWithOptions(api *http.ServeMux, ready func(context.Context) error, logger *slog.Logger, o Options) http.Handler {
	t := o.Timeouts
	api.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	api.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	api.Handle("GET /", http.FileServerFS(static))
	dist, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		panic(err)
	}
	api.Handle("GET /app/", appHandler(dist))
	return recoverer(logger, logRequests(logger, securityHeaders(appCSP(dist, o.FrameSrc...), deadlines(t, api))))
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer
// (needed for WebSocket hijacking and streaming).
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Flush forwards streaming flushes (MCP server-sent events).
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards connection takeover (WebSocket upgrade).
func (s *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("httpserver: response writer cannot hijack")
	}
	s.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			return
		}
		logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"ms", time.Since(start).Milliseconds())
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				logger.Error("panic", "path", r.URL.Path, "value", v)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// PreviewHandler wraps the preview listener's routes (round 57): the
// reverse proxy only. No deadlines (the live-reload WebSockets stay open);
// its pages may be framed by the app on the same host name, any port.
func PreviewHandler(mux *http.ServeMux, logger *slog.Logger) http.Handler {
	return recoverer(logger, logRequests(logger, previewHeaders(mux)))
}
