package guard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "plain command",
			in:   "go build ./...",
			want: "go build ./...",
		},
		{
			name: "env var secret",
			in:   "FOO=bar API_KEY=secret_12345 baz",
			want: "FOO=bar API_KEY=*** baz",
		},
		{
			name: "flag token",
			in:   "curl --token secret_tok_xyz https://example.com",
			want: "curl --token *** https://example.com",
		},
		{
			name: "flag password with equals",
			in:   "mysql --password=supersecret -u root",
			want: "mysql --password=*** -u root",
		},
		{
			name: "authorization header",
			in:   "curl -H 'Authorization: Bearer secret_bearer_token' http://api",
			want: "curl -H 'Authorization: ***' http://api",
		},
		{
			name: "url user pass",
			in:   "git clone https://admin:password123@github.com/org/repo.git",
			want: "git clone https://***@github.com/org/repo.git",
		},
		{
			name: "ghp token",
			in:   "push with ghp_1234567890abcdef1234567890abcdef",
			want: "push with ***",
		},
		{
			name: "sk token",
			in:   "openai key sk-ant-api03-abcdef123456789",
			want: "openai key ***",
		},
		{
			name: "colony token",
			in:   "colony token col_abcd1234efgh5678",
			want: "colony token ***",
		},
		{
			name: "aws akia",
			in:   "aws creds AKIAIOSFODNN7EXAMPLE for user",
			want: "aws creds *** for user",
		},
		{
			name: "jwt like",
			in:   "jwt eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c ok",
			want: "jwt *** ok",
		},
		{
			name: "long hex string",
			in:   "sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: "sha256: ***",
		},
		{
			name: "control characters",
			in:   "line1\nline2\x1b[31mred\x1b[0m\tline3\x00",
			want: "line1 line2 red line3",
		},
		{
			name: "cap at 80 characters",
			in:   "echo " + strings.Repeat("hello world ", 10),
			want: ("echo " + strings.Repeat("hello world ", 10))[:80],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Redact(tt.in)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRelPath(t *testing.T) {
	base1 := "/home/user/project"
	base2 := "/home/user/granted"

	if got := RelPath("/home/user/project/src/main.go", base1, base2); got != "src/main.go" {
		t.Errorf("expected src/main.go, got %q", got)
	}
	if got := RelPath("/home/user/granted/docs/readme.md", base1, base2); got != "docs/readme.md" {
		t.Errorf("expected docs/readme.md, got %q", got)
	}
	if got := RelPath("/etc/hosts", base1, base2); got != "hosts" {
		t.Errorf("expected hosts, got %q", got)
	}
	if got := RelPath("", base1); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestClaudeCodeStream(t *testing.T) {
	fixture, err := os.ReadFile("testdata/claude_stream.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var steps []Step
	cc := &ClaudeCode{
		Bin: "claude",
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return fixture, nil, nil
		},
	}

	req := DirectRequest{
		Prompt: "test direct",
		Dir:    "/workspace",
		OnStep: func(s Step) {
			steps = append(steps, s)
		},
	}

	res, err := cc.Direct(context.Background(), req)
	if err != nil {
		t.Fatalf("cc.Direct failed: %v", err)
	}

	if res.Text != "Build completed successfully." {
		t.Errorf("expected text 'Build completed successfully.', got %q", res.Text)
	}
	if res.SessionID != "sess_claude_123" {
		t.Errorf("expected session_id 'sess_claude_123', got %q", res.SessionID)
	}
	// Last assistant message usage: 1700 + 12000 + 0 = 13700
	if res.ContextTokens != 13700 {
		t.Errorf("expected ContextTokens 13700, got %d", res.ContextTokens)
	}

	// Verify steps: think, read (active/done), bash (active/done), answer
	if len(steps) < 4 {
		t.Fatalf("expected at least 4 step events, got %d", len(steps))
	}
	kinds := make([]string, len(steps))
	for i, s := range steps {
		kinds[i] = s.Kind
	}
	// Must contain think, read, command, answer
	foundThink, foundRead, foundCmd, foundAnswer := false, false, false, false
	for _, s := range steps {
		if s.Kind == StepKindThink {
			foundThink = true
		}
		if s.Kind == StepKindRead {
			foundRead = true
			if s.Label != "src/app.go" {
				t.Errorf("expected rel path 'src/app.go', got %q", s.Label)
			}
		}
		if s.Kind == StepKindCommand {
			foundCmd = true
			if strings.Contains(s.Label, "secret123") {
				t.Errorf("command label should be redacted, got %q", s.Label)
			}
		}
		if s.Kind == StepKindAnswer {
			foundAnswer = true
		}
	}
	if !foundThink || !foundRead || !foundCmd || !foundAnswer {
		t.Errorf("missing step kinds in %v", kinds)
	}
}

func TestClaudeCodeStreamMissingResult(t *testing.T) {
	lines := `{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}`
	cc := &ClaudeCode{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return []byte(lines), nil, nil
		},
	}
	_, err := cc.Direct(context.Background(), DirectRequest{Dir: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "missing result line") {
		t.Fatalf("expected missing result line error, got: %v", err)
	}
}

func TestAntigravityStream(t *testing.T) {
	fixture, err := os.ReadFile("testdata/antigravity_stream.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var steps []Step
	agy := &Antigravity{
		Bin: "agy",
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return fixture, nil, nil
		},
	}

	req := DirectRequest{
		Prompt: "test agy",
		Dir:    "/workspace",
		OnStep: func(s Step) {
			steps = append(steps, s)
		},
	}

	res, err := agy.Direct(context.Background(), req)
	if err != nil {
		t.Fatalf("agy.Direct failed: %v", err)
	}

	if res.Text != "Tests pass." {
		t.Errorf("expected 'Tests pass.', got %q", res.Text)
	}
	if res.ConversationID != "conv_agy_456" {
		t.Errorf("expected conversation_id 'conv_agy_456', got %q", res.ConversationID)
	}
	// Last agent_response DONE usage: 18000 + 14000 = 32000
	if res.ContextTokens != 32000 {
		t.Errorf("expected ContextTokens 32000, got %d", res.ContextTokens)
	}

	foundRead, foundCmd, foundAnswer := false, false, false
	for _, s := range steps {
		if s.Kind == StepKindRead {
			foundRead = true
			if s.Label != "main.go" {
				t.Errorf("expected rel path 'main.go', got %q", s.Label)
			}
		}
		if s.Kind == StepKindCommand {
			foundCmd = true
			if strings.Contains(s.Label, "super_secret_key") {
				t.Errorf("command label should be redacted, got %q", s.Label)
			}
		}
		if s.Kind == StepKindAnswer {
			foundAnswer = true
		}
	}
	if !foundRead || !foundCmd || !foundAnswer {
		t.Errorf("missing expected steps in antigravity run: %+v", steps)
	}
}

func TestAntigravityStreamErrorAfterAnswer(t *testing.T) {
	// Status ERROR with answer and output_tokens > 0: kept with ClientWarning
	lines := `{"event":"step_update","step_index":1,"state":"DONE","step_type":"agent_response","usage":{"input_tokens":1000,"output_tokens":50}}
{"event":"result","status":"ERROR","response":"I answered despite error","usage":{"input_tokens":1000,"output_tokens":50}}`
	agy := &Antigravity{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return []byte(lines), nil, nil
		},
	}
	res, err := agy.Direct(context.Background(), DirectRequest{Dir: "/workspace"})
	if err != nil {
		t.Fatalf("expected success with warning, got err: %v", err)
	}
	if res.Text != "I answered despite error" {
		t.Errorf("expected text preserved, got %q", res.Text)
	}
	if !strings.Contains(res.ClientWarning, "run status \"ERROR\" after an answer") {
		t.Errorf("expected warning, got %q", res.ClientWarning)
	}

	// Status ERROR with output_tokens == 0: rejected
	linesZero := `{"event":"result","status":"ERROR","response":"internal error crash","usage":{"input_tokens":1000,"output_tokens":0}}`
	agyZero := &Antigravity{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return []byte(linesZero), nil, nil
		},
	}
	_, err = agyZero.Direct(context.Background(), DirectRequest{Dir: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "run status \"ERROR\"") {
		t.Fatalf("expected error for zero output tokens, got: %v", err)
	}
}

func TestStreamHugeLine(t *testing.T) {
	hugeLine := fmt.Sprintf(`{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]}}`, strings.Repeat("x", 1024*1024))
	resultLine := `{"type":"result","result":"done","session_id":"s1","is_error":false,"usage":{"input_tokens":10,"output_tokens":10}}`
	payload := hugeLine + "\n" + resultLine + "\n"

	cc := &ClaudeCode{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return []byte(payload), nil, nil
		},
	}

	res, err := cc.Direct(context.Background(), DirectRequest{Dir: "/workspace"})
	if err != nil {
		t.Fatalf("stream with 1MB line failed: %v", err)
	}
	if res.Text != "done" {
		t.Errorf("expected text 'done', got %q", res.Text)
	}
}

