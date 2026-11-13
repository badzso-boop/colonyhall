package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/decision"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/platform/ids"
)

// BackfillReport is what `colonyhall rag-backfill` prints.
type BackfillReport struct {
	Events     int64 // events the indexer read
	Sources    int64 // of them, events of an indexed kind
	Workspaces int
	Decisions  int
	Embedded   int
	Pending    int
}

var indexedEvents = map[string]bool{
	conversation.EventMessageCreated: true, conversation.EventMessageRetracted: true,
	wiki.EventCreated: true, wiki.EventAppended: true, wiki.EventEdited: true, wiki.EventDeleted: true, wiki.EventRestored: true,
	decision.EventRecorded: true, "compaction.done": true,
}

func (a *App) searchConsumer() (*eventlog.Consumer, error) {
	for _, c := range a.Consumers {
		if c.Name == search.ConsumerName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("server: no search consumer")
}

// scanEvents reads the whole log: how many events, how many of an indexed
// kind, and the workspaces in it.
func (a *App) scanEvents(ctx context.Context) (n, sources int64, workspaces []ids.ID, err error) {
	seen := map[ids.ID]bool{}
	var after int64
	for {
		evs, err := a.Events.Since(ctx, after, 1000)
		if err != nil {
			return 0, 0, nil, err
		}
		if len(evs) == 0 {
			return n, sources, workspaces, nil
		}
		for _, e := range evs {
			n++
			if indexedEvents[e.Type] {
				sources++
			}
			if e.WorkspaceID != ids.Nil && !seen[e.WorkspaceID] {
				seen[e.WorkspaceID] = true
				workspaces = append(workspaces, e.WorkspaceID)
			}
			after = e.Seq
		}
	}
}

// SearchBackfill (re)builds the index from the event log: with restart
// the indexer reads the log from the start (idempotent: unchanged chunks
// keep their embeddings), then every decision is indexed (decisions older
// than decision.recorded have no event). With embed the chunks are also
// embedded here, not only by the running server's worker. dryRun only
// counts. progress gets a line now and then.
func (a *App) SearchBackfill(ctx context.Context, restart, embed, dryRun bool, progress func(string)) (BackfillReport, error) {
	var rep BackfillReport
	n, sources, workspaces, err := a.scanEvents(ctx)
	if err != nil {
		return rep, err
	}
	rep.Events, rep.Sources, rep.Workspaces = n, sources, len(workspaces)
	if dryRun {
		for _, ws := range workspaces {
			ds, err := a.Indexer.Decisions.AllForIndex(ctx, ws)
			if err != nil {
				return rep, err
			}
			rep.Decisions += len(ds)
		}
		return rep, nil
	}
	c, err := a.searchConsumer()
	if err != nil {
		return rep, err
	}
	if restart {
		if err := a.Events.SetOffset(ctx, c.Name, 0); err != nil {
			return rep, err
		}
	}
	done := make(chan error, 1)
	go func() { done <- c.Drain(ctx) }()
	head, _ := a.Events.Head(ctx)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
wait:
	for {
		select {
		case err := <-done:
			if err != nil {
				return rep, err
			}
			break wait
		case <-tick.C:
			off, _ := a.Events.Offset(ctx, c.Name)
			progress(fmt.Sprintf("indexing: event %d of %d", off, head))
		}
	}
	for _, ws := range workspaces {
		k, err := a.Indexer.BackfillDecisions(ctx, ws)
		if err != nil {
			return rep, err
		}
		rep.Decisions += k
	}
	if embed {
		for {
			k, err := a.Search.EmbedPending(ctx)
			if err != nil {
				return rep, err
			}
			if k == 0 {
				break
			}
			rep.Embedded += k
			if rep.Embedded%200 < k {
				progress(fmt.Sprintf("embedding: %d chunks", rep.Embedded))
			}
		}
	}
	for _, ws := range workspaces {
		st, err := a.Search.Status(ctx, ws)
		if err != nil {
			return rep, err
		}
		rep.Pending += st.Index.Pending
	}
	return rep, nil
}

// EvalSeed loads a synthetic set's seed into this (empty, dev-seeded)
// server and returns the label → reference map. The fake agent is a member
// of every channel that is not private.
func (a *App) EvalSeed(ctx context.Context, seed *search.EvalSeed) (map[string]string, error) {
	admin, err := a.Identity.Authenticate(ctx, DevAdminToken)
	if err != nil {
		return nil, err
	}
	agent, err := a.Identity.Authenticate(ctx, DevAgentToken)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	channels := map[string]ids.ID{}
	for _, c := range seed.Channels {
		kind := conversation.KindPublic
		if c.Private {
			kind = conversation.KindPrivate
		}
		ch, err := a.Conversation.CreateChannel(ctx, admin, c.Name, "", kind)
		if err != nil {
			return nil, fmt.Errorf("seed channel %s: %w", c.Name, err)
		}
		channels[c.Name] = ch.ID
		if !c.Private {
			if err := a.Conversation.AddMember(ctx, admin, ch.ID, agent.ID, true); err != nil {
				return nil, err
			}
		}
	}
	msgIDs := map[string]ids.ID{}
	for _, m := range seed.Messages {
		ch, ok := channels[m.Channel]
		if !ok {
			return nil, fmt.Errorf("seed message %s: unknown channel %s", m.Label, m.Channel)
		}
		in := conversation.SendInput{ChannelID: ch, Body: m.Body}
		if m.Thread != "" {
			in.ThreadRootID = msgIDs[m.Thread]
		}
		res, err := a.Conversation.SendMessage(ctx, admin, in)
		if err != nil {
			return nil, fmt.Errorf("seed message %s: %w", m.Label, err)
		}
		if res.Message == nil {
			return nil, fmt.Errorf("seed message %s: became a reaction", m.Label)
		}
		msgIDs[m.Label] = res.Message.ID
		refs[m.Label] = "message:" + res.Message.ID.String()
	}
	for _, w := range seed.Wiki {
		p, err := a.Wiki.Create(ctx, admin, wiki.CreateInput{Title: w.Title, Body: w.Body})
		if err != nil {
			return nil, fmt.Errorf("seed wiki %s: %w", w.Label, err)
		}
		refs[w.Label] = "wiki:" + p.Slug
	}
	for _, d := range seed.Decisions {
		ch := channels[d.Channel]
		got, err := a.Decisions.AddDecision(ctx, admin, decision.AddDecisionInput{Text: d.Text, Rationale: d.Rationale, ChannelID: &ch})
		if err != nil {
			return nil, fmt.Errorf("seed decision %s: %w", d.Label, err)
		}
		refs[d.Label] = "decision:" + got.FormattedNumber()
	}
	if err := a.Drain(ctx); err != nil {
		return nil, err
	}
	for {
		k, err := a.Search.EmbedPending(ctx)
		if err != nil {
			return nil, err
		}
		if k == 0 {
			break
		}
	}
	return refs, nil
}

// EvalConfigs are the configurations an evaluation compares.
var EvalConfigs = []struct {
	Name   string
	Mode   search.Mode
	Rerank bool
}{
	{"keyword", search.ModeKeyword, false},
	{"semantic", search.ModeSemantic, false},
	{"hybrid", search.ModeHybrid, false},
	{"hybrid+rerank", search.ModeHybrid, true},
}

// RunEval runs every question in every configuration as actor; refs maps
// seed labels to references (nil for a real set). Searches are not logged.
func (a *App) RunEval(ctx context.Context, set search.EvalSet, actor identity.Actor, refs map[string]string) ([]search.EvalMetrics, error) {
	var out []search.EvalMetrics
	for _, cfg := range EvalConfigs {
		run := search.EvalRun{Config: cfg.Name}
		for _, q := range set.Questions {
			relevant := make([]string, 0, len(q.Relevant))
			for _, r := range q.Relevant {
				if ref, ok := refs[r]; ok {
					r = ref
				} else if refs != nil && !strings.Contains(r, ":") {
					return nil, fmt.Errorf("question %s: unknown label %q", q.ID, r)
				}
				relevant = append(relevant, r)
			}
			start := time.Now()
			res, err := a.Search.Search(ctx, actor, search.Query{Text: q.Query, Mode: cfg.Mode, Limit: 10,
				NoRerank: !cfg.Rerank, NoLog: true})
			if err != nil {
				return nil, fmt.Errorf("question %s: %w", q.ID, err)
			}
			ranked := make([]string, len(res.Hits))
			for i, h := range res.Hits {
				ranked[i] = search.HitRef(h.Chunk)
			}
			run.Add(q.ID, ranked, relevant, time.Since(start))
		}
		out = append(out, run.Metrics())
	}
	return out, nil
}
