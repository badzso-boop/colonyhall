// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type eventsAppender interface {
	Append(ctx context.Context, e *eventlog.Event) error
}

type busNotifier interface {
	Notify(seq int64)
}

// ConversationReader checks access and reads messages.
type ConversationReader interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
}

// ChannelReader checks access to channels.
type ChannelReader = ConversationReader

// ActorReader loads actor metadata.
type ActorReader interface {
	ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
}

// HostAssignee checks whether an agent has an assignment on a host.
type HostAssignee interface {
	Assignment(ctx context.Context, agentID ids.ID) (hosts.Assignment, error)
}

// FsPusher delivers an ephemeral fs.request frame to an agent's open connections.
type FsPusher interface {
	PushFsRequest(agentID ids.ID, requestID string) bool
}

// Service implements the business logic of the files module.
type Service struct {
	Store        Store
	Storage      StorageBackend
	Conversation ConversationReader
	Actors       ActorReader
	Hosts        HostAssignee
	Events       eventsAppender
	Bus          busNotifier
	Clock        clock.Clock
	IDs          ids.Generator
	Tx           db.TxRunner
	MaxUploadMB  int
	DetectMime   func(head []byte) string
	FsPusher     FsPusher
	FsRelay      *FsRelay
	FsTimeout    time.Duration
	Logger       *slog.Logger
	// Agents finds a person's agents for "My files" (round 54).
	Agents OwnedAgents
	// Notifier warns the owner at 80%, 95% and full (round 54); nil: none.
	Notifier StorageNotifier
}

// MaxUploadBytes returns the maximum allowed upload size in bytes.
func (s *Service) MaxUploadBytes() int64 {
	mb := s.MaxUploadMB
	if mb <= 0 {
		mb = DefaultMaxUploadMB
	}
	return int64(mb) * 1024 * 1024
}

// Upload stores a file content addressed by its sha256 hash, saves metadata,
// and records an audit event.
func (s *Service) Upload(ctx context.Context, by identity.Actor, channelID ids.ID, filename string, r io.Reader) (File, error) {
	if by.Halted {
		return File{}, identity.ErrHalted
	}

	// Uploading puts content into the channel: it needs write access, not
	// just read access (a read-only member must not fill the storage).
	canWrite, err := s.Conversation.CanWrite(ctx, by, channelID)
	if err != nil {
		return File{}, err
	}
	if !canWrite {
		return File{}, ErrForbidden
	}
	if c, err := s.Conversation.ChannelByID(ctx, channelID); err != nil {
		return File{}, err
	} else if c.Archived() {
		return File{}, conversation.ErrChannelArchived
	} else if by.Kind == identity.KindAgent && c.AgentLocked() {
		return File{}, conversation.ErrChannelLocked
	}
	return s.upload(ctx, by, channelID, filename, r)
}

// UploadAuthorized stores a file in a channel for another module that has
// already decided who may do it (the compaction module: the scribe chosen by
// an admin writes a channel's summary without being its member). It skips
// the channel check; never expose it to a request directly.
func (s *Service) UploadAuthorized(ctx context.Context, by identity.Actor, channelID ids.ID, filename string, r io.Reader) (File, error) {
	return s.upload(ctx, by, channelID, filename, r)
}

