// Package httpapi exposes hosts over REST: admin routes (session or
// bearer) and guard routes (host token).
package httpapi

import (
	"context"
	"net/http"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(hosts.ErrForbidden, hosts.ErrNotAgent)
	httpx.RegisterValidation(hosts.ErrBadPath, hosts.ErrBadMode, hosts.ErrBadClient, hosts.ErrBadName,
		hosts.ErrBadState, hosts.ErrNotAssigned, hosts.ErrHostRevoked, hosts.ErrBadAgentMode, hosts.ErrBadRootDir,
		hosts.ErrWorktreeDirectOnly, hosts.ErrBadApprovals, hosts.ErrApprovalsChatClient)
	httpx.RegisterStatus(http.StatusUnauthorized, hosts.ErrBadCode, hosts.ErrBadHostToken)
}

type hostKey struct{}

func host(r *http.Request) hosts.Host { return r.Context().Value(hostKey{}).(hosts.Host) }

// requireHost authenticates a guard by its host token.
func requireHost(svc *hosts.Service, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, err := svc.AuthenticateHost(r.Context(), httpx.BearerToken(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), hostKey{}, h)))
	})
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}

// hostOut is the API shape of a host. The report is the module's own type:
// its JSON matches the HostReport schema.
type hostOut struct {
	ID         ids.ID              `json:"id"`
	Name       string              `json:"name"`
	State      string              `json:"state"`
	CreatedAt  time.Time           `json:"created_at"`
	LastSeenAt *time.Time          `json:"last_seen_at,omitempty"`
	Report     hosts.Report        `json:"report"`
	Agents     []apispec.HostAgent `json:"agents"`
}

func agentOut(a hosts.AgentConfig) apispec.HostAgent {
	approvals := a.Approvals
	if approvals == "" {
		approvals = hosts.ApprovalsNone
	}
	out := apispec.HostAgent{AgentId: a.AgentID, Handle: a.Handle, HostId: a.HostID, Client: apispec.HostAgentClient(a.Client),
		Model: a.Model, WorkModel: a.WorkModel, AllowWork: a.AllowWork, PermissionMode: a.PermissionMode,
		Mode: apispec.HostAgentMode(a.Mode), RootDir: a.RootDir, WorktreeRepo: a.WorktreeRepo, Sandbox: a.Sandbox, Approvals: apispec.HostAgentApprovals(approvals),
		Grants: make([]apispec.FolderGrant, 0, len(a.Grants))}
	for _, g := range a.Grants {
		out.Grants = append(out.Grants, grantOut(g, nil))
	}
	return out
}

func grantOut(g hosts.Grant, outside *bool) apispec.FolderGrant {
	return apispec.FolderGrant{Id: g.ID, Path: g.Path, Mode: apispec.FolderGrantMode(g.Mode), CreatedAt: g.CreatedAt, OutsideRoots: outside}
}

// Register mounts the hosts routes on mux.
func Register(mux *http.ServeMux, svc *hosts.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("POST /api/v1/hosts/pairing-codes", wrap(func(w http.ResponseWriter, r *http.Request) {
		code, exp, err := svc.CreatePairingCode(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.PairingCode{Code: code, ExpiresAt: exp})
	}))

	mux.HandleFunc("POST /api/v1/hosts/enroll", func(w http.ResponseWriter, r *http.Request) {
		var in apispec.EnrollRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		h, secret, err := svc.Enroll(r.Context(), in.Code, in.Name)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.EnrollResponse{HostId: h.ID, Token: secret})
	})

	mux.Handle("GET /api/v1/hosts", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListHosts(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := struct {
			Hosts []hostOut `json:"hosts"`
		}{Hosts: make([]hostOut, 0, len(list))}
		for _, h := range list {
			ho := hostOut{ID: h.ID, Name: h.Name, State: h.State, CreatedAt: h.CreatedAt, LastSeenAt: h.LastSeenAt,
				Report: h.Report, Agents: make([]apispec.HostAgent, 0, len(h.Agents))}
			if ho.Report.Roots == nil {
				ho.Report.Roots = []string{}
			}
			for _, a := range h.Agents {
				ho.Agents = append(ho.Agents, agentOut(a))
			}
			out.Hosts = append(out.Hosts, ho)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("PUT /api/v1/hosts/{hostId}/state", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "hostId")
		if !ok {
			return
		}
		var in apispec.SetHostStateRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := svc.SetHostState(r.Context(), httpx.Actor(r), id, string(in.State)); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PUT /api/v1/agents/{agentId}/host", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "agentId")
		if !ok {
			return
		}
		var in apispec.AssignRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var mode string
		if in.Mode != nil {
			mode = string(*in.Mode)
		}
		var approvals string
		if in.Approvals != nil {
			approvals = string(*in.Approvals)
		}
		a, err := svc.Assign(r.Context(), httpx.Actor(r), id, hosts.AssignInput{
			HostID: in.HostId, Client: string(in.Client),
			Model: httpx.Deref(in.Model, ""), WorkModel: httpx.Deref(in.WorkModel, ""), AllowWork: httpx.Deref(in.AllowWork, false),
			PermissionMode: httpx.Deref(in.PermissionMode, ""),
			Mode:           mode, RootDir: httpx.Deref(in.RootDir, ""), WorktreeRepo: httpx.Deref(in.WorktreeRepo, ""), Sandbox: in.Sandbox, Approvals: approvals,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, agentOut(a))
	}))

	mux.Handle("DELETE /api/v1/agents/{agentId}/host", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "agentId")
		if !ok {
			return
		}
		if err := svc.Unassign(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/agents/{agentId}/grants", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "agentId")
		if !ok {
			return
		}
		var in apispec.GrantRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		g, err := svc.AddGrant(r.Context(), httpx.Actor(r), id, in.Path, string(in.Mode))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, grantOut(g.Grant, &g.OutsideRoots))
	}))

	mux.Handle("DELETE /api/v1/grants/{grantId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "grantId")
		if !ok {
			return
		}
		if err := svc.RevokeGrant(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// Guard routes.
	mux.Handle("POST /api/v1/host/report", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		var in hosts.Report
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		if err := svc.SaveReport(r.Context(), host(r), in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/host/config", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		cfg, err := svc.Config(r.Context(), host(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		etag := `"` + cfg.Version + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		out := apispec.HostConfig{Version: cfg.Version, Agents: make([]apispec.HostAgent, 0, len(cfg.Agents))}
		for _, a := range cfg.Agents {
			out.Agents = append(out.Agents, agentOut(a))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/host/agents/{agentId}/token", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "agentId")
		if !ok {
			return
		}
		secret, exp, err := svc.AgentToken(r.Context(), host(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.AgentToken{Token: secret, ExpiresAt: exp})
	}))
}
