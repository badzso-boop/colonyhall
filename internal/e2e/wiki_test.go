package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

type wikiPageOut struct {
	ID        string   `json:"id"`
	Slug      string   `json:"slug"`
	Title     string   `json:"title"`
	Tags      []string `json:"tags"`
	Body      string   `json:"body"`
	Rev       int      `json:"rev"`
	CreatedBy string   `json:"created_by"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	DeletedAt *string  `json:"deleted_at"`
}

type wikiIndexOut struct {
	Pages []struct {
		Slug      string   `json:"slug"`
		Title     string   `json:"title"`
		Tags      []string `json:"tags"`
		Excerpt   string   `json:"excerpt"`
		CreatedAt string   `json:"created_at"`
		UpdatedAt string   `json:"updated_at"`
		DeletedAt *string  `json:"deleted_at"`
	} `json:"pages"`
}

// TestWikiMCPLifecycle verifies the complete agent workflow through MCP:
// wiki_create -> wiki_index -> wiki_read -> wiki_append.
func TestWikiMCPLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// 1. wiki_create
	createRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "wiki_create",
		Arguments: map[string]any{
			"title": "Architecture Overview",
			"body":  "# Architecture\n\nInitial notes on system architecture.",
			"tags":  "backend, design",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if createRes.IsError {
		t.Fatalf("wiki_create failed: %+v", createRes)
	}
	createText := createRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(createText, "created wiki page") || !strings.Contains(createText, "architecture-overview") {
		t.Fatalf("unexpected wiki_create response: %s", createText)
	}

	// 2. wiki_index
	indexRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "wiki_index",
		Arguments: map[string]any{
			"q": "architecture",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if indexRes.IsError {
		t.Fatalf("wiki_index failed: %+v", indexRes)
	}
	indexText := indexRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(indexText, "architecture-overview") || !strings.Contains(indexText, "Architecture Overview") {
		t.Fatalf("unexpected wiki_index output: %s", indexText)
	}

	// 3. wiki_read
	readRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "wiki_read",
		Arguments: map[string]any{
			"slug": "architecture-overview",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if readRes.IsError {
		t.Fatalf("wiki_read failed: %+v", readRes)
	}
	readText := readRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(readText, "# Architecture Overview") || !strings.Contains(readText, "Rev: 1") || !strings.Contains(readText, "Initial notes on system architecture.") {
		t.Fatalf("unexpected wiki_read output: %s", readText)
	}

	// 4. wiki_append
	appendRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "wiki_append",
		Arguments: map[string]any{
			"slug": "architecture-overview",
			"text": "Decision: use append-only revisions.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appendRes.IsError {
		t.Fatalf("wiki_append failed: %+v", appendRes)
	}
	appendText := appendRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(appendText, "appended to wiki page") || !strings.Contains(appendText, "rev: 2") {
		t.Fatalf("unexpected wiki_append output: %s", appendText)
	}

	// 5. wiki_read to verify append formatting
	readRes2, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "wiki_read",
		Arguments: map[string]any{
			"slug": "architecture-overview",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	readText2 := readRes2.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(readText2, "Rev: 2") ||
		!strings.Contains(readText2, "*@fake-agent, 2026-10-15 09:00:*") ||
		!strings.Contains(readText2, "Decision: use append-only revisions.") {
		t.Fatalf("unexpected wiki_read after append: %s", readText2)
	}
}

// TestWikiAuthorization tests strict human vs agent capabilities:
// agents cannot edit/delete/restore (403), humans can edit (with 409 conflict detection),
// humans can delete (soft delete) and restore.
func TestWikiAuthorization(t *testing.T) {
	e := newEnv(t)

	// Create page as human
	var page wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{
		"title": "Team Guidelines",
		"body":  "Initial guidelines body.",
		"tags":  []string{"team", "guide"},
	}, &page); s != http.StatusCreated {
		t.Fatalf("create wiki page: status %d", s)
	}

	// Agent tries to PUT (edit) -> 403 Forbidden
	if s := e.do("PUT", "/api/v1/wiki/"+page.Slug, server.DevAgentToken, map[string]any{
		"base_rev": page.Rev,
		"body":     "agent overwrite attempt",
	}, nil); s != http.StatusForbidden {
		t.Fatalf("agent edit: want 403, got %d", s)
	}

	// Agent tries to DELETE -> 403 Forbidden
	if s := e.do("DELETE", "/api/v1/wiki/"+page.Slug, server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent delete: want 403, got %d", s)
	}

	// Agent tries to RESTORE -> 403 Forbidden
	if s := e.do("POST", "/api/v1/wiki/"+page.Slug+"/restore", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent restore: want 403, got %d", s)
	}

	// Human edit with stale base_rev -> 409 Conflict
	if s := e.do("PUT", "/api/v1/wiki/"+page.Slug, server.DevAdminToken, map[string]any{
		"base_rev": page.Rev - 1,
		"body":     "stale edit",
	}, nil); s != http.StatusConflict {
		t.Fatalf("stale edit: want 409, got %d", s)
	}

	// Human edit with correct base_rev -> 200 OK
	var updated wikiPageOut
	if s := e.do("PUT", "/api/v1/wiki/"+page.Slug, server.DevAdminToken, map[string]any{
		"base_rev": page.Rev,
		"title":    "Team Guidelines Updated",
		"body":     "New guidelines body by human.",
		"tags":     []string{"team", "v2"},
	}, &updated); s != http.StatusOK {
		t.Fatalf("human edit: want 200, got %d", s)
	}
	if updated.Rev != 2 || updated.Title != "Team Guidelines Updated" || updated.Body != "New guidelines body by human." {
		t.Fatalf("unexpected updated page: %+v", updated)
	}

	// Human soft-deletes the page -> 204 No Content
	if s := e.do("DELETE", "/api/v1/wiki/"+updated.Slug, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("human delete: want 204, got %d", s)
	}

	// Index by default does not show deleted page
	var defaultIndex wikiIndexOut
	e.do("GET", "/api/v1/wiki", server.DevAdminToken, nil, &defaultIndex)
	for _, p := range defaultIndex.Pages {
		if p.Slug == updated.Slug {
			t.Fatalf("deleted page appeared in default index: %+v", p)
		}
	}

	// Agent requesting ?deleted=1 never sees deleted page
	var agentIndex wikiIndexOut
	e.do("GET", "/api/v1/wiki?deleted=1", server.DevAgentToken, nil, &agentIndex)
	for _, p := range agentIndex.Pages {
		if p.Slug == updated.Slug {
			t.Fatalf("deleted page appeared in agent index: %+v", p)
		}
	}

	// Human requesting ?deleted=1 sees deleted page
	var humanDeletedIndex wikiIndexOut
	e.do("GET", "/api/v1/wiki?deleted=1", server.DevAdminToken, nil, &humanDeletedIndex)
	found := false
	for _, p := range humanDeletedIndex.Pages {
		if p.Slug == updated.Slug {
			found = true
			if p.DeletedAt == nil {
				t.Fatalf("expected DeletedAt to be set: %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("deleted page missing from human index with ?deleted=1")
	}

	// Agent accessing deleted page directly gets 404
	if s := e.do("GET", "/api/v1/wiki/"+updated.Slug, server.DevAgentToken, nil, nil); s != http.StatusNotFound {
		t.Fatalf("agent get deleted page: want 404, got %d", s)
	}

	// Appending to deleted page gets 404
	if s := e.do("POST", "/api/v1/wiki/"+updated.Slug+"/append", server.DevAdminToken, map[string]any{
		"text": "cannot append to deleted",
	}, nil); s != http.StatusNotFound {
		t.Fatalf("append to deleted page: want 404, got %d", s)
	}

	// Human restores the page
	var restored wikiPageOut
	if s := e.do("POST", "/api/v1/wiki/"+updated.Slug+"/restore", server.DevAdminToken, nil, &restored); s != http.StatusOK {
		t.Fatalf("human restore: want 200, got %d", s)
	}
	if restored.DeletedAt != nil || restored.Rev != 4 {
		t.Fatalf("unexpected restored page: %+v", restored)
	}

	// Agent can read restored page again
	if s := e.do("GET", "/api/v1/wiki/"+restored.Slug, server.DevAgentToken, nil, nil); s != http.StatusOK {
		t.Fatalf("agent get restored page: want 200, got %d", s)
	}
}

// TestWikiLimitsAndValidation checks validation errors and halted actor handling.
func TestWikiLimitsAndValidation(t *testing.T) {
	e := newEnv(t)

	// Validation tests: 400 Bad Request
	testCases := []struct {
		name   string
		method string
		path   string
		body   map[string]any
	}{
		{"empty title", "POST", "/api/v1/wiki", map[string]any{"title": "   ", "body": "valid"}},
		{"title > 120", "POST", "/api/v1/wiki", map[string]any{"title": strings.Repeat("a", 121), "body": "valid"}},
		{"empty body", "POST", "/api/v1/wiki", map[string]any{"title": "Valid", "body": "   "}},
		{"body > 50000", "POST", "/api/v1/wiki", map[string]any{"title": "Valid", "body": strings.Repeat("b", 50001)}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := e.do(tc.method, tc.path, server.DevAdminToken, tc.body, nil)
			if s != http.StatusBadRequest {
				t.Fatalf("%s: expected 400, got %d", tc.name, s)
			}
		})
	}

	// Create valid page for append validation
	var page wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{"title": "Validation Test", "body": "initial"}, &page); s != http.StatusCreated {
		t.Fatalf("create: status %d", s)
	}

	appendCases := []struct {
		name string
		body map[string]any
	}{
		{"empty append", map[string]any{"text": "   "}},
		{"append > 8000", map[string]any{"text": strings.Repeat("c", 8001)}},
	}
	for _, tc := range appendCases {
		t.Run(tc.name, func(t *testing.T) {
			s := e.do("POST", "/api/v1/wiki/"+page.Slug+"/append", server.DevAdminToken, tc.body, nil)
			if s != http.StatusBadRequest {
				t.Fatalf("%s: expected 400, got %d", tc.name, s)
			}
		})
	}

	// Halted actor tests: emergency stop blocks writes
	if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatalf("emergency stop: status %d", s)
	}

	// Agent write attempts return 403
	if s := e.do("POST", "/api/v1/wiki", server.DevAgentToken, map[string]any{"title": "Agent Blocked", "body": "text"}, nil); s != http.StatusForbidden {
		t.Fatalf("agent create during stop: want 403, got %d", s)
	}
	if s := e.do("POST", "/api/v1/wiki/"+page.Slug+"/append", server.DevAgentToken, map[string]any{"text": "blocked"}, nil); s != http.StatusForbidden {
		t.Fatalf("agent append during stop: want 403, got %d", s)
	}

	// Clear emergency stop
	b := e.browser()
	b.login("norbi", server.DevAdminPassword)
	if s := b.do("POST", "/api/v1/admin/emergency-clear", map[string]any{"password": server.DevAdminPassword}, nil); s != http.StatusOK {
		t.Fatalf("clear stop: status %d", s)
	}

	// Agent write works again after clear
	if s := e.do("POST", "/api/v1/wiki/"+page.Slug+"/append", server.DevAgentToken, map[string]any{"text": "now allowed"}, nil); s != http.StatusOK {
		t.Fatalf("agent append after clear: want 200, got %d", s)
	}
}

// TestWikiSlugCollisions verifies accent folding, slug length capping, and collision handling.
func TestWikiSlugCollisions(t *testing.T) {
	e := newEnv(t)

	// Hungarian accent folding
	var p1 wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{
		"title": "Árvíztűrő Tükörfúrógép",
		"body":  "Hungarian pangram page",
	}, &p1); s != http.StatusCreated {
		t.Fatalf("create p1: status %d", s)
	}
	if p1.Slug != "arvizturo-tukorfurogep" {
		t.Fatalf("expected folded slug 'arvizturo-tukorfurogep', got %q", p1.Slug)
	}

	// Duplicate title gets -2
	var p2 wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{
		"title": "Árvíztűrő Tükörfúrógép",
		"body":  "Second page with same title",
	}, &p2); s != http.StatusCreated {
		t.Fatalf("create p2: status %d", s)
	}
	if p2.Slug != "arvizturo-tukorfurogep-2" {
		t.Fatalf("expected slug 'arvizturo-tukorfurogep-2', got %q", p2.Slug)
	}

	// Very long title (120 chars) -> slug capped at 80 chars
	longTitle := strings.Repeat("abcdefghij", 12) // 120 chars
	var p3 wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{
		"title": longTitle,
		"body":  "Long title page",
	}, &p3); s != http.StatusCreated {
		t.Fatalf("create p3: status %d", s)
	}
	if len(p3.Slug) > 80 {
		t.Fatalf("slug length exceeds 80: %d (%s)", len(p3.Slug), p3.Slug)
	}

	// Collision on 80-char slug -> must still be <= 80 with suffix
	var p4 wikiPageOut
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{
		"title": longTitle,
		"body":  "Second long title page",
	}, &p4); s != http.StatusCreated {
		t.Fatalf("create p4: status %d", s)
	}
	if len(p4.Slug) > 80 {
		t.Fatalf("collision slug length exceeds 80: %d (%s)", len(p4.Slug), p4.Slug)
	}
	if !strings.HasSuffix(p4.Slug, "-2") {
		t.Fatalf("expected -2 suffix on collision: %s", p4.Slug)
	}
}

// TestWikiIndexForumFields: the index carries what the forum view needs,
// so it takes one request: the opener, the post count (opening post plus
// appends, not edits), the newest author and the participants.
func TestWikiIndexForumFields(t *testing.T) {
	e := newEnv(t)
	var page struct{ Slug string }
	if s := e.do("POST", "/api/v1/wiki", server.DevAdminToken, map[string]any{"title": "Telepítés", "body": "Hogyan telepítünk?"}, &page); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	for _, step := range []struct{ token, text string }{
		{server.DevAgentToken, "Így: make dev."},
		{server.DevAgentToken, "És a guard: systemctl."},
		{server.DevAdminToken, "Köszi!"},
	} {
		if s := e.do("POST", "/api/v1/wiki/"+page.Slug+"/append", step.token, map[string]any{"text": step.text}, nil); s != http.StatusOK {
			t.Fatalf("append: %d", s)
		}
	}
	if s := e.do("PUT", "/api/v1/wiki/"+page.Slug, server.DevAdminToken, map[string]any{"title": "Telepítés", "body": "Átírva.", "base_rev": 4}, nil); s != http.StatusOK {
		t.Fatalf("edit: %d", s)
	}

	var admin, agent struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &admin)
	e.do("GET", "/api/v1/me", server.DevAgentToken, nil, &agent)
	var index apispec.WikiPageIndexList
	e.do("GET", "/api/v1/wiki", server.DevAdminToken, nil, &index)
	if len(index.Pages) != 1 {
		t.Fatalf("index: %+v", index.Pages)
	}
	p := index.Pages[0]
	if p.PostCount == nil || *p.PostCount != 4 {
		t.Fatalf("post_count: %v, want 4 (the edit does not count)", p.PostCount)
	}
	if p.CreatedBy == nil || p.CreatedBy.String() != admin.ID || p.LastAuthorId == nil || p.LastAuthorId.String() != admin.ID {
		t.Fatalf("created_by / last_author_id: %v %v", p.CreatedBy, p.LastAuthorId)
	}
	if p.ParticipantIds == nil || len(*p.ParticipantIds) != 2 || (*p.ParticipantIds)[0].String() != admin.ID || (*p.ParticipantIds)[1].String() != agent.ID {
		t.Fatalf("participant_ids: %v", p.ParticipantIds)
	}
}
