// Package httpx holds HTTP helpers shared by every module's httpapi package:
// the authenticated actor in the request context, JSON encoding and
// RFC 9457 problem responses.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
)

type actorKey struct{}

// WithActor stores the authenticated actor in ctx.
func WithActor(ctx context.Context, a identity.Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
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

// RequireAuth rejects requests without a valid bearer token.
func RequireAuth(auth identity.Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, err := auth.Authenticate(r.Context(), BearerToken(r))
		if err != nil {
			Error(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), a)))
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

// Validation errors registered by modules map to 400 responses.
var validationErrors []error

// RegisterValidation marks errors that should be reported as 400 Bad Request.
func RegisterValidation(errs ...error) { validationErrors = append(validationErrors, errs...) }

// Forbidden errors registered by modules map to 403 responses.
var forbiddenErrors = []error{identity.ErrForbidden}

// RegisterForbidden marks errors that should be reported as 403 Forbidden.
func RegisterForbidden(errs ...error) { forbiddenErrors = append(forbiddenErrors, errs...) }

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Error writes err as an application/problem+json response.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	status, title, detail := classify(err)
	if status >= 500 {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		detail = ""
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: "about:blank", Title: title, Status: status, Detail: detail})
}

func classify(err error) (int, string, string) {
	var bad *BadRequest
	switch {
	case errors.As(err, &bad):
		return http.StatusBadRequest, "Bad Request", bad.Detail
	case errors.Is(err, identity.ErrUnauthorized):
		return http.StatusUnauthorized, "Unauthorized", err.Error()
	case errors.Is(err, db.ErrNotFound):
		return http.StatusNotFound, "Not Found", err.Error()
	case errors.Is(err, db.ErrConflict):
		return http.StatusConflict, "Conflict", err.Error()
	}
	for _, f := range forbiddenErrors {
		if errors.Is(err, f) {
			return http.StatusForbidden, "Forbidden", err.Error()
		}
	}
	for _, v := range validationErrors {
		if errors.Is(err, v) {
			return http.StatusBadRequest, "Bad Request", err.Error()
		}
	}
	return http.StatusInternalServerError, "Internal Server Error", err.Error()
}
