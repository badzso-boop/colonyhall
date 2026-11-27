// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package httpapi exposes the files module over REST.
package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func init() {
	httpx.RegisterValidation(
		files.ErrEmptyContent,
		files.ErrFileTooLarge,
		files.ErrChannelMismatch,
		files.ErrNotSender,
		files.ErrPathNotInMessage,
		files.ErrAuthorNotAgent,
		files.ErrAgentNotAssigned,
		files.ErrInvalidLimit,
		files.ErrInvalidRequestState,
		files.ErrUnknownErrorCode,
	)
	httpx.RegisterForbidden(files.ErrForbidden)
	httpx.RegisterStatus(http.StatusNotFound, files.ErrNotFound)
	httpx.RegisterStatus(http.StatusRequestEntityTooLarge, files.ErrFileTooLarge)
	httpx.RegisterStatus(http.StatusGatewayTimeout, files.ErrFsTimeout)
	httpx.RegisterStatus(http.StatusGone, files.ErrFsGone, files.ErrGone)
	// Round 54.
	httpx.RegisterForbidden(files.ErrNotUploader, files.ErrNotOwner, files.ErrNotAdmin)
	httpx.RegisterValidation(files.ErrInvalidQuota)
	httpx.RegisterProblemType(http.StatusUnprocessableEntity, "quota_above_plan", files.ErrQuotaAbovePlan)
	httpx.RegisterProblemType(http.StatusConflict, "plan_over_quota", files.ErrPlanOverQuota)
	httpx.RegisterProblemType(http.StatusInsufficientStorage, "storage_full", files.ErrStorageFull)
}

// StorageToAPI converts a usage to the owner's view (never per person).
func StorageToAPI(u files.Usage) apispec.WorkspaceStorage {
	b := u.Breakdown()
	out := apispec.WorkspaceStorage{
		QuotaBytes:       u.QuotaBytes,
		LimitBytes:       u.Limit(),
		UsedBytes:        u.Used(),
		AttachmentsBytes: b.Attachments,
		ChatSearchBytes:  b.ChatSearch,
		OtherBytes:       b.Other,
		DbMeasuredAt:     u.DBMeasuredAt,
		State:            apispec.StorageState(u.State()),
	}
	if p := u.Plan; p != nil {
		out.Plan = &apispec.WorkspacePlan{Name: p.Name, StorageBytes: p.StorageBytes, CpuCores: p.CPUCores, MemoryBytes: p.MemoryBytes}
		if p.ExtCallsMonth > 0 {
			out.Plan.ExtCallsMonth = httpx.Ptr(p.ExtCallsMonth)
		}
	}
	return out
}

// A "My files" cursor is "<created_at, unix nanoseconds>_<file id>".
func formatFileCursor(f files.File) string {
	return strconv.FormatInt(f.CreatedAt.UnixNano(), 10) + "_" + f.ID.String()
}

func parseFileCursor(c string) (time.Time, ids.ID, error) {
	ns, id, ok := strings.Cut(c, "_")
	if !ok {
		return time.Time{}, ids.Nil, errors.New("bad cursor")
	}
	n, err := strconv.ParseInt(ns, 10, 64)
	if err != nil {
		return time.Time{}, ids.Nil, err
	}
	fid, err := ids.Parse(id)
	if err != nil {
		return time.Time{}, ids.Nil, err
	}
	return time.Unix(0, n).UTC(), fid, nil
}

func myFileToAPI(v files.MyFileView, withUploader bool) apispec.MyFile {
	out := apispec.MyFile{
		Id: v.ID, Name: v.Name, Mime: v.Mime, Size: v.Size, CreatedAt: v.CreatedAt, SharedCount: v.SharedCount,
	}
	if v.Channel != nil {
		out.Channel = &apispec.MyFileChannel{Id: v.Channel.ID, Name: v.Channel.Name, Kind: v.Channel.Kind}
	}
	if v.MessageID != ids.Nil {
		m := v.MessageID
		out.MessageId = &m
	}
	if v.ThreadRootID != ids.Nil {
		t := v.ThreadRootID
		out.ThreadRootId = &t
	}
	if withUploader {
		u := v.UploadedBy
		out.UploadedBy = &u
	}
	return out
}

