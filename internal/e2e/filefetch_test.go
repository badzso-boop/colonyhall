// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

func TestFileFetchSecurityAndPermissions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	adminToken := server.DevAdminToken
	agentToken := server.DevAgentToken

	// 1. Human token on agent endpoints -> 403
	code := e.do("GET", "/api/v1/agent/file-fetch", adminToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("GET /agent/file-fetch with human token: got %d, want 403", code)
	}

	dummyID := ids.V7{}.New().String()
	code = e.do("PUT", "/api/v1/agent/file-fetch/"+dummyID+"/content", adminToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("PUT /agent/file-fetch/{id}/content with human token: got %d, want 403", code)
	}

	code = e.do("POST", "/api/v1/agent/file-fetch/"+dummyID+"/fail", adminToken, map[string]any{"error_code": "denied"}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("POST /agent/file-fetch/{id}/fail with human token: got %d, want 403", code)
	}

	// 2. Agent token on human endpoints -> 403
	code = e.do("POST", "/api/v1/messages/"+dummyID+"/file-fetch", agentToken, map[string]any{"path": "/tmp/a.txt"}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("POST /messages/{id}/file-fetch with agent token: got %d, want 403", code)
	}

	code = e.do("GET", "/api/v1/file-fetch/"+dummyID, agentToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("GET /file-fetch/{id} with agent token: got %d, want 403", code)
	}

	code = e.do("GET", "/api/v1/settings/file-fetch", agentToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("GET /settings/file-fetch with agent token: got %d, want 403", code)
	}

	code = e.do("PUT", "/api/v1/settings/file-fetch", agentToken, map[string]any{"max_image_mb": 200, "max_file_mb": 100}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("PUT /settings/file-fetch with agent token: got %d, want 403", code)
	}

	// Setup channel and messages
	general := e.generalID()

	// Human writes a message -> cannot request file fetch on it (400)
	var humanMsg apispec.SendResult
	e.do("POST", "/api/v1/channels/"+general+"/messages", adminToken, map[string]any{
		"body": "I am human: /home/norbi/test.txt",
	}, &humanMsg)

	code = e.do("POST", "/api/v1/messages/"+humanMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path": "/home/norbi/test.txt",
	}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("file-fetch on human message: got %d, want 400", code)
	}

	// Assign fake-agent to a host so it can have file fetches
	agent, _ := e.app.Identity.Authenticate(ctx, agentToken)
	hostID := ids.V7{}.New()
	_ = e.app.Hosts.Store.CreateHost(ctx, hosts.Host{
		ID:          hostID,
		WorkspaceID: agent.WorkspaceID,
		Name:        "homelab",
		CreatedAt:   time.Now(),
	})
	_ = e.app.Hosts.Store.SetAssignment(ctx, hosts.Assignment{
		AgentID:     agent.ID,
		HostID:      hostID,
		WorkspaceID: agent.WorkspaceID,
		CreatedAt:   time.Now(),
	})

	// Agent writes a message
	var agentMsg apispec.SendResult
	e.do("POST", "/api/v1/channels/"+general+"/messages", agentToken, map[string]any{
		"body": "Here is the log: /home/fake-agent/scheduler.log",
	}, &agentMsg)

	// Path not in message -> 400
	code = e.do("POST", "/api/v1/messages/"+agentMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path": "/etc/shadow",
	}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("path not in message: got %d, want 400", code)
	}

	// Non-member of private channel -> 403
	var privChan apispec.Channel
	e.do("POST", "/api/v1/channels", adminToken, map[string]any{
		"name": "secret-ops",
		"kind": "private",
	}, &privChan)

	var privMsg apispec.SendResult
	// Add agent to private channel
	_ = e.app.Conversation.EnsureMember(ctx, privChan.Id, agent.ID)
	e.do("POST", "/api/v1/channels/"+privChan.Id.String()+"/messages", agentToken, map[string]any{
		"body": "Private log: /home/fake-agent/private.log",
	}, &privMsg)

	// Create another human user not in private channel
	admin, _ := e.app.Identity.Authenticate(ctx, adminToken)
	_, bobToken, err := e.app.Identity.CreateActor(ctx, admin, identity.NewActorInput{
		Kind:        identity.KindHuman,
		Handle:      "bob",
		DisplayName: "Bob",
		Role:        identity.RoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}

	code = e.do("POST", "/api/v1/messages/"+privMsg.Message.Id.String()+"/file-fetch", bobToken, map[string]any{
		"path": "/home/fake-agent/private.log",
	}, nil)
	if code != http.StatusForbidden {
		t.Fatalf("non-member POST file-fetch: got %d, want 403", code)
	}
}

