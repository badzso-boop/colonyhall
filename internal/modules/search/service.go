package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// Timeouts and back-off (docs/11-rag-terv.md, 9.).
const (
	queryEmbedTimeout = 1500 * time.Millisecond
	// defaultRerankTimeout: 10 candidates take 2.8 s (p50) and 3.6 s (p95)
	// on the PC's CPU with jina-reranker-v2 int8 (real data, 2026-11-10).
	defaultRerankTimeout = 5 * time.Second
	batchEmbedTimeout    = 30 * time.Second
	// skipAfterFailure: after a failed call the service answers without
	// that backend at once for this long, instead of waiting every time.
	skipAfterFailure = 30 * time.Second
	embedBatch       = 8
	minBackoff       = 5 * time.Second
	maxBackoff       = 5 * time.Minute
	// LogRetention is how long the search log is kept.
	LogRetention = 30 * 24 * time.Hour
	// DefaultRerankTop is how many fused candidates go to the reranker.
	// Norbi's decision (2026-11-10, real eval set): 10, MRR@10 0.70 against
	// 0.78 with 20-30, at a third of the time; a GPU reranker may raise it.
	DefaultRerankTop = 10
)

// backend tracks one remote service's health for the status page and the
// fast-fail path.
type backend struct {
	mu       sync.Mutex
	model    string
	lastOK   time.Time
	lastFail time.Time
	lastErr  string
	failing  bool // the last call failed
}

func (b *backend) ok(now time.Time, model string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastOK, b.model, b.lastErr, b.failing = now, model, "", false
}

func (b *backend) fail(now time.Time, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastFail, b.lastErr, b.failing = now, errClass(err), true
}

// errClass is the status page's short error: never the raw error, which
// carries the server's address (an environment value).
func errClass(err error) string {
	var nerr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()):
		return "timeout"
	case errors.As(err, &nerr):
		return "unreachable"
	case errors.Is(err, ErrModelMismatch):
		return "model changed"
	default:
		return "invalid answer"
	}
}

// skip: the last call failed less than skipAfterFailure ago.
func (b *backend) skip(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failing && now.Sub(b.lastFail) < skipAfterFailure
}

// BackendState is a remote service's state for the status page.
type BackendState struct {
	Configured bool
	Reachable  bool
	Model      string
	LastOK     *time.Time
	LastError  string
}

func (b *backend) state(configured bool) BackendState {
	b.mu.Lock()
	defer b.mu.Unlock()
	st := BackendState{Configured: configured, Model: b.model, LastError: b.lastErr}
	if !b.lastOK.IsZero() {
		t := b.lastOK
		st.LastOK = &t
	}
	st.Reachable = configured && !b.lastOK.IsZero() && !b.failing
	return st
}

// Service is the search use case, the embedding worker and the index
// maintenance commands.
type Service struct {
	Store     Store
	Conv      Conversation
	Directory Directory
	Embedder  Embedder // nil: keyword search only
	Reranker  Reranker // nil: no reranking
	RerankTop int      // default DefaultRerankTop
	// RerankTimeout bounds one rerank call (default defaultRerankTimeout);
	// past it the search falls back to the fused order.
	RerankTimeout time.Duration
	IDs           ids.Generator
	Clock         Clock
	Logger        *slog.Logger

	embed  backend
	rerank backend
	wake   chan struct{}
	once   sync.Once
	// mismatch is set when the embedder's model differs from the stored
	// vectors' model: the worker stops writing until a re-embed.
	mu       sync.Mutex
	mismatch string
}

func (s *Service) log() *slog.Logger {
	if s.Logger == nil {
		return slog.Default()
	}
	return s.Logger
}

func (s *Service) wakeCh() chan struct{} {
	s.once.Do(func() { s.wake = make(chan struct{}, 1) })
	return s.wake
}

// Wake tells the embedding worker that new chunks are waiting.
func (s *Service) Wake() {
	select {
	case s.wakeCh() <- struct{}{}:
	default:
	}
}

