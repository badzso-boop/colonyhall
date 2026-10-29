// Package httpapi exposes run errors over REST.
package httpapi

import (
	"net/http"
	"strconv"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(runs.ErrForbidden)
	httpx.RegisterValidation(runs.ErrEmpty)
	httpx.RegisterValidation(runs.ErrInvalidRun)
	httpx.RegisterValidation(runs.ErrInvalidDays)
}

// Register mounts the runs routes on mux.
func Register(mux *http.ServeMux, svc *runs.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("POST /api/v1/agent/errors", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.RunErrorRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		e, err := svc.ReportError(r.Context(), httpx.Actor(r), runs.ReportInput{ChannelID: in.ChannelId,
			ThreadRootID: httpx.Deref(in.ThreadRootId, ids.Nil), Summary: in.Summary, Detail: httpx.Deref(in.Detail, "")})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.RunErrorRef{Id: e.ID})
	}))

	mux.Handle("GET /api/v1/errors/{errorId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, err := ids.Parse(r.PathValue("errorId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "errorId must be a UUID"})
			return
		}
		e, err := svc.GetError(r.Context(), httpx.Actor(r), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.RunError{Id: e.ID, AgentId: e.AgentID, ChannelId: e.ChannelID, Summary: e.Summary, Detail: e.Detail, CreatedAt: e.CreatedAt}
		if e.ThreadRootID != ids.Nil {
			out.ThreadRootId = &e.ThreadRootID
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/agent/runs", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.ReportRunRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var webReads []string
		if in.WebReads != nil {
			webReads = *in.WebReads
		}
		var steps []runs.Step
		if in.Steps != nil {
			for _, st := range *in.Steps {
				var p string
				if st.Path != nil {
					p = *st.Path
				}
				var dur int64
				if st.DurationMs != nil {
					dur = *st.DurationMs
				}
				steps = append(steps, runs.Step{
					Kind:       string(st.Kind),
					Label:      st.Label,
					Path:       p,
					State:      string(st.State),
					StartedAt:  st.StartedAt,
					DurationMs: dur,
				})
			}
		}
		run, err := svc.ReportRun(r.Context(), httpx.Actor(r), runs.ReportRunInput{
			HostID:           httpx.Deref(in.HostId, ids.Nil),
			Phase:            string(in.Phase),
			Client:           in.Client,
			Model:            in.Model,
			SessionID:        in.SessionId,
			ChannelID:        httpx.Deref(in.ChannelId, ids.Nil),
			ThreadRootID:     httpx.Deref(in.ThreadRootId, ids.Nil),
			Items:            in.Items,
			InputTokens:      in.InputTokens,
			OutputTokens:     in.OutputTokens,
			CacheReadTokens:  in.CacheReadTokens,
			CacheWriteTokens: in.CacheWriteTokens,
			ContextTokens:    httpx.Deref(in.ContextTokens, 0),
			DurationMS:       in.DurationMs,
			Status:           string(in.Status),
			ErrorID:          httpx.Deref(in.ErrorId, ids.Nil),
			WebReads:         webReads,
			Steps:            steps,
			ReplyMessageID:   httpx.Deref(in.ReplyMessageId, ids.Nil),
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, apispec.RunRef{Id: run.ID})
	}))

	mux.Handle("GET /api/v1/agent/budget", wrap(func(w http.ResponseWriter, r *http.Request) {
		b, err := svc.AgentBudget(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.AgentBudget{
			Limit:     b.Limit,
			UsedToday: b.UsedToday,
			Remaining: b.Remaining,
			ResetsAt:  b.ResetsAt,
		})
	}))

	mux.Handle("GET /api/v1/runs", wrap(func(w http.ResponseWriter, r *http.Request) {
		var agentID ids.ID
		if raw := r.URL.Query().Get("agent_id"); raw != "" {
			var err error
			agentID, err = ids.Parse(raw)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "agent_id must be a UUID"})
				return
			}
		}
		limit := 50
		if raw := r.URL.Query().Get("limit"); raw != "" {
			l, err := strconv.Atoi(raw)
			if err != nil || l < 1 {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "limit must be a positive integer"})
				return
			}
			limit = l
		}
		list, err := svc.ListRuns(r.Context(), httpx.Actor(r), agentID, limit)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		items := make([]apispec.Run, 0, len(list))
		for _, item := range list {
			ar := apispec.Run{
				Id:               item.ID,
				WorkspaceId:      item.WorkspaceID,
				AgentId:          item.AgentID,
				Phase:            apispec.RunPhase(item.Phase),
				Client:           item.Client,
				Model:            item.Model,
				SessionId:        item.SessionID,
				Items:            item.Items,
				InputTokens:      item.InputTokens,
				OutputTokens:     item.OutputTokens,
				CacheReadTokens:  item.CacheReadTokens,
				CacheWriteTokens: item.CacheWriteTokens,
				DurationMs:       item.DurationMS,
				Status:           apispec.RunStatus(item.Status),
				CreatedAt:        item.CreatedAt,
			}
			if item.HostID != ids.Nil {
				ar.HostId = &item.HostID
			}
			if item.ChannelID != ids.Nil {
				ar.ChannelId = &item.ChannelID
			}
			if item.ThreadRootID != ids.Nil {
				ar.ThreadRootId = &item.ThreadRootID
			}
			if item.ErrorID != ids.Nil {
				ar.ErrorId = &item.ErrorID
			}
			if item.ContextTokens > 0 {
				ct := item.ContextTokens
				ar.ContextTokens = &ct
			}
			if item.ReplyMessageID != ids.Nil {
				ar.ReplyMessageId = &item.ReplyMessageID
			}
			if len(item.WebReads) > 0 {
				wr := item.WebReads
				ar.WebReads = &wr
			}
			if len(item.Steps) > 0 {
				stList := make([]apispec.AgentStep, len(item.Steps))
				for i, s := range item.Steps {
					as := apispec.AgentStep{
						Kind:      apispec.AgentStepKind(s.Kind),
						Label:     s.Label,
						State:     apispec.AgentStepState(s.State),
						StartedAt: s.StartedAt,
					}
					if s.Path != "" {
						p := s.Path
						as.Path = &p
					}
					if s.DurationMs > 0 {
						dms := s.DurationMs
						as.DurationMs = &dms
					}
					stList[i] = as
				}
				ar.Steps = &stList
			}
			items = append(items, ar)
		}
		httpx.JSON(w, http.StatusOK, apispec.RunList{Runs: items})
	}))

	mux.Handle("GET /api/v1/usage", wrap(func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if raw := r.URL.Query().Get("days"); raw != "" {
			d, err := strconv.Atoi(raw)
			if err != nil || d < 1 || d > 90 {
				httpx.Error(w, r, runs.ErrInvalidDays)
				return
			}
			days = d
		}
		usage, err := svc.Usage(r.Context(), httpx.Actor(r), days)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		items := make([]apispec.DayUsage, 0, len(usage))
		for _, u := range usage {
			items = append(items, apispec.DayUsage{
				AgentId:          u.AgentID,
				Day:              u.Day,
				InputTokens:      u.InputTokens,
				OutputTokens:     u.OutputTokens,
				CacheReadTokens:  u.CacheReadTokens,
				CacheWriteTokens: u.CacheWriteTokens,
				TotalTokens:      u.TotalTokens,
				RunCount:         u.RunCount,
				ErrorCount:       u.ErrorCount,
			})
		}
		httpx.JSON(w, http.StatusOK, apispec.UsageResponse{Usage: items})
	}))
}
