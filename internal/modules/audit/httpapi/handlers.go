// Package httpapi exposes the audit log to admins over REST.
package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(audit.ErrForbidden)
}

// noFormula keeps a spreadsheet from running a cell as a formula.
func noFormula(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// filter reads the audit filters from the query.
func filter(w http.ResponseWriter, r *http.Request) (audit.Filter, bool) {
	q := r.URL.Query()
	f := audit.Filter{Actions: q["action"], TargetType: q.Get("target_type")}
	if raw := q.Get("actor_id"); raw != "" {
		id, err := ids.Parse(raw)
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "actor_id must be a UUID"})
			return f, false
		}
		f.ActorID = id
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := q.Get(name); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be an RFC 3339 time"})
				return f, false
			}
			*dst = &t
		}
	}
	return f, true
}

// Register mounts the audit routes on mux.
func Register(mux *http.ServeMux, svc *audit.Service, auth identity.RequestAuthenticator, dir identity.Directory) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/admin/audit", wrap(func(w http.ResponseWriter, r *http.Request) {
		f, ok := filter(w, r)
		if !ok {
			return
		}
		f.Before, _ = strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		f.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		entries, err := svc.Search(r.Context(), httpx.Actor(r), f)
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

	mux.Handle("GET /api/v1/admin/audit/actions", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.Actions(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.AuditActions{Actions: list})
	}))

	mux.Handle("GET /api/v1/admin/audit/export", wrap(func(w http.ResponseWriter, r *http.Request) {
		f, ok := filter(w, r)
		if !ok {
			return
		}
		handles := map[ids.ID]string{}
		handle := func(id ids.ID) string {
			if h, ok := handles[id]; ok {
				return h
			}
			h := ""
			if a, err := dir.ActorByID(r.Context(), id); err == nil {
				h = a.Handle
			}
			handles[id] = h
			return h
		}
		var out *csv.Writer
		err := svc.Export(r.Context(), httpx.Actor(r), f, func(total int64) error {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="colonyhall-audit.csv"`)
			// The BOM makes Excel read UTF-8.
			if _, err := w.Write([]byte("\xef\xbb\xbf")); err != nil {
				return err
			}
			if total > audit.MaxExportRows {
				if _, err := fmt.Fprintf(w, "# truncated: %d of %d rows\n", audit.MaxExportRows, total); err != nil {
					return err
				}
			}
			out = csv.NewWriter(w)
			return out.Write([]string{"seq", "at", "actor", "action", "target_type", "target_id", "payload"})
		}, func(e audit.Entry) error {
			return out.Write([]string{strconv.FormatInt(e.Seq, 10), e.At.UTC().Format(time.RFC3339), noFormula(handle(e.ActorID)), noFormula(e.Action),
				noFormula(e.TargetType), e.TargetID.String(), noFormula(string(e.Payload))})
		})
		if out != nil {
			out.Flush()
		}
		if err != nil && out == nil {
			httpx.Error(w, r, err)
		}
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
