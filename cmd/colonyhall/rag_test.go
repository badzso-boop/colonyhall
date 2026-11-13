package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"colonyhall/internal/modules/search"
)

// TestSyntheticEvalSet runs the synthetic evaluation set (CI): the
// tooling works end to end and the keyword leg finds what it must. The
// fake embedder is lexical, so the semantic numbers mean nothing here;
// the real Hungarian set is measured with the real models (private set).
func TestSyntheticEvalSet(t *testing.T) {
	f, err := os.Open("../../testdata/rag-eval/synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	set, err := search.ReadEvalSet(f)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := EvalSet(context.Background(), set, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + search.FormatMetrics(set.Name, ms))
	by := map[string]search.EvalMetrics{}
	for _, m := range ms {
		by[m.Config] = m
		if m.N != len(set.Questions) || m.Recall10 < 0 || m.Recall10 > 1 || m.MRR10 > 1 || m.NDCG10 > 1 {
			t.Fatalf("%s: %+v", m.Config, m)
		}
	}
	if by["keyword"].Recall10 < 0.8 || by["hybrid"].Recall10 < 0.8 {
		t.Fatalf("keyword and hybrid must find the seeded answers: %+v", ms)
	}
	// Round 53. Accents: the keyword leg finds every question, accented or
	// not (q19-q22; "kozos mentese" for "közös mentése"). Crowding: a
	// 12-reply thread must not push the decision that answers q23 out of
	// the rerank pool and the page. q19 may still miss in the hybrid
	// configurations: the fake embedder is not accent-aware and gives every
	// text some similarity, so its 50 semantic candidates outrank a
	// keyword-only hit (KeywordWeight 0.2); the real model does not.
	if m := by["keyword"].Missed; len(m) != 0 {
		t.Fatalf("keyword missed %v", m)
	}
	for _, cfg := range []string{"hybrid", "hybrid+rerank"} {
		for _, id := range by[cfg].Missed {
			if id != "q19" {
				t.Fatalf("%s missed %s (all missed: %v)", cfg, id, by[cfg].Missed)
			}
		}
	}
	if !strings.Contains(search.FormatMetrics("x", ms), "hybrid+rerank") {
		t.Fatal("report misses a configuration")
	}
}
