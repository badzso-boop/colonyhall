// Package httpapi exposes the files module over REST.
package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterValidation(files.ErrEmptyContent, files.ErrFileTooLarge, files.ErrChannelMismatch, files.ErrNotSender)
	httpx.RegisterForbidden(files.ErrForbidden)
	httpx.RegisterStatus(http.StatusNotFound, files.ErrNotFound)
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
	}
}

// Register mounts the files endpoints on mux.
func Register(mux *http.ServeMux, svc *files.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	// POST /api/v1/channels/{channelId}/files
	mux.Handle("POST /api/v1/channels/{channelId}/files", wrap(func(w http.ResponseWriter, r *http.Request) {
		channelID, ok := pathID(w, r, "channelId")
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
		fileID, ok := pathID(w, r, "fileId")
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
		fileID, ok := pathID(w, r, "fileId")
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
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))

		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}))
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (ids.ID, bool) {
	id, err := ids.Parse(r.PathValue(name))
	if err != nil {
		httpx.Error(w, r, &httpx.BadRequest{Detail: name + " must be a UUID"})
		return ids.Nil, false
	}
	return id, true
}
