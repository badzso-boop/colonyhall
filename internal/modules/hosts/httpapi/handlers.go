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
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(hosts.ErrForbidden, hosts.ErrNotAgent)
	httpx.RegisterValidation(hosts.ErrBadPath, hosts.ErrBadMode, hosts.ErrBadClient, hosts.ErrBadName, hosts.ErrBadColor, hosts.ErrOutsideRoots, hosts.ErrClientNotReported, hosts.ErrBadDailyTokenLimit, hosts.ErrBadMaxSessionTokens,
		hosts.ErrBadState, hosts.ErrNotAssigned, hosts.ErrHostRevoked, hosts.ErrBadAgentMode, hosts.ErrBadRootDir,
		hosts.ErrWorktreeDirectOnly, hosts.ErrPreviewFullAccess, hosts.ErrWebReadSandboxedOnly, hosts.ErrBadApprovals, hosts.ErrApprovalsChatClient,
		hosts.ErrBadWriteScope, hosts.ErrDocsOnlyClient, hosts.ErrDocsOnlyApprovals, hosts.ErrDocsOnlyPreview, hosts.ErrPartnerRequired,
		hosts.ErrPartnerDocsOnly, hosts.ErrBadPartner)
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
	Color      string              `json:"color"`
	State      string              `json:"state"`
	CreatedAt  time.Time           `json:"created_at"`
	LastSeenAt *time.Time          `json:"last_seen_at,omitempty"`
	Report     hosts.Report        `json:"report"`
	Agents     []apispec.HostAgent `json:"agents"`
	OwnerID    *ids.ID             `json:"owner_id,omitempty"`
}

func toHostOut(h hosts.HostView) hostOut {
	ho := hostOut{ID: h.ID, Name: h.Name, Color: h.Color, State: h.State, CreatedAt: h.CreatedAt, LastSeenAt: h.LastSeenAt,
		Report: h.Report, Agents: make([]apispec.HostAgent, 0, len(h.Agents))}
	if h.OwnerID != ids.Nil {
		owner := h.OwnerID
		ho.OwnerID = &owner
	}
	if ho.Report.Roots == nil {
		ho.Report.Roots = []string{}
	}
	for _, a := range h.Agents {
		ho.Agents = append(ho.Agents, AgentOut(a))
	}
	return ho
}

// AgentOut is the API view of an agent's machine configuration.
func AgentOut(a hosts.AgentConfig) apispec.HostAgent {
	approvals := a.Approvals
	if approvals == "" {
		approvals = hosts.ApprovalsNone
	}
	out := apispec.HostAgent{AgentId: a.AgentID, Handle: a.Handle, HostId: a.HostID, Client: apispec.HostAgentClient(a.Client),
		Model: a.Model, WorkModel: a.WorkModel, AllowWork: a.AllowWork, PermissionMode: a.PermissionMode,
		Mode: apispec.HostAgentMode(a.Mode), RootDir: a.RootDir, WorktreeRepo: a.WorktreeRepo, Sandbox: a.Sandbox, Approvals: apispec.HostAgentApprovals(approvals),
		MaxSessionTokens: a.MaxSessionTokens, DailyTokenLimit: a.DailyTokenLimit,
		AllowPreview: &a.AllowPreview,
		AllowWebRead: &a.AllowWebRead,
		Grants:       make([]apispec.FolderGrant, 0, len(a.Grants))}
	scope := a.WriteScope
	if scope == "" {
		scope = hosts.WriteScopeAll
	}
	ws := apispec.HostAgentWriteScope(scope)
	out.WriteScope = &ws
	if a.PartnerID != ids.Nil {
		pid, ph := a.PartnerID, a.PartnerHandle
		out.PartnerId, out.PartnerHandle = &pid, &ph
	}
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
			out.Hosts = append(out.Hosts, toHostOut(h))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("PATCH /api/v1/hosts/{hostId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "hostId")
		if !ok {
			return
		}
		var in apispec.UpdateHostRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var color *string
		if in.Color != nil {
			c := string(*in.Color)
			color = &c
		}
		by := httpx.Actor(r)
		h, err := svc.UpdateHost(r.Context(), by, id, hosts.UpdateHostInput{Name: in.Name, Color: color})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		hostView(w, r, svc, by, h.ID)
	}))

	mux.Handle("PUT /api/v1/hosts/{hostId}/owner", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "hostId")
		if !ok {
			return
		}
		var in apispec.SetOwnerRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		by := httpx.Actor(r)
		h, err := svc.SetHostOwner(r.Context(), by, id, httpx.Deref(in.OwnerId, ids.Nil))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		hostView(w, r, svc, by, h.ID)
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

	// The agent configuration endpoints are closed (round 33): an agent is
	// configured once with POST /agents; a change means deleting it and
	// creating a new one. They stay in the contract so old clients get a
	// clear 409 instead of a 404. Only signed-in callers reach the answer.
	immutable := wrap(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, identity.ErrAgentConfigImmutable)
	})
	mux.Handle("PUT /api/v1/agents/{agentId}/host", immutable)
	mux.Handle("DELETE /api/v1/agents/{agentId}/host", immutable)
	mux.Handle("POST /api/v1/agents/{agentId}/grants", immutable)
	mux.Handle("DELETE /api/v1/grants/{grantId}", immutable)

	mux.Handle("PATCH /api/v1/agents/{agentId}/limits", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "agentId")
		if !ok {
			return
		}
		var in apispec.AgentLimitsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		a, err := svc.SetLimits(r.Context(), httpx.Actor(r), id, in.DailyTokenLimit, in.MaxSessionTokens)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.AgentLimits{DailyTokenLimit: a.DailyTokenLimit, MaxSessionTokens: a.MaxSessionTokens})
	}))

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
			out.Agents = append(out.Agents, AgentOut(a))
		}
		previews := make([]apispec.HostPreview, 0, len(cfg.Previews))
		for _, p := range cfg.Previews {
			var urlPtr, errPtr *string
			if p.URL != "" {
				urlPtr = &p.URL
			}
			if p.Error != "" {
				errPtr = &p.Error
			}
			previews = append(previews, apispec.HostPreview{
				Id:          p.ID,
				AgentId:     p.AgentID,
				Name:        p.Name,
				ComposePath: p.ComposePath,
				State:       apispec.HostPreviewState(p.State),
				Url:         urlPtr,
				Error:       errPtr,
				ExpiresAt:   p.ExpiresAt,
			})
		}
		out.Previews = &previews
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

// hostView answers with one machine as the caller sees it on the Machines page.
func hostView(w http.ResponseWriter, r *http.Request, svc *hosts.Service, by identity.Actor, id ids.ID) {
	views, err := svc.ListHosts(r.Context(), by)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	for _, v := range views {
		if v.ID == id {
			httpx.JSON(w, http.StatusOK, toHostOut(v))
			return
		}
	}
	httpx.Error(w, r, db.ErrNotFound)
}
