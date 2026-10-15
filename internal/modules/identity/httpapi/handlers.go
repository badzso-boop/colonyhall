// Package httpapi exposes the identity module over REST.
// Request and response shapes come from api/openapi.yaml (internal/apispec).
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterValidation(identity.ErrInvalidHandle, identity.ErrInvalidKind)
}

// ToAPI converts an actor to its API shape.
func ToAPI(a identity.Actor) apispec.Actor {
	return apispec.Actor{Id: a.ID, Kind: apispec.ActorKind(a.Kind), Handle: a.Handle, DisplayName: a.DisplayName,
		Role: apispec.Role(a.Role), State: a.State, CreatedAt: a.CreatedAt}
}

// Register mounts the identity routes on mux. Every route requires auth.
func Register(mux *http.ServeMux, svc *identity.Service) {
	auth := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(svc, h) }

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
}
