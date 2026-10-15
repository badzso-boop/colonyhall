// Package httpapi exposes the identity module over REST.
// Request and response shapes come from api/openapi.yaml (internal/apispec).
package httpapi

import (
	"mime"
	"net/http"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(identity.ErrInvalidHandle, identity.ErrInvalidKind, identity.ErrWeakPassword,
		identity.ErrWorkspaceRequired)
	httpx.RegisterStatus(http.StatusUnauthorized, identity.ErrBadCredentials)
	httpx.RegisterStatus(http.StatusTooManyRequests, identity.ErrTooManyAttempts)
}

// Options configures the identity routes.
type Options struct {
	// SecureCookies marks the session cookie Secure (HTTPS only).
	// Enable it everywhere except plain-HTTP local development.
	SecureCookies bool
}

// ToAPI converts an actor to its API shape.
func ToAPI(a identity.Actor) apispec.Actor {
	return apispec.Actor{Id: a.ID, Kind: apispec.ActorKind(a.Kind), Handle: a.Handle, DisplayName: a.DisplayName,
		Role: apispec.Role(a.Role), State: a.State, CreatedAt: a.CreatedAt}
}

// Register mounts the identity routes on mux.
func Register(mux *http.ServeMux, svc *identity.Service, opt Options) {
	auth := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(svc, h) }

	setCookie := func(w http.ResponseWriter, value string, expires time.Time) {
		http.SetCookie(w, &http.Cookie{
			Name: httpx.SessionCookie, Value: value, Path: "/", Expires: expires,
			HttpOnly: true, Secure: opt.SecureCookies, SameSite: http.SameSiteLaxMode,
		})
	}

	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		// Requiring JSON blocks cross-site HTML form posts (login CSRF).
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "Content-Type must be application/json"})
			return
		}
		var in apispec.LoginRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		a, sess, secret, err := svc.Login(r.Context(), identity.LoginInput{
			Handle: in.Handle, Password: in.Password, Workspace: httpx.Deref(in.Workspace, ""), UserAgent: r.UserAgent(),
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		setCookie(w, secret, sess.ExpiresAt)
		httpx.JSON(w, http.StatusOK, apispec.SessionInfo{Actor: ToAPI(a), CsrfToken: sess.CSRFToken})
	})

	mux.Handle("POST /api/v1/auth/logout", auth(func(w http.ResponseWriter, r *http.Request) {
		if sess, ok := httpx.Session(r); ok {
			if err := svc.Logout(r.Context(), sess.ID); err != nil {
				httpx.Error(w, r, err)
				return
			}
		}
		setCookie(w, "", time.Unix(0, 0))
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/auth/session", auth(func(w http.ResponseWriter, r *http.Request) {
		info := apispec.SessionInfo{Actor: ToAPI(httpx.Actor(r))}
		if sess, ok := httpx.Session(r); ok {
			info.CsrfToken = sess.CSRFToken
		}
		httpx.JSON(w, http.StatusOK, info)
	}))

	mux.Handle("GET /api/v1/me", auth(func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, ToAPI(httpx.Actor(r)))
	}))

	mux.Handle("GET /api/v1/actors", auth(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListActors(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ActorList{Actors: make([]apispec.Actor, 0, len(list))}
		for _, a := range list {
			out.Actors = append(out.Actors, ToAPI(a))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/actors", auth(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateActorRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		role := identity.Role(httpx.Deref(in.Role, apispec.Member))
		if role == identity.RoleOwner {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "role owner cannot be assigned"})
			return
		}
		a, secret, err := svc.CreateActor(r.Context(), httpx.Actor(r), identity.NewActorInput{
			Kind: identity.Kind(in.Kind), Handle: in.Handle, DisplayName: httpx.Deref(in.DisplayName, ""), Role: role,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.CreateActorResponse{Actor: ToAPI(a), Token: secret})
	}))

	mux.Handle("PUT /api/v1/actors/{id}/password", auth(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "id must be a UUID"})
			return
		}
		var in apispec.SetPasswordRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var current ids.ID
		if sess, ok := httpx.Session(r); ok {
			current = sess.ID
		}
		if err := svc.SetPassword(r.Context(), httpx.Actor(r), current, id, httpx.Deref(in.CurrentPassword, ""), in.NewPassword); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
