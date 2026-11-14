// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

func uploadMultipart(t *testing.T, e *env, channelID, filename string, content []byte, token string) (int, *apispec.File) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/channels/"+channelID+"/files", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	e.checkContract(req, res, raw)
	if res.StatusCode == http.StatusCreated {
		var f apispec.File
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, &f
	}
	return res.StatusCode, nil
}

func TestFilesUploadDownloadAndAttach(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()

	// 1. Upload text file with path traversal attempt in name
	textContent := []byte("Hello Colonyhall file sharing!\nLine 2")
	status, file := uploadMultipart(t, e, general, "../../sensitive/report.txt", textContent, server.DevAdminToken)
	if status != http.StatusCreated || file == nil {
		t.Fatalf("expected 201 Created, got %d", status)
	}
	if file.Name != "report.txt" {
		t.Errorf("expected sanitized name 'report.txt', got %q", file.Name)
	}
	if !strings.HasPrefix(file.Mime, "text/") {
		t.Errorf("expected text/* mime, got %q", file.Mime)
	}
	if file.Size != int64(len(textContent)) {
		t.Errorf("expected size %d, got %d", len(textContent), file.Size)
	}
	// The stored hash is the content's sha256 (the sniffed head was once
	// hashed twice; found while deploying round 40, 2026-11-13).
	if want := fmt.Sprintf("%x", sha256.Sum256(textContent)); file.Sha256 != want {
		t.Errorf("sha256 = %s, want %s", file.Sha256, want)
	}

	// 2. Upload PNG file and verify MIME sniffing
	pngHeader := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")
	status2, imgFile := uploadMultipart(t, e, general, "pixel.png", pngHeader, server.DevAdminToken)
	if status2 != http.StatusCreated || imgFile == nil {
		t.Fatalf("expected 201 Created for png, got %d", status2)
	}
	if imgFile.Mime != "image/png" {
		t.Errorf("expected image/png mime, got %q", imgFile.Mime)
	}

	// 3. Attach files to a message
	sendReq := apispec.SendMessageRequest{
		Body:    "Check out these documents",
		FileIds: &[]ids.ID{file.Id, imgFile.Id},
	}
	var sendRes apispec.SendResult
	sendCode := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, sendReq, &sendRes)
	if sendCode != http.StatusCreated || sendRes.Message == nil {
		t.Fatalf("failed to send message with attachments, status: %d", sendCode)
	}
	if sendRes.Message.Attachments == nil || len(*sendRes.Message.Attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %+v", sendRes.Message.Attachments)
	}
	atts := *sendRes.Message.Attachments
	if atts[0].Id != file.Id || atts[0].Name != "report.txt" {
		t.Errorf("unexpected first attachment: %+v", atts[0])
	}
	if atts[1].Id != imgFile.Id || atts[1].Name != "pixel.png" {
		t.Errorf("unexpected second attachment: %+v", atts[1])
	}

	// 4. Verify attachments returned in list messages and thread
	var msgList apispec.MessageList
	e.do("GET", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, nil, &msgList)
	foundMsg := false
	for _, m := range msgList.Messages {
		if m.Id == sendRes.Message.Id {
			foundMsg = true
			if m.Attachments == nil || len(*m.Attachments) != 2 {
				t.Fatalf("expected 2 attachments in list, got %+v", m.Attachments)
			}
		}
	}
	if !foundMsg {
		t.Fatal("message not found in message list")
	}

	// 5. GET /api/v1/files/{fileId} metadata
	var meta apispec.File
	metaCode := e.do("GET", "/api/v1/files/"+file.Id.String(), server.DevAdminToken, nil, &meta)
	if metaCode != http.StatusOK || meta.Id != file.Id {
		t.Fatalf("GET metadata failed, status: %d", metaCode)
	}

	// 6. GET /api/v1/files/{fileId}/content download
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+file.Id.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+server.DevAdminToken)
	dlRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dlRes.Body.Close() }()
	dlBody, _ := io.ReadAll(dlRes.Body)
	e.checkContract(req, dlRes, dlBody)

	if dlRes.StatusCode != http.StatusOK {
		t.Fatalf("download failed, status: %d", dlRes.StatusCode)
	}
	disp := dlRes.Header.Get("Content-Disposition")
	if !strings.Contains(disp, "attachment") || !strings.Contains(disp, "report.txt") {
		t.Errorf("expected Content-Disposition attachment with filename, got %q", disp)
	}
	if nosniff := dlRes.Header.Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %q", nosniff)
	}
	if !bytes.Equal(dlBody, textContent) {
		t.Errorf("download content mismatch: got %q, want %q", string(dlBody), string(textContent))
	}
}

