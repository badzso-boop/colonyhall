// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes run errors over REST.
package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(runs.ErrInvalidPrice, runs.ErrInvalidRate, runs.ErrInvalidGroup)
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
			HostID:              httpx.Deref(in.HostId, ids.Nil),
			Phase:               string(in.Phase),
			Client:              in.Client,
			Model:               in.Model,
			SessionID:           in.SessionId,
			ChannelID:           httpx.Deref(in.ChannelId, ids.Nil),
			ThreadRootID:        httpx.Deref(in.ThreadRootId, ids.Nil),
			Items:               in.Items,
			InputTokens:         in.InputTokens,
			OutputTokens:        in.OutputTokens,
			CacheReadTokens:     in.CacheReadTokens,
			CacheWriteTokens:    in.CacheWriteTokens,
			ContextTokens:       httpx.Deref(in.ContextTokens, 0),
			InstructionsVersion: httpx.Deref(in.InstructionsVersion, 0),
			DurationMS:          in.DurationMs,
			Status:              string(in.Status),
			ErrorID:             httpx.Deref(in.ErrorId, ids.Nil),
			WebReads:            webReads,
			Steps:               steps,
			ReplyMessageID:      httpx.Deref(in.ReplyMessageId, ids.Nil),
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
		f, ok := runFilter(w, r)
		if !ok {
			return
		}
		// Round 51b: the operator's break-glass access sees that runs
		// happened (agent, machine, time, tokens, cost), never where: no
		// conversation filter, and no conversation, message or session ids
		// (a DM's would tell who talks to which agent).
		breakGlass := httpx.Actor(r).BreakGlass != nil
		if breakGlass {
			f.ChannelID = ids.Nil
		}
		list, next, err := svc.SearchRuns(r.Context(), httpx.Actor(r), f)
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
				EstimatedUsd:     item.EstimatedUSD,
				Billing:          httpx.Ptr(apispec.RunBilling(item.Billing)),
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
			if item.InstructionsVersion > 0 {
				v := item.InstructionsVersion
				ar.InstructionsVersion = &v
			}
			if item.ReplyMessageID != ids.Nil {
				ar.ReplyMessageId = &item.ReplyMessageID
			}
			if breakGlass {
				ar.ChannelId, ar.ThreadRootId, ar.ReplyMessageId, ar.SessionId = nil, nil, nil, ""
			}
			// Round 51b: nor what it read or did (it may have been a DM).
			if len(item.WebReads) > 0 && !breakGlass {
				wr := item.WebReads
				ar.WebReads = &wr
			}
			if len(item.Steps) > 0 && !breakGlass {
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
		out := apispec.RunList{Runs: items}
		if next != nil {
			c := next.CreatedAt.Format(time.RFC3339Nano) + "_" + next.ID.String()
			out.NextCursor = &c
		}
		httpx.JSON(w, http.StatusOK, out)
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
		owner := ids.Nil
		if raw := r.URL.Query().Get("owner_id"); raw != "" {
			id, err := ids.Parse(raw)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "owner_id must be a UUID"})
				return
			}
			owner = id
		}
		usage, err := svc.UsageSummary(r.Context(), httpx.Actor(r), days, r.URL.Query().Get("group_by"), owner)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		items := make([]apispec.DayUsage, 0, len(usage.Days))
		for _, u := range usage.Days {
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
				EstimatedUsd:     u.EstimatedUSD,
			})
		}
		groups := make([]apispec.UsageGroup, 0, len(usage.Groups))
		for _, g := range usage.Groups {
			groups = append(groups, apispec.UsageGroup{Key: g.Key, Day: g.Day, InputTokens: g.InputTokens, OutputTokens: g.OutputTokens,
				CacheReadTokens: g.CacheReadTokens, CacheWriteTokens: g.CacheWriteTokens, TotalTokens: g.TotalTokens,
				RunCount: g.RunCount, ErrorCount: g.ErrorCount, EstimatedUsd: g.EstimatedUSD})
		}
		t := usage.Totals
		httpx.JSON(w, http.StatusOK, apispec.UsageResponse{Usage: items, Groups: &groups, Totals: &apispec.UsageTotals{
			InputTokens: t.InputTokens, OutputTokens: t.OutputTokens, CacheReadTokens: t.CacheReadTokens, CacheWriteTokens: t.CacheWriteTokens,
			TotalTokens: t.TotalTokens, RunCount: t.RunCount, ErrorCount: t.ErrorCount, AvgDurationMs: t.AvgDurationMS,
			SubscriptionUsd: t.SubscriptionUSD, ApiUsd: t.APIUSD, UnpricedRuns: t.UnpricedRuns,
		}})
	}))

	mux.Handle("GET /api/v1/settings/prices", wrap(func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.Prices(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.PriceList{Prices: make([]apispec.ModelPrice, 0, len(list))}
		for _, p := range list {
			out.Prices = append(out.Prices, priceToAPI(p))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	mux.Handle("POST /api/v1/settings/prices", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.AddPriceRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		p, err := svc.AddPrice(r.Context(), httpx.Actor(r), runs.Price{Model: in.Model, InputPerMTok: in.InputPerMtok, OutputPerMTok: in.OutputPerMtok,
			CacheReadPerMTok: in.CacheReadPerMtok, CacheWritePerMTok: in.CacheWritePerMtok, ValidFrom: httpx.Deref(in.ValidFrom, time.Time{})})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, priceToAPI(p))
	}))
	mux.Handle("GET /api/v1/settings/currency", wrap(func(w http.ResponseWriter, r *http.Request) {
		c, ok, err := svc.Currency(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.CurrencySettings{}
		if ok {
			out.UsdHuf, out.UpdatedAt = &c.USDHUF, &c.UpdatedAt
		}
		httpx.JSON(w, http.StatusOK, out)
	}))
	mux.Handle("PUT /api/v1/settings/currency", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.SetCurrencyRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		c, err := svc.SetCurrency(r.Context(), httpx.Actor(r), in.UsdHuf)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.CurrencySettings{UsdHuf: &c.USDHUF, UpdatedAt: &c.UpdatedAt})
	}))
}

