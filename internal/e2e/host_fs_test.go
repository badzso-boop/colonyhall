package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/apispec"
	"colonyhall/internal/guard"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// hostWS opens /ws/host with token and returns the connection after the
// welcome frame.
func (e *env) hostWS(ctx context.Context, token string) (*websocket.Conn, map[string]any) {
	e.t.Helper()
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/host", nil)
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = ws.CloseNow() })
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": token}); err != nil {
		e.t.Fatal(err)
	}
	var welcome map[string]any
	if err := wsjson.Read(ctx, ws, &welcome); err != nil {
		e.t.Fatalf("welcome: %v", err)
	}
	return ws, welcome
}

// waitConnected waits until the gateway registered the host's socket (the
// welcome is written just before registration).
func (e *env) waitHostConnected(hostID string) {
	e.t.Helper()
	id, err := ids.Parse(hostID)
	if err != nil {
		e.t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e.app.HostGateway.Connected(id) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("host %s did not connect", hostID)
}

type problemOut struct {
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

type fsCall struct {
	code int
	out  apispec.HostDirListing
	prob problemOut
}

func (e *env) hostFsAsync(hostID, token string, body map[string]any) chan fsCall {
	ch := make(chan fsCall, 1)
	go func() {
		var raw struct {
			apispec.HostDirListing
			problemOut
		}
		code := e.do("POST", "/api/v1/hosts/"+hostID+"/fs", token, body, &raw)
		ch <- fsCall{code: code, out: raw.HostDirListing, prob: raw.problemOut}
	}()
	return ch
}

func TestHostFsRelayAuthorization(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hostA, tokenA := e.enroll("box-a")
	hostB, tokenB := e.enroll("box-b")

	admin, _ := e.app.Identity.Authenticate(ctx, server.DevAdminToken)
	_, memberToken, err := e.app.Identity.CreateActor(ctx, admin, identity.NewActorInput{
		Kind: identity.KindHuman, Handle: "member", DisplayName: "Member", Role: identity.RoleMember})
	if err != nil {
		t.Fatal(err)
	}
	list := map[string]any{"op": "list_dirs"}

	// Only human admins: a member and an agent are refused.
	for name, tok := range map[string]string{"member": memberToken, "agent": server.DevAgentToken} {
		if c := e.do("POST", "/api/v1/hosts/"+hostA+"/fs", tok, list, nil); c != http.StatusForbidden {
			t.Fatalf("%s: got %d, want 403", name, c)
		}
	}
	// A host token is not a user token.
	if c := e.do("POST", "/api/v1/hosts/"+hostA+"/fs", tokenA, list, nil); c != http.StatusUnauthorized {
		t.Fatalf("host token on the admin route: got %d, want 401", c)
	}
	if c := e.do("POST", "/api/v1/hosts/"+hostA+"/fs", server.DevAdminToken, map[string]any{"op": "list"}, nil); c != http.StatusBadRequest {
		t.Fatalf("bad op: got %d, want 400", c)
	}
	if c := e.do("POST", "/api/v1/hosts/"+hostA+"/fs", server.DevAdminToken, map[string]any{"op": "list_dirs", "path": "relative/../x"}, nil); c != http.StatusBadRequest {
		t.Fatalf("bad path: got %d, want 400", c)
	}

	// No host socket: host_offline.
	var prob problemOut
	if c := e.do("POST", "/api/v1/hosts/"+hostA+"/fs", server.DevAdminToken, list, &prob); c != http.StatusConflict || prob.Detail != "host_offline" {
		t.Fatalf("offline: got %d %+v, want 409 host_offline", c, prob)
	}

	// The host socket accepts only host tokens.
	bad, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/host", nil)
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	_ = wsjson.Write(ctx, bad, map[string]any{"type": "hello", "token": server.DevAgentToken})
	var frame map[string]any
	if err := wsjson.Read(ctx, bad, &frame); err == nil {
		t.Fatalf("agent token on /ws/host was accepted: %+v", frame)
	}
	_ = bad.CloseNow()

	wsA, welcome := e.hostWS(ctx, tokenA)
	if welcome["type"] != "welcome" || welcome["host_id"] != hostA {
		t.Fatalf("welcome: %+v", welcome)
	}
	wsB, _ := e.hostWS(ctx, tokenB)
	e.waitHostConnected(hostA)
	e.waitHostConnected(hostB)

	path := "/srv/projects"
	call := e.hostFsAsync(hostA, server.DevAdminToken, map[string]any{"op": "list_dirs", "path": path})
	var req map[string]any
	if err := wsjson.Read(ctx, wsA, &req); err != nil {
		t.Fatal(err)
	}
	reqID, _ := req["request_id"].(string)
	if req["type"] != "host.fs.request" || reqID == "" {
		t.Fatalf("frame: %+v", req)
	}
	// Host B's socket did not get the frame.
	readCtx, readCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	if err := wsjson.Read(readCtx, wsB, &frame); err == nil {
		t.Fatalf("host B got a frame of host A: %+v", frame)
	}
	readCancel()

	// Another host can neither read nor answer the request.
	if c := e.do("GET", "/api/v1/host/fs/"+reqID, tokenB, nil, nil); c != http.StatusNotFound {
		t.Fatalf("host B reads A's request: %d", c)
	}
	if c := e.do("POST", "/api/v1/host/fs/"+reqID+"/result", tokenB, map[string]any{"op": "list_dirs", "entries": []any{}}, nil); c != http.StatusNotFound {
		t.Fatalf("host B answers A's request: %d", c)
	}
	// Users cannot use the guard routes.
	if c := e.do("GET", "/api/v1/host/fs/"+reqID, server.DevAdminToken, nil, nil); c != http.StatusUnauthorized {
		t.Fatalf("admin on the guard route: %d", c)
	}

	var pending apispec.HostFsPendingRequest
	if c := e.do("GET", "/api/v1/host/fs/"+reqID, tokenA, nil, &pending); c != http.StatusOK || pending.Path == nil || *pending.Path != path || pending.Op != apispec.HostFsPendingRequestOpListDirs {
		t.Fatalf("host A reads its request: %d %+v", c, pending)
	}
	if c := e.do("POST", "/api/v1/host/fs/"+reqID+"/result", tokenA, map[string]any{
		"op": "list_dirs", "path": path,
		"entries": []map[string]any{{"name": "colonyhall", "path": path + "/colonyhall", "is_git_repo": true, "has_children": true}},
	}, nil); c != http.StatusNoContent {
		t.Fatalf("host A answers: %d", c)
	}
	got := <-call
	if got.code != http.StatusOK || len(got.out.Entries) != 1 || got.out.Entries[0].Name != "colonyhall" {
		t.Fatalf("admin got %d %+v", got.code, got.out)
	}
	if c := e.do("POST", "/api/v1/host/fs/"+reqID+"/result", tokenA, map[string]any{"op": "list_dirs", "entries": []any{}}, nil); c != http.StatusGone {
		t.Fatalf("second answer: got %d, want 410", c)
	}

	// Audited without the path.
	events, err := e.app.Events.Since(ctx, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	audited := 0
	for _, ev := range events {
		if ev.Type == "host.fs_requested" {
			audited++
			if strings.Contains(string(ev.Payload), "srv") || !strings.Contains(string(ev.Payload), hostA) {
				t.Fatalf("audit payload: %s", ev.Payload)
			}
		}
	}
	if audited == 0 {
		t.Fatal("no host.fs_requested event")
	}

	// Timeout: 504, and a late answer is 410.
	e.app.HostFs.Timeout = 50 * time.Millisecond
	call = e.hostFsAsync(hostA, server.DevAdminToken, list)
	if err := wsjson.Read(ctx, wsA, &req); err != nil {
		t.Fatal(err)
	}
	if got := <-call; got.code != http.StatusGatewayTimeout {
		t.Fatalf("timeout: got %d, want 504", got.code)
	}
	if c := e.do("POST", "/api/v1/host/fs/"+req["request_id"].(string)+"/result", tokenA, map[string]any{"op": "list_dirs", "entries": []any{}}, nil); c != http.StatusGone {
		t.Fatalf("late answer: got %d, want 410", c)
	}

	// A revoked host is offline even with an open socket.
	if c := e.do("PUT", "/api/v1/hosts/"+hostB+"/state", server.DevAdminToken, map[string]any{"state": "revoked"}, nil); c != http.StatusNoContent {
		t.Fatalf("revoke: %d", c)
	}
	if c := e.do("POST", "/api/v1/hosts/"+hostB+"/fs", server.DevAdminToken, list, &prob); c != http.StatusConflict || prob.Detail != "host_offline" {
		t.Fatalf("revoked host: got %d %+v", c, prob)
	}
}

// TestHostFsRelayWithGuard runs the guard's host WebSocket against the
// server: an admin lists the roots and a folder of a temporary tree.
func TestHostFsRelayWithGuard(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hostID, token := e.enroll("guard-box")

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"repo/.git", "docs", ".ssh"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	hw := &guard.HostWS{
		Client:  &guard.Client{BaseURL: e.srv.URL, Token: token, HTTP: &http.Client{Timeout: 5 * time.Second}},
		Scope:   guard.HostFsScope{Roots: []string{base}},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Backoff: func(int) time.Duration { return 10 * time.Millisecond },
	}
	go hw.Run(ctx)
	e.waitHostConnected(hostID)

	got := <-e.hostFsAsync(hostID, server.DevAdminToken, map[string]any{"op": "list_dirs"})
	if got.code != http.StatusOK || len(got.out.Entries) != 1 || got.out.Entries[0].Path != base {
		t.Fatalf("roots: %d %+v", got.code, got.out)
	}
	got = <-e.hostFsAsync(hostID, server.DevAdminToken, map[string]any{"op": "list_dirs", "path": base})
	if got.code != http.StatusOK || len(got.out.Entries) != 3 {
		t.Fatalf("list: %d %+v", got.code, got.out)
	}
	m := map[string]apispec.HostDirEntry{}
	for _, en := range got.out.Entries {
		m[en.Name] = en
	}
	if r := m["repo"]; r.IsGitRepo == nil || !*r.IsGitRepo {
		t.Errorf("repo: %+v", r)
	}
	if s := m[".ssh"]; s.Denied == nil || !*s.Denied {
		t.Errorf(".ssh: %+v", s)
	}
	got = <-e.hostFsAsync(hostID, server.DevAdminToken, map[string]any{"op": "list_dirs", "path": filepath.Dir(base)})
	if got.code != http.StatusOK || got.out.ErrorCode == nil || *got.out.ErrorCode != apispec.HostDirListingErrorCodeDenied {
		t.Fatalf("outside the roots: %d %+v", got.code, got.out)
	}
}
