package storetest

import (
	"context"
	"math"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// unitVec is a 384-dimensional vector pointing mostly along axis i.
func unitVec(i int, tilt float32) []float32 {
	v := make([]float32, 384)
	v[i] = 1
	v[(i+1)%384] = tilt
	n := float32(math.Sqrt(float64(1 + tilt*tilt)))
	v[i] /= n
	v[(i+1)%384] /= n
	return v
}

func testSearch(t *testing.T, b Backend) {
	ctx := context.Background()
	f := setup(t, b)
	ctx = db.WithWorkspace(ctx, f.ws.ID) // round 45: every store call is scoped
	s := b.Search

	// A private channel and a DM the agent is not a member of.
	private := conversation.Channel{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: conversation.KindPrivate, Name: "secret-plans", CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, b.Conversation.CreateChannel(ctx, private))

	mk := func(kind search.SourceKind, src ids.ID, part int, vis search.Visibility, ch ids.ID, body string, at time.Time) search.Chunk {
		return search.Chunk{ID: gen.New(), WorkspaceID: f.ws.ID, SourceKind: kind, SourceID: src, Part: part, Visibility: vis,
			ChannelID: ch, AuthorID: f.human.ID, SourceAt: at, Header: "#general", Body: body,
			ContentHash: search.ContentHash("#general", body), IndexedAt: at}
	}
	pub := gen.New()
	sec := gen.New()
	wik := gen.New()
	wikPlain := gen.New()
	must(t, s.ReplaceSource(ctx, search.SourceMessage, pub, []search.Chunk{
		mk(search.SourceMessage, pub, 0, search.VisibilityChannel, f.channel.ID, "A szerver a homelabon fut Dockerben.", t0),
	}))
	must(t, s.ReplaceSource(ctx, search.SourceMessage, sec, []search.Chunk{
		mk(search.SourceMessage, sec, 0, search.VisibilityChannel, private.ID, "A titkos szerver terve a homelabon.", t0.Add(time.Minute)),
	}))
	must(t, s.ReplaceSource(ctx, search.SourceWiki, wik, []search.Chunk{
		mk(search.SourceWiki, wik, 0, search.VisibilityWorkspace, ids.Nil, "Wiki: a homelab szerver telepítése.", t0.Add(2*time.Minute)),
	}))

	agentAcc := search.Access{WorkspaceID: f.ws.ID, ChannelIDs: []ids.ID{f.channel.ID}}
	adminAcc := search.Access{WorkspaceID: f.ws.ID, ChannelIDs: []ids.ID{f.channel.ID, private.ID}}

	sources := func(cs []search.Candidate) map[ids.ID]bool {
		m := map[ids.ID]bool{}
		for _, c := range cs {
			m[c.Chunk.SourceID] = true
		}
		return m
	}

	// Keyword: access is applied before ranking.
	got, err := s.Keyword(ctx, agentAcc, "homelabon", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); !m[pub] || m[sec] {
		t.Fatalf("keyword as a member of #general only: %v", m)
	}
	got, err = s.Keyword(ctx, adminAcc, "homelabon", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); !m[pub] || !m[sec] {
		t.Fatalf("keyword with the private channel: %v", m)
	}
	// Workspace-visible chunks need no channel.
	got, err = s.Keyword(ctx, search.Access{WorkspaceID: f.ws.ID}, "telepítése", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); len(m) != 1 || !m[wik] {
		t.Fatalf("keyword, no channels: %v", m)
	}
	if got[0].Rank != 1 || got[0].Snippet == "" {
		t.Fatalf("rank and snippet: %+v", got[0])
	}
	// Accents (round 53): people type "telepitese" for "telepítése", and
	// an unaccented word must find the accented text, and the other way.
	got, err = s.Keyword(ctx, search.Access{WorkspaceID: f.ws.ID}, "telepitese", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); len(m) != 1 || !m[wik] {
		t.Fatalf("keyword without accents: %v", m)
	}
	must(t, s.ReplaceSource(ctx, search.SourceWiki, wikPlain, []search.Chunk{
		mk(search.SourceWiki, wikPlain, 0, search.VisibilityWorkspace, ids.Nil, "Wiki: a konyvtar rendezese.", t0.Add(3*time.Minute)),
	}))
	got, err = s.Keyword(ctx, search.Access{WorkspaceID: f.ws.ID}, "könyvtár", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); len(m) != 1 || !m[wikPlain] {
		t.Fatalf("accented keyword on unaccented text: %v", m)
	}
	must(t, s.DeleteSource(ctx, search.SourceWiki, wikPlain))
	// Another workspace sees nothing.
	got, err = s.Keyword(ctx, search.Access{WorkspaceID: gen.New(), ChannelIDs: []ids.ID{f.channel.ID}}, "homelabon", search.Filters{}, 10)
	must(t, err)
	if len(got) != 0 {
		t.Fatalf("other workspace: %+v", got)
	}
	// Filters.
	got, err = s.Keyword(ctx, adminAcc, "homelabon", search.Filters{Kinds: []search.SourceKind{search.SourceWiki}}, 10)
	must(t, err)
	if m := sources(got); m[pub] || m[sec] {
		t.Fatalf("kind filter: %v", m)
	}
	since := t0.Add(30 * time.Second)
	got, err = s.Keyword(ctx, adminAcc, "homelabon", search.Filters{Since: &since}, 10)
	must(t, err)
	if m := sources(got); m[pub] || !m[sec] {
		t.Fatalf("since filter: %v", m)
	}

	// Embedding queue.
	pending, err := s.PendingEmbeddings(ctx, t0, 10)
	must(t, err)
	if len(pending) != 3 {
		t.Fatalf("pending: %d", len(pending))
	}
	byBody := map[string]ids.ID{}
	for _, p := range pending {
		byBody[p.Body] = p.ID
	}
	pubChunk := byBody["A szerver a homelabon fut Dockerben."]
	secChunk := byBody["A titkos szerver terve a homelabon."]
	wikChunk := byBody["Wiki: a homelab szerver telepítése."]
	must(t, s.SetEmbedding(ctx, pubChunk, unitVec(0, 0), "m1", t0))
	must(t, s.SetEmbedding(ctx, secChunk, unitVec(0, 0.1), "m1", t0))
	must(t, s.SetEmbedding(ctx, wikChunk, unitVec(5, 0), "m1", t0))
	if pending, _ = s.PendingEmbeddings(ctx, t0, 10); len(pending) != 0 {
		t.Fatalf("pending after embedding: %d", len(pending))
	}

	// Semantic: the same access rule.
	got, err = s.Semantic(ctx, agentAcc, unitVec(0, 0.05), "m1", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); !m[pub] || m[sec] || !m[wik] {
		t.Fatalf("semantic as a member of #general only: %v", m)
	}
	if got[0].Chunk.SourceID != pub || got[0].Rank != 1 || got[0].Score < 0.9 {
		t.Fatalf("semantic ranking: %+v", got[0])
	}
	got, err = s.Semantic(ctx, adminAcc, unitVec(0, 0.1), "m1", search.Filters{}, 2)
	must(t, err)
	if len(got) != 2 || got[0].Chunk.SourceID != sec {
		t.Fatalf("semantic with the private channel, limit 2: %+v", got)
	}
	// Another model's vectors are never compared.
	if got, _ = s.Semantic(ctx, adminAcc, unitVec(0, 0), "m2", search.Filters{}, 10); len(got) != 0 {
		t.Fatalf("other model: %+v", got)
	}

	// Re-indexing an unchanged part keeps its embedding; a changed one loses it.
	must(t, s.ReplaceSource(ctx, search.SourceMessage, pub, []search.Chunk{
		mk(search.SourceMessage, pub, 0, search.VisibilityChannel, f.channel.ID, "A szerver a homelabon fut Dockerben.", t0),
	}))
	if pending, _ = s.PendingEmbeddings(ctx, t0, 10); len(pending) != 0 {
		t.Fatalf("unchanged re-index queued again: %+v", pending)
	}
	must(t, s.ReplaceSource(ctx, search.SourceMessage, pub, []search.Chunk{
		mk(search.SourceMessage, pub, 0, search.VisibilityChannel, f.channel.ID, "Átköltözött a szerver.", t0),
		mk(search.SourceMessage, pub, 1, search.VisibilityChannel, f.channel.ID, "Második darab.", t0),
	}))
	if pending, _ = s.PendingEmbeddings(ctx, t0, 10); len(pending) != 2 {
		t.Fatalf("changed re-index: %+v", pending)
	}
	// Fewer parts remove the rest.
	must(t, s.ReplaceSource(ctx, search.SourceMessage, pub, []search.Chunk{
		mk(search.SourceMessage, pub, 0, search.VisibilityChannel, f.channel.ID, "Átköltözött a szerver.", t0),
	}))
	if pending, _ = s.PendingEmbeddings(ctx, t0, 10); len(pending) != 1 {
		t.Fatalf("shrunk source: %+v", pending)
	}

	// Failed embeddings wait for their retry time.
	must(t, s.EmbedFailed(ctx, []ids.ID{pending[0].ID}, t0.Add(time.Hour)))
	if p, _ := s.PendingEmbeddings(ctx, t0, 10); len(p) != 0 {
		t.Fatalf("retry time ignored: %+v", p)
	}
	if p, _ := s.PendingEmbeddings(ctx, t0.Add(2*time.Hour), 10); len(p) != 1 {
		t.Fatalf("retry time passed: %+v", p)
	}

	if ms, err := s.EmbedModels(ctx); err != nil || len(ms) != 1 || ms[0] != "m1" {
		t.Fatalf("embed models: %v %v", ms, err)
	}
	// A model change re-queues the other models' rows.
	n, err := s.ResetEmbeddings(ctx, "m2")
	must(t, err)
	if n != 2 {
		t.Fatalf("reset: %d", n)
	}

	// Deleting a source.
	must(t, s.DeleteSource(ctx, search.SourceMessage, sec))
	got, err = s.Keyword(ctx, adminAcc, "titkos", search.Filters{}, 10)
	must(t, err)
	if len(got) != 0 {
		t.Fatalf("deleted source still found: %+v", got)
	}

	// Summaries are replaced per unit.
	root := gen.New()
	sum1, sum2 := gen.New(), gen.New()
	c1 := mk(search.SourceSummary, sum1, 0, search.VisibilityChannel, f.channel.ID, "Összefoglaló egy.", t0)
	c1.ThreadRootID = root
	c2 := mk(search.SourceSummary, sum2, 0, search.VisibilityChannel, f.channel.ID, "Összefoglaló csatorna.", t0)
	must(t, s.ReplaceSource(ctx, search.SourceSummary, sum1, []search.Chunk{c1}))
	must(t, s.ReplaceSource(ctx, search.SourceSummary, sum2, []search.Chunk{c2}))
	must(t, s.DeleteSummaries(ctx, f.channel.ID, root))
	got, err = s.Keyword(ctx, adminAcc, "összefoglaló", search.Filters{}, 10)
	must(t, err)
	if m := sources(got); m[sum1] || !m[sum2] {
		t.Fatalf("thread summary removed, channel summary kept: %v", m)
	}

	// Stats.
	must(t, s.SetEmbedding(ctx, wikChunk, unitVec(5, 0), "m2", t0.Add(time.Hour)))
	st, err := s.Stats(ctx, f.ws.ID)
	must(t, err)
	if st.Chunks != 3 || st.Pending != 2 || st.LastEmbedAt == nil || !st.LastEmbedAt.Equal(t0.Add(time.Hour)) || len(st.Models) != 1 || st.Models[0] != "m2" {
		t.Fatalf("stats: %+v", st)
	}

	// Log and retention.
	must(t, s.InsertLog(ctx, search.LogEntry{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, At: t0, Query: "régi", Mode: search.ModeHybrid}))
	must(t, s.InsertLog(ctx, search.LogEntry{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, At: t0.Add(40 * 24 * time.Hour), Query: "új", Mode: search.ModeKeyword,
		ResultCount: 1, ResultRefs: []string{"message:x"}, TimingsMS: map[string]int64{"total": 3}, Degraded: []string{search.DegradedNoReranker}}))
	purged, err := s.PurgeLogs(ctx, t0.Add(10*24*time.Hour))
	must(t, err)
	if purged != 1 {
		t.Fatalf("purged %d", purged)
	}
}
