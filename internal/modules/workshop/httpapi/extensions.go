// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"colonyhall/internal/apispec"
	"colonyhall/internal/extrunner"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(workshop.ErrBadManifest, workshop.ErrCompile, workshop.ErrNotApproved, workshop.ErrNoLiveVersion)
	httpx.RegisterStatus(http.StatusNotFound, workshop.ErrVersionNotFound)
	httpx.RegisterStatus(http.StatusServiceUnavailable, extrunner.ErrNoRunner)
}

func extToAPI(v workshop.ExtensionView) apispec.Extension {
	out := apispec.Extension{Id: v.ID, Slug: v.Slug, Title: v.Title, Description: v.Description, BotId: v.BotID,
		State: apispec.ExtensionState(v.State), ApprovalOff: v.ApprovalOff, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
		Versions: make([]apispec.ExtensionVersionSummary, 0, len(v.Versions))}
	if v.BotHandle != "" {
		out.BotHandle = httpx.Ptr(v.BotHandle)
	}
	if v.MaintainerName != "" {
		out.MaintainerHandle = httpx.Ptr(v.MaintainerName)
	}
	if v.LiveVersion > 0 {
		out.LiveVersion = httpx.Ptr(v.LiveVersion)
	}
	if v.DisabledReason != "" {
		out.DisabledReason = httpx.Ptr(v.DisabledReason)
	}
	for _, ver := range v.Versions {
		sum := apispec.ExtensionVersionSummary{Version: ver.Version, Approved: v.Approved[ver.Version], CreatedAt: ver.CreatedAt,
			CreatedBy: ver.CreatedBy, Tools: make([]apispec.ExtensionToolSummary, 0, len(ver.Manifest.Tools))}
		for _, t := range ver.Manifest.Tools {
			ts := apispec.ExtensionToolSummary{Name: t.Name, Kind: apispec.ExtensionToolSummaryKind(t.Kind), Risk: apispec.ToolRisk(t.Risk)}
			if t.Description != "" {
				ts.Description = httpx.Ptr(t.Description)
			}
			sum.Tools = append(sum.Tools, ts)
		}
		out.Versions = append(out.Versions, sum)
	}
	return out
}

func diffToAPI(d workshop.Diff) apispec.ExtensionDiff {
	out := apispec.ExtensionDiff{CodeChanged: d.CodeChanged}
	if len(d.NewConnectors) > 0 {
		out.NewConnectors = &d.NewConnectors
	}
	if len(d.NewEgress) > 0 {
		out.NewEgress = &d.NewEgress
	}
	if len(d.NewStore) > 0 {
		out.NewStore = &d.NewStore
	}
	if len(d.NewTools) > 0 {
		out.NewTools = &d.NewTools
	}
	if len(d.NewRoutines) > 0 {
		out.NewRoutines = &d.NewRoutines
	}
	if len(d.RaisedRisks) > 0 {
		out.RaisedRisks = &d.RaisedRisks
	}
	return out
}

