package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGiteaProvider_ParseURL(t *testing.T) {
	p, err := NewGitea("https://gitea.example.com", "secret-token")
	if err != nil {
		t.Fatalf("NewGitea: %v", err)
	}

	tests := []struct {
		url      string
		wantOK   bool
		wantKind ResourceKind
		wantID   string
	}{
		{"https://gitea.example.com/org/repo/pulls/42", true, ResourcePull, "42"},
		{"https://gitea.example.com/org/repo/issues/100", true, ResourceIssue, "100"},
		{"https://gitea.example.com/org/repo/commit/abcdef123456", true, ResourceCommit, "abcdef123456"},
		{"https://gitea.example.com/org/repo/commits/abcdef123456", true, ResourceCommit, "abcdef123456"},
		{"https://gitea.example.com/org/repo", true, ResourceRepo, ""},
		{"https://otherhost.com/org/repo/pulls/42", false, "", ""},
		{"https://gitea.example.com/assets/logo.png", false, "", ""},
		{"https://gitea.example.com/api/v1/repos", false, "", ""},
		{"https://gitea.example.com/", false, "", ""},
	}

	for _, tc := range tests {
		ref, ok := p.ParseURL(tc.url)
		if ok != tc.wantOK {
			t.Errorf("ParseURL(%q) ok = %v, want %v", tc.url, ok, tc.wantOK)
			continue
		}
		if ok {
			if ref.Kind != tc.wantKind {
				t.Errorf("ParseURL(%q) Kind = %v, want %v", tc.url, ref.Kind, tc.wantKind)
			}
			if ref.ID != tc.wantID {
				t.Errorf("ParseURL(%q) ID = %q, want %q", tc.url, ref.ID, tc.wantID)
			}
		}
	}
}

func TestGiteaProvider_ParseRef(t *testing.T) {
	p, err := NewGitea("https://gitea.example.com", "")
	if err != nil {
		t.Fatalf("NewGitea: %v", err)
	}

	tests := []struct {
		ref      string
		wantOK   bool
		wantKind ResourceKind
		wantID   string
	}{
		{"org/repo#42", true, ResourceIssue, "42"},
		{"org/repo@abcdef", true, ResourceCommit, "abcdef"},
		{"org/repo", true, ResourceRepo, ""},
		{"https://gitea.example.com/org/repo/pulls/99", true, ResourcePull, "99"},
		{"invalid", false, "", ""},
	}

	for _, tc := range tests {
		ref, ok := p.ParseRef(tc.ref)
		if ok != tc.wantOK {
			t.Errorf("ParseRef(%q) ok = %v, want %v", tc.ref, ok, tc.wantOK)
			continue
		}
		if ok {
			if ref.Kind != tc.wantKind {
				t.Errorf("ParseRef(%q) Kind = %v, want %v", tc.ref, ref.Kind, tc.wantKind)
			}
			if ref.ID != tc.wantID {
				t.Errorf("ParseRef(%q) ID = %q, want %q", tc.ref, ref.ID, tc.wantID)
			}
		}
	}
}

func TestGiteaProvider_FetchResource(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/repos/org/repo/pulls/10", func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "token test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"title":"Add feature","state":"open","merged":false,"user":{"login":"alice"}}`))
	})
	mux.HandleFunc("GET /api/v1/repos/org/repo/commits/abc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sha":"abc1234","commit":{"message":"Fix bug\n\nMore details","author":{"name":"Bob"}}}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewGitea(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewGitea: %v", err)
	}

	// PR fetch
	card, err := p.FetchResource(context.Background(), ResourceRef{
		Kind:  ResourcePull,
		Owner: "org",
		Repo:  "repo",
		ID:    "10",
		URL:   srv.URL + "/org/repo/pulls/10",
	})
	if err != nil {
		t.Fatalf("FetchResource PR: %v", err)
	}
	if card.Title != "#10 Add feature" || card.State != "open" || card.Author != "alice" {
		t.Errorf("unexpected card: %+v", card)
	}

	// Commit fetch
	card, err = p.FetchResource(context.Background(), ResourceRef{
		Kind:  ResourceCommit,
		Owner: "org",
		Repo:  "repo",
		ID:    "abc",
		URL:   srv.URL + "/org/repo/commit/abc",
	})
	if err != nil {
		t.Fatalf("FetchResource Commit: %v", err)
	}
	if card.Title != "Fix bug" || card.State != "commit" || card.Author != "Bob" {
		t.Errorf("unexpected commit card: %+v", card)
	}
}

func TestGiteaProvider_ParseWebhook(t *testing.T) {
	p, err := NewGitea("https://gitea.example.com", "")
	if err != nil {
		t.Fatalf("NewGitea: %v", err)
	}

	secret := "webhook-secret-123"
	payload := []byte(`{"action":"opened","number":15,"pull_request":{"title":"Cool PR","number":15,"user":{"login":"charlie"}},"repository":{"full_name":"org/repo"}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/hooks/gitea", bytes.NewReader(payload))
	req.Header.Set("X-Gitea-Event", "pull_request")
	req.Header.Set("X-Gitea-Delivery", "delivery-uuid-1")
	req.Header.Set("X-Gitea-Signature", sig)

	ev, err := p.ParseWebhook(req, secret)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if ev.DeliveryID != "delivery-uuid-1" || ev.Repo != "org/repo" || ev.Action != "opened" {
		t.Errorf("unexpected event: %+v", ev)
	}

	// Bad signature
	badReq := httptest.NewRequest(http.MethodPost, "/hooks/gitea", bytes.NewReader(payload))
	badReq.Header.Set("X-Gitea-Signature", "wrong")
	_, err = p.ParseWebhook(badReq, secret)
	if !errors.Is(err, ErrUnauthorizedWebhook) {
		t.Errorf("expected ErrUnauthorizedWebhook, got %v", err)
	}
}
