// Package postgres implements the PostgreSQL store for the files module.
package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/files/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements files.Store on PostgreSQL.
type Store struct {
	DB *db.Postgres
}

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(s.DB.Q(ctx))
}

// Insert saves a new file's metadata.
func (s *Store) Insert(ctx context.Context, f files.File) error {
	return s.q(ctx).InsertFile(ctx, sqlcgen.InsertFileParams{
		ID:          uuid.UUID(f.ID),
		WorkspaceID: uuid.UUID(f.WorkspaceID),
		UploadedBy:  uuid.UUID(f.UploadedBy),
		ChannelID:   uuid.UUID(f.ChannelID),
		Name:        f.Name,
		Mime:        f.Mime,
		Size:        f.Size,
		Sha256:      f.SHA256,
		CreatedAt:   f.CreatedAt,
	})
}

// ByID loads a file by ID.
func (s *Store) ByID(ctx context.Context, id ids.ID) (files.File, error) {
	row, err := s.q(ctx).FileByID(ctx, uuid.UUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.File{}, db.ErrNotFound
		}
		return files.File{}, err
	}
	return toDomain(row), nil
}

// ByIDs loads multiple files by IDs.
func (s *Store) ByIDs(ctx context.Context, idList []ids.ID) ([]files.File, error) {
	uList := make([]uuid.UUID, len(idList))
	for i, id := range idList {
		uList[i] = uuid.UUID(id)
	}
	rows, err := s.q(ctx).FilesByIDs(ctx, uList)
	if err != nil {
		return nil, err
	}
	out := make([]files.File, len(rows))
	for i, r := range rows {
		out[i] = toDomain(r)
	}
	return out, nil
}

// ListByChannel returns files uploaded in channel, newest first.
func (s *Store) ListByChannel(ctx context.Context, channelID ids.ID) ([]files.File, error) {
	rows, err := s.q(ctx).ListFilesByChannel(ctx, uuid.UUID(channelID))
	if err != nil {
		return nil, err
	}
	out := make([]files.File, len(rows))
	for i, r := range rows {
		out[i] = toDomain(r)
	}
	return out, nil
}

// AttachToMessage links a file to a message with a position index.
func (s *Store) AttachToMessage(ctx context.Context, messageID, fileID ids.ID, position int) error {
	return s.q(ctx).AttachFileToMessage(ctx, sqlcgen.AttachFileToMessageParams{
		MessageID: uuid.UUID(messageID),
		FileID:    uuid.UUID(fileID),
		Position:  int32(position),
	})
}

// AttachmentsForMessage returns attachments for a single message.
func (s *Store) AttachmentsForMessage(ctx context.Context, messageID ids.ID) ([]files.Attachment, error) {
	rows, err := s.q(ctx).AttachmentsForMessage(ctx, uuid.UUID(messageID))
	if err != nil {
		return nil, err
	}
	out := make([]files.Attachment, len(rows))
	for i, r := range rows {
		out[i] = files.Attachment{
			ID:   ids.ID(r.ID),
			Name: r.Name,
			Mime: r.Mime,
			Size: r.Size,
		}
	}
	return out, nil
}

// AttachmentsForMessages returns attachments grouped by message ID.
func (s *Store) AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]files.Attachment, error) {
	uList := make([]uuid.UUID, len(messageIDs))
	for i, id := range messageIDs {
		uList[i] = uuid.UUID(id)
	}
	rows, err := s.q(ctx).AttachmentsForMessages(ctx, uList)
	if err != nil {
		return nil, err
	}
	res := make(map[ids.ID][]files.Attachment, len(messageIDs))
	for _, mID := range messageIDs {
		res[mID] = []files.Attachment{}
	}
	for _, r := range rows {
		mID := ids.ID(r.MessageID)
		res[mID] = append(res[mID], files.Attachment{
			ID:   ids.ID(r.ID),
			Name: r.Name,
			Mime: r.Mime,
			Size: r.Size,
		})
	}
	return res, nil
}

func toDomain(r sqlcgen.File) files.File {
	return files.File{
		ID:          ids.ID(r.ID),
		WorkspaceID: ids.ID(r.WorkspaceID),
		UploadedBy:  ids.ID(r.UploadedBy),
		ChannelID:   ids.ID(r.ChannelID),
		Name:        r.Name,
		Mime:        r.Mime,
		Size:        r.Size,
		SHA256:      r.Sha256,
		CreatedAt:   r.CreatedAt,
	}
}

var _ files.Store = (*Store)(nil)