// access computes the reader's searchable channels: the channels they are
// a member of (their own DMs included), and for admins every channel that
// is not a DM (decision D1: other people's DMs never appear in a search).
func (s *Service) access(ctx context.Context, by identity.Actor) (Access, error) {
	acc := Access{WorkspaceID: by.WorkspaceID}
	member, err := s.Conv.ChannelIDsOf(ctx, by, by.ID)
	if err != nil {
		return acc, err
	}
	seen := map[ids.ID]bool{}
	for _, id := range member {
		if !seen[id] {
			seen[id] = true
			acc.ChannelIDs = append(acc.ChannelIDs, id)
		}
	}
	if by.IsAdmin() {
		all, err := s.Conv.ListChannels(ctx, by, true)
		if err != nil {
			return acc, err
		}
		for _, c := range all {
			if c.Kind != conversation.KindDM && !seen[c.ID] {
				seen[c.ID] = true
				acc.ChannelIDs = append(acc.ChannelIDs, c.ID)
			}
		}
	}
	return acc, nil
}

// Search runs a search for by. Retrieval failures of the embedder or the
// reranker degrade the result (keyword only, no rerank) instead of failing.
func (s *Service) Search(ctx context.Context, by identity.Actor, q Query) (Result, error) {
	start := s.Clock.Now()
	q.Text = strings.TrimSpace(q.Text)
	if q.Text == "" {
		return Result{}, ErrEmptyQuery
	}
	if utf8.RuneCountInString(q.Text) > MaxQueryLen {
		return Result{}, ErrQueryTooLong
	}
	switch q.Mode {
	case "":
		q.Mode = ModeHybrid
	case ModeKeyword, ModeSemantic, ModeHybrid:
	default:
		return Result{}, ErrBadMode
	}
	if q.Limit <= 0 {
		q.Limit = DefaultLimit
	}
	q.Limit = min(q.Limit, MaxLimit)

	acc, err := s.access(ctx, by)
	if err != nil {
		return Result{}, err
	}
	res := Result{Mode: q.Mode, Timings: map[string]int64{}}
	lap := func(name string, from time.Time) { res.Timings[name] = s.Clock.Now().Sub(from).Milliseconds() }

	var kw, sem []Candidate
	if q.Mode != ModeSemantic {
		t := s.Clock.Now()
		if kw, err = s.Store.Keyword(ctx, acc, q.Text, q.Filters, CandidateLimit); err != nil {
			return Result{}, err
		}
		lap("keyword", t)
	}
	if q.Mode != ModeKeyword {
		t := s.Clock.Now()
		vec, model, ok := s.embedQuery(ctx, q.Text)
		if ok {
			if sem, err = s.Store.Semantic(ctx, acc, vec, model, q.Filters, CandidateLimit); err != nil {
				return Result{}, err
			}
		} else {
			res.Degraded = append(res.Degraded, DegradedNoEmbedder)
			if q.Mode == ModeSemantic {
				// Semantic only, without an embedder: answer with keywords.
				if kw, err = s.Store.Keyword(ctx, acc, q.Text, q.Filters, CandidateLimit); err != nil {
					return Result{}, err
				}
			}
		}
		lap("semantic", t)
	}

	hits := fuse(kw, sem)
	// NoRerank: the evaluation's configurations without a reranker.
	if s.Reranker != nil && len(hits) > 1 && !q.NoRerank {
		t := s.Clock.Now()
		if !s.rerankHits(ctx, q.Text, hits) {
			res.Degraded = append(res.Degraded, DegradedNoReranker)
		}
		lap("rerank", t)
	} else if s.Reranker == nil && !q.NoRerank {
		res.Degraded = append(res.Degraded, DegradedNoReranker)
	}
	if len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	res.Hits = hits
	res.Notice = notice(res.Degraded, s.Embedder != nil, s.Reranker != nil)
	lap("total", start)
	if !q.NoLog {
		s.writeLog(ctx, by, q, res)
	}
	return res, nil
}

// notice is the short line a degraded answer starts with. A backend that
// is not configured at all is not news on every search.
func notice(degraded []string, embedder, reranker bool) string {
	var parts []string
	for _, d := range degraded {
		switch {
		case d == DegradedNoEmbedder && embedder:
			parts = append(parts, "semantic search unavailable: the embedding server is not responding; keyword results only")
		case d == DegradedNoReranker && reranker:
			parts = append(parts, "reranker unavailable; results are not reranked")
		}
	}
	return strings.Join(parts, "; ")
}

