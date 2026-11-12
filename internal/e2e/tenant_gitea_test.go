package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/modules/git"
	"colonyhall/internal/server"
)

// Round 45, decision 12/4: a workspace may use its own Gitea. Its webhook
// comes to its own subdomain with its own secret; the shared webhook (bare
// base domain, shared secret) fans out to the other workspaces only.
func TestOwnGiteaWebhookPerSubdomain(t *testing.T) {
	const sharedSecret, acmeSecret = "shared-webhook-secret", "acme-own-webhook-secret"
	acme, beta := mtEnv(t, func(o *server.Options) {
		o.GiteaURL, o.GiteaToken, o.GiteaWebhookSecret = "http://127.0.0.1:9", "shared-read-token", sharedSecret
	})
	ctx := context.Background()
	if err := acme.app.Git.SetWorkspaceGitea(ctx, git.WorkspaceGitea{WorkspaceID: acmeWorkspace(t, acme), Enabled: true,
		URL: "http://127.0.0.1:10", Token: "acme-read-token", WebhookSecret: acmeSecret}); err != nil {
		t.Fatal(err)
	}

	// Both workspaces map the same repository name to a channel.
	acmeCh := acme.generalID()
	var created struct{ ID string }
	if s := beta.do("POST", "/api/v1/channels", betaOwnerToken, map[string]any{"name": "dev", "kind": "public"}, &created); s != http.StatusCreated {
		t.Fatalf("beta channel: %d", s)
	}
	betaCh := created.ID
	for _, m := range []struct {
		e     *env
		token string
		ch    string
	}{{acme, server.DevAdminToken, acmeCh}, {beta, betaOwnerToken, betaCh}} {
		if s := m.e.do("POST", "/api/v1/git/mappings", m.token, map[string]any{"repo": "team/app", "channel_id": m.ch}, nil); s != http.StatusOK {
			t.Fatalf("mapping: %d", s)
		}
	}

	hook := func(host, secret, delivery, title string) int {
		payload, _ := json.Marshal(map[string]any{
			"action":       "opened",
			"repository":   map[string]any{"full_name": "team/app"},
			"pull_request": map[string]any{"number": 7, "title": title, "html_url": "http://git.example/team/app/pulls/7"},
			"sender":       map[string]any{"username": "dev"},
		})
		req, _ := http.NewRequest(http.MethodPost, acme.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(payload))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Gitea-Delivery", delivery)
		req.Header.Set("X-Gitea-Event", "pull_request")
		req.Header.Set("X-Gitea-Signature", computeHMAC(secret, payload))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	bodies := func(e *env, token, ch string) string {
		var page struct{ Messages []struct{ Body string } }
		e.do("GET", "/api/v1/channels/"+ch+"/messages", token, nil, &page)
		var all []string
		for _, m := range page.Messages {
			all = append(all, m.Body)
		}
		return strings.Join(all, "\n")
	}

	// The shared webhook on the bare domain: beta only (acme has its own).
	if s := hook(baseDomain, sharedSecret, "d-1", "shared one"); s != http.StatusOK {
		t.Fatalf("shared webhook: %d", s)
	}
	// acme's own webhook on acme's subdomain, with acme's secret: acme only.
	// The same delivery id as the shared one: ids are kept per workspace.
	if s := hook("acme."+baseDomain, acmeSecret, "d-1", "own one"); s != http.StatusOK {
		t.Fatalf("own webhook: %d", s)
	}
	// Wrong secret on a subdomain, or a subdomain without its own Gitea: refused.
	if s := hook("acme."+baseDomain, sharedSecret, "d-2", "forged"); s != http.StatusUnauthorized {
		t.Fatalf("shared secret on acme's subdomain: %d", s)
	}
	if s := hook("beta."+baseDomain, sharedSecret, "d-3", "forged"); s != http.StatusUnauthorized {
		t.Fatalf("webhook on a subdomain without its own Gitea: %d", s)
	}

	a, b := bodies(acme, server.DevAdminToken, acmeCh), bodies(beta, betaOwnerToken, betaCh)
	if !strings.Contains(a, "own one") || strings.Contains(a, "shared one") || strings.Contains(a, "forged") {
		t.Fatalf("acme's channel: %q", a)
	}
	if !strings.Contains(b, "shared one") || strings.Contains(b, "own one") || strings.Contains(b, "forged") {
		t.Fatalf("beta's channel: %q", b)
	}

	// Switched off, acme is back on the shared Gitea and its own webhook is refused.
	if err := acme.app.Git.SetWorkspaceGitea(ctx, git.WorkspaceGitea{WorkspaceID: acmeWorkspace(t, acme)}); err != nil {
		t.Fatal(err)
	}
	if s := hook("acme."+baseDomain, acmeSecret, "d-4", "after off"); s != http.StatusUnauthorized {
		t.Fatalf("own webhook after switching off: %d", s)
	}
}
