// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Round 54: "My files", deleting a file, the workspace storage, its quota
// and warnings.

// storageFixture: the owner (the dev admin token, @norbi), an admin, a
// member colleague and an API-key agent the colleague owns, all in #general.
type storageFixture struct {
	e                      *env
	general                string
	adminID, adminTok      string
	colleagueID, colleague string
	agentID, agentTok      string
	otherAgent, otherAgTok string
}

func newStorageFixture(t *testing.T) storageFixture {
	e := newEnv(t)
	f := storageFixture{e: e, general: e.generalID()}
	f.adminID, f.adminTok = e.createActor("human", "admin2", "admin")
	f.colleagueID, f.colleague = e.createActor("human", "kollega", "member")
	var created struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken,
		map[string]any{"handle": "kollega-bot", "api_key": true, "owner_id": f.colleagueID}, &created); s != http.StatusCreated {
		t.Fatalf("create owned agent: %d", s)
	}
	f.agentID, f.agentTok = created.Actor.ID, created.Token
	f.otherAgent, f.otherAgTok = e.createActor("agent", "nobodys-bot", "member")
	for _, id := range []string{f.adminID, f.colleagueID, f.agentID, f.otherAgent} {
		e.do("POST", "/api/v1/channels/"+f.general+"/members", server.DevAdminToken, map[string]any{"actor_id": id}, nil)
	}
	return f
}

func (f storageFixture) upload(token, channel, name string, content []byte) *apispec.File {
	f.e.t.Helper()
	st, file := uploadMultipart(f.e.t, f.e, channel, name, content, token)
	if st != http.StatusCreated {
		f.e.t.Fatalf("upload %s: %d", name, st)
	}
	return file
}

func (f storageFixture) myFiles(token, query string) apispec.MyFilePage {
	f.e.t.Helper()
	var page apispec.MyFilePage
	if s := f.e.do("GET", "/api/v1/me/files"+query, token, nil, &page); s != http.StatusOK {
		f.e.t.Fatalf("my files %s: %d", query, s)
	}
	return page
}