// TestAntigravityStreamRealFormat replays output captured from agy 1.4.7,
// which nests each event's fields under the event name. Reading only the
// flat fields failed every direct run with `run status ""`.
func TestAntigravityStreamRealFormat(t *testing.T) {
	fixture, err := os.ReadFile("testdata/antigravity_stream_real.jsonl")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var steps []Step
	agy := &Antigravity{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			return fixture, nil, nil
		},
	}
	res, err := agy.Direct(context.Background(), DirectRequest{Dir: "/workspace", OnStep: func(s Step) { steps = append(steps, s) }})
	if err != nil {
		t.Fatalf("Direct: %v", err)
	}
	if res.Text != "alma\n" {
		t.Errorf("text = %q", res.Text)
	}
	if res.ConversationID != "aa1279cf-ac7e-4933-8fe6-fe852277a5e4" {
		t.Errorf("conversation id = %q", res.ConversationID)
	}
	if res.Usage.Input != 21477 || res.Usage.Output != 107 {
		t.Errorf("usage = %+v", res.Usage)
	}
	// The last model call's input is the real context.
	if res.ContextTokens != 7319 {
		t.Errorf("context tokens = %d", res.ContextTokens)
	}
	var read, cmd bool
	for _, s := range steps {
		if s.Kind == StepKindRead && s.Label == "note.txt" {
			read = true
		}
		if s.Kind == StepKindCommand && s.Label == "ls" {
			cmd = true
		}
	}
	if !read || !cmd {
		t.Errorf("missing read/command steps: %+v", steps)
	}
}