func TestFilesAccessControl(t *testing.T) {
	e := newEnv(t)
	// Create private channel owned by admin
	var privChan apispec.Channel
	privKind := apispec.Private
	privReq := apispec.CreateChannelRequest{
		Name: "finance-confidential",
		Kind: &privKind,
	}
	e.do("POST", "/api/v1/channels", server.DevAdminToken, privReq, &privChan)

	// Admin uploads file to private channel
	status, privFile := uploadMultipart(t, e, privChan.Id.String(), "budget.csv", []byte("q1,100\nq2,200"), server.DevAdminToken)
	if status != http.StatusCreated || privFile == nil {
		t.Fatalf("expected 201 Created for private file, got %d", status)
	}

	// Agent (who is not in the private channel) tries to read metadata -> 403
	code := e.do("GET", "/api/v1/files/"+privFile.Id.String(), server.DevAgentToken, nil, nil)
	if code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for agent reading private file, got %d", code)
	}

	// Agent tries to download content -> 403
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+privFile.Id.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+server.DevAgentToken)
	dlRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = dlRes.Body.Close()
	if dlRes.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden on download, got %d", dlRes.StatusCode)
	}

	// Agent tries to upload to private channel -> 403
	upCode, _ := uploadMultipart(t, e, privChan.Id.String(), "spy.txt", []byte("secret"), server.DevAgentToken)
	if upCode != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden on agent upload to private channel, got %d", upCode)
	}

	// Admin tries to attach private channel's file to a message in #general -> 400 (channel mismatch)
	general := e.generalID()
	mismatchReq := apispec.SendMessageRequest{
		Body:    "Leaking private file to general",
		FileIds: &[]ids.ID{privFile.Id},
	}
	mismatchCode := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAdminToken, mismatchReq, nil)
	if mismatchCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on cross-channel attachment, got %d", mismatchCode)
	}

	// Admin uploads file to #general, agent tries to attach admin's file -> 400 (not sender)
	_, genFile := uploadMultipart(t, e, general, "admin-note.txt", []byte("admin note"), server.DevAdminToken)
	notSenderReq := apispec.SendMessageRequest{
		Body:    "Agent pretending it uploaded admin's file",
		FileIds: &[]ids.ID{genFile.Id},
	}
	notSenderCode := e.do("POST", "/api/v1/channels/"+general+"/messages", server.DevAgentToken, notSenderReq, nil)
	if notSenderCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when non-sender attaches file, got %d", notSenderCode)
	}
}

func TestFilesLimits(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()

	// 1. Empty file (0 bytes) -> 400 Bad Request
	status, _ := uploadMultipart(t, e, general, "empty.txt", []byte{}, server.DevAdminToken)
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for empty file, got %d", status)
	}

	// 2. Oversized file (with MaxUploadMB=1)
	e.app.Files.MaxUploadMB = 1
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "large.bin")
	largeData := make([]byte, 1024*1024+100) // 1MB + 100 bytes
	_, _ = part.Write(largeData)
	_ = writer.Close()

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/channels/"+general+"/files", &body)
	req.Header.Set("Authorization", "Bearer "+server.DevAdminToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	e.checkContract(req, res, raw)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for oversized file, got %d", res.StatusCode)
	}
}