// registerStorage mounts "My files", the file delete and the workspace
// storage endpoints (round 54).
func registerStorage(mux *http.ServeMux, svc *files.Service, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/me/files", wrap(func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		q := files.MyFilesQuery{}
		who := files.UploaderMe
		switch v := qs.Get("uploader"); v {
		case "", "me":
		case "agents":
			who = files.UploaderAgents
		default:
			httpx.Error(w, r, &httpx.BadRequest{Detail: "uploader must be me or agents"})
			return
		}
		if c := qs.Get("channel"); c != "" {
			id, err := ids.Parse(c)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "channel must be a UUID"})
				return
			}
			q.ChannelID = id
		}
		switch k := qs.Get("kind"); k {
		case "", files.KindImage, files.KindDocument, files.KindOther:
			q.Kind = k
		default:
			httpx.Error(w, r, &httpx.BadRequest{Detail: "kind must be image, document or other"})
			return
		}
		if c := qs.Get("cursor"); c != "" {
			at, id, err := parseFileCursor(c)
			if err != nil {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "cursor is not a next_cursor value"})
				return
			}
			q.BeforeAt, q.BeforeID = &at, id
		}
		q.Limit = files.DefaultMyFilesLimit
		if l := qs.Get("limit"); l != "" {
			n, err := strconv.Atoi(l)
			if err != nil || n < 1 || n > files.MaxMyFilesLimit {
				httpx.Error(w, r, &httpx.BadRequest{Detail: "limit must be between 1 and 200"})
				return
			}
			q.Limit = n
		}
		rows, err := svc.ListMine(r.Context(), httpx.Actor(r), q, who)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.MyFilePage{Files: make([]apispec.MyFile, 0, len(rows))}
		for _, v := range rows {
			out.Files = append(out.Files, myFileToAPI(v, who == files.UploaderAgents))
		}
		if len(rows) == q.Limit {
			out.NextCursor = httpx.Ptr(formatFileCursor(rows[len(rows)-1].File))
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("DELETE /api/v1/files/{fileId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		id, ok := httpx.PathID(w, r, "fileId")
		if !ok {
			return
		}
		if err := svc.Delete(r.Context(), httpx.Actor(r), id); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("GET /api/v1/workspace/storage", wrap(func(w http.ResponseWriter, r *http.Request) {
		u, err := svc.StorageUsage(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, StorageToAPI(u))
	}))

	mux.Handle("PUT /api/v1/workspace/storage/quota", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.StorageQuotaRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		u, err := svc.SetQuota(r.Context(), httpx.Actor(r), in.QuotaBytes)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, StorageToAPI(u))
	}))
}

// FileToAPI converts a files.File model to apispec.File.
func FileToAPI(f files.File) apispec.File {
	return apispec.File{
		Id:          f.ID,
		WorkspaceId: f.WorkspaceID,
		UploadedBy:  f.UploadedBy,
		ChannelId:   f.ChannelID,
		Name:        f.Name,
		Mime:        f.Mime,
		Size:        f.Size,
		Sha256:      f.SHA256,
		CreatedAt:   f.CreatedAt,
		HumanOnly:   &f.HumanOnly,
	}
}

// FileFetchRequestToAPI converts a FileFetchRequest model to apispec.FileFetchRequest.
func FileFetchRequestToAPI(req files.FileFetchRequest, f *files.File) apispec.FileFetchRequest {
	out := apispec.FileFetchRequest{
		Id:          req.ID,
		WorkspaceId: req.WorkspaceID,
		ChannelId:   req.ChannelID,
		AgentId:     req.AgentID,
		RequestedBy: req.RequestedBy,
		Path:        req.Path,
		State:       apispec.FileFetchRequestState(req.State),
		CreatedAt:   req.CreatedAt,
		CompletedAt: req.CompletedAt,
	}
	if req.MessageID != ids.Nil {
		msgID := openapi_types.UUID(req.MessageID)
		out.MessageId = &msgID
	}
	if req.FileID != ids.Nil {
		fid := req.FileID
		out.FileId = &fid
	}
	if req.ErrorCode != "" {
		code := apispec.FileFetchRequestErrorCode(req.ErrorCode)
		out.ErrorCode = &code
	}
	if f != nil {
		out.File = &apispec.FileAttachmentWithFetchedAt{
			Id:        f.ID,
			Name:      f.Name,
			Mime:      f.Mime,
			Size:      f.Size,
			FetchedAt: f.CreatedAt,
		}
	}
	return out
}

