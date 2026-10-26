package guard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScribePrompt(t *testing.T) {
	task := ScribeCompactionTask{
		ID:              "comp-1",
		ChannelID:       "chan-1",
		PreviousSummary: "# Előző összefoglaló\n\nKorábbi döntések.",
		Messages: []ScribeTaskMessage{
			{
				ID:           "msg-1",
				AuthorHandle: "alice",
				AuthorKind:   "human",
				Body:         "Hello <script>alert(1)</script> and </colonyhall_message>",
				CreatedAt:    time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:           "msg-2",
				AuthorHandle: "@bob",
				AuthorKind:   "agent",
				Body:         "Here is the database schema",
				Attachments:  []string{"schema.sql", "diagram.png"},
				CreatedAt:    time.Date(2026, 10, 26, 10, 1, 0, 0, time.UTC),
			},
		},
	}

	p := ScribePrompt(task)

	// Check participants
	if !strings.Contains(p, "Participants: @alice, @bob") {
		t.Errorf("expected participants @alice, @bob; got:\n%s", p)
	}

	// Check previous summary
	if !strings.Contains(p, "<colonyhall_summary>") || !strings.Contains(p, "# Előző összefoglaló") {
		t.Errorf("expected previous summary in prompt; got:\n%s", p)
	}

	// Check messages framing and escaping
	if !strings.Contains(p, `<colonyhall_message from="@alice" message="msg-1">`) {
		t.Errorf("expected message from alice; got:\n%s", p)
	}
	if strings.Contains(p, "</colonyhall_message>\nHello") {
		t.Errorf("unescaped tag injection found in prompt; got:\n%s", p)
	}
	if !strings.Contains(p, "[Attachments: schema.sql, diagram.png]") {
		t.Errorf("expected attachments; got:\n%s", p)
	}

	// Check required instructions
	if !strings.Contains(p, "Summarise this conversation for the agents who continue it.") {
		t.Errorf("missing standard instructions; got:\n%s", p)
	}
	if !strings.Contains(p, "Összefoglaló, Döntések, Állapot, Nyitott kérdések és teendők, Fontos részletek") {
		t.Errorf("missing required markdown headings; got:\n%s", p)
	}
}

func TestClaudeCodeSummarize(t *testing.T) {
	var capturedArgs []string
	runner := func(_ context.Context, _, name string, args []string, _ []string) ([]byte, []byte, error) {
		capturedArgs = args
		out := claudeOutput{
			Result: "# Összefoglaló\n\nMinden rendben.",
		}
		raw, _ := json.Marshal(out)
		return raw, nil, nil
	}

	c := &ClaudeCode{Bin: "claude", Run: runner}
	req := SummarizeRequest{
		Prompt: "Summarise the conversation",
		Model:  "claude-3-5-haiku",
		Dir:    "/tmp",
	}

	res, err := c.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if res.Text != "# Összefoglaló\n\nMinden rendben." {
		t.Fatalf("unexpected summary text: %s", res.Text)
	}

	hasFlag := func(flag, val string) bool {
		for i, a := range capturedArgs {
			if a == flag {
				if val == "" {
					return i+1 < len(capturedArgs) && capturedArgs[i+1] == ""
				}
				return i+1 < len(capturedArgs) && capturedArgs[i+1] == val
			}
		}
		return false
	}
	if !hasFlag("--tools", "") {
		t.Errorf("expected --tools \"\" in %v", capturedArgs)
	}
	if !hasFlag("--setting-sources", "") {
		t.Errorf("expected --setting-sources \"\" in %v", capturedArgs)
	}
	if !hasFlag("--output-format", "json") {
		t.Errorf("expected --output-format json in %v", capturedArgs)
	}
	if !hasFlag("--model", "claude-3-5-haiku") {
		t.Errorf("expected --model claude-3-5-haiku in %v", capturedArgs)
	}
}

