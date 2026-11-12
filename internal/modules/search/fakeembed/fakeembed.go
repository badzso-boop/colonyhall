// Package fakeembed is a deterministic stand-in for the embedding and
// reranking servers, for tests, the dev stack and the synthetic evaluation:
// a text's vector is its hashed bag of word stems (the first five letters),
// so texts sharing words are close. It is no language model; it only makes
// the semantic leg and the reranker exercise the same code paths.
package fakeembed

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"unicode"

	"colonyhall/internal/modules/search"
)

// Dims matches the production text model (384).
const Dims = 384

// Model is the name the fake reports.
const Model = "fake-embed@1"

// Fake implements search.Embedder and search.Reranker.
type Fake struct {
	mu sync.Mutex
	// Down makes every call fail (the server is unreachable).
	Down bool
	// Calls counts calls, for tests.
	Calls int
	// ModelName overrides Model (tests of a model change).
	ModelName string
}

var (
	_ search.Embedder = (*Fake)(nil)
	_ search.Reranker = (*Fake)(nil)
)

// ErrDown is returned while Down is set.
var ErrDown = errors.New("fakeembed: server down")

func (f *Fake) model() string {
	if f.ModelName != "" {
		return f.ModelName
	}
	return Model
}

func (f *Fake) call() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	if f.Down {
		return ErrDown
	}
	return nil
}

// SetDown switches the fake server off or on.
func (f *Fake) SetDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Down = down
}

func stems(text string) []string {
	text = strings.TrimPrefix(strings.TrimPrefix(text, "query: "), "passage: ")
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		r := []rune(w)
		if len(r) < 3 {
			continue
		}
		out = append(out, string(r[:min(5, len(r))]))
	}
	return out
}

// Vector is the fake embedding of text.
func Vector(text string) []float32 {
	v := make([]float32, Dims)
	for _, s := range stems(text) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(s))
		v[h.Sum32()%Dims]++
	}
	var n float64
	for _, x := range v {
		n += float64(x * x)
	}
	if n == 0 {
		v[0] = 1
		return v
	}
	n = math.Sqrt(n)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v
}

// Embed implements search.Embedder.
func (f *Fake) Embed(_ context.Context, texts []string) ([][]float32, string, error) {
	if err := f.call(); err != nil {
		return nil, "", err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = Vector(t)
	}
	return out, f.model(), nil
}

// Rerank implements search.Reranker: the share of query stems in the doc.
func (f *Fake) Rerank(_ context.Context, query string, docs []string) ([]float64, string, error) {
	if err := f.call(); err != nil {
		return nil, "", err
	}
	qs := stems(query)
	out := make([]float64, len(docs))
	for i, d := range docs {
		ds := map[string]bool{}
		for _, s := range stems(d) {
			ds[s] = true
		}
		hit := 0
		for _, s := range qs {
			if ds[s] {
				hit++
			}
		}
		if len(qs) > 0 {
			out[i] = float64(hit) / float64(len(qs))
		}
	}
	return out, "fake-rerank@1", nil
}
