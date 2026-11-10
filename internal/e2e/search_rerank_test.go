package e2e

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/modules/search/fakeembed"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// slowReranker is the fake reranker with a delay, and it records how many
// documents each call got.
type slowReranker struct {
	fakeembed.Fake
	delay time.Duration
	mu    sync.Mutex
	docs  []int
}

func (r *slowReranker) Rerank(ctx context.Context, query string, docs []string) ([]float64, string, error) {
	r.mu.Lock()
	r.docs = append(r.docs, len(docs))
	r.mu.Unlock()
	select {
	case <-time.After(r.delay):
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	return r.Fake.Rerank(ctx, query, docs)
}

// The reranker gets 10 candidates by default (Norbi's decision on the real
// eval set, 2026-11-10), and its time limit comes from the configuration: a
// real reranker on a CPU needs about 3 s for 10, the old fixed 2 s dropped
// every rerank.
func TestSearchRerankTopAndTimeout(t *testing.T) {
	for _, tc := range []struct {
		name     string
		timeout  time.Duration
		reranked bool
	}{
		{"within the configured limit", 2 * time.Second, true},
		{"past the configured limit", 50 * time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			embed, rr := &fakeembed.Fake{}, &slowReranker{delay: 300 * time.Millisecond}
			e := newEnvWith(t, func(o *server.Options) {
				o.Embedder, o.Reranker, o.RerankTimeout = embed, rr, tc.timeout
			})
			s := &searchEnv{env: e, embed: embed, ctx: context.Background()}
			admin, err := e.app.Identity.Authenticate(s.ctx, server.DevAdminToken)
			if err != nil {
				t.Fatal(err)
			}
			s.admin = admin
			general, _ := ids.Parse(s.generalID())
			for i := range 15 {
				s.send(s.admin, general, fmt.Sprintf("A reranker teszt üzenet %d a homelabról.", i))
			}
			s.index()
			r := s.search(server.DevAdminToken, "reranker homelab", nil)
			notReranked := r.Notice != nil && strings.Contains(*r.Notice, "not reranked")
			if notReranked == tc.reranked {
				t.Fatalf("reranked=%v, notice %v", !notReranked, r.Notice)
			}
			rr.mu.Lock()
			defer rr.mu.Unlock()
			if len(rr.docs) != 1 || rr.docs[0] != 10 {
				t.Fatalf("reranker calls and candidates: %v, want one call with 10", rr.docs)
			}
		})
	}
}
