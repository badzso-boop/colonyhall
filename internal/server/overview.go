package server

import (
	"context"
	"errors"
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/gateway"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/lease"
	"colonyhall/internal/modules/notes"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func registerOverview(
	mux *http.ServeMux,
	idSvc *identity.Service,
	cvSvc *conversation.Service,
	hoSvc *hosts.Service,
	ruSvc *runs.Service,
	ntSvc *notes.Service,
	lsSvc *lease.Service,
	qSvc *question.Service,
	gw *gateway.Gateway,
	clk clock.Clock,
) {
	mux.Handle("GET /api/v1/agents/{id}/overview", httpx.RequireAuth(idSvc, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := httpx.Actor(r)
		if caller.ID == ids.Nil {
			httpx.Error(w, r, identity.ErrUnauthorized)
			return
		}
		if caller.Kind != identity.KindHuman {
			httpx.Error(w, r, identity.ErrForbidden)
			return
		}

		targetID, err := ids.Parse(r.PathValue("id"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "invalid id"})
			return
		}

		ctx := r.Context()
		targetActor, err := idSvc.ActorByID(ctx, targetID)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				httpx.Error(w, r, db.ErrNotFound)
				return
			}
			httpx.Error(w, r, err)
			return
		}
		if targetActor.WorkspaceID != caller.WorkspaceID {
			httpx.Error(w, r, db.ErrNotFound)
			return
		}

		actorSpec := toOverviewActorSpec(targetActor)

		// For humans: profile, role, their channels in common.
		if targetActor.Kind == identity.KindHuman {
			chans, err := cvSvc.ListChannels(ctx, caller)
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			var common []apispec.OverviewCommonChannel
			for _, ch := range chans {
				if ok, _ := cvSvc.CanRead(ctx, targetActor, ch.ID); ok {
					common = append(common, apispec.OverviewCommonChannel{
						Id:   ch.ID,
						Name: ch.Name,
						Kind: string(ch.Kind),
					})
				}
			}
			if common == nil {
				common = []apispec.OverviewCommonChannel{}
			}
			res := apispec.AgentOverview{
				Actor:          actorSpec,
				State:          targetActor.State,
				CommonChannels: &common,
			}
			httpx.JSON(w, http.StatusOK, res)
			return
		}

		// For agents:
		// 1. Activity (gateway)
		working := false
		if gw != nil {
			working = gw.IsWorking(targetActor.ID)
		}
		activity := &struct {
			Working bool `json:"working"`
		}{Working: working}

		// 2. Assignment
		var overviewAssignment *apispec.OverviewAssignment
		asgn, err := hoSvc.AssignmentByAgent(ctx, targetActor.ID)
		if err == nil {
			host, err := hoSvc.HostByID(ctx, asgn.HostID)
			hostName := ""
			if err == nil {
				hostName = host.Name
			}
			var wtBranch *string
			for _, wt := range host.Report.Worktrees {
				if wt.AgentID == targetActor.ID.String() && wt.Branch != "" {
					b := wt.Branch
					wtBranch = &b
					break
				}
			}
			grants, _ := hoSvc.ActiveGrants(ctx, asgn.HostID, targetActor.ID)
			fgrants := make([]apispec.FolderGrant, 0, len(grants))
			for _, g := range grants {
				fgrants = append(fgrants, apispec.FolderGrant{
					Id:        g.ID,
					Path:      g.Path,
					Mode:      apispec.FolderGrantMode(g.Mode),
					CreatedAt: g.CreatedAt,
				})
			}
			overviewAssignment = &apispec.OverviewAssignment{
				HostId:           asgn.HostID,
				HostName:         strPtr(hostName),
				Client:           asgn.Client,
				Model:            asgn.Model,
				WorkModel:        strPtr(asgn.WorkModel),
				Mode:             asgn.Mode,
				Sandbox:          asgn.Sandbox,
				RootDir:          asgn.RootDir,
				WorktreeRepo:     asgn.WorktreeRepo,
				WorktreeBranch:   wtBranch,
				PermissionMode:   asgn.PermissionMode,
				Approvals:        asgn.Approvals,
				MaxSessionTokens: asgn.MaxSessionTokens,
				DailyTokenLimit:  asgn.DailyTokenLimit,
				Grants:           fgrants,
			}
		}

		// 3. Runs (context per thread & usage)
		runsList, _ := ruSvc.ListRuns(ctx, caller, targetActor.ID, 200)
		lastRuns := make([]apispec.OverviewRun, 0)
		for i, r := range runsList {
			if i >= 10 {
				break
			}
			tokens := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
			var errID *openapi_types.UUID
			if r.ErrorID != ids.Nil {
				u := r.ErrorID
				errID = &u
			}
			lastRuns = append(lastRuns, apispec.OverviewRun{
				Id:      r.ID,
				Time:    r.CreatedAt,
				Phase:   r.Phase,
				Result:  r.Status,
				Tokens:  tokens,
				ErrorId: errID,
			})
		}

		seenThreads := make(map[ids.ID]bool)
		overviewThreads := make([]apispec.OverviewThread, 0)
		for _, r := range runsList {
			key := r.ThreadRootID
			if key == ids.Nil {
				key = r.ChannelID
			}
			if seenThreads[key] {
				continue
			}
			seenThreads[key] = true
			var chName *string
			if ch, err := cvSvc.ChannelByID(ctx, r.ChannelID); err == nil {
				chName = strPtr(ch.Name)
			}
			var trID *openapi_types.UUID
			if r.ThreadRootID != ids.Nil {
				u := r.ThreadRootID
				trID = &u
			}
			tokens := r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
			maxSess := 0
			if overviewAssignment != nil {
				maxSess = overviewAssignment.MaxSessionTokens
			}
			overviewThreads = append(overviewThreads, apispec.OverviewThread{
				ChannelId:        r.ChannelID,
				ChannelName:      chName,
				ThreadRootId:     trID,
				SessionId:        r.SessionID,
				LastRunTokens:    tokens,
				MaxSessionTokens: maxSess,
			})
		}

		todayTokens, _ := ruSvc.TokensToday(ctx, caller.WorkspaceID, targetActor.ID)
		dailyLimit := int64(0)
		if overviewAssignment != nil {
			dailyLimit = overviewAssignment.DailyTokenLimit
		}
		overviewUsage := &apispec.OverviewUsage{
			TodayTokens:     todayTokens,
			DailyTokenLimit: dailyLimit,
			LastRuns:        lastRuns,
		}

		// 4. Collaboration: notes, leases, questions
		agentNotes, _ := ntSvc.ListNotes(ctx, caller, targetActor.ID, nil)
		seenNoteThreads := make(map[ids.ID]bool)
		notesList := make([]apispec.AgentNote, 0)
		for _, n := range agentNotes {
			if seenNoteThreads[n.ThreadRootID] {
				continue
			}
			seenNoteThreads[n.ThreadRootID] = true
			notesList = append(notesList, apispec.AgentNote{
				Id:           n.ID,
				AgentId:      n.AgentID,
				ThreadRootId: n.ThreadRootID,
				Text:         n.Text,
				CreatedAt:    n.CreatedAt,
			})
		}

		activeLeases, _ := lsSvc.ListActive(ctx, caller)
		leasesList := make([]apispec.Lease, 0)
		for _, l := range activeLeases {
			if l.HolderID == targetActor.ID {
				var note *string
				if l.Note != "" {
					note = &l.Note
				}
				var relBy *openapi_types.UUID
				if l.ReleasedBy != nil {
					u := *l.ReleasedBy
					relBy = &u
				}
				leasesList = append(leasesList, apispec.Lease{
					Id:           l.ID,
					WorkspaceId:  l.WorkspaceID,
					Resource:     l.Resource,
					HolderId:     l.HolderID,
					HolderHandle: l.HolderHandle,
					Note:         note,
					AcquiredAt:   l.AcquiredAt,
					ExpiresAt:    l.ExpiresAt,
					ReleasedAt:   l.ReleasedAt,
					ReleasedBy:   relBy,
				})
			}
		}

		allQuestions, _ := qSvc.List(ctx, caller, nil)
		questionsList := make([]apispec.Question, 0)
		for _, q := range allQuestions {
			if (q.AskedBy == targetActor.ID || (q.ToActorID != nil && *q.ToActorID == targetActor.ID)) &&
				(q.State == "open" || q.State == "escalated") {
				questionsList = append(questionsList, toOverviewQuestionSpec(ctx, q, idSvc, cvSvc))
			}
		}

		overviewCollaboration := &apispec.OverviewCollaboration{
			Notes:         notesList,
			ActiveLeases:  leasesList,
			OpenQuestions: questionsList,
		}

		res := apispec.AgentOverview{
			Actor:         actorSpec,
			State:         targetActor.State,
			Activity:      activity,
			Assignment:    overviewAssignment,
			Threads:       &overviewThreads,
			Usage:         overviewUsage,
			Collaboration: overviewCollaboration,
		}
		httpx.JSON(w, http.StatusOK, res)
	})))
}

