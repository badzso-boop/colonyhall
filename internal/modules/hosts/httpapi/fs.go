// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterValidation(hosts.ErrBadFsOp, hosts.ErrTooMany)
	httpx.RegisterStatus(http.StatusConflict, hosts.ErrHostOffline)
	httpx.RegisterStatus(http.StatusGatewayTimeout, hosts.ErrFsTimeout)
	httpx.RegisterStatus(http.StatusGone, hosts.ErrFsGone)
}

// RegisterFs mounts the host folder listing relay: the admin route and the
// guard's routes (host token).
func RegisterFs(mux *http.ServeMux, svc *hosts.Service, relay *hosts.FsRelay, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("POST /api/v1/hosts/{hostId}/fs", wrap(func(w http.ResponseWriter, r *http.Request) {
		hostID, ok := httpx.PathID(w, r, "hostId")
		if !ok {
			return
		}
		var in apispec.HostFsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if in.Op != apispec.HostFsRequestOp(hosts.OpListDirs) {
			httpx.Error(w, r, hosts.ErrBadFsOp)
			return
		}
		res, err := relay.ListDirs(r.Context(), httpx.Actor(r), hostID, httpx.Deref(in.Path, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, res)
	}))

	mux.Handle("GET /api/v1/host/fs/{requestId}", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		p, err := relay.Request(host(r), r.PathValue("requestId"))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.HostFsPendingRequest{RequestId: p.ID, Op: apispec.HostFsPendingRequestOp(p.Op)}
		if p.Path != "" {
			out.Path = &p.Path
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/host/fs/{requestId}/result", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		// httpx.Decode caps the body at 1 MiB.
		var in apispec.HostDirListing
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := relay.Deliver(host(r), r.PathValue("requestId"), in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