func TestMyFilesAndOnlyTheUploaderDeletes(t *testing.T) {
	f := newStorageFixture(t)
	e := f.e
	doc := f.upload(f.colleague, f.general, "notes.txt", []byte("colleague notes"))
	img := f.upload(f.colleague, f.general, "pixel.png", []byte("\x89PNG\r\n\x1a\nsomething"))
	var sent apispec.SendResult
	if s := e.do("POST", "/api/v1/channels/"+f.general+"/messages", f.colleague,
		apispec.SendMessageRequest{Body: "here", FileIds: &[]ids.ID{doc.Id}}, &sent); s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}

	// The colleague's own files, newest first, with the message and channel.
	page := f.myFiles(f.colleague, "")
	if len(page.Files) != 2 || page.Files[0].Id != img.Id || page.Files[1].Id != doc.Id {
		t.Fatalf("my files = %+v", page.Files)
	}
	if m := page.Files[1].MessageId; m == nil || *m != sent.Message.Id {
		t.Fatalf("message link = %v", m)
	}
	if c := page.Files[1].Channel; c == nil || c.Name != "general" {
		t.Fatalf("channel = %+v", c)
	}
	if page.Files[0].UploadedBy != nil {
		t.Fatal("uploader is only listed for the agents' files")
	}
	if p := f.myFiles(f.colleague, "?kind=image"); len(p.Files) != 1 || p.Files[0].Id != img.Id {
		t.Fatalf("kind filter: %+v", p.Files)
	}
	if p := f.myFiles(f.colleague, "?limit=1"); len(p.Files) != 1 || p.NextCursor == nil {
		t.Fatalf("first page: %+v", p)
	} else if p2 := f.myFiles(f.colleague, "?limit=1&cursor="+*p.NextCursor); len(p2.Files) != 1 || p2.Files[0].Id != doc.Id {
		t.Fatalf("second page: %+v", p2.Files)
	}
	// The owner never sees who uploaded what: their own list is empty.
	if p := f.myFiles(server.DevAdminToken, ""); len(p.Files) != 0 {
		t.Fatalf("the owner's list shows others' files: %+v", p.Files)
	}
	if s := e.do("GET", "/api/v1/me/files", f.agentTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agents have no My files: %d", s)
	}
	if s := e.do("GET", "/api/v1/me/files?kind=video", f.colleague, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("bad kind: %d", s)
	}

	// Only the uploader deletes: not the owner, not an admin, not an agent.
	for name, tok := range map[string]string{"owner": server.DevAdminToken, "admin": f.adminTok, "agent": f.agentTok} {
		if s := e.do("DELETE", "/api/v1/files/"+doc.Id.String(), tok, nil, nil); s != http.StatusForbidden {
			t.Fatalf("%s deleted another person's file: %d", name, s)
		}
	}
	if s := e.do("DELETE", "/api/v1/files/"+doc.Id.String(), f.colleague, nil, nil); s != http.StatusNoContent {
		t.Fatalf("uploader delete: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/files/"+doc.Id.String(), f.colleague, nil, nil); s != http.StatusGone {
		t.Fatalf("second delete: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/files/"+ids.V7{}.New().String(), f.colleague, nil, nil); s != http.StatusNotFound {
		t.Fatalf("unknown file: %d", s)
	}

	// The download is gone; the message shows "File deleted" without a name.
	if s := e.do("GET", "/api/v1/files/"+doc.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusGone {
		t.Fatalf("metadata of a deleted file: %d", s)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+doc.Id.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+server.DevAdminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	e.checkContract(req, res, raw)
	if res.StatusCode != http.StatusGone {
		t.Fatalf("download of a deleted file: %d", res.StatusCode)
	}
	var list apispec.MessageList
	e.do("GET", "/api/v1/channels/"+f.general+"/messages", server.DevAdminToken, nil, &list)
	found := false
	for _, m := range list.Messages {
		if m.Id == sent.Message.Id {
			found = true
			if m.Attachments == nil || len(*m.Attachments) != 1 {
				t.Fatalf("attachments = %+v", m.Attachments)
			}
			a := (*m.Attachments)[0]
			if a.Deleted == nil || !*a.Deleted || a.Name != "" || a.Size != 0 || a.Mime != "" {
				t.Fatalf("deleted attachment = %+v", a)
			}
		}
	}
	if !found {
		t.Fatal("message not listed")
	}
	if p := f.myFiles(f.colleague, ""); len(p.Files) != 1 || p.Files[0].Id != img.Id {
		t.Fatalf("my files after delete: %+v", p.Files)
	}
	// A deleted file cannot be attached again.
	if s := e.do("POST", "/api/v1/channels/"+f.general+"/messages", f.colleague,
		apispec.SendMessageRequest{Body: "again", FileIds: &[]ids.ID{doc.Id}}, nil); s == http.StatusCreated {
		t.Fatal("a deleted file was attached")
	}
}

func TestMyAgentsFilesBelongToTheirOwner(t *testing.T) {
	f := newStorageFixture(t)
	e := f.e
	mine := f.upload(f.agentTok, f.general, "agent-report.txt", []byte("written by my agent"))
	nobodys := f.upload(f.otherAgTok, f.general, "orphan.txt", []byte("written by nobody's agent"))

	page := f.myFiles(f.colleague, "?uploader=agents")
	if len(page.Files) != 1 || page.Files[0].Id != mine.Id || page.Files[0].UploadedBy == nil || page.Files[0].UploadedBy.String() != f.agentID {
		t.Fatalf("my agents' files = %+v", page.Files)
	}
	if p := f.myFiles(f.colleague, ""); len(p.Files) != 0 {
		t.Fatalf("the agent's files are not the colleague's own: %+v", p.Files)
	}
	// The workspace owner does not own the colleague's agent.
	if p := f.myFiles(server.DevAdminToken, "?uploader=agents"); len(p.Files) != 0 {
		t.Fatalf("owner sees the colleague's agent's files: %+v", p.Files)
	}
	if s := e.do("DELETE", "/api/v1/files/"+mine.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("owner deleted the colleague's agent's file: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/files/"+nobodys.Id.String(), f.colleague, nil, nil); s != http.StatusForbidden {
		t.Fatalf("colleague deleted nobody's agent's file: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/files/"+mine.Id.String(), f.colleague, nil, nil); s != http.StatusNoContent {
		t.Fatalf("agent owner delete: %d", s)
	}
	// The agent itself may delete what it uploaded.
	if s := e.do("DELETE", "/api/v1/files/"+nobodys.Id.String(), f.otherAgTok, nil, nil); s != http.StatusNoContent {
		t.Fatalf("uploading agent delete: %d", s)
	}
}

func TestMyFilesHideAChannelILeft(t *testing.T) {
	f := newStorageFixture(t)
	e := f.e
	var priv apispec.Channel
	kind := apispec.Private
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{Name: "secret-room", Kind: &kind}, &priv)
	e.do("POST", "/api/v1/channels/"+priv.Id.String()+"/members", server.DevAdminToken, map[string]any{"actor_id": f.colleagueID}, nil)
	file := f.upload(f.colleague, priv.Id.String(), "plan.txt", []byte("the plan"))
	e.do("POST", "/api/v1/channels/"+priv.Id.String()+"/messages", f.colleague,
		apispec.SendMessageRequest{Body: "plan", FileIds: &[]ids.ID{file.Id}}, nil)
	if p := f.myFiles(f.colleague, ""); len(p.Files) != 1 || p.Files[0].Channel == nil || p.Files[0].MessageId == nil {
		t.Fatalf("before leaving: %+v", p.Files)
	}
	if s := e.do("DELETE", "/api/v1/channels/"+priv.Id.String()+"/members/"+f.colleagueID, server.DevAdminToken, nil, nil); s >= 300 {
		t.Fatalf("remove member: %d", s)
	}
	p := f.myFiles(f.colleague, "")
	if len(p.Files) != 1 || p.Files[0].Channel != nil || p.Files[0].MessageId != nil || p.Files[0].ThreadRootId != nil {
		t.Fatalf("after leaving the channel the file shows it: %+v", p.Files)
	}
	// It is still theirs to delete.
	if s := e.do("DELETE", "/api/v1/files/"+file.Id.String(), f.colleague, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete after leaving: %d", s)
	}
}

// storageDMs returns the bodies the storage bot wrote to the owner.
func (f storageFixture) storageDMs() []string {
	f.e.t.Helper()
	var actors struct {
		Actors []struct{ ID, Handle string }
	}
	f.e.do("GET", "/api/v1/actors", server.DevAdminToken, nil, &actors)
	bot := ""
	for _, a := range actors.Actors {
		if a.Handle == "colonyhall-storage" {
			bot = a.ID
		}
	}
	if bot == "" {
		return nil
	}
	var dm apispec.Channel
	f.e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{bot}}, &dm)
	var list apispec.MessageList
	f.e.do("GET", "/api/v1/channels/"+dm.Id.String()+"/messages", server.DevAdminToken, nil, &list)
	var out []string
	for _, m := range list.Messages {
		out = append(out, m.Body)
	}
	return out
}

func (f storageFixture) storage(token string) apispec.WorkspaceStorage {
	f.e.t.Helper()
	var st apispec.WorkspaceStorage
	if s := f.e.do("GET", "/api/v1/workspace/storage", token, nil, &st); s != http.StatusOK {
		f.e.t.Fatalf("storage: %d", s)
	}
	return st
}

func bytesOf(n int, b byte) []byte { return bytes.Repeat([]byte{b}, n) }

func TestStorageQuotaWarningsAndFull(t *testing.T) {
	f := newStorageFixture(t)
	e := f.e
	// Only the owner and admins see the storage; only the owner sets the quota.
	if s := e.do("GET", "/api/v1/workspace/storage", f.colleague, nil, nil); s != http.StatusForbidden {
		t.Fatalf("member reads storage: %d", s)
	}
	if st := f.storage(f.adminTok); st.QuotaBytes != 0 || st.State != apispec.StorageStateOk {
		t.Fatalf("default: %+v", st)
	}
	if s := e.do("PUT", "/api/v1/workspace/storage/quota", f.adminTok, map[string]any{"quota_bytes": 1000}, nil); s != http.StatusForbidden {
		t.Fatalf("admin sets quota: %d", s)
	}
	if s := e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": -1}, nil); s != http.StatusBadRequest {
		t.Fatalf("negative quota: %d", s)
	}
	// Without a quota nothing is ever full (the memory store has no
	// database share, so the files are the whole usage here).
	f.upload(f.colleague, f.general, "big.bin", bytesOf(700, 'a'))
	if st := f.storage(server.DevAdminToken); st.UsedBytes != 700 || st.AttachmentsBytes != 700 || st.State != apispec.StorageStateOk {
		t.Fatalf("unlimited: %+v", st)
	}
	if s := e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": 1000}, nil); s != http.StatusOK {
		t.Fatalf("set quota: %d", s)
	}
	if len(f.storageDMs()) != 0 {
		t.Fatal("warned below 80%")
	}

	// 85%: one warning; another upload inside the same level warns no more.
	f.upload(f.colleague, f.general, "b.bin", bytesOf(150, 'b'))
	f.upload(f.colleague, f.general, "b2.bin", bytesOf(10, 'c'))
	st := f.storage(server.DevAdminToken)
	if st.State != apispec.StorageStateWarning80 || st.UsedBytes != 860 {
		t.Fatalf("at 86%%: %+v", st)
	}
	dms := f.storageDMs()
	if len(dms) != 1 || !strings.Contains(dms[0], "80%") {
		t.Fatalf("80%% warning: %q", dms)
	}
	// The warning names numbers, never people or files.
	if strings.Contains(dms[0], "kollega") || strings.Contains(dms[0], "b.bin") {
		t.Fatalf("warning names a person or a file: %q", dms[0])
	}

	// 96%: the 95% warning.
	f.upload(f.colleague, f.general, "c.bin", bytesOf(100, 'd'))
	if dms := f.storageDMs(); len(dms) != 2 || !strings.Contains(dms[1], "95%") {
		t.Fatalf("95%% warning: %q", dms)
	}

	// An upload over the quota: 507 with the numbers, nothing stored.
	st5, body := rawUpload(t, e, f.general, "too-much.bin", bytesOf(100, 'e'), f.colleague)
	if st5 != http.StatusInsufficientStorage {
		t.Fatalf("over quota: %d %s", st5, body)
	}
	var problem struct {
		Type       string `json:"type"`
		Detail     string `json:"detail"`
		UsedBytes  int64  `json:"used_bytes"`
		QuotaBytes int64  `json:"quota_bytes"`
	}
	if err := json.Unmarshal(body, &problem); err != nil || problem.Type != "storage_full" || problem.UsedBytes != 960 ||
		problem.QuotaBytes != 1000 || !strings.Contains(problem.Detail, "My files") {
		t.Fatalf("problem = %+v %v", problem, err)
	}
	// A second copy of content the workspace has adds nothing: it is allowed.
	f.upload(f.colleague, f.general, "copy.bin", bytesOf(700, 'a'))
	// Exactly to the quota: full, and the owner hears of it.
	f.upload(f.colleague, f.general, "last.bin", bytesOf(40, 'f'))
	if st := f.storage(server.DevAdminToken); st.State != apispec.StorageStateFull || st.UsedBytes != 1000 {
		t.Fatalf("full: %+v", st)
	}
	if dms := f.storageDMs(); len(dms) != 3 || !strings.Contains(dms[2], "megtelt") {
		t.Fatalf("full warning: %q", dms)
	}
	// Full: no upload, but people still talk.
	if s, _ := rawUpload(t, e, f.general, "tiny.txt", []byte("x"), server.DevAdminToken); s != http.StatusInsufficientStorage {
		t.Fatalf("upload when full: %d", s)
	}
	if s := e.do("POST", "/api/v1/channels/"+f.general+"/messages", f.colleague, apispec.SendMessageRequest{Body: "the storage is full"}, nil); s != http.StatusCreated {
		t.Fatalf("message when full: %d", s)
	}
	// The agent's MCP upload says so and tells it not to retry.
	ctx := context.Background()
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", f.agentTok, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "upload", Arguments: map[string]any{
		"channel": "general", "name": "x.txt", "content": base64.StdEncoding.EncodeToString([]byte("agent data"))}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "Do not retry") {
		t.Fatalf("MCP upload when full: %+v", res.Content)
	}

	// The guards see the state in their host config.
	_, hostTok := e.enroll("pc")
	var cfg apispec.HostConfig
	if s := e.do("GET", "/api/v1/host/config", hostTok, nil, &cfg); s != http.StatusOK {
		t.Fatalf("host config: %d", s)
	}
	if cfg.Storage == nil || cfg.Storage.State != apispec.StorageStateFull {
		t.Fatalf("host config storage = %+v", cfg.Storage)
	}

	// Deleting makes room: below 80% no warning; crossing again warns again.
	page := f.myFiles(f.colleague, "")
	for _, file := range page.Files {
		if file.Name == "b.bin" || file.Name == "c.bin" {
			if s := e.do("DELETE", "/api/v1/files/"+file.Id.String(), f.colleague, nil, nil); s != http.StatusNoContent {
				t.Fatalf("delete %s: %d", file.Name, s)
			}
		}
	}
	if st := f.storage(server.DevAdminToken); st.State != apispec.StorageStateOk || st.UsedBytes != 750 {
		t.Fatalf("after deletes: %+v", st)
	}
	var cfg2 apispec.HostConfig
	e.do("GET", "/api/v1/host/config", hostTok, nil, &cfg2)
	if cfg2.Storage == nil || cfg2.Storage.State != apispec.StorageStateOk || cfg2.Version == cfg.Version {
		t.Fatalf("host config after room: %+v (version %s → %s)", cfg2.Storage, cfg.Version, cfg2.Version)
	}
	if len(f.storageDMs()) != 3 {
		t.Fatal("warned on the way down")
	}
	f.upload(f.colleague, f.general, "again.bin", bytesOf(60, 'g'))
	if dms := f.storageDMs(); len(dms) != 4 || !strings.Contains(dms[3], "80%") {
		t.Fatalf("crossing 80%% again: %q", dms)
	}

	// The quota and the state changes are audited; the owner's view and the
	// audit carry numbers, not people.
	var audit struct {
		Entries []struct{ Action string }
	}
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.do("GET", "/api/v1/admin/audit?limit=200", server.DevAdminToken, nil, &audit)
	seen := map[string]int{}
	for _, a := range audit.Entries {
		seen[a.Action]++
	}
	if seen["workspace.storage_quota_set"] != 1 || seen["workspace.storage_state_changed"] < 5 || seen["file.deleted"] != 2 {
		t.Fatalf("audit = %v", seen)
	}

	// Unlimited again: never full.
	if s := e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": 0}, nil); s != http.StatusOK {
		t.Fatalf("unlimited: %d", s)
	}
	if st := f.storage(server.DevAdminToken); st.State != apispec.StorageStateOk || st.QuotaBytes != 0 {
		t.Fatalf("unlimited: %+v", st)
	}
}

