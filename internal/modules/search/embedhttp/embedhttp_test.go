package embedhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmbedAndRerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/embeddings":
			var in struct{ Input []string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			type d struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}
			// Out of order on purpose: the client sorts by index.
			out := []d{}
			for i := len(in.Input) - 1; i >= 0; i-- {
				out = append(out, d{Index: i, Embedding: []float32{float32(i), 1}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "e5@abc", "data": out})
		case "/v1/rerank":
			_ = json.NewEncoder(w).Encode(map[string]any{"model": "rr@1", "results": []map[string]any{
				{"index": 1, "relevance_score": 0.9}, {"index": 0, "relevance_score": 0.1}}})
		}
	}))
	defer srv.Close()
	c, err := New(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	vecs, model, err := c.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil || model != "e5@abc" || len(vecs) != 3 || vecs[2][0] != 2 || vecs[0][0] != 0 {
		t.Fatalf("embed: %v %s %v", vecs, model, err)
	}
	scores, model, err := c.Rerank(context.Background(), "q", []string{"x", "y"})
	if err != nil || model != "rr@1" || scores[0] != 0.1 || scores[1] != 0.9 {
		t.Fatalf("rerank: %v %s %v", scores, model, err)
	}
}

func TestErrors(t *testing.T) {
	if _, err := New("ftp://x"); err == nil {
		t.Fatal("bad scheme accepted")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		default:
			if r.URL.Path == "/v1/embeddings" {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"embedding": []float32{1}}}})
				return
			}
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c, _ := New(srv.URL)
	if _, _, err := c.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("an answer without a model name must fail")
	}
	if _, _, err := c.Rerank(context.Background(), "q", []string{"a"}); err == nil {
		t.Fatal("HTTP 500 must fail")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer slow.Close()
	c, _ = New(slow.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, _, err := c.Embed(ctx, []string{"a"}); err == nil || time.Since(start) > 250*time.Millisecond {
		t.Fatalf("the deadline must cut the call: %v after %v", err, time.Since(start))
	}
}