func (s *Service) embedQuery(ctx context.Context, text string) ([]float32, string, bool) {
	if s.Embedder == nil {
		return nil, "", false
	}
	now := s.Clock.Now()
	if s.embed.skip(now) {
		return nil, "", false
	}
	ctx, cancel := context.WithTimeout(ctx, queryEmbedTimeout)
	defer cancel()
	vecs, model, err := s.Embedder.Embed(ctx, []string{EmbedQuery(text)})
	if err == nil && len(vecs) != 1 {
		err = fmt.Errorf("embedder returned %d vectors for 1 input", len(vecs))
	}
	if err != nil {
		s.embed.fail(s.Clock.Now(), err)
		s.log().Warn("search: query embedding failed", "err", err)
		return nil, "", false
	}
	s.embed.ok(s.Clock.Now(), model)
	return vecs[0], model, true
}

// fuse merges the legs with Reciprocal Rank Fusion.
func fuse(kw, sem []Candidate) []Hit {
	byID := map[ids.ID]*Hit{}
	var order []ids.ID
	add := func(c Candidate, keyword bool) {
		h, ok := byID[c.Chunk.ID]
		if !ok {
			h = &Hit{Chunk: c.Chunk}
			byID[c.Chunk.ID] = h
			order = append(order, c.Chunk.ID)
		}
		score := c.Score
		if keyword {
			h.Keyword = &score
			if c.Snippet != "" {
				h.Snippet = c.Snippet
			}
		} else {
			h.Semantic = &score
		}
		h.Fused += 1 / float64(RRFK+c.Rank)
	}
	for _, c := range kw {
		add(c, true)
	}
	for _, c := range sem {
		add(c, false)
	}
	out := make([]Hit, 0, len(order))
	for _, id := range order {
		h := byID[id]
		if h.Snippet == "" {
			h.Snippet = clip(h.Chunk.Body, 240)
		}
		out = append(out, *h)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Fused != out[j].Fused {
			return out[i].Fused > out[j].Fused
		}
		return out[i].Chunk.SourceAt.After(out[j].Chunk.SourceAt)
	})
	return out
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// rerankHits reorders the top RerankTop hits by the reranker's score; the
// rest keep their fused order after them. False when the reranker failed.
func (s *Service) rerankHits(ctx context.Context, query string, hits []Hit) bool {
	now := s.Clock.Now()
	if s.rerank.skip(now) {
		return false
	}
	top := s.RerankTop
	if top <= 0 {
		top = DefaultRerankTop
	}
	n := min(top, len(hits))
	docs := make([]string, n)
	for i := range n {
		docs[i] = hits[i].Chunk.Header + "\n" + hits[i].Chunk.Body
	}
	timeout := s.RerankTimeout
	if timeout <= 0 {
		timeout = defaultRerankTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	scores, model, err := s.Reranker.Rerank(ctx, query, docs)
	if err == nil && len(scores) != n {
		err = fmt.Errorf("reranker returned %d scores for %d documents", len(scores), n)
	}
	if err != nil {
		s.rerank.fail(s.Clock.Now(), err)
		s.log().Warn("search: rerank failed", "err", err)
		return false
	}
	s.rerank.ok(s.Clock.Now(), model)
	for i := range n {
		sc := scores[i]
		hits[i].Rerank = &sc
	}
	sort.SliceStable(hits[:n], func(i, j int) bool { return *hits[i].Rerank > *hits[j].Rerank })
	return true
}

func (s *Service) writeLog(ctx context.Context, by identity.Actor, q Query, res Result) {
	refs := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		refs = append(refs, string(h.Chunk.SourceKind)+":"+h.Chunk.SourceID.String())
	}
	err := s.Store.InsertLog(ctx, LogEntry{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ActorID: by.ID, At: s.Clock.Now(),
		Query: q.Text, Mode: q.Mode, ResultCount: len(res.Hits), ResultRefs: refs, TimingsMS: res.Timings, Degraded: res.Degraded})
	if err != nil {
		s.log().Warn("search: log write failed", "err", err)
	}
}

