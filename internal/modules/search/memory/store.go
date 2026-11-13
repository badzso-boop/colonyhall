// Package memory is the in-memory reference implementation of
// search.Store. Its keyword leg is a simple word match (a word, or a
// word's start for terms of five letters or more, standing in for
// stemming); the contract tests only rely on what both adapters share.
package memory

import (
	"context"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"colonyhall/internal/modules/search"
	"colonyhall/internal/platform/ids"
)

type row struct {
	c          search.Chunk
	vec        []float32
	embedded   *time.Time
	attempts   int
	retryAt    *time.Time
	insertedAt int
}

// Store is an in-memory search.Store.
type Store struct {
	mu   sync.RWMutex
	rows map[ids.ID]*row
	seq  int
	logs []search.LogEntry
}

var _ search.Store = (*Store)(nil)

// New returns an empty store.
func New() *Store { return &Store{rows: map[ids.ID]*row{}} }

func (s *Store) ReplaceSource(_ context.Context, kind search.SourceKind, sourceID ids.ID, chunks []search.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := map[int]*row{}
	for id, r := range s.rows {
		if r.c.SourceKind == kind && r.c.SourceID == sourceID {
			old[r.c.Part] = r
			delete(s.rows, id)
		}
	}
	for _, c := range chunks {
		r := &row{c: c}
		if o, ok := old[c.Part]; ok {
			r.c.ID = o.c.ID
			r.insertedAt = o.insertedAt
			if o.c.ContentHash == c.ContentHash {
				r.vec, r.embedded, r.c.EmbedModel = o.vec, o.embedded, o.c.EmbedModel
				r.attempts, r.retryAt = o.attempts, o.retryAt
			} else {
				r.c.EmbedModel = ""
			}
		} else {
			s.seq++
			r.insertedAt = s.seq
			r.c.EmbedModel = ""
		}
		s.rows[r.c.ID] = r
	}
	return nil
}

func (s *Store) DeleteSource(_ context.Context, kind search.SourceKind, sourceID ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.rows {
		if r.c.SourceKind == kind && r.c.SourceID == sourceID {
			delete(s.rows, id)
		}
	}
	return nil
}

func (s *Store) DeleteSummaries(_ context.Context, channelID, threadRootID ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.rows {
		if r.c.SourceKind == search.SourceSummary && r.c.ChannelID == channelID && r.c.ThreadRootID == threadRootID {
			delete(s.rows, id)
		}
	}
	return nil
}

func (s *Store) PendingEmbeddings(_ context.Context, now time.Time, limit int) ([]search.PendingChunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var rs []*row
	for _, r := range s.rows {
		if r.vec == nil && (r.retryAt == nil || !r.retryAt.After(now)) {
			rs = append(rs, r)
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].insertedAt < rs[j].insertedAt })
	var out []search.PendingChunk
	for _, r := range rs {
		if len(out) == limit {
			break
		}
		out = append(out, search.PendingChunk{ID: r.c.ID, Header: r.c.Header, Body: r.c.Body})
	}
	return out, nil
}

func (s *Store) SetEmbedding(_ context.Context, id ids.ID, vec []float32, model string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rows[id]; ok {
		r.vec, r.c.EmbedModel, r.retryAt = slices.Clone(vec), model, nil
		t := at
		r.embedded = &t
	}
	return nil
}

func (s *Store) EmbedFailed(_ context.Context, chunkIDs []ids.ID, retryAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range chunkIDs {
		if r, ok := s.rows[id]; ok {
			r.attempts++
			t := retryAt
			r.retryAt = &t
		}
	}
	return nil
}

