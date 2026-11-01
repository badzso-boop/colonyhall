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
	httpx.RegisterProblemType(http.StatusConflict, "agent_config_immutable", identity.ErrAgentConfigImmutable)
	httpx.RegisterValidation(identity.ErrNotAnAgent)
	httpx.RegisterValidation(identity.ErrInvalidHandle, identity.ErrInvalidKind, identity.ErrInvalidRole, identity.ErrWeakPassword,
		identity.ErrWorkspaceRequired, identity.ErrInvalidState, identity.ErrInvalidTimezone,
		identity.ErrInvalidDescription, identity.ErrInvalidStatusText, identity.ErrInvalidAvatarEmoji, identity.ErrInvalidColor)
	httpx.RegisterStatus(http.StatusUnauthorized, identity.ErrBadCredentials)
	httpx.RegisterStatus(http.StatusTooManyRequests, identity.ErrTooManyAttempts)
	httpx.RegisterForbidden(identity.ErrHalted)
}

func tokenToAPI(t identity.Token) apispec.Token {
	return apispec.Token{Id: t.ID, ActorId: t.ActorID, Label: t.Label, CreatedAt: t.CreatedAt, RevokedAt: t.RevokedAt}
}

func workspaceStatus(w identity.Workspace) apispec.AdminStatus {
	st := apispec.AdminStatus{EmergencyStop: apispec.EmergencyStop{Active: w.EmergencyStopAt != nil, Since: w.EmergencyStopAt}}
	if w.EmergencyStopAt != nil {
		st.EmergencyStop.By = &w.EmergencyStopBy
	}
	return st
}

// Options configures the identity routes.
type Options struct {
	// SecureCookies marks the session cookie Secure (HTTPS only).
	// Enable it everywhere except plain-HTTP local development.
	SecureCookies bool
}

// ToAPI converts an actor to its API shape.
func ToAPI(a identity.Actor) apispec.Actor {
	var desc, emoji, tz, status *string
	if a.Description != "" {
		desc = &a.Description
	}
	if a.AvatarEmoji != "" {
		emoji = &a.AvatarEmoji
	}
	if a.Timezone != "" {
		tz = &a.Timezone
	}
	if a.StatusText != "" {
		status = &a.StatusText
	}
	return apispec.Actor{
		Id:             a.ID,
		Kind:           apispec.ActorKind(a.Kind),
		Handle:         a.Handle,
		DisplayName:    a.DisplayName,
		Role:           apispec.Role(a.Role),
		State:          a.State,
		CreatedAt:      a.CreatedAt,
		Description:    desc,
		AvatarEmoji:    emoji,
		Timezone:       tz,
		StatusText:     status,
		Color:          hue(a.Color),
		EffectiveColor: hue(a.EffectiveColor),
		ArchivedAt:     a.ArchivedAt,
	}
}

func hue(c string) *apispec.Hue {
	if c == "" {
		return nil
	}
	h := apispec.Hue(c)
	return &h
}

// meToAPI is ToAPI plus the computed halted flag, which is only known for
// the actor making the request.
func meToAPI(a identity.Actor) apispec.Actor {
	out := ToAPI(a)
	out.Halted = &a.Halted
	return out
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
		httpx.JSON(w, http.StatusOK, meToAPI(httpx.Actor(r)))
	}))

	mux.Handle("GET /api/v1/actors", auth(func(w http.ResponseWriter, r *http.Request) {
		list := svc.ListActors
		if r.URL.Query().Get("archived") == "true" {
			list = svc.ListActorsWithArchived
		}
		actors, err := list(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ActorList{Actors: make([]apispec.Actor, 0, len(actors))}
		for _, a := range actors {
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

	mux.Handle("GET /api/v1/actors/{id}/tokens", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		list, err := svc.ListTokens(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.TokenList{Tokens: make([]apispec.Token, 0, len(list))}
		for _, t := range list {
			out.Tokens = append(out.Tokens, tokenToAPI(t))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/actors/{id}/tokens", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in apispec.IssueTokenRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		t, secret, err := svc.IssueToken(r.Context(), httpx.Actor(r), id, httpx.Deref(in.Label, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.IssueTokenResponse{Token: tokenToAPI(t), Secret: secret})
	}))

	mux.Handle("DELETE /api/v1/tokens/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := svc.RevokeToken(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PUT /api/v1/actors/{id}/state", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in apispec.SetStateRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := svc.SetActorState(r.Context(), httpx.Actor(r), id, string(in.State)); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PATCH /api/v1/actors/{id}/profile", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in apispec.UpdateActorProfileRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		updated, err := svc.UpdateProfile(r.Context(), httpx.Actor(r), id, identity.UpdateProfileInput{
			Description: in.Description,
			AvatarEmoji: in.AvatarEmoji,
			Timezone:    in.Timezone,
			StatusText:  in.StatusText,
			Color:       in.Color,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, ToAPI(updated))
	}))

	mux.Handle("GET /api/v1/admin/status", auth(func(w http.ResponseWriter, r *http.Request) {
		ws, err := svc.Workspace(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, workspaceStatus(ws))
	}))

	mux.Handle("POST /api/v1/admin/emergency-stop", auth(func(w http.ResponseWriter, r *http.Request) {
		by := httpx.Actor(r)
		if err := svc.EmergencyStop(r.Context(), by); err != nil {
			httpx.Error(w, r, err)
			return
		}
		ws, err := svc.Workspace(r.Context(), by)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, workspaceStatus(ws))
	}))

	mux.Handle("POST /api/v1/admin/emergency-clear", auth(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.EmergencyClearRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		by := httpx.Actor(r)
		if err := svc.ClearEmergency(r.Context(), by, in.Password); err != nil {
			httpx.Error(w, r, err)
			return
		}
		ws, err := svc.Workspace(r.Context(), by)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, workspaceStatus(ws))
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

func pathID(w http.ResponseWriter, r *http.Request) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: "id must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}