func (s *Service) upload(ctx context.Context, by identity.Actor, channelID ids.ID, filename string, r io.Reader) (File, error) {
	// A paused or stopped actor never writes, whatever the caller allowed.
	if by.Halted {
		return File{}, identity.ErrHalted
	}
	var err error
	cleanName := SanitizeFileName(filename)
	maxBytes := s.MaxUploadBytes()

	// Read first 512 bytes for MIME detection while streaming into hash and buffer
	var headBuf [512]byte
	headN, err := io.ReadFull(r, headBuf[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return File{}, err
	}
	if headN == 0 {
		return File{}, ErrEmptyContent
	}

	mimeType := "application/octet-stream"
	if s.DetectMime != nil {
		mimeType = s.DetectMime(headBuf[:headN])
	}

	// The head is hashed with the rest below: combined starts with it.
	hasher := sha256.New()

	// Buffer or stream to temp in StorageBackend
	// We combine the head with the rest of the stream
	combined := io.MultiReader(bytes.NewReader(headBuf[:headN]), r)

	// We wrap in a LimitedReader that allows 1 extra byte beyond maxBytes to detect overflow
	lr := &io.LimitedReader{R: combined, N: maxBytes + 1}

	// Storage.Put needs the hash up front, so the upload is staged in a
	// temporary file ($TMPDIR; the hardened container points it at the
	// files volume), never in memory: parallel uploads must fit the
	// container's memory limit (round 40).
	tmp, err := os.CreateTemp("", "colonyhall-upload-*")
	if err != nil {
		return File{}, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	n, err := io.Copy(tmp, io.TeeReader(lr, hasher))
	if err != nil {
		return File{}, err
	}
	if n > maxBytes {
		return File{}, ErrFileTooLarge
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return File{}, err
	}

	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	now := s.Clock.Now()
	f := File{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		UploadedBy:  by.ID,
		ChannelID:   channelID,
		Name:        cleanName,
		Mime:        mimeType,
		Size:        n,
		SHA256:      sha256Hex,
		CreatedAt:   now,
	}

	evPayload := UploadedPayload{
		ID:          f.ID.String(),
		WorkspaceID: f.WorkspaceID.String(),
		UploadedBy:  f.UploadedBy.String(),
		ChannelID:   f.ChannelID.String(),
		Name:        f.Name,
		Mime:        f.Mime,
		Size:        f.Size,
		SHA256:      f.SHA256,
	}

	err = s.storeNew(ctx, by.ID, newFile{file: f, content: tmp,
		evType: EventFileUploaded, evChannel: f.ChannelID, evAgg: f.ID, evPayload: evPayload})
	if err != nil {
		return File{}, err
	}
	return f, nil
}

// Get returns file metadata if the caller has read access to its channel.
func (s *Service) Get(ctx context.Context, by identity.Actor, fileID ids.ID) (File, error) {
	f, err := s.Store.ByID(ctx, fileID)
	if err != nil {
		return File{}, err
	}
	if f.WorkspaceID != by.WorkspaceID {
		return File{}, ErrNotFound
	}
	if f.DeletedAt != nil {
		// Before the access checks: a deleted file is gone for everyone.
		return File{}, ErrGone
	}

	if f.HumanOnly && by.Kind != identity.KindHuman {
		return File{}, ErrForbidden
	}
	if f.HumanOnly && !by.IsAdmin() && s.Actors != nil {
		// A file fetched from another person's agent's folder is not found
		// (round 38); the fetching agent is its uploader.
		agent, err := s.Actors.ActorByID(ctx, f.UploadedBy)
		if err != nil {
			return File{}, err
		}
		if !by.SeesAgent(agent) {
			return File{}, ErrNotFound
		}
	}

	canRead, err := s.Conversation.CanRead(ctx, by, f.ChannelID)
	if err != nil {
		return File{}, err
	}
	if !canRead {
		return File{}, ErrForbidden
	}

	return f, nil
}

// OpenContent opens a reader for the file's content if the caller has access.
// OpenContentAuthorized opens a file for another module that has already
// decided who may read it (the compaction module: the scribe reads the
// previous summary). It skips the channel check; never expose it directly.
func (s *Service) OpenContentAuthorized(ctx context.Context, fileID ids.ID) (File, io.ReadCloser, error) {
	f, err := s.Store.ByID(ctx, fileID)
	if err != nil {
		return File{}, nil, err
	}
	if f.DeletedAt != nil {
		return File{}, nil, ErrGone
	}
	rc, err := s.Storage.Open(ctx, f.SHA256)
	if err != nil {
		return File{}, nil, err
	}
	return f, rc, nil
}

func (s *Service) OpenContent(ctx context.Context, by identity.Actor, fileID ids.ID) (File, io.ReadCloser, error) {
	f, err := s.Get(ctx, by, fileID)
	if err != nil {
		return File{}, nil, err
	}

	rc, err := s.Storage.Open(ctx, f.SHA256)
	if err != nil {
		return File{}, nil, err
	}

	return f, rc, nil
}

// ListByChannel returns files uploaded in channel, newest first.
func (s *Service) ListByChannel(ctx context.Context, by identity.Actor, channelID ids.ID) ([]File, error) {
	canRead, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil {
		return nil, err
	}
	if !canRead {
		return nil, ErrForbidden
	}

	list, err := s.Store.ListByChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if by.Kind != identity.KindHuman {
		filtered := make([]File, 0, len(list))
		for _, f := range list {
			if !f.HumanOnly {
				filtered = append(filtered, f)
			}
		}
		return filtered, nil
	}
	if !by.IsAdmin() && s.Actors != nil {
		// Files fetched from another person's agent's folder stay theirs.
		sees := map[ids.ID]bool{}
		filtered := make([]File, 0, len(list))
		for _, f := range list {
			if f.HumanOnly {
				ok, known := sees[f.UploadedBy]
				if !known {
					agent, err := s.Actors.ActorByID(ctx, f.UploadedBy)
					if err != nil {
						return nil, err
					}
					ok = by.SeesAgent(agent)
					sees[f.UploadedBy] = ok
				}
				if !ok {
					continue
				}
			}
			filtered = append(filtered, f)
		}
		return filtered, nil
	}
	return list, nil
}

// ValidateAndAttach validates that all fileIDs belong to the same channel,
// were uploaded by the sender, and attaches them to messageID.
func (s *Service) ValidateAndAttach(ctx context.Context, by identity.Actor, channelID ids.ID, messageID ids.ID, fileIDs []ids.ID) error {
	if len(fileIDs) == 0 {
		return nil
	}

	files, err := s.Store.ByIDs(ctx, fileIDs)
	if err != nil {
		return err
	}
	if len(files) != len(fileIDs) {
		return ErrNotFound
	}

	byID := make(map[ids.ID]File, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}

	for i, fid := range fileIDs {
		f, ok := byID[fid]
		if !ok {
			return ErrNotFound
		}
		if f.WorkspaceID != by.WorkspaceID {
			return ErrNotFound
		}
		if f.DeletedAt != nil {
			return ErrGone
		}
		if f.ChannelID != channelID {
			return ErrChannelMismatch
		}
		if f.UploadedBy != by.ID {
			return ErrNotSender
		}
		if err := s.Store.AttachToMessage(ctx, messageID, fid, i); err != nil {
			return err
		}
	}

	return nil
}

// CopyToMessage implements conversation.Attachments for a forward: each
// live attachment of the source message that the caller may read (Get: the
// source channel, and a human-only file only for whom it is meant) gets a
// new row in the target channel, uploaded by the caller, with the same
// content (stored once: a copy in the same workspace adds no usage), the
// same human-only mark, and the same order. The rows are written through
// storeNew in the caller's transaction, so the forward and its copies
// commit together. An attachment the caller may not read is left out.
func (s *Service) CopyToMessage(ctx context.Context, by identity.Actor, sourceMessageID, channelID, messageID ids.ID) error {
	atts, err := s.Store.AttachmentsForMessage(ctx, sourceMessageID)
	if err != nil {
		return err
	}
	pos := 0
	for _, a := range atts {
		if a.Deleted {
			continue
		}
		src, err := s.Get(ctx, by, a.ID)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) || errors.Is(err, ErrGone) {
			continue
		}
		if err != nil {
			return err
		}
		content, err := s.Storage.Open(ctx, src.SHA256)
		if err != nil {
			return err
		}
		f := File{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, UploadedBy: by.ID, ChannelID: channelID,
			Name: src.Name, Mime: src.Mime, Size: src.Size, SHA256: src.SHA256, HumanOnly: src.HumanOnly,
			CreatedAt: s.Clock.Now()}
		at := pos
		err = s.storeNew(ctx, by.ID, newFile{file: f, content: content,
			extra:  func(ctx context.Context) error { return s.Store.AttachToMessage(ctx, messageID, f.ID, at) },
			evType: EventFileUploaded, evChannel: f.ChannelID, evAgg: f.ID,
			evPayload: UploadedPayload{ID: f.ID.String(), WorkspaceID: f.WorkspaceID.String(), UploadedBy: f.UploadedBy.String(),
				ChannelID: f.ChannelID.String(), Name: f.Name, Mime: f.Mime, Size: f.Size, SHA256: f.SHA256}})
		_ = content.Close()
		if err != nil {
			return err
		}
		pos++
	}
	return nil
}

// AttachmentsForMessage loads attachments for a single message.
func (s *Service) AttachmentsForMessage(ctx context.Context, messageID ids.ID) ([]Attachment, error) {
	return s.Store.AttachmentsForMessage(ctx, messageID)
}

// AttachmentsForMessages loads attachments for multiple messages.
func (s *Service) AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]Attachment, error) {
	return s.Store.AttachmentsForMessages(ctx, messageIDs)
}