// rawUpload uploads and returns the status and the body (for problems).
func rawUpload(t *testing.T, e *env, channelID, filename string, content []byte, token string) (int, []byte) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", filename)
	_, _ = part.Write(content)
	_ = w.Close()
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/channels/"+channelID+"/files", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	e.checkContract(req, res, raw)
	return res.StatusCode, raw
}

// TestPostgresStorageRefreshAndSweep: the hourly maintenance measures the
// workspace's database share, which counts towards the quota, and removes
// the content of deleted files from the files folder; the same content
// uploaded again is stored again. Runs with `make test-pg`.
func TestPostgresStorageRefreshAndSweep(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	dir := t.TempDir()
	clk := clock.NewManual(time.Now().UTC())
	app, err := server.New(ctx, server.Options{Store: "postgres", DatabaseURL: url, Clock: clk, IDs: ids.V7{}, FilesDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	e := &env{t: t, app: app, srv: srv, router: specRouter(t, srv.URL), clock: clk}
	general := e.generalID()
	// The database outlives the test: start from no quota.
	e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": 0}, nil)

	content := []byte(fmt.Sprintf("unique content %d", clk.Now().UnixNano()))
	st, file := uploadMultipart(t, e, general, "sweep.txt", content, server.DevAdminToken)
	if st != http.StatusCreated {
		t.Fatalf("upload: %d", st)
	}
	stored := filepath.Join(dir, file.Sha256)
	if _, err := os.Stat(stored); err != nil {
		t.Fatalf("content not stored: %v", err)
	}

	app.Maintain(ctx)
	var ws apispec.WorkspaceStorage
	if s := e.do("GET", "/api/v1/workspace/storage", server.DevAdminToken, nil, &ws); s != http.StatusOK {
		t.Fatalf("storage: %d", s)
	}
	if ws.DbMeasuredAt == nil || ws.ChatSearchBytes <= 0 || ws.UsedBytes <= ws.AttachmentsBytes || ws.AttachmentsBytes < int64(len(content)) {
		t.Fatalf("after the refresh: %+v", ws)
	}

	// The database share alone can fill the quota: the next refresh says so.
	e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": ws.UsedBytes / 2}, nil)
	app.Maintain(ctx)
	e.do("GET", "/api/v1/workspace/storage", server.DevAdminToken, nil, &ws)
	if ws.State != apispec.StorageStateFull {
		t.Fatalf("database share over the quota: %+v", ws)
	}
	e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": 0}, nil)

	// Deleted: the sweep removes the content (nobody else references it).
	if s := e.do("DELETE", "/api/v1/files/"+file.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete: %d", s)
	}
	if _, err := os.Stat(stored); err != nil {
		t.Fatal("the content went before the sweep")
	}
	app.Maintain(ctx)
	if _, err := os.Stat(stored); !os.IsNotExist(err) {
		t.Fatalf("content still stored after the sweep: %v", err)
	}
	// The same content again is stored again and downloads.
	st, again := uploadMultipart(t, e, general, "sweep-again.txt", content, server.DevAdminToken)
	if st != http.StatusCreated {
		t.Fatalf("upload again: %d", st)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/files/"+again.Id.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+server.DevAdminToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || !bytes.Equal(got, content) {
		t.Fatalf("download after the sweep: %d %q", res.StatusCode, got)
	}
	// A shared content stays while one live copy references it.
	app.Maintain(ctx)
	if _, err := os.Stat(stored); err != nil {
		t.Fatalf("live content removed: %v", err)
	}
}