func TestFileFetchLifecycleAndHumanOnly(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	adminToken := server.DevAdminToken
	agentToken := server.DevAgentToken

	agent, _ := e.app.Identity.Authenticate(ctx, agentToken)
	hostID := ids.V7{}.New()
	_ = e.app.Hosts.Store.CreateHost(ctx, hosts.Host{
		ID:          hostID,
		WorkspaceID: agent.WorkspaceID,
		Name:        "laptop",
		CreatedAt:   time.Now(),
	})
	_ = e.app.Hosts.Store.SetAssignment(ctx, hosts.Assignment{
		AgentID:     agent.ID,
		HostID:      hostID,
		WorkspaceID: agent.WorkspaceID,
		CreatedAt:   time.Now(),
	})

	general := e.generalID()

	// Agent posts message with file link
	var agentMsg apispec.SendResult
	e.do("POST", "/api/v1/channels/"+general+"/messages", agentToken, map[string]any{
		"body": "Elkészült az analízis: file:///home/fake-agent/reports/data.csv és látható.",
	}, &agentMsg)

	// 1. Human requests file fetch
	var fetchReq apispec.FileFetchRequest
	code := e.do("POST", "/api/v1/messages/"+agentMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path": "/home/fake-agent/reports/data.csv",
	}, &fetchReq)
	if code != http.StatusCreated {
		t.Fatalf("request file fetch: got %d, want 201", code)
	}
	if fetchReq.State != "pending" || fetchReq.Path != "/home/fake-agent/reports/data.csv" {
		t.Fatalf("unexpected fetchReq: %+v", fetchReq)
	}

	// Calling again without refresh -> 200 OK with same pending request
	var sameReq apispec.FileFetchRequest
	code = e.do("POST", "/api/v1/messages/"+agentMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path": "/home/fake-agent/reports/data.csv",
	}, &sameReq)
	if code != http.StatusOK {
		t.Fatalf("request file fetch without refresh: got %d, want 200", code)
	}
	if sameReq.Id != fetchReq.Id {
		t.Fatalf("expected same request ID, got %s != %s", sameReq.Id, fetchReq.Id)
	}

	// 2. Agent checks pending requests
	var pendingList []apispec.AgentPendingFileFetch
	code = e.do("GET", "/api/v1/agent/file-fetch", agentToken, nil, &pendingList)
	if code != http.StatusOK {
		t.Fatalf("get pending: got %d, want 200", code)
	}
	if len(pendingList) != 1 || pendingList[0].Id != fetchReq.Id {
		t.Fatalf("unexpected pendingList: %+v", pendingList)
	}
	if pendingList[0].MaxFileBytes != 100*1024*1024 || pendingList[0].MaxImageBytes != 200*1024*1024 {
		t.Fatalf("unexpected limits: %+v", pendingList[0])
	}

	// 3. Agent streams file content to PUT /api/v1/agent/file-fetch/{id}/content
	content := "header1,header2\nval1,val2\nval3,val4\n"
	req, _ := http.NewRequest("PUT", e.srv.URL+"/api/v1/agent/file-fetch/"+fetchReq.Id.String()+"/content", strings.NewReader(content))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "text/csv")
	req.Header.Set("X-File-Name", "data.csv")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	e.checkContract(req, res, raw)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT content: got %d, want 200: %s", res.StatusCode, string(raw))
	}

	var completedReq apispec.FileFetchRequest
	_ = json.Unmarshal(raw, &completedReq)
	if completedReq.State != "done" || completedReq.File == nil || completedReq.File.Name != "data.csv" {
		t.Fatalf("unexpected completedReq: %+v", completedReq)
	}
	fileID := completedReq.File.Id.String()

	// 4. Human queries request status
	var gotReq apispec.FileFetchRequest
	code = e.do("GET", "/api/v1/file-fetch/"+fetchReq.Id.String(), adminToken, nil, &gotReq)
	if code != http.StatusOK {
		t.Fatalf("GET /file-fetch/{id}: got %d, want 200", code)
	}
	if gotReq.State != "done" || gotReq.File == nil || gotReq.File.Id.String() != fileID {
		t.Fatalf("unexpected gotReq: %+v", gotReq)
	}

	// 5. Human-only isolation: agent cannot read metadata or content of this file
	code = e.do("GET", "/api/v1/files/"+fileID, agentToken, nil, nil)
	if code != http.StatusForbidden {
		t.Fatalf("agent GET /files/{id} on human-only file: got %d, want 403", code)
	}

	reqContent, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+fileID+"/content", nil)
	reqContent.Header.Set("Authorization", "Bearer "+agentToken)
	resContent, err := http.DefaultClient.Do(reqContent)
	if err != nil {
		t.Fatal(err)
	}
	_ = resContent.Body.Close()
	if resContent.StatusCode != http.StatusForbidden {
		t.Fatalf("agent GET /files/{id}/content: got %d, want 403", resContent.StatusCode)
	}

	// Channel files list for agent omits human-only file
	genID, _ := ids.Parse(general)
	agentFiles, err := e.app.Files.ListByChannel(ctx, agent, genID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range agentFiles {
		if f.ID.String() == fileID {
			t.Fatalf("human-only file appeared in agent channel files list!")
		}
	}

	// 6. Human can download content and use Range header
	reqDL, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+fileID+"/content", nil)
	reqDL.Header.Set("Authorization", "Bearer "+adminToken)
	resDL, err := http.DefaultClient.Do(reqDL)
	if err != nil {
		t.Fatal(err)
	}
	bodyDL, _ := io.ReadAll(resDL.Body)
	_ = resDL.Body.Close()
	if resDL.StatusCode != http.StatusOK {
		t.Fatalf("human download: got %d, want 200", resDL.StatusCode)
	}
	if string(bodyDL) != content {
		t.Fatalf("download body = %q, want %q", string(bodyDL), content)
	}

	// Range download
	reqRange, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+fileID+"/content", nil)
	reqRange.Header.Set("Authorization", "Bearer "+adminToken)
	reqRange.Header.Set("Range", "bytes=0-6")
	resRange, err := http.DefaultClient.Do(reqRange)
	if err != nil {
		t.Fatal(err)
	}
	bodyRange, _ := io.ReadAll(resRange.Body)
	_ = resRange.Body.Close()
	if resRange.StatusCode != http.StatusPartialContent {
		t.Fatalf("range download: got %d, want 206", resRange.StatusCode)
	}
	if string(bodyRange) != "header1" {
		t.Fatalf("range body = %q, want %q", string(bodyRange), "header1")
	}

	// 7. Request with refresh=true creates new pending request
	var refreshReq apispec.FileFetchRequest
	code = e.do("POST", "/api/v1/messages/"+agentMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path":    "/home/fake-agent/reports/data.csv",
		"refresh": true,
	}, &refreshReq)
	if code != http.StatusCreated {
		t.Fatalf("refresh file fetch: got %d, want 201", code)
	}
	if refreshReq.Id == fetchReq.Id {
		t.Fatalf("refresh should create new request ID")
	}

	// 8. Fail file fetch request
	var failedReq apispec.FileFetchRequest
	code = e.do("POST", "/api/v1/agent/file-fetch/"+refreshReq.Id.String()+"/fail", agentToken, map[string]any{
		"error_code": "not_found",
	}, &failedReq)
	if code != http.StatusOK {
		t.Fatalf("POST fail: got %d, want 200", code)
	}
	if failedReq.State != "failed" || failedReq.ErrorCode == nil || *failedReq.ErrorCode != "not_found" {
		t.Fatalf("unexpected failedReq: %+v", failedReq)
	}

	// 9. Timeout / sweep test with manual clock
	var timeoutReq apispec.FileFetchRequest
	e.do("POST", "/api/v1/messages/"+agentMsg.Message.Id.String()+"/file-fetch", adminToken, map[string]any{
		"path":    "/home/fake-agent/reports/data.csv",
		"refresh": true,
	}, &timeoutReq)

	// Advance clock past 2 minutes
	e.clock.Advance(2*time.Minute + time.Second)

	// Fetch request now sweeps to host_offline
	var sweptReq apispec.FileFetchRequest
	e.do("GET", "/api/v1/file-fetch/"+timeoutReq.Id.String(), adminToken, nil, &sweptReq)
	if sweptReq.State != "failed" || sweptReq.ErrorCode == nil || *sweptReq.ErrorCode != "host_offline" {
		t.Fatalf("expected swept request to fail with host_offline: %+v", sweptReq)
	}
}

