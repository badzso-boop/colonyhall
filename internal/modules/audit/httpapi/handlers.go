// Package httpapi exposes the audit log to admins over REST.
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterForbidden(audit.ErrForbidden)
}

// Register mounts the audit routes on mux.
func Register(mux *http.ServeMux, svc *audit.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/admin/audit", wrap(func(w http.ResponseWriter, r *http.Request) {
		before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		entries, err := svc.List(r.Context(), httpx.Actor(r), before, limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.AuditPage{Entries: make([]apispec.AuditEntry, 0, len(entries))}
		for _, e := range entries {
			var payload map[string]interface{}
			_ = json.Unmarshal(e.Payload, &payload)
			out.Entries = append(out.Entries, apispec.AuditEntry{Seq: e.Seq, ActorId: e.ActorID, Action: e.Action,
				TargetType: e.TargetType, TargetId: e.TargetID, Payload: payload, At: e.At})
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("GET /api/v1/admin/audit/verify", wrap(func(w http.ResponseWriter, r *http.Request) {
		res, err := svc.Verify(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.AuditVerify{Ok: res.OK, Checked: res.Checked}
		if !res.OK {
			out.BrokenAt = &res.BrokenAt
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