func TestFilesMCP(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// 1. Tool: upload text file
	fileContent := "Agent generated data\nRow 1: 42"
	b64Content := base64.StdEncoding.EncodeToString([]byte(fileContent))
	upRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "upload",
		Arguments: map[string]any{
			"channel": "general",
			"name":    "data.txt",
			"content": b64Content,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if upRes.IsError {
		t.Fatalf("MCP upload failed: %+v", upRes)
	}
	upText := upRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(upText, "uploaded file id=") || !strings.Contains(upText, "data.txt") {
		t.Fatalf("unexpected upload result: %s", upText)
	}

	// Extract UUID from result
	reUUID := regexp.MustCompile(`id=([0-9a-f-]{36})`)
	matches := reUUID.FindStringSubmatch(upText)
	if len(matches) < 2 {
		t.Fatalf("could not extract file id from: %s", upText)
	}
	fileID := matches[1]

	// 2. Tool: download small text file
	dlRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"file_id": fileID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dlRes.IsError {
		t.Fatalf("MCP download failed: %+v", dlRes)
	}
	dlText := dlRes.Content[0].(*mcp.TextContent).Text
	if dlText != fileContent {
		t.Errorf("MCP download content mismatch: got %q, want %q", dlText, fileContent)
	}

	// 3. Tool: upload binary file and download it
	binData := []byte{0x00, 0xFF, 0xFE, 0xFD, 0x12, 0x34}
	binB64 := base64.StdEncoding.EncodeToString(binData)
	upBinRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "upload",
		Arguments: map[string]any{
			"channel": "general",
			"name":    "binary.bin",
			"content": binB64,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	upBinText := upBinRes.Content[0].(*mcp.TextContent).Text
	binMatches := reUUID.FindStringSubmatch(upBinText)
	if len(binMatches) < 2 {
		t.Fatalf("could not extract bin file id from: %s", upBinText)
	}
	binFileID := binMatches[1]

	// Download binary file -> should return description and URL
	dlBinRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "download",
		Arguments: map[string]any{
			"file_id": binFileID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dlBinText := dlBinRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(dlBinText, "download url is") || !strings.Contains(dlBinText, binFileID) {
		t.Errorf("expected download URL description, got %q", dlBinText)
	}

	// 4. Send message attaching the uploaded file
	sendRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "send",
		Arguments: map[string]any{
			"channel": "general",
			"text":    "Here is the data file",
			"files":   []string{fileID},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendRes.IsError {
		t.Fatalf("MCP send failed: %+v", sendRes)
	}

	// 5. Read channel messages with MCP read tool
	readRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "read",
		Arguments: map[string]any{
			"channel": "general",
			"limit":   5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	readText := readRes.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(readText, "data.txt") || !strings.Contains(readText, fileID) {
		t.Errorf("expected read output to show attached files, got: %s", readText)
	}
}

// A read-only member may download a channel's files but not upload: an
// upload puts content into the channel, so it needs write access.
func TestFilesReadOnlyMemberCannotUpload(t *testing.T) {
	e := newEnv(t)
	privKind := apispec.Private
	var ch apispec.Channel
	e.do("POST", "/api/v1/channels", server.DevAdminToken, apispec.CreateChannelRequest{Name: "read-only-room", Kind: &privKind}, &ch)
	if s := e.do("POST", "/api/v1/channels/"+ch.Id.String()+"/members", server.DevAdminToken,
		map[string]any{"actor_id": e.agentID(), "can_write": false}, nil); s != http.StatusNoContent && s != http.StatusCreated {
		t.Fatalf("add read-only member: %d", s)
	}
	_, f := uploadMultipart(t, e, ch.Id.String(), "notes.txt", []byte("hello"), server.DevAdminToken)
	if f == nil {
		t.Fatal("admin upload failed")
	}
	if code := e.do("GET", "/api/v1/files/"+f.Id.String(), server.DevAgentToken, nil, nil); code != http.StatusOK {
		t.Errorf("read-only member reading file metadata: got %d, want 200", code)
	}
	if code, _ := uploadMultipart(t, e, ch.Id.String(), "spam.txt", []byte("spam"), server.DevAgentToken); code != http.StatusForbidden {
		t.Errorf("read-only member upload: got %d, want 403", code)
	}
}