func (s *Store) EmbedModels(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, r := range s.rows {
		if r.vec != nil && !seen[r.c.EmbedModel] {
			seen[r.c.EmbedModel] = true
			out = append(out, r.c.EmbedModel)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) ResetEmbeddings(_ context.Context, keepModel string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.rows {
		if r.vec != nil && r.c.EmbedModel != keepModel {
			r.vec, r.embedded, r.c.EmbedModel, r.retryAt, r.attempts = nil, nil, "", nil, 0
			n++
		}
	}
	return n, nil
}

func allowed(r *row, acc search.Access, f search.Filters) bool {
	c := r.c
	if c.WorkspaceID != acc.WorkspaceID {
		return false
	}
	if c.Visibility != search.VisibilityWorkspace && !slices.Contains(acc.ChannelIDs, c.ChannelID) {
		return false
	}
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, c.SourceKind) {
		return false
	}
	if f.ChannelID != ids.Nil && c.ChannelID != f.ChannelID {
		return false
	}
	if f.AuthorID != ids.Nil && c.AuthorID != f.AuthorID {
		return false
	}
	if f.Since != nil && c.SourceAt.Before(*f.Since) {
		return false
	}
	if f.Until != nil && c.SourceAt.After(*f.Until) {
		return false
	}
	return true
}

func words(s string) []string {
	return strings.FieldsFunc(fold(strings.ToLower(s)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// accents maps accented lower-case letters to their base letter, like the
// Postgres adapter's unaccent vector (round 53): "konyv" matches "könyv".
var accents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "ő", "o", "õ", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ű", "u",
	"ç", "c", "ñ", "n", "ý", "y", "ž", "z", "š", "s", "č", "c", "ř", "r",
)

func fold(s string) string { return accents.Replace(s) }

func termMatches(term string, ws []string) bool {
	for _, w := range ws {
		if w == term || (len([]rune(term)) >= 5 && strings.HasPrefix(w, term)) {
			return true
		}
	}
	return false
}

func (s *Store) Keyword(_ context.Context, acc search.Access, query string, f search.Filters, limit int) ([]search.Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	terms := words(query)
	var out []search.Candidate
	for _, r := range s.rows {
		if !allowed(r, acc, f) {
			continue
		}
		ws := words(r.c.Header + " " + r.c.Body)
		score := 0.0
		for _, t := range terms {
			if termMatches(t, ws) {
				score++
			}
		}
		if score == 0 {
			continue
		}
		out = append(out, search.Candidate{Chunk: r.c, Score: score / float64(len(terms)), Snippet: snippet(r.c.Body, terms)})
	}
	sortAndRank(out, limit)
	return trim(out, limit), nil
}

func snippet(body string, terms []string) string {
	fields := strings.Fields(body)
	for i, w := range fields {
		lw := words(w)
		for _, t := range terms {
			if termMatches(t, lw) {
				fields[i] = "«" + w + "»"
				break
			}
		}
	}
	out := strings.Join(fields, " ")
	if r := []rune(out); len(r) > 240 {
		out = string(r[:240]) + "…"
	}
	return out
}

func (s *Store) Semantic(_ context.Context, acc search.Access, vec []float32, model string, f search.Filters, limit int) ([]search.Candidate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []search.Candidate
	for _, r := range s.rows {
		if r.vec == nil || r.c.EmbedModel != model || !allowed(r, acc, f) {
			continue
		}
		out = append(out, search.Candidate{Chunk: r.c, Score: cosine(vec, r.vec)})
	}
	sortAndRank(out, limit)
	return trim(out, limit), nil
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func sortAndRank(cs []search.Candidate, _ int) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Score != cs[j].Score {
			return cs[i].Score > cs[j].Score
		}
		return cs[i].Chunk.SourceAt.After(cs[j].Chunk.SourceAt)
	})
	for i := range cs {
		cs[i].Rank = i + 1
	}
}

func trim(cs []search.Candidate, limit int) []search.Candidate {
	if len(cs) > limit {
		return cs[:limit]
	}
	return cs
}

func (s *Store) InsertLog(_ context.Context, e search.LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, e)
	return nil
}

func (s *Store) PurgeLogs(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.logs[:0]
	n := 0
	for _, e := range s.logs {
		if e.At.Before(before) {
			n++
			continue
		}
		kept = append(kept, e)
	}
	s.logs = kept
	return n, nil
}

func (s *Store) Stats(_ context.Context, workspaceID ids.ID) (search.Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var st search.Stats
	models := map[string]bool{}
	for _, r := range s.rows {
		if r.c.WorkspaceID != workspaceID {
			continue
		}
		st.Chunks++
		if r.vec == nil {
			st.Pending++
			continue
		}
		models[r.c.EmbedModel] = true
		if st.LastEmbedAt == nil || r.embedded.After(*st.LastEmbedAt) {
			t := *r.embedded
			st.LastEmbedAt = &t
		}
	}
	for m := range models {
		st.Models = append(st.Models, m)
	}
	sort.Strings(st.Models)
	return st, nil
}
