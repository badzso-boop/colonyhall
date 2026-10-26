package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(delegation.ErrForbidden)
}

// ToAPI converts a DelegationView to its API shape.
func ToAPI(v delegation.DelegationView) apispec.Delegation {
	var threadRoot *ids.ID
	if v.ThreadRootID != ids.Nil {
		threadRoot = &v.ThreadRootID
	}
	var fromHandle *string
	if v.FromAgentHandle != "" {
		fromHandle = &v.FromAgentHandle
	}
	var toHandle *string
	if v.ToAgentHandle != "" {
		toHandle = &v.ToAgentHandle
	}
	return apispec.Delegation{
		Id:              v.ID,
		WorkspaceId:     v.WorkspaceID,
		MessageId:       v.MessageID,
		ChannelId:       v.ChannelID,
		ThreadRootId:    threadRoot,
		FromAgentId:     v.FromAgentID,
		FromAgentHandle: fromHandle,
		ToAgentId:       v.ToAgentID,
		ToAgentHandle:   toHandle,
		QuestionId:      v.QuestionID,
		State:           apispec.DelegationState(v.State),
		DecidedBy:       v.DecidedBy,
		DecidedByHandle: v.DecidedByHandle,
		DecidedAt:       v.DecidedAt,
		CreatedAt:       v.CreatedAt,
	}
}

// Register mounts the delegation routes on mux.
func Register(mux *http.ServeMux, svc *delegation.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/delegations", wrap(func(w http.ResponseWriter, r *http.Request) {
		var state *string
		if s := r.URL.Query().Get("state"); s != "" {
			state = &s
		}
		items, err := svc.List(r.Context(), httpx.Actor(r), state)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.DelegationList{
			Delegations: make([]apispec.Delegation, 0, len(items)),
		}
		for _, it := range items {
			out.Delegations = append(out.Delegations, ToAPI(it))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