// EmbedPending embeds one batch of waiting chunks. It returns how many it
// embedded; (0, nil) means nothing was waiting.
func (s *Service) EmbedPending(ctx context.Context) (int, error) {
	if s.Embedder == nil {
		return 0, nil
	}
	pending, err := s.Store.PendingEmbeddings(ctx, s.Clock.Now(), embedBatch)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	texts := make([]string, len(pending))
	for i, p := range pending {
		texts[i] = EmbedText(p.Header, p.Body)
	}
	cctx, cancel := context.WithTimeout(ctx, batchEmbedTimeout)
	vecs, model, err := s.Embedder.Embed(cctx, texts)
	cancel()
	if err == nil && len(vecs) != len(pending) {
		err = fmt.Errorf("embedder returned %d vectors for %d inputs", len(vecs), len(pending))
	}
	if err != nil {
		s.embed.fail(s.Clock.Now(), err)
		// The batch waits before it is tried again, so one bad chunk cannot
		// block the queue for the others.
		failed := make([]ids.ID, len(pending))
		for i, p := range pending {
			failed[i] = p.ID
		}
		if ferr := s.Store.EmbedFailed(ctx, failed, s.Clock.Now().Add(minBackoff)); ferr != nil {
			return 0, errors.Join(err, ferr)
		}
		return 0, err
	}
	s.embed.ok(s.Clock.Now(), model)
	if err := s.checkModel(ctx, model); err != nil {
		return 0, err
	}
	now := s.Clock.Now()
	for i, p := range pending {
		if err := s.Store.SetEmbedding(ctx, p.ID, vecs[i], model, now); err != nil {
			return i, err
		}
	}
	return len(pending), nil
}

// ErrModelMismatch: the embedder serves another model than the stored
// vectors; nothing is written until `colonyhall rag-reembed`.
var ErrModelMismatch = errors.New("search: the embedding model changed")

func (s *Service) checkModel(ctx context.Context, model string) error {
	models, err := s.Store.EmbedModels(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range models {
		if m != model {
			s.mismatch = fmt.Sprintf("stored vectors use %s, the embedding server serves %s: run colonyhall rag-reembed", m, model)
			return fmt.Errorf("%w: %s", ErrModelMismatch, s.mismatch)
		}
	}
	s.mismatch = ""
	return nil
}

// RunEmbedder embeds waiting chunks until ctx ends: at once when woken,
// otherwise every few seconds; after a failure it backs off from 5 s to
// 5 minutes, so a dead embedding server is not hammered.
func (s *Service) RunEmbedder(ctx context.Context) {
	if s.Embedder == nil {
		return
	}
	backoff := time.Duration(0)
	tick := 5 * time.Second
	for {
		n, err := s.EmbedPending(ctx)
		var wait time.Duration
		switch {
		case err != nil && ctx.Err() == nil:
			backoff = min(max(backoff*2, minBackoff), maxBackoff)
			s.log().Warn("search: embedding failed; backing off", "err", err, "retry_in", backoff)
			wait = backoff
		case n > 0:
			backoff, wait = 0, 0
		default:
			backoff, wait = 0, tick
		}
		if wait == 0 {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-s.wakeCh():
			t.Stop()
			if backoff > 0 {
				// Woken during a back-off: still wait it out.
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
			}
		case <-t.C:
		}
	}
}

// PurgeLogs removes search log rows older than LogRetention.
func (s *Service) PurgeLogs(ctx context.Context) (int, error) {
	return s.Store.PurgeLogs(ctx, s.Clock.Now().Add(-LogRetention))
}

// Reembed re-queues every vector not made by the embedder's current model.
func (s *Service) Reembed(ctx context.Context) (int, string, error) {
	if s.Embedder == nil {
		return 0, "", errors.New("search: no embedding server configured (COLONY_EMBED_URL)")
	}
	_, model, err := s.Embedder.Embed(ctx, []string{EmbedQuery("model check")})
	if err != nil {
		return 0, "", err
	}
	n, err := s.Store.ResetEmbeddings(ctx, model)
	if err == nil {
		s.mu.Lock()
		s.mismatch = ""
		s.mu.Unlock()
		s.Wake()
	}
	return n, model, err
}

// Status is the search's state for the status page.
type Status struct {
	Index         Stats
	Embedder      BackendState
	Reranker      BackendState
	ModelMismatch string
}

// Status reports the index and the remote services.
func (s *Service) Status(ctx context.Context, workspaceID ids.ID) (Status, error) {
	st, err := s.Store.Stats(ctx, workspaceID)
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	mm := s.mismatch
	s.mu.Unlock()
	return Status{Index: st, Embedder: s.embed.state(s.Embedder != nil), Reranker: s.rerank.state(s.Reranker != nil), ModelMismatch: mm}, nil
}