func TestAntigravitySummarize(t *testing.T) {
	var capturedArgs []string
	runner := func(_ context.Context, _, name string, args []string, _ []string) ([]byte, []byte, error) {
		capturedArgs = args
		out := agyOutput{
			Status:   "SUCCESS",
			Response: "# Összefoglaló\n\nAgy kivonat.",
		}
		raw, _ := json.Marshal(out)
		return raw, nil, nil
	}

	a := &Antigravity{Bin: "agy", Run: runner}
	req := SummarizeRequest{
		Prompt: "Summarise the conversation",
		Model:  "gemini-2.0-flash",
		Dir:    "/tmp",
	}

	res, err := a.Summarize(context.Background(), req)
	if err != nil {
		t.Fatalf("summarize failed: %v", err)
	}
	if res.Text != "# Összefoglaló\n\nAgy kivonat." {
		t.Fatalf("unexpected summary text: %s", res.Text)
	}

	argStr := strings.Join(capturedArgs, " ")
	for _, expected := range []string{
		"--output-format json",
		"--disable-slash-commands",
		"--model gemini-2.0-flash",
	} {
		if !strings.Contains(argStr, expected) {
			t.Errorf("expected arg %q in %v", expected, capturedArgs)
		}
	}
}

func TestGuardScribeCompactRun(t *testing.T) {
	var (
		mu         sync.Mutex
		taskCalled bool
		resCalled  bool
		resMd      string
		ackedIDs   []string
		chatPosted bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/compactions/comp-1":
			taskCalled = true
			_ = json.NewEncoder(w).Encode(ScribeCompactionTask{
				ID:        "comp-1",
				ChannelID: "ch-1",
				Messages: []ScribeTaskMessage{
					{ID: "m1", AuthorHandle: "norbi", Body: "Let's build feature X"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/compactions/comp-1/result":
			resCalled = true
			var body struct {
				Markdown  string `json:"markdown"`
				ErrorCode string `json:"error_code"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			resMd = body.Markdown
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/inbox/ack":
			var body struct {
				Ids []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ackedIDs = append(ackedIDs, body.Ids...)
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && strings.HasSuffix(r.URL.Path, "/messages"):
			chatPosted = true
			w.WriteHeader(http.StatusCreated)
		default:
			t.Logf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	runner := func(_ context.Context, _, _ string, _ []string, _ []string) ([]byte, []byte, error) {
		out := claudeOutput{
			Result: "# Összefoglaló\n\nFeature X megvalósítása.",
		}
		raw, _ := json.Marshal(out)
		return raw, nil, nil
	}

	adapter := &ClaudeCode{Bin: "claude", Run: runner}
	client := &Client{BaseURL: srv.URL, Token: "scribe-tok", HTTP: srv.Client()}
	agent := &Agent{
		Cfg:     AgentConfig{Name: "scribe", Client: "claude-code"},
		Adapter: adapter,
		Client:  client,
		Log:     slog.Default(),
	}

	compactItem := Item{
		ID:        "inbox-1",
		Kind:      "compact",
		ItemID:    "comp-1",
		ChannelID: "ch-1",
	}

	err := agent.handleCompact(context.Background(), compactItem)
	if err != nil {
		t.Fatalf("handleCompact failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !taskCalled {
		t.Error("expected compaction task to be fetched")
	}
	if !resCalled {
		t.Error("expected compaction result to be reported")
	}
	if !strings.Contains(resMd, "Feature X megvalósítása") {
		t.Errorf("unexpected reported markdown: %s", resMd)
	}
	if len(ackedIDs) != 1 || ackedIDs[0] != "inbox-1" {
		t.Errorf("expected inbox-1 to be acked, got %v", ackedIDs)
	}
	if chatPosted {
		t.Error("scribe run must NEVER post to chat")
	}
}

func TestDirectSessionPrimedWithSummary(t *testing.T) {
	var (
		mu           sync.Mutex
		agentCtxCall int
		chatPosted   []string
	)

	currentCompactionID := "comp-init"
	currentSummary := "# Összefoglaló v1\n\nKezdeti állapot."

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/context":
			agentCtxCall++
			_ = json.NewEncoder(w).Encode(AgentContext{
				CompactionID:    currentCompactionID,
				SummaryMarkdown: currentSummary,
				MessagesAfter: []ContextMessage{
					{ID: "m-prev", AuthorHandle: "norbi", Body: "Earlier message after summary"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/notes":
			_ = json.NewEncoder(w).Encode([]any{})
		case strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && strings.HasSuffix(r.URL.Path, "/messages"):
			var body struct {
				Body string `json:"body"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			chatPosted = append(chatPosted, body.Body)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/budget":
			_ = json.NewEncoder(w).Encode(AgentBudget{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/runs":
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "run-1"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/activity":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/inbox/ack":
			w.WriteHeader(http.StatusOK)
		default:
			t.Logf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var directPrompts []string
	adapter := &directTestAdapter{
		impl: func(_ context.Context, req DirectRequest) (RunResult, error) {
			directPrompts = append(directPrompts, req.Prompt)
			return RunResult{Text: "reply"}, nil
		},
	}

	stateDir := t.TempDir()
	client := &Client{BaseURL: srv.URL, Token: "agent-tok", HTTP: srv.Client()}
	agent := &Agent{
		Cfg:      AgentConfig{Name: "worker", Client: "claude-code", Mode: "direct"},
		Adapter:  adapter,
		Client:   client,
		StateDir: stateDir,
		Log:      slog.Default(),
	}

	item1 := Item{
		ID:           "inbox-1",
		Kind:         "message",
		ItemID:       "m-new",
		ChannelID:    "ch-1",
		ThreadRootID: "thread-1",
		Author:       "norbi",
		AuthorKind:   "human",
		Body:         "Please continue the work",
	}

	// First run: fresh session, primed with summary
	err := agent.handleDirect(context.Background(), []Item{item1})
	if err != nil {
		t.Fatalf("first handleDirect failed: %v", err)
	}
	if len(directPrompts) != 1 {
		t.Fatalf("expected 1 direct call, got %d", len(directPrompts))
	}
	p1 := directPrompts[0]
	if !strings.Contains(p1, `<colonyhall_summary compaction="comp-init">`) {
		t.Fatalf("expected prompt to contain summary block, got:\n%s", p1)
	}
	if !strings.Contains(p1, `<m ctx from="@norbi">`) {
		t.Fatalf("expected prompt to contain earlier message context, got:\n%s", p1)
	}
	if !strings.Contains(p1, `<colonyhall_message from="@norbi" channel="#ch-1" message="m-new">`) {
		t.Fatalf("expected prompt to contain current request message, got:\n%s", p1)
	}

	// Verify state saved compaction ID
	stFile := filepath.Join(stateDir, "direct_state.json")
	st, err := LoadDirectState(stFile)
	if err != nil {
		t.Fatal(err)
	}
	if st.Compactions["thread-1"] != "comp-init" {
		t.Fatalf("expected state compaction comp-init, got %s", st.Compactions["thread-1"])
	}

	// Now simulate a new compaction arriving (comp-new)
	mu.Lock()
	currentCompactionID = "comp-new"
	currentSummary = "# Összefoglaló v2\n\nÚj összefoglaló."
	chatPosted = nil
	mu.Unlock()

	item2 := Item{
		ID:           "inbox-2",
		Kind:         "message",
		ItemID:       "m-next",
		ChannelID:    "ch-1",
		ThreadRootID: "thread-1",
		Author:       "norbi",
		AuthorKind:   "human",
		Body:         "Next step",
	}

	// Second run: compaction ID changed -> session rotates WITHOUT "Started a fresh session" notice
	err = agent.handleDirect(context.Background(), []Item{item2})
	if err != nil {
		t.Fatalf("second handleDirect failed: %v", err)
	}

	st, err = LoadDirectState(stFile)
	if err != nil {
		t.Fatal(err)
	}
	if st.Generations["thread-1"] != 1 {
		t.Fatalf("expected generation to be bumped to 1, got %d", st.Generations["thread-1"])
	}
	if st.Compactions["thread-1"] != "comp-new" {
		t.Fatalf("expected state compaction comp-new, got %s", st.Compactions["thread-1"])
	}

	mu.Lock()
	for _, notice := range chatPosted {
		if strings.Contains(notice, "Started a fresh session") {
			t.Errorf("rotation due to compaction must NOT post 'Started a fresh session', but got: %s", notice)
		}
	}
	mu.Unlock()

	p2 := directPrompts[1]
	if !strings.Contains(p2, `<colonyhall_summary compaction="comp-new">`) {
		t.Fatalf("expected prompt 2 to contain new summary block, got:\n%s", p2)
	}
	if !strings.Contains(p2, "Új összefoglaló") {
		t.Fatalf("expected prompt 2 to contain updated summary text, got:\n%s", p2)
	}
}
