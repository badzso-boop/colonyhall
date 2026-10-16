// Package httpapi exposes the git module over REST.
package httpapi

import (
	"errors"
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/git"
	"colonyhall/internal/modules/identity"
)

func init() {
	httpx.RegisterForbidden(identity.ErrForbidden)
	httpx.RegisterValidation(git.ErrInvalidRepo, git.ErrInvalidWebhook)
	httpx.RegisterStatus(http.StatusNotFound, git.ErrMappingNotFound, git.ErrLinkNotFound)
	httpx.RegisterStatus(http.StatusUnauthorized, git.ErrUnauthorizedWebhook)
}

// Register mounts the git REST and webhook routes on mux.
func Register(mux *http.ServeMux, svc *git.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/git/mappings", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListMappings(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		mappings := make([]apispec.GitRepoMapping, 0, len(list))
		for _, m := range list {
			mappings = append(mappings, apispec.GitRepoMapping{
				Repo:      m.Repo,
				ChannelId: m.ChannelID,
				CreatedAt: m.CreatedAt,
			})
		}
		httpx.JSON(w, http.StatusOK, apispec.GitRepoMappingList{Mappings: mappings})
	}))

	mux.Handle("POST /api/v1/git/mappings", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.SetGitRepoMappingRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		res, err := svc.SetMapping(r.Context(), httpx.Actor(r), in.Repo, in.ChannelId)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.GitRepoMapping{
			Repo:      res.Repo,
			ChannelId: res.ChannelID,
			CreatedAt: res.CreatedAt,
		})
	}))

	mux.Handle("DELETE /api/v1/git/mappings/{owner}/{repo}", wrap(func(w http.ResponseWriter, r *http.Request) {
		owner := r.PathValue("owner")
		repo := r.PathValue("repo")
		repoName := owner + "/" + repo
		if err := svc.DeleteMapping(r.Context(), httpx.Actor(r), repoName); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /api/v1/hooks/gitea", func(w http.ResponseWriter, r *http.Request) {
		if svc == nil || svc.Provider == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		ev, err := svc.Provider.ParseWebhook(r, svc.WebhookSecret)
		if err != nil {
			if errors.Is(err, git.ErrUnauthorizedWebhook) {
				httpx.Error(w, r, git.ErrUnauthorizedWebhook)
				return
			}
			httpx.Error(w, r, err)
			return
		}
		if err := svc.HandleWebhookEvent(r.Context(), ev); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
