package search

import (
	"testing"

	"colonyhall/internal/platform/ids"
)

// TestDiversifyKeepsTheDecisionInThePool: a long thread used to fill the
// rerank pool, and the decision the reranker would have put first never
// reached it (round 53, the real evaluation set's D-1 and D-5).
func TestDiversifyKeepsTheDecisionInThePool(t *testing.T) {
	gen := &ids.Sequential{}
	thread := gen.New()
	var hits []Hit
	for range 12 { // twelve replies of one thread, fused first
		hits = append(hits, Hit{Chunk: Chunk{ID: gen.New(), SourceKind: SourceMessage, SourceID: gen.New(), ThreadRootID: thread}})
	}
	decision := Hit{Chunk: Chunk{ID: gen.New(), SourceKind: SourceDecision, SourceID: gen.New()}}
	hits = append(hits, decision)

	got := diversify(hits, 5)
	if len(got) != len(hits) {
		t.Fatalf("hits dropped: %d of %d", len(got), len(hits))
	}
	if got[3].Chunk.ID != decision.Chunk.ID {
		t.Fatalf("the decision is not right after the thread's first %d replies: %v", PoolPerThread, refs(got[:5]))
	}
	// The thread's chunks keep their order, before and after the cap.
	last := -1
	for _, h := range got {
		if h.Chunk.ThreadRootID != thread {
			continue
		}
		i := indexOf(hits, h.Chunk.ID)
		if i < last {
			t.Fatalf("thread order changed: %v", refs(got))
		}
		last = i
	}

	// Without the cap the decision would be 13th: outside a pool of 5.
	if indexOf(hits, decision.Chunk.ID) < 5 {
		t.Fatal("the test no longer reproduces the crowding")
	}
}

func TestDiversifyCapsChunksOfOneSource(t *testing.T) {
	gen := &ids.Sequential{}
	page := gen.New()
	var hits []Hit
	for part := range 4 { // four parts of one wiki page
		hits = append(hits, Hit{Chunk: Chunk{ID: gen.New(), SourceKind: SourceWiki, SourceID: page, Part: part}})
	}
	other := Hit{Chunk: Chunk{ID: gen.New(), SourceKind: SourceWiki, SourceID: gen.New()}}
	hits = append(hits, other)
	got := diversify(hits, 3)
	if got[2].Chunk.ID != other.Chunk.ID {
		t.Fatalf("a third part of one page took the pool's last place: %v", refs(got))
	}
}

// With too few distinct sources the pool is still filled to n.
func TestDiversifyFillsThePoolWhenSourcesRunOut(t *testing.T) {
	gen := &ids.Sequential{}
	thread := gen.New()
	var hits []Hit
	for range 6 {
		hits = append(hits, Hit{Chunk: Chunk{ID: gen.New(), SourceKind: SourceMessage, SourceID: gen.New(), ThreadRootID: thread}})
	}
	got := diversify(hits, 5)
	for i := range got {
		if got[i].Chunk.ID != hits[i].Chunk.ID {
			t.Fatalf("with one thread only the order must stay: %v", refs(got))
		}
	}
	if len(diversify(nil, 5)) != 0 || len(diversify(hits[:1], 5)) != 1 {
		t.Fatal("empty and single inputs")
	}
}

func indexOf(hs []Hit, id ids.ID) int {
	for i, h := range hs {
		if h.Chunk.ID == id {
			return i
		}
	}
	return -1
}

func refs(hs []Hit) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = string(h.Chunk.SourceKind) + ":" + h.Chunk.ID.String()[30:]
	}
	return out
}
