// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The protocol (docs/25 13.5): the server posts a job to the runner
// (POST /v1/run, the shared COLONY_EXT_RUNNER_TOKEN); the runner answers
// the script's host calls by posting them back to the server's internal
// listener (POST /v1/host) with a token that is valid for that one job
// only. The runner holds no database connection and no secret.

type runRequest struct {
	Bundle    string          `json:"bundle"`
	Fn        string          `json:"fn"`
	Input     json.RawMessage `json:"input"`
	TimeoutMS int64           `json:"timeout_ms"`
	Output    int             `json:"output_bytes"`
	HostCalls int             `json:"host_calls"`
	HostToken string          `json:"host_token"`
}

type runResponse struct {
	Output json.RawMessage `json:"output,omitempty"`
	Logs   []string        `json:"logs,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
}

type hostRequest struct {
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

type hostResponse struct {
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
}

var codes = map[string]error{"timeout": ErrTimeout, "memory": ErrMemory, "output": ErrOutput, "host_calls": ErrHostCalls, "busy": ErrBusy}

func codeOf(err error) string {
	for c, e := range codes {
		if errors.Is(err, e) {
			return c
		}
	}
	return "error"
}

func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func same(a, b string) bool { return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

const maxBody = 4 << 20

// Server is the ext-runner role's HTTP handler.
type Server struct {
	Engine  Engine
	Token   string // COLONY_EXT_RUNNER_TOKEN
	HostURL string // the server's internal listener, e.g. http://server:8096
	HTTP    *http.Client
	Logger  *slog.Logger
}

// Handler serves POST /v1/run and GET /healthz.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") })
	mux.HandleFunc("POST /v1/run", s.run)
	return mux
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	if !same(bearer(r), s.Token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req runRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	hc := s.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 70 * time.Second}
	}
	host := func(ctx context.Context, kind string, payload json.RawMessage) (json.RawMessage, error) {
		body, _ := json.Marshal(hostRequest{Kind: kind, Payload: payload})
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.HostURL, "/")+"/v1/host", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		hreq.Header.Set("Authorization", "Bearer "+req.HostToken)
		hreq.Header.Set("Content-Type", "application/json")
		res, err := hc.Do(hreq)
		if err != nil {
			return nil, errors.New("the server could not be reached")
		}
		defer func() { _ = res.Body.Close() }()
		var hr hostResponse
		if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&hr); err != nil {
			return nil, fmt.Errorf("host answer: HTTP %d", res.StatusCode)
		}
		if hr.Error != "" {
			return nil, errors.New(hr.Error)
		}
		return hr.Value, nil
	}
	res, err := s.Engine.Run(r.Context(), Job{Bundle: req.Bundle, Fn: req.Fn, Input: req.Input,
		Limits: Limits{Timeout: time.Duration(req.TimeoutMS) * time.Millisecond, OutputBytes: req.Output, HostCalls: req.HostCalls}}, host)
	out := runResponse{Output: res.Output, Logs: res.Logs}
	if err != nil {
		out = runResponse{Error: err.Error(), Code: codeOf(err)}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Client is the server's side: an Engine that sends jobs to the runner
// and answers their host calls through Hosts.
type Client struct {
	URL   string // COLONY_EXT_RUNNER_URL
	Token string
	Hosts *Hosts
	HTTP  *http.Client
}

// Run implements Engine.
func (c *Client) Run(ctx context.Context, job Job, host Host) (Result, error) {
	if c == nil || c.URL == "" {
		return Result{}, ErrNoRunner
	}
	token, done := c.Hosts.register(host)
	defer done()
	lim := job.Limits
	body, _ := json.Marshal(runRequest{Bundle: job.Bundle, Fn: job.Fn, Input: job.Input, TimeoutMS: lim.Timeout.Milliseconds(),
		Output: lim.OutputBytes, HostCalls: lim.HostCalls, HostToken: token})
	timeout := lim.Timeout
	if timeout <= 0 {
		timeout = DefaultLimits.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/v1/run", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	res, err := hc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ErrTimeout
		}
		return Result{}, ErrNoRunner
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("the code runner answered HTTP %d", res.StatusCode)
	}
	var rr runResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBody)).Decode(&rr); err != nil {
		return Result{}, errors.New("the code runner's answer is unreadable")
	}
	if rr.Error != "" {
		if e, ok := codes[rr.Code]; ok {
			return Result{}, e
		}
		return Result{}, errors.New(rr.Error)
	}
	return Result{Output: rr.Output, Logs: rr.Logs}, nil
}

// Hosts holds the host functions of the jobs in flight, by token. Its
// Handler is the server's internal listener (never the public one).
type Hosts struct {
	mu    sync.Mutex
	hosts map[string]Host
}

func (h *Hosts) register(host Host) (string, func()) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	h.mu.Lock()
	if h.hosts == nil {
		h.hosts = map[string]Host{}
	}
	h.hosts[token] = host
	h.mu.Unlock()
	return token, func() {
		h.mu.Lock()
		delete(h.hosts, token)
		h.mu.Unlock()
	}
}

// Handler serves POST /v1/host.
func (h *Hosts) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/host", func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r)
		h.mu.Lock()
		var host Host
		for t, x := range h.hosts {
			if same(token, t) {
				host = x
			}
		}
		h.mu.Unlock()
		if host == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req hostRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		v, err := host(r.Context(), req.Kind, req.Payload)
		out := hostResponse{Value: v}
		if err != nil {
			out = hostResponse{Error: err.Error()}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	return mux
}
