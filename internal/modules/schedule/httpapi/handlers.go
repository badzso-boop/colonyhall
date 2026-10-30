// Package httpapi exposes schedules over REST.
package httpapi

import (
	"net/http"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(schedule.ErrForbidden)
	httpx.RegisterValidation(schedule.ErrBadTime, schedule.ErrBadCron, schedule.ErrBadZone, schedule.ErrBadCatchUp, schedule.ErrEmptyBody)
}

// parseAt accepts RFC 3339 or a local "YYYY-MM-DD HH:MM" in zone.
func parseAt(s, zone string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	if zone == "" {
		zone = schedule.DefaultTimezone
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, schedule.ErrBadZone
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006.01.02 15:04", "2006.01.02. 15:04"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return &t, nil
		}
	}
	return nil, schedule.ErrBadTime
}

func toAPI(sc schedule.Schedule) apispec.Schedule {
	out := apispec.Schedule{Id: sc.ID, CreatedBy: sc.CreatedBy, ChannelId: sc.ChannelID, Body: sc.Body, Cron: sc.Cron,
		Timezone: sc.Timezone, CatchUp: apispec.ScheduleCatchUp(sc.CatchUp), State: apispec.ScheduleState(sc.State),
		NextRunAt: sc.NextRunAt, LastRunAt: sc.LastRunAt, LastError: sc.LastError, RunCount: sc.RunCount, CreatedAt: sc.CreatedAt}
	if sc.ThreadRootID != ids.Nil {
		out.ThreadRootId = &sc.ThreadRootID
	}
	if sc.LastMessageID != ids.Nil {
		out.LastMessageId = &sc.LastMessageID
	}
	return out
}

// Register mounts the schedule routes on mux.
func Register(mux *http.ServeMux, svc *schedule.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("POST /api/v1/schedules", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateScheduleRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		zone := httpx.Deref(in.Timezone, "")
		at, err := parseAt(httpx.Deref(in.At, ""), zone)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		catchUp := ""
		if in.CatchUp != nil {
			catchUp = string(*in.CatchUp)
		}
		sc, err := svc.Create(r.Context(), httpx.Actor(r), schedule.CreateInput{ChannelID: in.ChannelId,
			ThreadRootID: httpx.Deref(in.ThreadRootId, ids.Nil), Body: in.Body, At: at, Cron: httpx.Deref(in.Cron, ""),
			Timezone: zone, CatchUp: catchUp})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, toAPI(sc))
	}))

	mux.Handle("POST /api/v1/schedules/preview", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.SchedulePreviewRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		zone := httpx.Deref(in.Timezone, "")
		at, err := parseAt(httpx.Deref(in.At, ""), zone)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		runs, used, err := svc.Preview(r.Context(), httpx.Actor(r), schedule.CreateInput{At: at, Cron: httpx.Deref(in.Cron, ""), Timezone: zone})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.SchedulePreview{Cron: used.Cron, Timezone: used.Timezone, NextRuns: runs})
	}))

	mux.Handle("GET /api/v1/schedules", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.List(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.ScheduleList{Schedules: make([]apispec.Schedule, 0, len(list))}
		for _, sc := range list {
			out.Schedules = append(out.Schedules, toAPI(sc))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("DELETE /api/v1/schedules/{scheduleId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("scheduleId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "scheduleId must be a UUID"})
			return
		}
		if err := svc.Cancel(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