func TestFileFetchSettings(t *testing.T) {
	e := newEnv(t)

	adminToken := server.DevAdminToken

	// Default settings
	var st apispec.FileFetchSettings
	code := e.do("GET", "/api/v1/settings/file-fetch", adminToken, nil, &st)
	if code != http.StatusOK {
		t.Fatalf("GET settings: got %d, want 200", code)
	}
	if st.MaxImageMb != files.DefaultMaxImageMB || st.MaxFileMb != files.DefaultMaxFileMB {
		t.Fatalf("unexpected defaults: %+v", st)
	}

	// Update settings
	var updatedSt apispec.FileFetchSettings
	code = e.do("PUT", "/api/v1/settings/file-fetch", adminToken, map[string]any{
		"max_image_mb": 400,
		"max_file_mb":  250,
	}, &updatedSt)
	if code != http.StatusOK {
		t.Fatalf("PUT settings: got %d, want 200", code)
	}
	if updatedSt.MaxImageMb != 400 || updatedSt.MaxFileMb != 250 {
		t.Fatalf("unexpected updated settings: %+v", updatedSt)
	}

	// Validation: out of range -> 400
	code = e.do("PUT", "/api/v1/settings/file-fetch", adminToken, map[string]any{
		"max_image_mb": 5000,
		"max_file_mb":  250,
	}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("invalid settings: got %d, want 400", code)
	}
}
