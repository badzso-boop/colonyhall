// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi is the search's REST transport: GET /api/v1/search.
package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(search.ErrEmptyQuery, search.ErrQueryTooLong, search.ErrBadMode)
}

// Register mounts the search endpoint.
func Register(mux *http.ServeMux, svc *search.Service, auth identity.RequestAuthenticator) {
	mux.Handle("GET /api/v1/search", httpx.RequireAuth(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, err := parseQuery(r)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		by := httpx.Actor(r)
		res, err := svc.Search(r.Context(), by, q)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, Response(res, by.Kind == identity.KindHuman && by.IsAdmin()))
	})))
}

func parseQuery(r *http.Request) (search.Query, error) {
	v := r.URL.Query()
	q := search.Query{Text: v.Get("q"), Mode: search.Mode(v.Get("mode"))}
	for _, k := range v["kind"] {
		for _, part := range strings.Split(k, ",") {
			switch kind := search.SourceKind(strings.TrimSpace(part)); kind {
			case search.SourceMessage, search.SourceWiki, search.SourceDecision, search.SourceSummary:
				q.Filters.Kinds = append(q.Filters.Kinds, kind)
			case "":
			default:
				return q, &httpx.BadRequest{Detail: "unknown kind: " + part}
			}
		}
	}
	var err error
	if q.Filters.ChannelID, err = optID(v.Get("channel_id")); err != nil {
		return q, err
	}
	if q.Filters.AuthorID, err = optID(v.Get("author_id")); err != nil {
		return q, err
	}
	if q.Filters.Since, err = optTime(v.Get("since")); err != nil {
		return q, err
	}
	if q.Filters.Until, err = optTime(v.Get("until")); err != nil {
		return q, err
	}
	if s := v.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > search.MaxLimit {
			return q, &httpx.BadRequest{Detail: "limit must be 1–20"}
		}
		q.Limit = n
	}
	return q, nil
}

func optID(s string) (ids.ID, error) {
	if s == "" {
		return ids.Nil, nil
	}
	id, err := ids.Parse(s)
	if err != nil {
		return ids.Nil, &httpx.BadRequest{Detail: "invalid id: " + s}
	}
	return id, nil
}

func optTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, &httpx.BadRequest{Detail: "invalid time: " + s}
	}
	return &t, nil
}

func optUUID(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

// Response maps a result to the contract; scores are for admins only.
func Response(res search.Result, scores bool) apispec.SearchResponse {
	out := apispec.SearchResponse{Mode: apispec.SearchResponseMode(res.Mode), Hits: []apispec.SearchHit{}, Degraded: []apispec.SearchResponseDegraded{}}
	for _, d := range res.Degraded {
		out.Degraded = append(out.Degraded, apispec.SearchResponseDegraded(d))
	}
	if res.Notice != "" {
		n := res.Notice
		out.Notice = &n
	}
	for _, h := range res.Hits {
		c := h.Chunk
		hit := apispec.SearchHit{Kind: apispec.SearchSourceKind(c.SourceKind), SourceId: uuid.UUID(c.SourceID), Part: c.Part,
			ChannelId: optUUID(c.ChannelID), ThreadRootId: optUUID(c.ThreadRootID), AuthorId: optUUID(c.AuthorID),
			At: c.SourceAt, Header: c.Header, Snippet: h.Snippet}
		if c.SourceRef != "" {
			ref := c.SourceRef
			hit.Ref = &ref
		}
		if scores {
			hit.Scores = &apispec.SearchScores{Keyword: h.Keyword, Semantic: h.Semantic, Fused: h.Fused, Rerank: h.Rerank}
		}
		out.Hits = append(out.Hits, hit)
	}
	return out
}

// Status maps the search state for the status page.
func Status(st search.Status) apispec.SearchStatus {
	backend := func(b search.BackendState) apispec.SearchBackend {
		out := apispec.SearchBackend{Configured: b.Configured, Reachable: b.Reachable, LastOkAt: b.LastOK}
		if b.Model != "" {
			m := b.Model
			out.Model = &m
		}
		if b.LastError != "" {
			e := b.LastError
			out.LastError = &e
		}
		return out
	}
	out := apispec.SearchStatus{Chunks: st.Index.Chunks, Pending: st.Index.Pending, LastEmbedAt: st.Index.LastEmbedAt,
		Models: st.Index.Models, Embedder: backend(st.Embedder), Reranker: backend(st.Reranker)}
	if out.Models == nil {
		out.Models = []string{}
	}
	if st.ModelMismatch != "" {
		m := st.ModelMismatch
		out.ModelMismatch = &m
	}
	return out
}
