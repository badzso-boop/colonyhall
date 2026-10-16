package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"colonyhall/internal/eventlog"
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

// ChannelReader checks access to channels.
type ChannelReader interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
}

// Service implements the business logic of the files module.
type Service struct {
	Store        Store
	Storage      StorageBackend
	Conversation ChannelReader
	Events       eventsAppender
	Bus          busNotifier
	Clock        clock.Clock
	IDs          ids.Generator
	Tx           db.TxRunner
	MaxUploadMB  int
	DetectMime   func(head []byte) string
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

	hasher := sha256.New()
	hasher.Write(headBuf[:headN])

	// Buffer or stream to temp in StorageBackend
	// We combine the head with the rest of the stream
	combined := io.MultiReader(bytes.NewReader(headBuf[:headN]), r)

	// We wrap in a LimitedReader that allows 1 extra byte beyond maxBytes to detect overflow
	lr := &io.LimitedReader{R: combined, N: maxBytes + 1}

	// We tee reader to hasher while streaming to storage
	// But Storage.Put needs sha256Hex upfront!
	// So we buffer into memory or temp file.
	// Since max upload is typically 25MB, reading into bytes or a temp pipe:
	var buf bytes.Buffer
	tr := io.TeeReader(lr, hasher)
	n, err := buf.ReadFrom(tr)
	if err != nil {
		return File{}, err
	}
	if n > maxBytes {
		return File{}, ErrFileTooLarge
	}

	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	// Put into storage backend
	_, err = s.Storage.Put(ctx, sha256Hex, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return File{}, fmt.Errorf("storage put: %w", err)
	}

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

	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.Insert(ctx, f); err != nil {
			return err
		}
		if s.Events != nil {
			ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventFileUploaded, by.ID, f.ChannelID, "files", f.ID, evPayload, now)
			if err != nil {
				return err
			}
			if err := s.Events.Append(ctx, &ev); err != nil {
				return err
			}
			seq = ev.Seq
		}
		return nil
	})
	if err != nil {
		return File{}, err
	}
	if seq > 0 && s.Bus != nil {
		s.Bus.Notify(seq)
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

	return s.Store.ListByChannel(ctx, channelID)
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

// AttachmentsForMessage loads attachments for a single message.
func (s *Service) AttachmentsForMessage(ctx context.Context, messageID ids.ID) ([]Attachment, error) {
	return s.Store.AttachmentsForMessage(ctx, messageID)
}

// AttachmentsForMessages loads attachments for multiple messages.
func (s *Service) AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]Attachment, error) {
	return s.Store.AttachmentsForMessages(ctx, messageIDs)
}
