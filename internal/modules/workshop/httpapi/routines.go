// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"errors"
	"io"
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/tenant"
)

func init() {
	httpx.RegisterValidation(workshop.ErrNotWebhook, workshop.ErrRoutineTarget)
	httpx.RegisterStatus(http.StatusNotFound, workshop.ErrNoRoutine, workshop.ErrBadWebhook)
	httpx.RegisterStatus(http.StatusRequestEntityTooLarge, errTooLarge)
}

var errTooLarge = errors.New("the webhook body is larger than 256 KB")

func opt(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func registerRoutines(mux *http.ServeMux, svc *workshop.Service, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/workshop/extensions/{extensionId}/routines", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "extensionId")
		if !ok {
			return
		}
		rs, runs, err := svc.Routines(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.RoutineList{Routines: make([]apispec.RoutineInfo, 0, len(rs)), Runs: make([]apispec.RoutineRunInfo, 0, len(runs))}
		for _, rv := range rs {
			ri := apispec.RoutineInfo{Name: rv.Name, Trigger: apispec.RoutineInfoTrigger(rv.Trigger.Kind()), Cron: opt(rv.Trigger.Cron),
				Timezone: opt(rv.Trigger.Timezone), Action: opt(rv.Trigger.Action), Tool: opt(rv.Tool), Agent: opt(rv.Agent), Output: opt(rv.Output)}
			if rv.Trigger.Message != nil {
				ri.Channel = opt(rv.Trigger.Message.Channel)
			}
			if rv.State != nil {
				ri.NextRunAt, ri.LastRunAt, ri.HasWebhook = rv.State.NextRunAt, rv.State.LastRunAt, len(rv.State.WebhookTokenHash) > 0
			}
			out.Routines = append(out.Routines, ri)
		}
		for _, run := range runs {
			out.Runs = append(out.Runs, apispec.RoutineRunInfo{Id: run.ID, Routine: run.Routine, Trigger: apispec.RoutineRunInfoTrigger(run.Trigger),
				State: apispec.RoutineRunInfoState(run.State), Error: opt(run.Error), CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt})
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/workshop/extensions/{extensionId}/routines/{routine}/webhook", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "extensionId")
		if !ok {
			return
		}
		path, err := svc.CreateWebhook(r.Context(), httpx.Actor(r), id, r.PathValue("routine"))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.RoutineWebhook{Path: path})
	}))

	// The webhook itself: no login; every failure is the same 404.
	mux.HandleFunc("POST /api/v1/hooks/ext/{workspaceId}/{routineId}/{token}", func(w http.ResponseWriter, r *http.Request) {
		ws, err1 := ids.Parse(r.PathValue("workspaceId"))
		st, err2 := ids.Parse(r.PathValue("routineId"))
		if err1 != nil || err2 != nil {
			httpx.Error(w, r, workshop.ErrBadWebhook)
			return
		}
		// On a customer's subdomain only that customer's webhooks.
		if t, ok := tenant.From(r.Context()); ok && t.Workspace != ws {
			httpx.Error(w, r, workshop.ErrBadWebhook)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, workshop.MaxWebhookBody+1))
		if err != nil {
			httpx.Error(w, r, workshop.ErrBadWebhook)
			return
		}
		if len(body) > workshop.MaxWebhookBody {
			httpx.Error(w, r, errTooLarge)
			return
		}
		err = svc.ReceiveWebhook(db.WithWorkspace(r.Context(), ws), ws, st, r.PathValue("token"), body)
		if err != nil {
			if !errors.Is(err, workshop.ErrBadWebhook) && !errors.Is(err, workshop.ErrNotLive) && !errors.Is(err, db.ErrNotFound) {
				httpx.Error(w, r, err)
				return
			}
			httpx.Error(w, r, workshop.ErrBadWebhook)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
