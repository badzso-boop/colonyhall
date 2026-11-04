package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterValidation(hosts.ErrBadLoginToken)
	httpx.RegisterStatus(http.StatusUnprocessableEntity, hosts.ErrLoginCheckFailed, hosts.ErrLoginNotConfigured, hosts.ErrLoginWriteFailed)
	httpx.RegisterStatus(http.StatusTooManyRequests, hosts.ErrLoginTooMany)
	httpx.RegisterStatus(http.StatusGatewayTimeout, hosts.ErrLoginTimeout)
}

// RegisterLogin mounts the Claude login relay (round 38): the person's
// routes and the guard's routes (host token). The token is never logged:
// the request log records only the method and path.
func RegisterLogin(mux *http.ServeMux, svc *hosts.Service, relay *hosts.LoginRelay, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("PUT /api/v1/hosts/{hostId}/claude-login", wrap(func(w http.ResponseWriter, r *http.Request) {
		hostID, ok := pathID(w, r, "hostId")
		if !ok {
			return
		}
		var in apispec.SetClaudeLoginRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		res, err := relay.Set(r.Context(), httpx.Actor(r), hostID, in.Token)
		in.Token = ""
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.ClaudeLoginState{SetAt: res.SetAt})
	}))

	mux.Handle("DELETE /api/v1/hosts/{hostId}/claude-login", wrap(func(w http.ResponseWriter, r *http.Request) {
		hostID, ok := pathID(w, r, "hostId")
		if !ok {
			return
		}
		if err := relay.Clear(r.Context(), httpx.Actor(r), hostID); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/host/claude-login/{requestId}", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		op, token, err := relay.Request(host(r), r.PathValue("requestId"))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.HostClaudeLoginRequest{RequestId: r.PathValue("requestId"), Op: apispec.HostClaudeLoginRequestOp(op)}
		if token != "" {
			out.Token = &token
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/host/claude-login/{requestId}/result", requireHost(svc, func(w http.ResponseWriter, r *http.Request) {
		var in apispec.HostClaudeLoginResult
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		res := hosts.LoginResult{OK: in.Ok, SetAt: in.SetAt}
		if in.ErrorCode != nil {
			res.ErrorCode = string(*in.ErrorCode)
		}
		if err := relay.Deliver(host(r), r.PathValue("requestId"), res); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
