// Package httpapi exposes the wiki module over REST.
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/wiki"
)

func init() {
	httpx.RegisterForbidden(wiki.ErrForbidden)
	httpx.RegisterValidation(
		wiki.ErrTitleEmpty,
		wiki.ErrTitleTooLong,
		wiki.ErrBodyEmpty,
		wiki.ErrBodyTooLong,
		wiki.ErrAppendEmpty,
		wiki.ErrAppendTooLong,
	)
	httpx.RegisterStatus(http.StatusNotFound, wiki.ErrNotFound, wiki.ErrDeleted)
	httpx.RegisterStatus(http.StatusConflict, wiki.ErrConflict)
}

func toPageAPI(p wiki.Page) apispec.WikiPage {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	return apispec.WikiPage{
		Id:        p.ID,
		Slug:      p.Slug,
		Title:     p.Title,
		Tags:      tags,
		Body:      p.Body,
		Rev:       p.Rev,
		CreatedBy: p.CreatedBy,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
		DeletedAt: p.DeletedAt,
	}
}

func toIndexAPI(idx wiki.PageIndex) apispec.WikiPageIndex {
	tags := idx.Tags
	if tags == nil {
		tags = []string{}
	}
	return apispec.WikiPageIndex{
		Slug:      idx.Slug,
		Title:     idx.Title,
		Tags:      tags,
		Excerpt:   idx.Excerpt,
		CreatedAt: idx.CreatedAt,
		UpdatedAt: idx.UpdatedAt,
		DeletedAt: idx.DeletedAt,
	}
}

func toRevisionAPI(rev wiki.Revision) apispec.WikiRevision {
	return apispec.WikiRevision{
		PageId:    rev.PageID,
		Rev:       rev.Rev,
		Body:      rev.Body,
		Kind:      apispec.WikiRevisionKind(rev.Kind),
		AuthorId:  rev.AuthorID,
		CreatedAt: rev.CreatedAt,
	}
}

// Register mounts the wiki REST routes on mux.
func Register(mux *http.ServeMux, svc *wiki.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/wiki", wrap(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		deleted := r.URL.Query().Get("deleted") == "1"
		list, err := svc.List(r.Context(), httpx.Actor(r), q, deleted)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.WikiPageIndexList{
			Pages: make([]apispec.WikiPageIndex, 0, len(list)),
		}
		for _, item := range list {
			out.Pages = append(out.Pages, toIndexAPI(item))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/wiki", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateWikiPageRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var tags []string
		if in.Tags != nil {
			tags = *in.Tags
		}
		page, err := svc.Create(r.Context(), httpx.Actor(r), wiki.CreateInput{
			Title: in.Title,
			Body:  in.Body,
			Tags:  tags,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toPageAPI(page))
	}))

	mux.Handle("GET /api/v1/wiki/{slug}", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		page, err := svc.Get(r.Context(), httpx.Actor(r), slug)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPageAPI(page))
	}))

	mux.Handle("PUT /api/v1/wiki/{slug}", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var in apispec.EditWikiPageRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		page, err := svc.Edit(r.Context(), httpx.Actor(r), slug, wiki.EditInput{
			BaseRev: in.BaseRev,
			Body:    in.Body,
			Title:   httpx.Deref(in.Title, ""),
			Tags:    in.Tags,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPageAPI(page))
	}))

	mux.Handle("DELETE /api/v1/wiki/{slug}", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if err := svc.Delete(r.Context(), httpx.Actor(r), slug); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/wiki/{slug}/append", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		var in apispec.AppendWikiPageRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		page, err := svc.Append(r.Context(), httpx.Actor(r), slug, in.Text)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPageAPI(page))
	}))

	mux.Handle("POST /api/v1/wiki/{slug}/restore", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		page, err := svc.Restore(r.Context(), httpx.Actor(r), slug)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, toPageAPI(page))
	}))

	mux.Handle("GET /api/v1/wiki/{slug}/revisions", wrap(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		revs, err := svc.Revisions(r.Context(), httpx.Actor(r), slug)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.WikiRevisionList{
			Revisions: make([]apispec.WikiRevision, 0, len(revs)),
		}
		for _, rev := range revs {
			out.Revisions = append(out.Revisions, toRevisionAPI(rev))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
