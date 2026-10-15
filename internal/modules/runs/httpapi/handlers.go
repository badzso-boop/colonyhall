// Package httpapi exposes run errors over REST.
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(runs.ErrForbidden)
	httpx.RegisterValidation(runs.ErrEmpty)
}

// Register mounts the runs routes on mux.
func Register(mux *http.ServeMux, svc *runs.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("POST /api/v1/agent/errors", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.RunErrorRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		e, err := svc.ReportError(r.Context(), httpx.Actor(r), runs.ReportInput{ChannelID: in.ChannelId,
			ThreadRootID: httpx.Deref(in.ThreadRootId, ids.Nil), Summary: in.Summary, Detail: httpx.Deref(in.Detail, "")})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.RunErrorRef{Id: e.ID})
	}))

	mux.Handle("GET /api/v1/errors/{errorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("errorId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "errorId must be a UUID"})
			return
		}
		e, err := svc.GetError(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.RunError{Id: e.ID, AgentId: e.AgentID, ChannelId: e.ChannelID, Summary: e.Summary, Detail: e.Detail, CreatedAt: e.CreatedAt}
		if e.ThreadRootID != ids.Nil {
			out.ThreadRootId = &e.ThreadRootID
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