// RequestFileFetch handles a human's request to fetch a file linked in an agent message.
func (s *Service) RequestFileFetch(ctx context.Context, by identity.Actor, messageID ids.ID, rawPath string, refresh bool) (FileFetchRequest, bool, error) {
	if by.Kind != identity.KindHuman {
		return FileFetchRequest{}, false, ErrForbidden
	}

	msg, err := s.Conversation.MessageByID(ctx, messageID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if msg.WorkspaceID != by.WorkspaceID {
		return FileFetchRequest{}, false, ErrNotFound
	}

	// A fetch puts the agent's file into the channel: a read-only member
	// may open what was fetched, not ask for more.
	canWrite, err := s.Conversation.CanWrite(ctx, by, msg.ChannelID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if !canWrite {
		return FileFetchRequest{}, false, ErrForbidden
	}

	if s.Actors == nil {
		return FileFetchRequest{}, false, ErrNotFound
	}
	author, err := s.Actors.ActorByID(ctx, msg.AuthorID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if author.WorkspaceID != by.WorkspaceID {
		return FileFetchRequest{}, false, ErrNotFound
	}
	if author.Kind != identity.KindAgent {
		return FileFetchRequest{}, false, ErrAuthorNotAgent
	}
	// Another person's agent's folder is not reachable (round 38).
	if !by.SeesAgent(author) {
		return FileFetchRequest{}, false, ErrNotFound
	}

	normPath := NormalizePath(rawPath)
	if normPath == "" {
		return FileFetchRequest{}, false, ErrPathNotInMessage
	}
	links := FileLinks(msg.Body)
	if !slices.Contains(links, normPath) {
		return FileFetchRequest{}, false, ErrPathNotInMessage
	}

	if s.Hosts == nil {
		return FileFetchRequest{}, false, ErrAgentNotAssigned
	}
	if _, err := s.Hosts.Assignment(ctx, author.ID); err != nil {
		return FileFetchRequest{}, false, ErrAgentNotAssigned
	}

	existing, err := s.Store.FileFetchRequestByMessageAndPath(ctx, by.WorkspaceID, msg.ID, normPath)
	if err == nil {
		if existing.State == FileFetchDone && !refresh {
			return existing, false, nil
		}
		if existing.State == FileFetchPending && !refresh {
			return existing, false, nil
		}
	}

	now := s.Clock.Now()
	req := FileFetchRequest{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		MessageID:   msg.ID,
		ChannelID:   msg.ChannelID,
		AgentID:     author.ID,
		RequestedBy: by.ID,
		Path:        normPath,
		State:       FileFetchPending,
		CreatedAt:   now,
	}

	err = s.commit(ctx, func(ctx context.Context) error { return s.Store.InsertFileFetchRequest(ctx, req) },
		by.WorkspaceID, EventFileFetchRequested, by.ID, msg.ChannelID, req.ID, FetchRequestedPayload{
			RequestID: req.ID.String(),
			AgentID:   author.ID.String(),
			MessageID: msg.ID.String(),
		}, now)
	if err != nil {
		return FileFetchRequest{}, false, err
	}

	return req, true, nil
}

// RequestAgentFileFetch handles a human's request to fetch a file in an agent's folder without a message context.
func (s *Service) RequestAgentFileFetch(ctx context.Context, by identity.Actor, agentID ids.ID, channelID ids.ID, rawPath string, refresh bool) (FileFetchRequest, bool, error) {
	if by.Kind != identity.KindHuman {
		return FileFetchRequest{}, false, ErrForbidden
	}
	if s.Actors == nil {
		return FileFetchRequest{}, false, ErrNotFound
	}
	targetAgent, err := s.Actors.ActorByID(ctx, agentID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if targetAgent.WorkspaceID != by.WorkspaceID || targetAgent.Kind != identity.KindAgent {
		return FileFetchRequest{}, false, ErrNotFound
	}
	// A fetch puts the agent's file into the channel (see RequestFileFetch).
	canWrite, err := s.Conversation.CanWrite(ctx, by, channelID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if !canWrite {
		return FileFetchRequest{}, false, ErrForbidden
	}
	members, err := s.Conversation.MemberIDs(ctx, channelID)
	if err != nil {
		return FileFetchRequest{}, false, err
	}
	if !slices.Contains(members, agentID) {
		return FileFetchRequest{}, false, ErrForbidden
	}
	// Another person's agent's folder is not reachable (round 38).
	if !by.SeesAgent(targetAgent) {
		return FileFetchRequest{}, false, ErrNotFound
	}
	if s.Hosts == nil {
		return FileFetchRequest{}, false, ErrAgentNotAssigned
	}
	if _, err := s.Hosts.Assignment(ctx, agentID); err != nil {
		return FileFetchRequest{}, false, ErrAgentNotAssigned
	}

	normPath := NormalizePath(rawPath)
	if normPath == "" {
		return FileFetchRequest{}, false, &httpx.BadRequest{Detail: "path is required"}
	}

	existing, err := s.Store.FileFetchRequestByAgentAndPath(ctx, by.WorkspaceID, agentID, normPath)
	if err == nil {
		if existing.State == FileFetchDone && !refresh {
			return existing, false, nil
		}
		if existing.State == FileFetchPending && !refresh {
			return existing, false, nil
		}
	}

	now := s.Clock.Now()
	req := FileFetchRequest{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		MessageID:   ids.Nil,
		ChannelID:   channelID,
		AgentID:     agentID,
		RequestedBy: by.ID,
		Path:        normPath,
		State:       FileFetchPending,
		CreatedAt:   now,
	}

	err = s.commit(ctx, func(ctx context.Context) error { return s.Store.InsertFileFetchRequest(ctx, req) },
		by.WorkspaceID, EventFileFetchRequested, by.ID, channelID, req.ID, FetchRequestedPayload{
			RequestID: req.ID.String(),
			AgentID:   agentID.String(),
		}, now)
	if err != nil {
		return FileFetchRequest{}, false, err
	}

	return req, true, nil
}

func (s *Service) fsRelay() *FsRelay {
	if s.FsRelay == nil {
		s.FsRelay = NewFsRelay()
	}
	return s.FsRelay
}

// terminalName is a terminal's name as the guard creates it.
var terminalName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// RequestAgentFs relays an ephemeral filesystem request to the agent's guard and waits up to 15s.
func (s *Service) RequestAgentFs(ctx context.Context, by identity.Actor, agentID ids.ID, channelID ids.ID, op, root, path, query, name string, lines int) (any, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	switch op {
	case "list", "search", "git_status", "term_list", "term_output", "term_kill":
	default:
		return nil, &httpx.BadRequest{Detail: "invalid op"}
	}
	if op == "term_kill" && !by.IsAdmin() {
		return nil, ErrForbidden
	}
	// The guard checks it too; a name like "../other-agent/build" must not
	// even leave the server (round 40).
	if (op == "term_output" || op == "term_kill") && !terminalName.MatchString(name) {
		return nil, &httpx.BadRequest{Detail: "invalid terminal name"}
	}
	if s.Actors == nil {
		return nil, ErrNotFound
	}
	targetAgent, err := s.Actors.ActorByID(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if targetAgent.WorkspaceID != by.WorkspaceID || targetAgent.Kind != identity.KindAgent {
		return nil, ErrNotFound
	}
	canRead, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil {
		return nil, err
	}
	if !canRead {
		return nil, ErrForbidden
	}
	members, err := s.Conversation.MemberIDs(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(members, agentID) {
		return nil, ErrForbidden
	}
	// Another person's agent's folders and terminals are not found (round 38).
	if !by.SeesAgent(targetAgent) {
		return nil, ErrNotFound
	}
	if s.Hosts == nil {
		return nil, ErrAgentNotAssigned
	}
	if _, err := s.Hosts.Assignment(ctx, agentID); err != nil {
		return nil, ErrAgentNotAssigned
	}

	killedBy := ""
	if op == "term_kill" {
		killedBy = "@" + by.Handle
	}

	// Audit request (who, which agent, op, path — never file names from answer, never content)
	if s.Logger != nil {
		if op == "term_kill" {
			s.Logger.Info("audit: term kill", "actor_id", by.ID, "actor_handle", by.Handle, "agent_id", agentID, "name", name, "killed_by", killedBy)
		} else {
			s.Logger.Info("audit: fs request", "actor_id", by.ID, "agent_id", agentID, "op", op, "path", path)
		}
	}

	reqID := s.IDs.New().String()
	relay := s.fsRelay()
	now := s.Clock.Now()
	pending := relay.Register(reqID, agentID, op, root, path, query, name, lines, killedBy, now)

	if s.FsPusher != nil {
		s.FsPusher.PushFsRequest(agentID, reqID)
	}

	timeout := s.FsTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	select {
	case res := <-pending.ResultCh:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Payload, nil
	case <-time.After(timeout):
		relay.Timeout(reqID)
		return nil, ErrFsTimeout
	case <-ctx.Done():
		relay.Timeout(reqID)
		return nil, ctx.Err()
	}
}

// GetAgentFsRequest returns a pending filesystem request for the calling agent.
func (s *Service) GetAgentFsRequest(ctx context.Context, by identity.Actor, requestID string) (*FsPendingRequest, error) {
	if by.Kind != identity.KindAgent {
		return nil, ErrForbidden
	}
	return s.fsRelay().Get(requestID, by.ID)
}

// DeliverAgentFsResult delivers the result from the agent's guard to the waiting human handler.
func (s *Service) DeliverAgentFsResult(ctx context.Context, by identity.Actor, requestID string, res FsResult) error {
	if by.Kind != identity.KindAgent {
		return ErrForbidden
	}
	return s.fsRelay().Deliver(requestID, by.ID, res)
}

// GetFileFetch returns a file fetch request, failing expired ones with host_offline, and includes file metadata when done.
func (s *Service) GetFileFetch(ctx context.Context, by identity.Actor, id ids.ID) (FileFetchRequest, *File, error) {
	if by.Kind != identity.KindHuman {
		return FileFetchRequest{}, nil, ErrForbidden
	}

	req, err := s.Store.FileFetchRequestByID(ctx, id)
	if err != nil {
		return FileFetchRequest{}, nil, err
	}
	if req.WorkspaceID != by.WorkspaceID {
		return FileFetchRequest{}, nil, ErrNotFound
	}
	if !by.IsAdmin() && s.Actors != nil {
		// A file fetched from another person's agent's folder stays theirs.
		agent, err := s.Actors.ActorByID(ctx, req.AgentID)
		if err != nil {
			return FileFetchRequest{}, nil, err
		}
		if !by.SeesAgent(agent) {
			return FileFetchRequest{}, nil, ErrNotFound
		}
	}

	canRead, err := s.Conversation.CanRead(ctx, by, req.ChannelID)
	if err != nil {
		return FileFetchRequest{}, nil, err
	}
	if !canRead {
		return FileFetchRequest{}, nil, ErrForbidden
	}

	if req.State == FileFetchPending && s.Clock.Now().Sub(req.CreatedAt) >= fetchTimeout {
		if failed, err := s.expire(ctx, req); err == nil {
			req = failed
		}
	}

	if req.State == FileFetchDone && req.FileID != ids.Nil {
		f, err := s.Store.ByID(ctx, req.FileID)
		if err == nil {
			return req, &f, nil
		}
	}

	return req, nil, nil
}

// AgentPendingFileFetch contains the pending request data for an agent.
type AgentPendingFileFetch struct {
	ID            ids.ID
	MessageID     ids.ID
	ChannelID     ids.ID
	Path          string
	MaxImageBytes int64
	MaxFileBytes  int64
}

// AgentPendingFileFetches returns pending requests for the calling agent, including byte limits.
func (s *Service) AgentPendingFileFetches(ctx context.Context, by identity.Actor) ([]AgentPendingFileFetch, error) {
	if by.Kind != identity.KindAgent {
		return nil, ErrForbidden
	}

	reqs, err := s.Store.ListFileFetchRequestsByAgent(ctx, by.WorkspaceID, by.ID, FileFetchPending)
	if err != nil {
		return nil, err
	}

	settings, err := s.Store.GetFileFetchSettings(ctx, by.WorkspaceID)
	if err != nil {
		settings = FileFetchSettings{
			WorkspaceID: by.WorkspaceID,
			MaxImageMB:  DefaultMaxImageMB,
			MaxFileMB:   DefaultMaxFileMB,
		}
	}
	maxImageBytes := int64(settings.MaxImageMB) * 1024 * 1024
	maxFileBytes := int64(settings.MaxFileMB) * 1024 * 1024

	now := s.Clock.Now()
	var out []AgentPendingFileFetch
	for _, r := range reqs {
		if now.Sub(r.CreatedAt) >= fetchTimeout {
			_, _ = s.expire(ctx, r)
			continue
		}
		out = append(out, AgentPendingFileFetch{
			ID:            r.ID,
			MessageID:     r.MessageID,
			ChannelID:     r.ChannelID,
			Path:          r.Path,
			MaxImageBytes: maxImageBytes,
			MaxFileBytes:  maxFileBytes,
		})
	}
	return out, nil
}

// SaveFetchedContent streams the file content to disk, enforces limits on the stream, stores as human-only File, and completes the request.
func (s *Service) SaveFetchedContent(ctx context.Context, by identity.Actor, requestID ids.ID, contentType, filename, mtime string, r io.Reader) (FileFetchRequest, File, error) {
	if by.Kind != identity.KindAgent {
		return FileFetchRequest{}, File{}, ErrForbidden
	}

	req, err := s.Store.FileFetchRequestByID(ctx, requestID)
	if err != nil {
		return FileFetchRequest{}, File{}, err
	}
	if req.WorkspaceID != by.WorkspaceID || req.AgentID != by.ID {
		return FileFetchRequest{}, File{}, ErrNotFound
	}
	if req.State != FileFetchPending {
		return FileFetchRequest{}, File{}, ErrInvalidRequestState
	}

	settings, err := s.Store.GetFileFetchSettings(ctx, by.WorkspaceID)
	if err != nil {
		settings = FileFetchSettings{
			WorkspaceID: by.WorkspaceID,
			MaxImageMB:  DefaultMaxImageMB,
			MaxFileMB:   DefaultMaxFileMB,
		}
	}

	cleanName := SanitizeFileName(filename)
	if cleanName == "attachment" && req.Path != "" {
		cleanName = SanitizeFileName(filepath.Base(req.Path))
	}

	isImage := isImageMime(contentType, cleanName)
	maxBytes := int64(settings.MaxFileMB) * 1024 * 1024
	if isImage {
		maxBytes = int64(settings.MaxImageMB) * 1024 * 1024
	}

	// Stream to a temp file on disk without loading entire body into memory
	tmp, err := os.CreateTemp("", "colonyhall-fetch-*")
	if err != nil {
		return FileFetchRequest{}, File{}, fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	hasher := sha256.New()
	lr := &io.LimitedReader{R: r, N: maxBytes + 1}
	tr := io.TeeReader(lr, hasher)
	n, err := io.Copy(tmp, tr)
	if err != nil {
		return FileFetchRequest{}, File{}, err
	}
	if n > maxBytes {
		return FileFetchRequest{}, File{}, ErrFileTooLarge
	}
	if n == 0 {
		return FileFetchRequest{}, File{}, ErrEmptyContent
	}

	if err := tmp.Sync(); err != nil {
		return FileFetchRequest{}, File{}, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return FileFetchRequest{}, File{}, err
	}

	sha256Hex := hex.EncodeToString(hasher.Sum(nil))
	now := s.Clock.Now()
	mimeType := contentType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	f := File{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		UploadedBy:  by.ID,
		ChannelID:   req.ChannelID,
		Name:        cleanName,
		Mime:        mimeType,
		Size:        n,
		SHA256:      sha256Hex,
		CreatedAt:   now,
		HumanOnly:   true,
	}

	err = s.storeNew(ctx, by.ID, newFile{file: f, content: tmp,
		extra: func(ctx context.Context) error {
			return s.Store.UpdateFileFetchRequestCompleted(ctx, req.ID, f.ID, now)
		},
		evType: EventFileFetchCompleted, evChannel: req.ChannelID, evAgg: req.ID,
		evPayload: FetchCompletedPayload{
			RequestID: req.ID.String(),
			AgentID:   by.ID.String(),
			MessageID: req.MessageID.String(),
			FileID:    f.ID.String(),
		}})
	if errors.Is(err, db.ErrConflict) {
		return FileFetchRequest{}, File{}, ErrInvalidRequestState
	}
	if err != nil {
		return FileFetchRequest{}, File{}, err
	}
	req.State = FileFetchDone
	req.FileID = f.ID
	req.CompletedAt = &now
	return req, f, nil
}

// FailFileFetch records failure for a requested file fetch.
func (s *Service) FailFileFetch(ctx context.Context, by identity.Actor, requestID ids.ID, errorCode FileFetchErrorCode) (FileFetchRequest, error) {
	if by.Kind != identity.KindAgent {
		return FileFetchRequest{}, ErrForbidden
	}

	if !isValidErrorCode(errorCode) {
		return FileFetchRequest{}, ErrUnknownErrorCode
	}

	req, err := s.Store.FileFetchRequestByID(ctx, requestID)
	if err != nil {
		return FileFetchRequest{}, err
	}
	if req.WorkspaceID != by.WorkspaceID || req.AgentID != by.ID {
		return FileFetchRequest{}, ErrNotFound
	}
	if req.State != FileFetchPending {
		return FileFetchRequest{}, ErrInvalidRequestState
	}

	now := s.Clock.Now()
	err = s.commit(ctx, func(ctx context.Context) error {
		return s.Store.UpdateFileFetchRequestFailed(ctx, req.ID, errorCode, now)
	}, by.WorkspaceID, EventFileFetchFailed, by.ID, req.ChannelID, req.ID, FetchFailedPayload{
		RequestID: req.ID.String(),
		AgentID:   by.ID.String(),
		MessageID: req.MessageID.String(),
		ErrorCode: string(errorCode),
	}, now)
	if err != nil {
		return FileFetchRequest{}, err
	}
	req.State = FileFetchFailed
	req.ErrorCode = errorCode
	req.CompletedAt = &now
	return req, nil
}

// SweepPending marks pending requests older than 2 minutes as failed with host_offline.
func (s *Service) SweepPending(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	cutoff := now.Add(-fetchTimeout)
	expired, err := s.Store.ListPendingFileFetchRequestsBefore(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range expired {
		if _, err := s.expire(ctx, r); err == nil {
			count++
		}
	}
	return count, nil
}

// GetFileFetchSettings loads file fetch limits for a workspace.
func (s *Service) GetFileFetchSettings(ctx context.Context, by identity.Actor) (FileFetchSettings, error) {
	if by.Kind != identity.KindHuman {
		return FileFetchSettings{}, ErrForbidden
	}
	return s.Store.GetFileFetchSettings(ctx, by.WorkspaceID)
}

// UpdateFileFetchSettings updates file fetch limits for a workspace. Admins only.
func (s *Service) UpdateFileFetchSettings(ctx context.Context, by identity.Actor, maxImageMB, maxFileMB int) (FileFetchSettings, error) {
	if !by.IsAdmin() {
		return FileFetchSettings{}, ErrForbidden
	}
	if maxImageMB < MinLimitMB || maxImageMB > MaxLimitMB || maxFileMB < MinLimitMB || maxFileMB > MaxLimitMB {
		return FileFetchSettings{}, ErrInvalidLimit
	}
	now := s.Clock.Now()
	st := FileFetchSettings{
		WorkspaceID: by.WorkspaceID,
		MaxImageMB:  maxImageMB,
		MaxFileMB:   maxFileMB,
		UpdatedAt:   now,
	}
	err := s.commit(ctx, func(ctx context.Context) error { return s.Store.UpsertFileFetchSettings(ctx, st) },
		by.WorkspaceID, EventFileFetchSettingsUpdated, by.ID, ids.Nil, by.WorkspaceID, FetchSettingsUpdatedPayload{
			MaxImageMB: maxImageMB,
			MaxFileMB:  maxFileMB,
		}, now)
	if err != nil {
		return FileFetchSettings{}, err
	}
	return st, nil
}

// fetchTimeout fails a request the guard has not answered: its machine is offline.
const fetchTimeout = 2 * time.Minute

// commit applies a state change and appends its event in one transaction
// (transactional outbox), then wakes the bus. A request that is no longer
// pending (the store reports a conflict) yields ErrInvalidRequestState.
func (s *Service) commit(ctx context.Context, change func(context.Context) error, workspaceID ids.ID, typ string, actorID, channelID, aggID ids.ID, payload any, at time.Time) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		if s.Events == nil {
			return nil
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, channelID, "files", aggID, payload, at)
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if errors.Is(err, db.ErrConflict) {
		return ErrInvalidRequestState
	}
	if err == nil && seq > 0 && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	return err
}

// expire fails a pending request with host_offline.
func (s *Service) expire(ctx context.Context, r FileFetchRequest) (FileFetchRequest, error) {
	now := s.Clock.Now()
	err := s.commit(ctx, func(ctx context.Context) error {
		return s.Store.UpdateFileFetchRequestFailed(ctx, r.ID, FileFetchErrorHostOffline, now)
	}, r.WorkspaceID, EventFileFetchFailed, r.AgentID, r.ChannelID, r.ID, FetchFailedPayload{
		RequestID: r.ID.String(),
		AgentID:   r.AgentID.String(),
		MessageID: r.MessageID.String(),
		ErrorCode: string(FileFetchErrorHostOffline),
	}, now)
	if err != nil {
		return r, err
	}
	r.State = FileFetchFailed
	r.ErrorCode = FileFetchErrorHostOffline
	r.CompletedAt = &now
	return r, nil
}

func isImageMime(mime, filename string) bool {
	if strings.HasPrefix(strings.ToLower(mime), "image/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg", ".bmp", ".ico", ".tiff":
		return true
	}
	return false
}

func isValidErrorCode(code FileFetchErrorCode) bool {
	switch code {
	case FileFetchErrorHostOffline,
		FileFetchErrorNotFound,
		FileFetchErrorDenied,
		FileFetchErrorTooLarge,
		FileFetchErrorNotAFile,
		FileFetchErrorTimeout:
		return true
	default:
		return false
	}
}
