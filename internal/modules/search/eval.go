package search

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// EvalSet is a search evaluation set (docs/11-rag-terv.md, "Mérés"):
// questions with the sources that answer them, optionally with the seed
// data to load into an empty, throwaway server. Without Seed the questions
// run against the configured database.
type EvalSet struct {
	Name      string         `json:"name"`
	Seed      *EvalSeed      `json:"seed,omitempty"`
	Questions []EvalQuestion `json:"questions"`
}

// EvalSeed is made-up data for a synthetic set.
type EvalSeed struct {
	Channels  []EvalChannel  `json:"channels"`
	Messages  []EvalMessage  `json:"messages"`
	Wiki      []EvalWiki     `json:"wiki"`
	Decisions []EvalDecision `json:"decisions"`
}

// EvalChannel is a seed channel; the agent is a member unless Private.
type EvalChannel struct {
	Name    string `json:"name"`
	Private bool   `json:"private,omitempty"`
}

// EvalMessage is a seed message. Label names it for the questions;
// Thread is the label of its thread root.
type EvalMessage struct {
	Label   string `json:"label"`
	Channel string `json:"channel"`
	Thread  string `json:"thread,omitempty"`
	Body    string `json:"body"`
}

// EvalWiki is a seed wiki page.
type EvalWiki struct {
	Label string `json:"label"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// EvalDecision is a seed decision.
type EvalDecision struct {
	Label     string `json:"label"`
	Channel   string `json:"channel"`
	Text      string `json:"text"`
	Rationale string `json:"rationale"`
}

// EvalQuestion is one query and what answers it. Relevant holds seed
// labels, or for a real set source references: message:<id>, wiki:<slug>,
// decision:D-12, summary:<id>.
type EvalQuestion struct {
	ID       string   `json:"id"`
	Query    string   `json:"query"`
	Relevant []string `json:"relevant"`
	// Kind groups the report (e.g. paraphrase, inflection, identifier).
	Kind string `json:"kind,omitempty"`
}

// ReadEvalSet parses and checks a set.
func ReadEvalSet(r io.Reader) (EvalSet, error) {
	var s EvalSet
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, fmt.Errorf("eval set: %w", err)
	}
	if len(s.Questions) == 0 {
		return s, fmt.Errorf("eval set: no questions")
	}
	for _, q := range s.Questions {
		if strings.TrimSpace(q.Query) == "" || len(q.Relevant) == 0 {
			return s, fmt.Errorf("eval set: question %q needs a query and at least one relevant source", q.ID)
		}
	}
	return s, nil
}

// HitRef is a hit's reference in the form the questions use.
func HitRef(c Chunk) string {
	switch c.SourceKind {
	case SourceWiki:
		return "wiki:" + c.SourceRef
	case SourceDecision:
		return "decision:" + c.SourceRef
	default:
		return string(c.SourceKind) + ":" + c.SourceID.String()
	}
}

// EvalMetrics are the averages over the questions of one configuration.
type EvalMetrics struct {
	Config   string
	N        int
	Recall5  float64
	Recall10 float64
	MRR10    float64
	NDCG10   float64
	P50      time.Duration
	P95      time.Duration
}

// EvalRun collects one configuration's per-question results.
type EvalRun struct {
	Config    string
	rankings  [][]string
	relevant  [][]string
	latencies []time.Duration
}

// Add records one question: the ranked hit references and the latency.
func (r *EvalRun) Add(ranked, relevant []string, latency time.Duration) {
	r.rankings = append(r.rankings, ranked)
	r.relevant = append(r.relevant, relevant)
	r.latencies = append(r.latencies, latency)
}

// Metrics computes recall@5, recall@10, MRR@10, nDCG@10 and the latency
// percentiles. A source with several relevant chunks counts once.
func (r *EvalRun) Metrics() EvalMetrics {
	m := EvalMetrics{Config: r.Config, N: len(r.rankings)}
	if m.N == 0 {
		return m
	}
	for i, ranked := range r.rankings {
		rel := map[string]bool{}
		for _, x := range r.relevant[i] {
			rel[x] = true
		}
		ranked = dedupe(ranked)
		found5, found10 := 0, 0
		var dcg float64
		for pos, ref := range ranked {
			if pos >= 10 {
				break
			}
			if !rel[ref] {
				continue
			}
			if pos < 5 {
				found5++
			}
			found10++
			if found10 == 1 {
				m.MRR10 += 1 / float64(pos+1)
			}
			dcg += 1 / math.Log2(float64(pos+2))
		}
		var idcg float64
		for pos := 0; pos < min(len(rel), 10); pos++ {
			idcg += 1 / math.Log2(float64(pos+2))
		}
		m.Recall5 += float64(found5) / float64(len(rel))
		m.Recall10 += float64(found10) / float64(len(rel))
		if idcg > 0 {
			m.NDCG10 += dcg / idcg
		}
	}
	n := float64(m.N)
	m.Recall5, m.Recall10, m.MRR10, m.NDCG10 = m.Recall5/n, m.Recall10/n, m.MRR10/n, m.NDCG10/n
	lat := append([]time.Duration(nil), r.latencies...)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	m.P50 = lat[(len(lat)-1)*50/100]
	m.P95 = lat[(len(lat)-1)*95/100]
	return m
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// FormatMetrics prints the configurations side by side.
func FormatMetrics(name string, ms []EvalMetrics) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%d questions)\n\n", name, ms[0].N)
	fmt.Fprintf(&b, "%-16s %9s %9s %8s %8s %9s %9s\n", "config", "recall@5", "recall@10", "MRR@10", "nDCG@10", "p50", "p95")
	for _, m := range ms {
		fmt.Fprintf(&b, "%-16s %9.3f %9.3f %8.3f %8.3f %9s %9s\n", m.Config, m.Recall5, m.Recall10, m.MRR10, m.NDCG10,
			m.P50.Round(100*time.Microsecond), m.P95.Round(100*time.Microsecond))
	}
	return b.String()
}
