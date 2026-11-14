// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package embedhttp is the HTTP adapter for the search's Embedder and
// Reranker ports: an OpenAI-compatible POST /v1/embeddings (the Raspberry
// Pi) and a Jina/Cohere-style POST /v1/rerank (the homelab reranker); see
// docs/11-rag-terv.md and deploy/embed/.
package embedhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"colonyhall/internal/modules/search"
)

// maxResponse bounds a response body (8 inputs × 384 floats is ~60 KB).
const maxResponse = 8 << 20

// Client calls one server.
type Client struct {
	BaseURL string
	HTTP    *http.Client // default: http.DefaultClient (deadlines come from ctx)
}

var (
	_ search.Embedder = (*Client)(nil)
	_ search.Reranker = (*Client)(nil)
)

// New checks the URL and returns a client.
func New(base string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("embedhttp: invalid server URL %q", base)
	}
	return &Client{BaseURL: u.String()}, nil
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponse {
		return errors.New("embedhttp: response too large")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embedhttp: %s answered %d", path, resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

type embedRequest struct {
	Model string   `json:"model,omitempty"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed implements search.Embedder.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, string, error) {
	var out embedResponse
	if err := c.post(ctx, "/v1/embeddings", embedRequest{Input: texts}, &out); err != nil {
		return nil, "", err
	}
	if out.Model == "" {
		return nil, "", errors.New("embedhttp: the answer names no model")
	}
	if len(out.Data) != len(texts) {
		return nil, "", fmt.Errorf("embedhttp: %d embeddings for %d inputs", len(out.Data), len(texts))
	}
	vecs := make([][]float32, len(texts))
	for i, d := range out.Data {
		idx := d.Index
		if idx < 0 || idx >= len(texts) || vecs[idx] != nil {
			idx = i
		}
		vecs[idx] = d.Embedding
	}
	dims := len(vecs[0])
	for _, v := range vecs {
		if len(v) == 0 || len(v) != dims {
			return nil, "", errors.New("embedhttp: inconsistent embedding dimensions")
		}
	}
	return vecs, out.Model, nil
}

type rerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type rerankResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
}

// Rerank implements search.Reranker; scores come back in document order.
func (c *Client) Rerank(ctx context.Context, query string, docs []string) ([]float64, string, error) {
	var out rerankResponse
	if err := c.post(ctx, "/v1/rerank", rerankRequest{Query: query, Documents: docs}, &out); err != nil {
		return nil, "", err
	}
	if len(out.Results) != len(docs) {
		return nil, "", fmt.Errorf("embedhttp: %d scores for %d documents", len(out.Results), len(docs))
	}
	scores := make([]float64, len(docs))
	seen := make([]bool, len(docs))
	for _, r := range out.Results {
		if r.Index < 0 || r.Index >= len(docs) || seen[r.Index] {
			return nil, "", errors.New("embedhttp: invalid rerank index")
		}
		seen[r.Index] = true
		scores[r.Index] = r.RelevanceScore
	}
	return scores, out.Model, nil
}
