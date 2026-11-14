// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpx holds HTTP helpers shared by every module's httpapi package:
// the authenticated actor in the request context, JSON encoding and
// RFC 9457 problem responses.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/tenant"
)

type actorKey struct{}

type sessionKey struct{}

// SessionCookie is the name of the browser session cookie.
const SessionCookie = "colony_session"

// HostSessionCookie is its name in multi-tenant mode over HTTPS (round 45):
// the __Host- prefix makes the browser refuse it from another subdomain
// (no Domain attribute, Secure, Path=/), so one customer cannot plant a
// cookie for another.
const HostSessionCookie = "__Host-colony_session"

// SessionCookieName is the session cookie's name for the request.
func SessionCookieName(r *http.Request) string {
	if t, ok := tenant.From(r.Context()); ok && t.HostCookie {
		return HostSessionCookie
	}
	return SessionCookie
}

// CSRFHeader carries the session's CSRF token on unsafe browser requests.
const CSRFHeader = "X-CSRF-Token"

// Session returns the browser session of the request, if it used one.
func Session(r *http.Request) (identity.Session, bool) {
	s, ok := r.Context().Value(sessionKey{}).(identity.Session)
	return s, ok
}

// WithActor stores the authenticated actor in ctx.
// It also narrows the database scope to the actor's workspace (round 45):
// every store call the handler makes sees that workspace's rows only.
func WithActor(ctx context.Context, a identity.Actor) context.Context {
	return context.WithValue(db.WithWorkspace(ctx, a.WorkspaceID), actorKey{}, a)
}

// Actor returns the authenticated actor. Handlers behind RequireAuth can rely on it.
func Actor(r *http.Request) identity.Actor {
	a, _ := r.Context().Value(actorKey{}).(identity.Actor)
	return a
}

// BearerToken extracts the token from an "Authorization: Bearer …" header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ErrCSRF is returned when a cookie-authenticated request lacks a valid CSRF token.
var ErrCSRF = errors.New("missing or invalid CSRF token")

// RequireAuth accepts a bearer token (agents, scripts) or a session cookie
// (browsers). Cookie requests that change state must also send the
// session's CSRF token in the X-CSRF-Token header.
func RequireAuth(auth identity.RequestAuthenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if tok := BearerToken(r); tok != "" {
			a, err := auth.Authenticate(ctx, tok)
			if err != nil {
				Error(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithActor(ctx, a)))
			return
		}
		c, err := r.Cookie(SessionCookieName(r))
		if err != nil {
			Error(w, r, identity.ErrUnauthorized)
			return
		}
		a, sess, err := auth.AuthenticateSession(ctx, c.Value)
		if err != nil {
			Error(w, r, err)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !sess.CSRFMatches(r.Header.Get(CSRFHeader)) {
				Error(w, r, ErrCSRF)
				return
			}
		}
		ctx = context.WithValue(WithActor(ctx, a), sessionKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// JSON writes v with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Decode reads a JSON body into v, rejecting unknown fields and bodies over 1 MiB.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &BadRequest{Detail: "invalid JSON body: " + err.Error()}
	}
	return nil
}

// BadRequest is a client error with a message safe to show.
type BadRequest struct{ Detail string }

func (e *BadRequest) Error() string { return e.Detail }

type statusErr struct {
	err    error
	status int
	typ    string // the problem type; "" is about:blank
}

// statusErrors maps module errors to HTTP statuses; modules register theirs
// from their httpapi package.
var statusErrors = []statusErr{
	{identity.ErrForbidden, http.StatusForbidden, ""},
	{ErrCSRF, http.StatusForbidden, ""},
}

// RegisterStatus reports the given errors with an HTTP status.
func RegisterStatus(status int, errs ...error) {
	for _, e := range errs {
		statusErrors = append(statusErrors, statusErr{e, status, ""})
	}
}

// RegisterProblemType reports the errors with a status and a problem type
// that clients can branch on (e.g. "agent_config_immutable").
func RegisterProblemType(status int, typ string, errs ...error) {
	for _, e := range errs {
		statusErrors = append(statusErrors, statusErr{e, status, typ})
	}
}

// RegisterValidation marks errors that should be reported as 400 Bad Request.
func RegisterValidation(errs ...error) { RegisterStatus(http.StatusBadRequest, errs...) }

// RegisterForbidden marks errors that should be reported as 403 Forbidden.
func RegisterForbidden(errs ...error) { RegisterStatus(http.StatusForbidden, errs...) }

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Error writes err as an application/problem+json response.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	status, title, detail, typ := classify(err)
	if typ == "" {
		typ = "about:blank"
	}
	if status >= 500 {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		detail = ""
	}
	// A lockout tells the client when to try again (round 40).
	var ra interface{ RetryAfter() time.Duration }
	if errors.As(err, &ra) {
		secs := int64(math.Ceil(ra.RetryAfter().Seconds()))
		w.Header().Set("Retry-After", strconv.FormatInt(max(secs, 1), 10))
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: typ, Title: title, Status: status, Detail: detail})
}

func classify(err error) (int, string, string, string) {
	var bad *BadRequest
	switch {
	case errors.As(err, &bad):
		return http.StatusBadRequest, "Bad Request", bad.Detail, ""
	case errors.Is(err, identity.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized", err.Error(), ""
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound, "Not Found", err.Error(), ""
	}
	// Registered errors first: a specific conflict (e.g. an immutable
	// agent) wraps db.ErrConflict-like meaning with its own type.
	for _, se := range statusErrors {
		if errors.Is(err, se.err) {
			return se.status, http.StatusText(se.status), err.Error(), se.typ
		}
	}
	if errors.Is(err, db.ErrConflict) {
		return http.StatusConflict, "Conflict", err.Error(), ""
	}
	return http.StatusInternalServerError, "Internal Server Error", err.Error(), ""
}

// Deref returns *p, or def when p is nil. Used with generated request types,
// where optional fields are pointers.
func Deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// Ptr returns a pointer to v, or nil when v is the zero value. Used to fill
// optional fields of generated response types.
func Ptr[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
