// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes drafts over REST (round 55, part A).
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/draft"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(draft.ErrForbidden)
	httpx.RegisterValidation(draft.ErrBodyTooLong, draft.ErrTooManyFiles, draft.ErrBadThread, draft.ErrBadFile)
}

// ToAPI converts a draft for the REST API.
func ToAPI(d draft.Draft) apispec.Draft {
	out := apispec.Draft{ChannelId: d.ChannelID, Body: d.Body, UpdatedAt: d.UpdatedAt,
		Attachments: make([]apispec.EventAttachment, len(d.Attachments))}
	if d.ThreadRootID != ids.Nil {
		out.ThreadRootId = httpx.Ptr(d.ThreadRootID)
	}
	for i, a := range d.Attachments {
		out.Attachments[i] = apispec.EventAttachment{Id: a.ID, Name: a.Name, Mime: a.Mime, Size: a.Size}
	}
	return out
}

// Register mounts the draft endpoints.
func Register(mux *http.ServeMux, svc *draft.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/me/drafts", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.List(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.DraftList{Drafts: make([]apispec.Draft, len(list))}
		for i, d := range list {
			out.Drafts[i] = ToAPI(d)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("PUT /api/v1/me/drafts", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.SaveDraftRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		in := draft.SaveInput{ChannelID: req.ChannelId, ThreadRootID: httpx.Deref(req.ThreadRootId, ids.Nil), Body: req.Body}
		if req.FileIds != nil {
			if len(*req.FileIds) > draft.MaxFiles {
				httpx.Error(w, r, draft.ErrTooManyFiles)
				return
			}
			in.FileIDs = append(in.FileIDs, *req.FileIds...)
		}
		d, err := svc.Save(r.Context(), httpx.Actor(r), in)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		if d == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		httpx.JSON(w, http.StatusOK, ToAPI(*d))
	}))

	mux.Handle("DELETE /api/v1/me/drafts", wrap(func(w http.ResponseWriter, r *http.Request) {
		ch, err := ids.Parse(r.URL.Query().Get("channel_id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "channel_id must be a UUID"})
			return
		}
		thread := ids.Nil
		if v := r.URL.Query().Get("thread_root_id"); v != "" {
			if thread, err = ids.Parse(v); err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "thread_root_id must be a UUID"})
				return
			}
		}
		if err := svc.Delete(r.Context(), httpx.Actor(r), ch, thread); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
