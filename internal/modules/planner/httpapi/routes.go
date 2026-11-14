// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes the planner over REST, for its human owner only.
package httpapi

import (
	"net/http"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/planner"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(planner.ErrHumansOnly, planner.ErrNotAllowed)
	httpx.RegisterValidation(planner.ErrTitle, planner.ErrDescription, planner.ErrStatus, planner.ErrCalendarName,
		planner.ErrCalendarColor, planner.ErrCalendarURL, planner.ErrBadRange, planner.ErrOwnerRequired)
	httpx.RegisterStatus(http.StatusNotFound, planner.ErrNotFound)
}

// TaskToAPI converts a task.
func TaskToAPI(t planner.Task) apispec.PlannerTask {
	return apispec.PlannerTask{Id: t.ID, Title: t.Title, Description: t.Description, Status: apispec.PlannerTaskStatus(t.Status),
		Position: t.Position, StartedAt: t.StartedAt, DoneAt: t.DoneAt, StatusChangedAt: t.StatusChangedAt,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt}
}

// CalendarToAPI converts a calendar; the URL is masked.
func CalendarToAPI(c planner.Calendar) apispec.PlannerCalendar {
	return apispec.PlannerCalendar{Id: c.ID, Name: c.Name, UrlMasked: planner.MaskURL(c.URL), Colour: c.Colour,
		LastFetchedAt: c.LastFetchedAt, LastError: c.LastError, CreatedAt: c.CreatedAt}
}

func accessToAPI(a planner.Access) apispec.PlannerAccess {
	ta, ca := a.TaskAgentIDs, a.CalAgentIDs
	if ta == nil {
		ta = []ids.ID{}
	}
	if ca == nil {
		ca = []ids.ID{}
	}
	return apispec.PlannerAccess{TaskAll: a.TaskAll, TaskAgentIds: ta, CalAgentIds: ca}
}

func pathID(w http.ResponseWriter, r *http.Request) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue("id"))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid id"})
		return ids.Nil, false
	}
	return id, true
}

// Register mounts the planner endpoints.
func Register(mux *http.ServeMux, svc *planner.Service, auth identity.RequestAuthenticator) {
	// Agents are refused before anything is read: the planner is the
	// owner's, and agents only read it through MCP.
	wrap := func(h http.HandlerFunc) http.Handler {
		return httpx.RequireAuth(auth, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if httpx.Actor(r).Kind != identity.KindHuman {
				httpx.Error(w, r, planner.ErrHumansOnly)
				return
			}
			h(w, r)
		}))
	}

	mux.Handle("GET /api/v1/planner/tasks", wrap(func(w http.ResponseWriter, r *http.Request) {
		tasks, err := svc.Tasks(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.PlannerTaskList{Tasks: make([]apispec.PlannerTask, 0, len(tasks))}
		for _, t := range tasks {
			out.Tasks = append(out.Tasks, TaskToAPI(t))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	mux.Handle("POST /api/v1/planner/tasks", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreatePlannerTaskRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		t, err := svc.CreateTask(r.Context(), httpx.Actor(r), in.Title, httpx.Deref(in.Description, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, TaskToAPI(t))
	}))
	mux.Handle("PATCH /api/v1/planner/tasks/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var in apispec.UpdatePlannerTaskRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		p := planner.TaskPatch{Title: in.Title, Description: in.Description, Move: httpx.Deref(in.Move, false),
			AfterID: httpx.Deref(in.AfterId, ids.Nil), BeforeID: httpx.Deref(in.BeforeId, ids.Nil)}
		if in.Status != nil {
			st := string(*in.Status)
			p.Status = &st
		}
		t, err := svc.UpdateTask(r.Context(), httpx.Actor(r), id, p)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, TaskToAPI(t))
	}))
	mux.Handle("DELETE /api/v1/planner/tasks/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteTask(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/planner/access", wrap(func(w http.ResponseWriter, r *http.Request) {
		a, err := svc.GetAccess(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, accessToAPI(a))
	}))
	mux.Handle("PUT /api/v1/planner/access", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.PlannerAccess
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		a, err := svc.SetAccess(r.Context(), httpx.Actor(r), in.TaskAll, in.TaskAgentIds, in.CalAgentIds)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, accessToAPI(a))
	}))

	mux.Handle("GET /api/v1/planner/calendars", wrap(func(w http.ResponseWriter, r *http.Request) {
		cals, err := svc.Calendars(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.PlannerCalendarList{Calendars: make([]apispec.PlannerCalendar, 0, len(cals))}
		for _, c := range cals {
			out.Calendars = append(out.Calendars, CalendarToAPI(c))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	mux.Handle("POST /api/v1/planner/calendars", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.AddPlannerCalendarRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.AddCalendar(r.Context(), httpx.Actor(r), in.Name, in.Url, httpx.Deref(in.Colour, ""))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, CalendarToAPI(c))
	}))
	mux.Handle("DELETE /api/v1/planner/calendars/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := svc.DeleteCalendar(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("POST /api/v1/planner/calendars/{id}/refresh", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		c, err := svc.RefreshCalendar(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, CalendarToAPI(c))
	}))
	mux.Handle("GET /api/v1/planner/events", wrap(func(w http.ResponseWriter, r *http.Request) {
		from, err1 := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, err2 := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err1 != nil || err2 != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "from and to must be RFC 3339 times"})
			return
		}
		evs, err := svc.MyEvents(r.Context(), httpx.Actor(r), from, to)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.PlannerEventList{Events: make([]apispec.PlannerEvent, 0, len(evs))}
		for _, e := range evs {
			out.Events = append(out.Events, apispec.PlannerEvent{CalendarId: e.CalendarID, Title: e.Title, Start: e.Start,
				End: e.End, AllDay: e.AllDay, Location: e.Location, Description: e.Description})
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
}
