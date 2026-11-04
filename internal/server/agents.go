package server

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/agents"
	"colonyhall/internal/modules/hosts"
	hostshttp "colonyhall/internal/modules/hosts/httpapi"
	"colonyhall/internal/modules/identity"
	identityhttp "colonyhall/internal/modules/identity/httpapi"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(agents.ErrInstructionsTooLong, agents.ErrInstructionsSecret, agents.ErrHostRequired,
		agents.ErrAPIKeyWithHost, agents.ErrEffectiveTooLong, agents.ErrChannelNotFound)
	httpx.RegisterStatus(http.StatusConflict, agents.ErrHandleTaken)
	httpx.RegisterForbidden(agents.ErrForbidden)
}

// registerAgents serves the agent lifecycle (round 33): POST /agents,
// DELETE /agents/{agentId}, GET /agents/{agentId}/instructions. They live
// here because the responses use the identity and hosts API views.
func registerAgents(mux *http.ServeMux, idSvc *identity.Service, agSvc *agents.Service) {
	auth := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(idSvc, h) }
	agentID := func(w http.ResponseWriter, r *http.Request) (ids.ID, bool) {
		id, err := ids.Parse(r.PathValue("agentId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid agent id"})
			return ids.Nil, false
		}
		return id, true
	}

	mux.Handle("POST /api/v1/agents", auth(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.CreateAgentRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		in := agents.CreateInput{
			Profile: identity.AgentActorInput{
				Handle: req.Handle, DisplayName: httpx.Deref(req.DisplayName, ""), AvatarEmoji: httpx.Deref(req.AvatarEmoji, ""),
				Description: httpx.Deref(req.Description, ""), OwnerID: httpx.Deref(req.OwnerId, ids.Nil),
			},
			APIKey:       httpx.Deref(req.ApiKey, false),
			Instructions: httpx.Deref(req.Instructions, ""),
		}
		if req.Color != nil {
			in.Profile.Color = string(*req.Color)
		}
		if h := req.Host; h != nil {
			in.Host = &hosts.AssignInput{
				HostID: h.HostId, Client: string(h.Client), Model: httpx.Deref(h.Model, ""), WorkModel: httpx.Deref(h.WorkModel, ""),
				AllowWork: httpx.Deref(h.AllowWork, false), PermissionMode: httpx.Deref(h.PermissionMode, ""),
				RootDir: httpx.Deref(h.RootDir, ""), WorktreeRepo: httpx.Deref(h.WorktreeRepo, ""), Sandbox: h.Sandbox,
				MaxSessionTokens: h.MaxSessionTokens, DailyTokenLimit: h.DailyTokenLimit,
				AllowPreview: httpx.Deref(h.AllowPreview, false), AllowWebRead: httpx.Deref(h.AllowWebRead, false),
				PartnerID: httpx.Deref(h.PartnerId, ids.Nil),
			}
			if h.WriteScope != nil {
				in.Host.WriteScope = string(*h.WriteScope)
			}
			if h.Mode != nil {
				in.Host.Mode = string(*h.Mode)
			}
			if h.Approvals != nil {
				in.Host.Approvals = string(*h.Approvals)
			}
		}
		if req.Grants != nil {
			for _, g := range *req.Grants {
				in.Grants = append(in.Grants, hosts.GrantInput{Path: g.Path, Mode: string(g.Mode)})
			}
		}
		if req.ChannelIds != nil {
			in.ChannelIDs = *req.ChannelIds
		}
		created, err := agSvc.Create(r.Context(), httpx.Actor(r), in)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.CreatedAgent{Actor: identityhttp.ToAPI(created.Actor)}
		if created.Config != nil {
			h := hostshttp.AgentOut(*created.Config)
			out.Host = &h
		}
		if created.Token != "" {
			out.Token = &created.Token
		}
		httpx.JSON(w, http.StatusCreated, out)
	}))

	mux.Handle("DELETE /api/v1/agents/{agentId}", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := agentID(w, r)
		if !ok {
			return
		}
		if err := agSvc.Archive(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PUT /api/v1/agents/{agentId}/owner", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := agentID(w, r)
		if !ok {
			return
		}
		var req apispec.SetOwnerRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		a, err := idSvc.SetAgentOwner(r.Context(), httpx.Actor(r), id, httpx.Deref(req.OwnerId, ids.Nil))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, identityhttp.ToAPI(a))
	}))

	mux.Handle("GET /api/v1/agents/{agentId}/instructions", auth(func(w http.ResponseWriter, r *http.Request) {
		id, ok := agentID(w, r)
		if !ok {
			return
		}
		v, err := agSvc.Instructions(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.AgentOwnInstructions{Text: v.Text, Effective: v.Effective}
		if v.Version > 0 {
			out.Version = &v.Version
			out.CreatedBy = &v.CreatedBy
			out.CreatedAt = &v.CreatedAt
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
