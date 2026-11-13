package e2e

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/fakeagent"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

type gitEnv struct {
	*env
	fakeGitea  *httptest.Server
	giteaHits  int64
	webhookSec string
}

func newGitEnv(t *testing.T) *gitEnv {
	t.Helper()
	ctx := context.Background()
	clk := clock.NewManual(time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC))

	var hits int64
	fakeGitea := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/repos/acme/backend/pulls/42":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"title": "Add caching layer",
				"state": "open",
				"user":  map[string]any{"username": "alice"},
			})
		case "/api/v1/repos/acme/backend/issues/10":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"title": "Memory leak on idle",
				"state": "closed",
				"user":  map[string]any{"username": "bob"},
			})
		case "/api/v1/repos/acme/backend":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "backend",
				"description": "Core backend service",
				"owner":       map[string]any{"username": "acme"},
			})
		case "/api/v1/repos/acme/backend/git/commits/a1b2c3d4e5f67890":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"commit": map[string]any{
					"message": "fix(auth): handle expired tokens\n\nDetailed description here",
					"author":  map[string]any{"name": "charlie"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fakeGitea.Close)

	webhookSecret := "gitea-secret-12345"

	app, err := server.New(ctx, server.Options{
		Store:              "memory",
		Clock:              clk,
		IDs:                &ids.Sequential{},
		GiteaURL:           fakeGitea.URL,
		GiteaToken:         "gitea-test-token",
		GiteaWebhookSecret: webhookSecret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)

	baseEnv := &env{t: t, app: app, srv: srv, router: specRouter(t, srv.URL), clock: clk}
	return &gitEnv{
		env:        baseEnv,
		fakeGitea:  fakeGitea,
		giteaHits:  0,
		webhookSec: webhookSecret,
	}
}

// computeHMAC computes the hex-encoded HMAC-SHA256 signature for a payload.
func computeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestGit_UnfurlLinkCardsAndCaching(t *testing.T) {
	ge := newGitEnv(t)
	ctx := context.Background()

	// 1. Get default general channel
	var chList struct {
		Channels []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"channels"`
	}
	if code := ge.do(http.MethodGet, "/api/v1/channels", server.DevAdminToken, nil, &chList); code != http.StatusOK {
		t.Fatalf("GET /channels = %d", code)
	}
	if len(chList.Channels) == 0 {
		t.Fatal("no channels found")
	}
	chID := chList.Channels[0].ID

	// 2. Post a message containing a Gitea pull request URL
	prURL := ge.fakeGitea.URL + "/acme/backend/pulls/42"
	var sendRes struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	sendInput := map[string]any{
		"body": fmt.Sprintf("Please review this PR: %s thanks!", prURL),
	}
	if code := ge.do(http.MethodPost, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, sendInput, &sendRes); code != http.StatusCreated {
		t.Fatalf("POST /messages = %d", code)
	}

	// 3. Drain async event consumers so link unfurling completes
	if err := ge.app.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// 4. Fetch channel messages and verify link_cards
	var msgList struct {
		Messages []struct {
			ID        string `json:"id"`
			Body      string `json:"body"`
			LinkCards []struct {
				URL    string `json:"url"`
				Kind   string `json:"kind"`
				Title  string `json:"title"`
				State  string `json:"state"`
				Author string `json:"author"`
			} `json:"link_cards"`
		} `json:"messages"`
	}
	if code := ge.do(http.MethodGet, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, nil, &msgList); code != http.StatusOK {
		t.Fatalf("GET /messages = %d", code)
	}
	if len(msgList.Messages) == 0 {
		t.Fatal("expected messages, got none")
	}
	msg := msgList.Messages[0]
	if len(msg.LinkCards) != 1 {
		t.Fatalf("expected 1 link card, got %d", len(msg.LinkCards))
	}
	card := msg.LinkCards[0]
	if card.URL != prURL {
		t.Errorf("card URL = %q, want %q", card.URL, prURL)
	}
	if card.Kind != "pull" {
		t.Errorf("card Kind = %q, want pull", card.Kind)
	}
	if card.Title != "#42 Add caching layer" {
		t.Errorf("card Title = %q, want '#42 Add caching layer'", card.Title)
	}
	if card.State != "open" {
		t.Errorf("card State = %q, want open", card.State)
	}
	if card.Author != "alice" {
		t.Errorf("card Author = %q, want alice", card.Author)
	}

	hitsBefore := atomic.LoadInt64(&ge.giteaHits)

	// 5. Post second message with the same link: should use cached card without new fetch
	sendInput2 := map[string]any{
		"body": fmt.Sprintf("Checking again: %s", prURL),
	}
	if code := ge.do(http.MethodPost, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, sendInput2, &sendRes); code != http.StatusCreated {
		t.Fatalf("POST second message = %d", code)
	}
	if err := ge.app.Drain(ctx); err != nil {
		t.Fatalf("Drain 2: %v", err)
	}

	hitsAfter := atomic.LoadInt64(&ge.giteaHits)
	if hitsAfter != hitsBefore {
		t.Errorf("expected cache hit (hits before: %d, hits after: %d)", hitsBefore, hitsAfter)
	}
}

func TestGit_SSRFPrevention(t *testing.T) {
	ge := newGitEnv(t)
	ctx := context.Background()

	var chList struct {
		Channels []struct {
			ID string `json:"id"`
		} `json:"channels"`
	}
	_ = ge.do(http.MethodGet, "/api/v1/channels", server.DevAdminToken, nil, &chList)
	chID := chList.Channels[0].ID

	// Post message with a URL pointing to another host (not configured Gitea)
	evilURL := "https://evil.internal/acme/backend/pulls/1"
	sendInput := map[string]any{
		"body": fmt.Sprintf("Check this evil link %s", evilURL),
	}
	var sendRes struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	if code := ge.do(http.MethodPost, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, sendInput, &sendRes); code != http.StatusCreated {
		t.Fatalf("POST message = %d", code)
	}
	if err := ge.app.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	var msgList struct {
		Messages []struct {
			ID        string `json:"id"`
			LinkCards []any  `json:"link_cards"`
		} `json:"messages"`
	}
	_ = ge.do(http.MethodGet, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, nil, &msgList)
	if len(msgList.Messages[0].LinkCards) != 0 {
		t.Errorf("expected 0 link cards for foreign host, got %d", len(msgList.Messages[0].LinkCards))
	}
}

func TestGit_RepoChannelMappingsAPI(t *testing.T) {
	ge := newGitEnv(t)

	var chList struct {
		Channels []struct {
			ID string `json:"id"`
		} `json:"channels"`
	}
	_ = ge.do(http.MethodGet, "/api/v1/channels", server.DevAdminToken, nil, &chList)
	chID := chList.Channels[0].ID

	// 1. Agent should be forbidden from setting mappings
	setPayload := map[string]any{
		"repo":       "acme/backend",
		"channel_id": chID,
	}
	var setRes map[string]any
	if code := ge.do(http.MethodPost, "/api/v1/git/mappings", server.DevAgentToken, setPayload, &setRes); code != http.StatusForbidden {
		t.Fatalf("agent POST /git/mappings = %d, want 403", code)
	}

	// 2. Admin sets mapping
	var mappingOut struct {
		Repo      string `json:"repo"`
		ChannelID string `json:"channel_id"`
		CreatedAt string `json:"created_at"`
	}
	if code := ge.do(http.MethodPost, "/api/v1/git/mappings", server.DevAdminToken, setPayload, &mappingOut); code != http.StatusOK {
		t.Fatalf("admin POST /git/mappings = %d", code)
	}
	if mappingOut.Repo != "acme/backend" || mappingOut.ChannelID != chID {
		t.Fatalf("unexpected mapping output: %+v", mappingOut)
	}

	// 3. List mappings
	var listOut struct {
		Mappings []struct {
			Repo      string `json:"repo"`
			ChannelID string `json:"channel_id"`
		} `json:"mappings"`
	}
	if code := ge.do(http.MethodGet, "/api/v1/git/mappings", server.DevAdminToken, nil, &listOut); code != http.StatusOK {
		t.Fatalf("GET /git/mappings = %d", code)
	}
	if len(listOut.Mappings) != 1 || listOut.Mappings[0].Repo != "acme/backend" {
		t.Fatalf("unexpected mappings list: %+v", listOut)
	}

	// 4. Delete mapping
	var delOut any
	if code := ge.do(http.MethodDelete, "/api/v1/git/mappings/acme/backend", server.DevAdminToken, nil, &delOut); code != http.StatusNoContent {
		t.Fatalf("DELETE /git/mappings = %d, want 204", code)
	}

	// 5. List mappings again -> empty
	_ = ge.do(http.MethodGet, "/api/v1/git/mappings", server.DevAdminToken, nil, &listOut)
	if len(listOut.Mappings) != 0 {
		t.Fatalf("expected empty mappings list after delete, got %d", len(listOut.Mappings))
	}
}

func TestGit_WebhookVerificationAndIdempotency(t *testing.T) {
	ge := newGitEnv(t)

	var chList struct {
		Channels []struct {
			ID string `json:"id"`
		} `json:"channels"`
	}
	_ = ge.do(http.MethodGet, "/api/v1/channels", server.DevAdminToken, nil, &chList)
	chID := chList.Channels[0].ID

	// Map repository to channel
	setPayload := map[string]any{"repo": "acme/backend", "channel_id": chID}
	var setRes any
	if code := ge.do(http.MethodPost, "/api/v1/git/mappings", server.DevAdminToken, setPayload, &setRes); code != http.StatusOK {
		t.Fatalf("set mapping: %d", code)
	}

	webhookPayload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"full_name": "acme/backend",
		},
		"pull_request": map[string]any{
			"number":   42,
			"title":    "Add caching layer",
			"html_url": ge.fakeGitea.URL + "/acme/backend/pulls/42",
		},
		"sender": map[string]any{
			"username": "alice",
		},
	}
	payloadBytes, _ := json.Marshal(webhookPayload)

	// 1. Missing signature -> 401
	req, _ := http.NewRequest(http.MethodPost, ge.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Delivery", "del-001")
	req.Header.Set("X-Gitea-Event", "pull_request")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing signature = %d, want 401", resp.StatusCode)
	}

	// 2. Invalid signature -> 401
	req, _ = http.NewRequest(http.MethodPost, ge.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Delivery", "del-001")
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", "invalid-sig")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid signature = %d, want 401", resp.StatusCode)
	}

	// 3. Valid signature -> 200 OK and message delivered to channel
	sig := computeHMAC(ge.webhookSec, payloadBytes)
	req, _ = http.NewRequest(http.MethodPost, ge.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Delivery", "del-001")
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", sig)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid webhook = %d, want 200", resp.StatusCode)
	}

	// Verify channel message posted by @gitea
	var msgList struct {
		Messages []struct {
			ID       string `json:"id"`
			AuthorID string `json:"author_id"`
			Body     string `json:"body"`
		} `json:"messages"`
	}
	_ = ge.do(http.MethodGet, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, nil, &msgList)
	if len(msgList.Messages) != 1 {
		t.Fatalf("expected 1 message in channel, got %d", len(msgList.Messages))
	}
	if !bytes.Contains([]byte(msgList.Messages[0].Body), []byte("PR #42 opened: Add caching layer by @\u200balice")) {
		t.Errorf("unexpected message body: %s", msgList.Messages[0].Body)
	}

	// 4. Replay the same signed body, even under a new (unsigned) delivery
	// id (round 40) -> 200 OK, but no duplicate message
	req, _ = http.NewRequest(http.MethodPost, ge.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Delivery", "replayed-with-a-new-id")
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", sig)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replay webhook = %d, want 200", resp.StatusCode)
	}

	_ = ge.do(http.MethodGet, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, nil, &msgList)
	if len(msgList.Messages) != 1 {
		t.Fatalf("expected still 1 message after replay, got %d", len(msgList.Messages))
	}

	// 5. Unmapped repository event -> 200 OK ignored
	unmappedPayload := map[string]any{
		"action": "opened",
		"repository": map[string]any{
			"full_name": "acme/unmapped",
		},
		"sender": map[string]any{"username": "alice"},
	}
	unmappedBytes, _ := json.Marshal(unmappedPayload)
	unmappedSig := computeHMAC(ge.webhookSec, unmappedBytes)
	req, _ = http.NewRequest(http.MethodPost, ge.srv.URL+"/api/v1/hooks/gitea", bytes.NewReader(unmappedBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gitea-Delivery", "del-002")
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Signature", unmappedSig)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unmapped repo = %d, want 200", resp.StatusCode)
	}

	_ = ge.do(http.MethodGet, "/api/v1/channels/"+chID+"/messages", server.DevAdminToken, nil, &msgList)
	if len(msgList.Messages) != 1 {
		t.Fatalf("messages count changed after unmapped repo webhook: %d", len(msgList.Messages))
	}
}

func TestGit_MCPStatusTool(t *testing.T) {
	ge := newGitEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := fakeagent.Connect(ctx, ge.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// 1. Full URL status
	prURL := ge.fakeGitea.URL + "/acme/backend/pulls/42"
	callRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "git_status",
		Arguments: map[string]any{
			"ref": prURL,
		},
	})
	if err != nil {
		t.Fatalf("CallTool git_status URL: %v", err)
	}
	text := callRes.Content[0].(*mcp.TextContent).Text
	if !bytes.Contains([]byte(text), []byte("Add caching layer")) || !bytes.Contains([]byte(text), []byte("open")) {
		t.Errorf("unexpected status output: %s", text)
	}

	// 2. Shorthand status e.g. acme/backend#42
	callRes2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "git_status",
		Arguments: map[string]any{
			"ref": "acme/backend#42",
		},
	})
	if err != nil {
		t.Fatalf("CallTool git_status shorthand: %v", err)
	}
	text2 := callRes2.Content[0].(*mcp.TextContent).Text
	if !bytes.Contains([]byte(text2), []byte("Add caching layer")) {
		t.Errorf("unexpected shorthand status output: %s", text2)
	}
}
