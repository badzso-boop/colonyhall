package server

import (
	"context"
	"net/http"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/eventlog"
	"colonyhall/internal/gateway"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	idhttp "colonyhall/internal/modules/identity/httpapi"
	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/platform/clock"
)

// guardOnlineWindow is how long a guard counts as online after it was seen.
const guardOnlineWindow = 90 * time.Second

// statusDeps is what the system status reads.
type statusDeps struct {
	Identity    *identity.Service
	Hosts       *hosts.Service
	Schedules   schedule.Store
	Compactions compaction.Store
	Events      eventlog.Store
	Consumers   []*eventlog.Consumer
	Gateway     *gateway.Gateway
	HostGateway *gateway.HostGateway
	Ready       func(context.Context) error
	Clock       clock.Clock
	StartedAt   time.Time
	GiteaOn     bool
}

func registerStatus(mux *http.ServeMux, d statusDeps) {
	mux.Handle("GET /api/v1/admin/status", httpx.RequireAuth(d.Identity, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		by := httpx.Actor(r)
		ws, err := d.Identity.Workspace(r.Context(), by)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := idhttp.WorkspaceStatus(ws)
		if by.IsAdmin() {
			sys, err := d.system(r.Context(), by)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			out.System = &sys
		}
		httpx.JSON(w, http.StatusOK, out)
	})))
}

func (d statusDeps) system(ctx context.Context, by identity.Actor) (apispec.SystemStatus, error) {
	now := d.Clock.Now()
	out := apispec.SystemStatus{Version: Version, UptimeSeconds: int64(now.Sub(d.StartedAt).Seconds()), GiteaConfigured: d.GiteaOn,
		Consumers: []apispec.StatusConsumer{}, Guards: []apispec.StatusGuard{}}

	begin := time.Now()
	out.Database.Ok = d.Ready(ctx) == nil
	out.Database.LatencyMs = float64(time.Since(begin).Microseconds()) / 1000

	head, err := d.Events.Head(ctx)
	if err != nil {
		return out, err
	}
	out.EventHead = head
	for _, c := range d.Consumers {
		offset, err := d.Events.Offset(ctx, c.Name)
		if err != nil {
			return out, err
		}
		row := apispec.StatusConsumer{Name: c.Name, Offset: offset, Lag: max(head-offset, 0)}
		if row.Lag > 0 {
			oldest, err := d.Events.Since(ctx, offset, 1)
			if err != nil {
				return out, err
			}
			if len(oldest) == 1 {
				row.LagSeconds = max(int64(now.Sub(oldest[0].At).Seconds()), 0)
			}
		}
		out.Consumers = append(out.Consumers, row)
	}

	out.Connections.Human, out.Connections.Agent = d.Gateway.ConnectionCounts()
	out.Connections.Host = d.HostGateway.Connections()

	views, err := d.Hosts.ListHosts(ctx, by)
	if err != nil {
		return out, err
	}
	for _, h := range views {
		row := apispec.StatusGuard{Id: h.ID, Name: h.Name, LastSeenAt: h.LastSeenAt, GuardVersion: strPtr(h.Report.GuardVersion)}
		row.Online = h.LastSeenAt != nil && now.Sub(*h.LastSeenAt) <= guardOnlineWindow
		out.Guards = append(out.Guards, row)
	}

	list, err := d.Schedules.List(ctx, by.WorkspaceID)
	if err != nil {
		return out, err
	}
	since := now.Add(-24 * time.Hour)
	for _, sc := range list {
		if sc.State == schedule.StateActive && (out.Scheduler.NextRunAt == nil || sc.NextRunAt.Before(*out.Scheduler.NextRunAt)) {
			at := sc.NextRunAt
			out.Scheduler.NextRunAt = &at
		}
		if sc.LastError != "" && sc.LastRunAt != nil && sc.LastRunAt.After(since) {
			out.Scheduler.Failed24h++
		}
	}

	out.Compaction.Running, out.Compaction.Failed24h, err = d.Compactions.CompactionCounts(ctx, by.WorkspaceID, since)
	return out, err
}