func manifestToAPI(m workshop.Manifest) apispec.ExtensionManifest {
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func registerExtensions(mux *http.ServeMux, svc *workshop.Service, wrap func(http.HandlerFunc) http.Handler) {
	extID := func(w http.ResponseWriter, r *http.Request) (ids.ID, bool) { return pathID(w, r, "extensionId") }
	version := func(w http.ResponseWriter, r *http.Request) (int, bool) {
		n, err := strconv.Atoi(r.PathValue("version"))
		if err != nil || n < 1 {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "version must be a positive integer"})
			return 0, false
		}
		return n, true
	}
	reply := func(w http.ResponseWriter, r *http.Request, id ids.ID, err error, status int) {
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		v, err := svc.GetExtension(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, status, extToAPI(v))
	}

	mux.Handle("GET /api/v1/workshop/extensions", wrap(func(w http.ResponseWriter, r *http.Request) {
		vs, err := svc.ListExtensions(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ExtensionList{Extensions: make([]apispec.Extension, len(vs)), RunnerAvailable: svc.Engine != nil}
		for i, v := range vs {
			out.Extensions[i] = extToAPI(v)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/workshop/extensions", wrap(func(w http.ResponseWriter, r *http.Request) {
		var req apispec.CreateExtensionRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		e, err := svc.CreateExtension(r.Context(), httpx.Actor(r), workshop.ExtensionInput{Slug: req.Slug, Title: req.Title,
			Description: httpx.Deref(req.Description, ""), BotHandle: httpx.Deref(req.BotHandle, ""), BotName: httpx.Deref(req.BotName, "")})
		reply(w, r, e.ID, err, http.StatusCreated)
	}))

	mux.Handle("GET /api/v1/workshop/extensions/{extensionId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := extID(w, r); ok {
			reply(w, r, id, nil, http.StatusOK)
		}
	}))

	mux.Handle("DELETE /api/v1/workshop/extensions/{extensionId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteExtension(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/versions", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		var req apispec.AddExtensionVersionRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var m workshop.Manifest
		raw, _ := json.Marshal(req.Manifest)
		if err := json.Unmarshal(raw, &m); err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid manifest: " + err.Error()})
			return
		}
		v, diff, err := svc.AddVersion(r.Context(), httpx.Actor(r), id, workshop.VersionInput{Manifest: m, Source: httpx.Deref(req.Source, "")})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.ExtensionVersion{Version: v.Version, Manifest: manifestToAPI(v.Manifest), Source: v.Source,
			Diff: diffToAPI(diff), CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy})
	}))

	mux.Handle("GET /api/v1/workshop/extensions/{extensionId}/versions/{version}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		n, ok := version(w, r)
		if !ok {
			return
		}
		v, diff, err := svc.GetVersion(r.Context(), httpx.Actor(r), id, n)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		ev, err := svc.GetExtension(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.ExtensionVersion{Version: v.Version, Manifest: manifestToAPI(v.Manifest), Source: v.Source,
			Approved: ev.Approved[v.Version], Diff: diffToAPI(diff), CreatedAt: v.CreatedAt, CreatedBy: v.CreatedBy})
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/versions/{version}/approve", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		n, ok := version(w, r)
		if !ok {
			return
		}
		_, _, err := svc.ApproveVersion(r.Context(), httpx.Actor(r), id, n)
		reply(w, r, id, err, http.StatusOK)
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/versions/{version}/test", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		n, ok := version(w, r)
		if !ok {
			return
		}
		var req apispec.TestExtensionToolRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		input := json.RawMessage("{}")
		if req.Input != nil {
			input, _ = json.Marshal(*req.Input)
		}
		res, logs, err := svc.TestTool(r.Context(), httpx.Actor(r), id, n, req.Tool, input)
		out := apispec.ExtensionTestResult{Output: res.Text, IsError: res.IsError, Logs: logs}
		if out.Logs == nil {
			out.Logs = []string{}
		}
		if err != nil {
			var forbidden bool
			for _, e := range []error{workshop.ErrForbidden, workshop.ErrNotFound, workshop.ErrVersionNotFound, workshop.ErrUnknownTool} {
				forbidden = forbidden || errorsIs(err, e)
			}
			if forbidden {
				httpx.Error(w, r, err)
				return
			}
			// The tool failed: the person sees why (they built it).
			out.Output, out.IsError = err.Error(), true
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/rollback", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		var req apispec.RollbackExtensionRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		_, err := svc.Rollback(r.Context(), httpx.Actor(r), id, req.Version)
		reply(w, r, id, err, http.StatusOK)
	}))

	mux.Handle("PUT /api/v1/workshop/extensions/{extensionId}/enabled", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		var req apispec.ExtensionEnabledRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		_, err := svc.SetEnabled(r.Context(), httpx.Actor(r), id, req.Enabled)
		reply(w, r, id, err, http.StatusOK)
	}))

	mux.Handle("PUT /api/v1/workshop/extensions/{extensionId}/approval", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		var req apispec.ConnectorApprovalRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
		_, err := svc.SetExtensionApprovalOff(r.Context(), httpx.Actor(r), id, req.ApprovalOff)
		reply(w, r, id, err, http.StatusOK)
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/take-over", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := extID(w, r)
		if !ok {
			return
		}
		_, err := svc.TakeOver(r.Context(), httpx.Actor(r), id)
		reply(w, r, id, err, http.StatusOK)
	}))
}