func toOverviewActorSpec(a identity.Actor) apispec.Actor {
	var desc, emoji, tz, status *string
	if a.Description != "" {
		desc = &a.Description
	}
	if a.AvatarEmoji != "" {
		emoji = &a.AvatarEmoji
	}
	if a.Timezone != "" {
		tz = &a.Timezone
	}
	if a.StatusText != "" {
		status = &a.StatusText
	}
	return apispec.Actor{
		Id:          a.ID,
		Kind:        apispec.ActorKind(a.Kind),
		Handle:      a.Handle,
		DisplayName: a.DisplayName,
		Role:        apispec.Role(a.Role),
		State:       a.State,
		CreatedAt:   a.CreatedAt,
		Description: desc,
		AvatarEmoji: emoji,
		Timezone:    tz,
		StatusText:  status,
	}
}

func toOverviewQuestionSpec(ctx context.Context, q question.Question, dir identity.Directory, conv conversation.Reader) apispec.Question {
	var askedByHandle string
	if a, err := dir.ActorByID(ctx, q.AskedBy); err == nil {
		askedByHandle = a.Handle
	}
	var toActorHandle *string
	if q.ToActorID != nil {
		if a, err := dir.ActorByID(ctx, *q.ToActorID); err == nil {
			h := a.Handle
			toActorHandle = &h
		}
	}
	var answeredByHandle *string
	if q.AnsweredBy != nil {
		if a, err := dir.ActorByID(ctx, *q.AnsweredBy); err == nil {
			h := a.Handle
			answeredByHandle = &h
		}
	}
	var channelName string
	if ch, err := conv.ChannelByID(ctx, q.ChannelID); err == nil {
		channelName = ch.Name
	}
	opts := q.Options
	if opts == nil {
		opts = []string{}
	}
	return apispec.Question{
		Id:               q.ID,
		Number:           q.Number,
		FormattedNumber:  q.FormattedNumber(),
		WorkspaceId:      q.WorkspaceID,
		AskedBy:          q.AskedBy,
		AskedByHandle:    askedByHandle,
		ToActorId:        q.ToActorID,
		ToActorHandle:    toActorHandle,
		ToHuman:          q.ToHuman,
		ChannelId:        q.ChannelID,
		ChannelName:      channelName,
		ThreadRootId:     q.ThreadRootID,
		Text:             q.Text,
		Options:          opts,
		DefaultOption:    q.DefaultOption,
		Deadline:         q.Deadline,
		State:            apispec.QuestionState(q.State),
		Answer:           q.Answer,
		AnsweredBy:       q.AnsweredBy,
		AnsweredByHandle: answeredByHandle,
		CreatedAt:        q.CreatedAt,
		AnsweredAt:       q.AnsweredAt,
		EscalatedAt:      q.EscalatedAt,
		WithdrawnAt:      q.WithdrawnAt,
		ExpiredAt:        q.ExpiredAt,
	}
}
