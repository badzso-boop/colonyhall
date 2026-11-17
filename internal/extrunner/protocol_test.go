// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The server and the runner over HTTP: a job runs, its host calls come
// back to the server with the job's token, and the token is dead after
// the job.
func TestRunnerOverHTTP(t *testing.T) {
	hosts := &Hosts{}
	hostSrv := httptest.NewServer(hosts.Handler())
	t.Cleanup(hostSrv.Close)
	runner := httptest.NewServer((&Server{Engine: engine, Token: "runner-token", HostURL: hostSrv.URL}).Handler())
	t.Cleanup(runner.Close)
	client := &Client{URL: runner.URL, Token: "runner-token", Hosts: hosts}

	var seenToken string
	src := `globalThis.__ext = { f: async (i, ctx) => (await ctx.colony.whoami({})) + "/" + i.x };`
	host := func(_ context.Context, kind string, payload json.RawMessage) (json.RawMessage, error) {
		if kind != "colony" || !strings.Contains(string(payload), `"tool":"whoami"`) {
			return nil, errors.New("unexpected host call")
		}
		hosts.mu.Lock()
		for tok := range hosts.hosts {
			seenToken = tok
		}
		hosts.mu.Unlock()
		return json.RawMessage(`"@bot"`), nil
	}
	res, err := client.Run(context.Background(), Job{Bundle: src, Fn: "f", Input: json.RawMessage(`{"x":1}`)}, host)
	if err != nil || string(res.Output) != `"@bot/1"` {
		t.Fatalf("%s %v", res.Output, err)
	}
	// After the job the token opens nothing.
	req, _ := http.NewRequest(http.MethodPost, hostSrv.URL+"/v1/host", strings.NewReader(`{"kind":"colony","payload":{}}`))
	req.Header.Set("Authorization", "Bearer "+seenToken)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a used host token answered %d", r.StatusCode)
	}
	// A wrong runner token is refused.
	bad := &Client{URL: runner.URL, Token: "wrong", Hosts: hosts}
	if _, err := bad.Run(context.Background(), Job{Bundle: src, Fn: "f"}, host); err == nil {
		t.Fatal("the runner accepted a wrong token")
	}
	// The limits' errors come back as themselves.
	_, err = client.Run(context.Background(), Job{Bundle: `globalThis.__ext = { f: async () => { for (;;) {} } };`, Fn: "f",
		Limits: Limits{Timeout: 500 * time.Millisecond}}, host)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout over HTTP: %v", err)
	}
}

// Without a runner the call fails cleanly.
func TestNoRunner(t *testing.T) {
	var c *Client
	if _, err := c.Run(context.Background(), Job{}, nil); !errors.Is(err, ErrNoRunner) {
		t.Fatalf("nil client: %v", err)
	}
	dead := &Client{URL: "http://127.0.0.1:1", Token: "x", Hosts: &Hosts{}}
	if _, err := dead.Run(context.Background(), Job{Bundle: "x", Fn: "f"}, nil); !errors.Is(err, ErrNoRunner) {
		t.Fatalf("dead runner: %v", err)
	}
}