// Register mounts the files endpoints on mux.
func Register(mux *http.ServeMux, svc *files.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }
	registerStorage(mux, svc, wrap)

	// POST /api/v1/channels/{channelId}/files
	mux.Handle("POST /api/v1/channels/{channelId}/files", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := httpx.PathID(w, r, "channelId")
		if !ok {
			return
		}

		maxBytes := svc.MaxUploadBytes()
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)

		mr, err := r.MultipartReader()
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "expected multipart/form-data"})
			return
		}

		var uploadedFile *files.File
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				httpx.Error(w, r, err)
				return
			}
			if part.FormName() == "file" {
				filename := part.FileName()
				f, err := svc.Upload(r.Context(), httpx.Actor(r), channelID, filename, part)
				_ = part.Close()
				if err != nil {
					httpx.Error(w, r, err)
					return
				}
				uploadedFile = &f
				break
			}
			_ = part.Close()
		}

		if uploadedFile == nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "missing 'file' field in multipart form"})
			return
		}

		httpx.JSON(w, http.StatusCreated, FileToAPI(*uploadedFile))
	}))

	// GET /api/v1/files/{fileId}
	mux.Handle("GET /api/v1/files/{fileId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		fileID, ok := httpx.PathID(w, r, "fileId")
		if !ok {
			return
		}
		f, err := svc.Get(r.Context(), httpx.Actor(r), fileID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, FileToAPI(f))
	}))

	// GET /api/v1/files/{fileId}/content
	mux.Handle("GET /api/v1/files/{fileId}/content", wrap(func(w http.ResponseWriter, r *http.Request) {
		fileID, ok := httpx.PathID(w, r, "fileId")
		if !ok {
			return
		}
		f, rc, err := svc.OpenContent(r.Context(), httpx.Actor(r), fileID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		defer func() { _ = rc.Close() }()

		mime := f.Mime
		// Never render HTML or SVG inline
		if strings.Contains(mime, "html") || strings.Contains(mime, "svg") {
			mime = "application/octet-stream"
		}

		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", f.Name))
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if rs, ok := rc.(io.ReadSeeker); ok {
			http.ServeContent(w, r, f.Name, f.CreatedAt, rs)
			return
		}

		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}))

	// POST /api/v1/messages/{messageId}/file-fetch
	mux.Handle("POST /api/v1/messages/{messageId}/file-fetch", wrap(func(w http.ResponseWriter, r *http.Request) {
		messageID, ok := httpx.PathID(w, r, "messageId")
		if !ok {
			return
		}
		var in apispec.CreateFileFetchRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		refresh := in.Refresh != nil && *in.Refresh
		req, created, err := svc.RequestFileFetch(r.Context(), httpx.Actor(r), messageID, in.Path, refresh)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		var f *files.File
		if req.State == files.FileFetchDone && req.FileID != ids.Nil {
			file, err := svc.Get(r.Context(), httpx.Actor(r), req.FileID)
			if err == nil {
				f = &file
			}
		}
		httpx.JSON(w, status, FileFetchRequestToAPI(req, f))
	}))

	// GET /api/v1/file-fetch/{id}
	mux.Handle("GET /api/v1/file-fetch/{id}", wrap(func(w http.ResponseWriter, r *http.Request) {
		reqID, ok := httpx.PathID(w, r, "id")
		if !ok {
			return
		}
		req, f, err := svc.GetFileFetch(r.Context(), httpx.Actor(r), reqID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, FileFetchRequestToAPI(req, f))
	}))

	// GET /api/v1/agent/file-fetch
	mux.Handle("GET /api/v1/agent/file-fetch", wrap(func(w http.ResponseWriter, r *http.Request) {
		fetches, err := svc.AgentPendingFileFetches(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := make([]apispec.AgentPendingFileFetch, 0, len(fetches))
		for _, f := range fetches {
			var msgUUID *openapi_types.UUID
			if f.MessageID != ids.Nil {
				u := openapi_types.UUID(f.MessageID)
				msgUUID = &u
			}
			out = append(out, apispec.AgentPendingFileFetch{
				Id:            f.ID,
				MessageId:     msgUUID,
				ChannelId:     f.ChannelID,
				Path:          f.Path,
				MaxImageBytes: f.MaxImageBytes,
				MaxFileBytes:  f.MaxFileBytes,
			})
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	// PUT /api/v1/agent/file-fetch/{id}/content
	mux.Handle("PUT /api/v1/agent/file-fetch/{id}/content", wrap(func(w http.ResponseWriter, r *http.Request) {
		reqID, ok := httpx.PathID(w, r, "id")
		if !ok {
			return
		}
		contentType := r.Header.Get("Content-Type")
		filename := r.Header.Get("X-File-Name")
		mtime := r.Header.Get("X-File-Mtime")

		req, f, err := svc.SaveFetchedContent(r.Context(), httpx.Actor(r), reqID, contentType, filename, mtime, r.Body)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, FileFetchRequestToAPI(req, &f))
	}))

	// POST /api/v1/agent/file-fetch/{id}/fail
	mux.Handle("POST /api/v1/agent/file-fetch/{id}/fail", wrap(func(w http.ResponseWriter, r *http.Request) {
		reqID, ok := httpx.PathID(w, r, "id")
		if !ok {
			return
		}
		var in apispec.FailFileFetchRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		req, err := svc.FailFileFetch(r.Context(), httpx.Actor(r), reqID, files.FileFetchErrorCode(in.ErrorCode))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, FileFetchRequestToAPI(req, nil))
	}))

	// GET /api/v1/settings/file-fetch
	mux.Handle("GET /api/v1/settings/file-fetch", wrap(func(w http.ResponseWriter, r *http.Request) {
		st, err := svc.GetFileFetchSettings(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.FileFetchSettings{
			MaxImageMb: st.MaxImageMB,
			MaxFileMb:  st.MaxFileMB,
		})
	}))

	// PUT /api/v1/settings/file-fetch
	mux.Handle("PUT /api/v1/settings/file-fetch", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.UpdateFileFetchSettingsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		st, err := svc.UpdateFileFetchSettings(r.Context(), httpx.Actor(r), in.MaxImageMb, in.MaxFileMb)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, apispec.FileFetchSettings{
			MaxImageMb: st.MaxImageMB,
			MaxFileMb:  st.MaxFileMB,
		})
	}))

	// POST /api/v1/agents/{agentId}/fs
	mux.Handle("POST /api/v1/agents/{agentId}/fs", wrap(func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := httpx.PathID(w, r, "agentId")
		if !ok {
			return
		}
		var in apispec.AgentFsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		var root, path, query, name string
		var lines int
		if in.Root != nil {
			root = *in.Root
		}
		if in.Path != nil {
			path = *in.Path
		}
		if in.Query != nil {
			query = *in.Query
		}
		if in.Name != nil {
			name = *in.Name
		}
		if in.Lines != nil {
			lines = *in.Lines
		}
		res, err := svc.RequestAgentFs(r.Context(), httpx.Actor(r), agentID, ids.ID(in.ChannelId), string(in.Op), root, path, query, name, lines)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, res)
	}))

	// POST /api/v1/agents/{agentId}/file-fetch
	mux.Handle("POST /api/v1/agents/{agentId}/file-fetch", wrap(func(w http.ResponseWriter, r *http.Request) {
		agentID, ok := httpx.PathID(w, r, "agentId")
		if !ok {
			return
		}
		var in apispec.CreateAgentFileFetchRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		refresh := in.Refresh != nil && *in.Refresh
		req, created, err := svc.RequestAgentFileFetch(r.Context(), httpx.Actor(r), agentID, ids.ID(in.ChannelId), in.Path, refresh)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		var f *files.File
		if req.State == files.FileFetchDone && req.FileID != ids.Nil {
			file, err := svc.Store.ByID(r.Context(), req.FileID)
			if err == nil {
				f = &file
			}
		}
		httpx.JSON(w, status, FileFetchRequestToAPI(req, f))
	}))

	// GET /api/v1/agent/fs/{requestId}
	mux.Handle("GET /api/v1/agent/fs/{requestId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.PathValue("requestId")
		if reqID == "" {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "requestId is required"})
			return
		}
		pending, err := svc.GetAgentFsRequest(r.Context(), httpx.Actor(r), reqID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.AgentFsPendingRequest{
			RequestId: pending.ID,
			Op:        apispec.AgentFsPendingRequestOp(pending.Op),
		}
		if pending.Root != "" {
			r := pending.Root
			out.Root = &r
		}
		if pending.Path != "" {
			p := pending.Path
			out.Path = &p
		}
		if pending.Query != "" {
			q := pending.Query
			out.Query = &q
		}
		if pending.Name != "" {
			n := pending.Name
			out.Name = &n
		}
		if pending.Lines > 0 {
			l := pending.Lines
			out.Lines = &l
		}
		if pending.KilledBy != "" {
			k := pending.KilledBy
			out.KilledBy = &k
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	// POST /api/v1/agent/fs/{requestId}/result
	mux.Handle("POST /api/v1/agent/fs/{requestId}/result", wrap(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.PathValue("requestId")
		if reqID == "" {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "requestId is required"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
		var in apispec.AgentFsResultRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		err := svc.DeliverAgentFsResult(r.Context(), httpx.Actor(r), reqID, files.FsResult{
			Payload: in,
		})
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
}