func priceToAPI(p runs.Price) apispec.ModelPrice {
	return apispec.ModelPrice{Id: p.ID, Model: p.Model, InputPerMtok: p.InputPerMTok, OutputPerMtok: p.OutputPerMTok,
		CacheReadPerMtok: p.CacheReadPerMTok, CacheWritePerMtok: p.CacheWritePerMTok, ValidFrom: p.ValidFrom, CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt}
}

// runFilter reads the query parameters of GET /runs.
func runFilter(w http.ResponseWriter, r *http.Request) (runs.RunFilter, bool) {
	q := r.URL.Query()
	var f runs.RunFilter
	bad := func(detail string) (runs.RunFilter, bool) {
		httpx.Error(w, r, &httpx.BadRequest{Detail: detail})
		return f, false
	}
	for name, dst := range map[string]*ids.ID{"agent_id": &f.AgentID, "host_id": &f.HostID, "channel_id": &f.ChannelID, "owner_id": &f.OwnerID} {
		if raw := q.Get(name); raw != "" {
			id, err := ids.Parse(raw)
			if err != nil {
				return bad(name + " must be a UUID")
			}
			*dst = id
		}
	}
	f.Status = q.Get("status")
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if raw := q.Get(name); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return bad(name + " must be an RFC 3339 time")
			}
			*dst = &t
		}
	}
	if raw := q.Get("cursor"); raw != "" {
		at, id, ok := strings.Cut(raw, "_")
		t, err1 := time.Parse(time.RFC3339Nano, at)
		parsed, err2 := ids.Parse(id)
		if !ok || err1 != nil || err2 != nil {
			return bad("invalid cursor")
		}
		f.Before = &runs.RunCursor{CreatedAt: t, ID: parsed}
	}
	if raw := q.Get("limit"); raw != "" {
		l, err := strconv.Atoi(raw)
		if err != nil || l < 1 {
			return bad("limit must be a positive integer")
		}
		f.Limit = l
	}
	return f, true
}
