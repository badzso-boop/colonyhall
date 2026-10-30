package httpapi

import (
	"time"

	"net/http"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/lease"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(lease.ErrForbidden, lease.ErrNotHolder)
	httpx.RegisterValidation(lease.ErrInvalidResource, lease.ErrInvalidTTL, lease.ErrNoteTooLong, lease.ErrBadState, lease.ErrBadSince)
	httpx.RegisterStatus(http.StatusNotFound, lease.ErrLeaseNotFound)
}

func toSpecLease(l lease.Lease) apispec.Lease {
	var note *string
	if l.Note != "" {
		note = &l.Note
	}
	var relBy *uuid.UUID
	if l.ReleasedBy != nil {
		u := uuid.UUID(*l.ReleasedBy)
		relBy = &u
	}
	return apispec.Lease{
		Id:           uuid.UUID(l.ID),
		WorkspaceId:  uuid.UUID(l.WorkspaceID),
		Resource:     l.Resource,
		HolderId:     uuid.UUID(l.HolderID),
		HolderHandle: l.HolderHandle,
		Note:         note,
		AcquiredAt:   l.AcquiredAt,
		ExpiresAt:    l.ExpiresAt,
		ReleasedAt:   l.ReleasedAt,
		ReleasedBy:   relBy,
	}
}

// Register mounts the lease HTTP endpoints on mux.
func Register(mux *http.ServeMux, svc *lease.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/leases", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		var since *time.Time
		if v := r.URL.Query().Get("since"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				httpx.Error(w, r, lease.ErrBadSince)
				return
			}
			since = &t
		}
		leasesList, err := svc.List(r.Context(), actor, r.URL.Query().Get("state"), since)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		out := apispec.LeasesResponse{
			Leases: make([]apispec.Lease, len(leasesList)),
		}
		for i, l := range leasesList {
			out.Leases[i] = toSpecLease(l)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/leases/{id}/release", wrap(func(w http.ResponseWriter, r *http.Request) {
		actor := httpx.Actor(r)
		leaseID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid lease id"})
			return
		}

		released, err := svc.ReleaseByID(r.Context(), actor, leaseID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}

		httpx.JSON(w, http.StatusOK, toSpecLease(released))
	}))
}
