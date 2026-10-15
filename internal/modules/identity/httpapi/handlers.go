// Package httpapi exposes the identity module over REST.
package httpapi

import (
	"net/http"
	"time"

	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterValidation(identity.ErrInvalidHandle, identity.ErrInvalidKind)
}

// ActorDTO is the JSON form of an actor.
type ActorDTO struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToDTO converts an actor.
func ToDTO(a identity.Actor) ActorDTO {
	return ActorDTO{ID: a.ID.String(), Kind: string(a.Kind), Handle: a.Handle, DisplayName: a.DisplayName,
		Role: string(a.Role), State: a.State, CreatedAt: a.CreatedAt}
}

// Register mounts the identity routes on mux. Every route requires auth.
func Register(mux *http.ServeMux, svc *identity.Service) {
	auth := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(svc, h) }

	mux.Handle("GET /api/v1/me", auth(func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, ToDTO(httpx.Actor(r)))
	}))

	mux.Handle("GET /api/v1/actors", auth(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListActors(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]ActorDTO, 0, len(list))
		for _, a := range list {
			out = append(out, ToDTO(a))
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"actors": out})
	}))

	mux.Handle("POST /api/v1/actors", auth(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Kind        string `json:"kind"`
			Handle      string `json:"handle"`
			DisplayName string `json:"display_name"`
			Role        string `json:"role"`
		}
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		role := identity.Role(in.Role)
		if role == "" {
			role = identity.RoleMember
		}
		if role == identity.RoleOwner {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "role owner cannot be assigned"})
			return
		}
		a, secret, err := svc.CreateActor(r.Context(), httpx.Actor(r), identity.NewActorInput{
			Kind: identity.Kind(in.Kind), Handle: in.Handle, DisplayName: in.DisplayName, Role: role,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"actor": ToDTO(a), "token": secret})
	}))
}
