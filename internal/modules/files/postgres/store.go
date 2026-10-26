// Package postgres implements the PostgreSQL store for the files module.
package postgres

import (
	"context"
	"errors"
	"time"

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
		HumanOnly:   f.HumanOnly,
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

// InsertFileFetchRequest creates a new file fetch request.
func (s *Store) InsertFileFetchRequest(ctx context.Context, req files.FileFetchRequest) error {
	var fileID *uuid.UUID
	if req.FileID != ids.Nil {
		u := uuid.UUID(req.FileID)
		fileID = &u
	}
	var errCode *string
	if req.ErrorCode != "" {
		str := string(req.ErrorCode)
		errCode = &str
	}
	var msgID *uuid.UUID
	if req.MessageID != ids.Nil {
		u := uuid.UUID(req.MessageID)
		msgID = &u
	}
	return s.q(ctx).InsertFileFetchRequest(ctx, sqlcgen.InsertFileFetchRequestParams{
		ID:          uuid.UUID(req.ID),
		WorkspaceID: uuid.UUID(req.WorkspaceID),
		MessageID:   msgID,
		ChannelID:   uuid.UUID(req.ChannelID),
		AgentID:     uuid.UUID(req.AgentID),
		RequestedBy: uuid.UUID(req.RequestedBy),
		Path:        req.Path,
		State:       string(req.State),
		FileID:      fileID,
		ErrorCode:   errCode,
		CreatedAt:   req.CreatedAt,
		CompletedAt: req.CompletedAt,
	})
}

// FileFetchRequestByID loads a file fetch request by ID.
func (s *Store) FileFetchRequestByID(ctx context.Context, id ids.ID) (files.FileFetchRequest, error) {
	row, err := s.q(ctx).FileFetchRequestByID(ctx, uuid.UUID(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.FileFetchRequest{}, db.ErrNotFound
		}
		return files.FileFetchRequest{}, err
	}
	return toDomainFetchRequest(row), nil
}

// FileFetchRequestByMessageAndPath loads the most recent request for a message and path.
func (s *Store) FileFetchRequestByMessageAndPath(ctx context.Context, workspaceID, messageID ids.ID, path string) (files.FileFetchRequest, error) {
	var msgID *uuid.UUID
	if messageID != ids.Nil {
		u := uuid.UUID(messageID)
		msgID = &u
	}
	row, err := s.q(ctx).FileFetchRequestByMessageAndPath(ctx, sqlcgen.FileFetchRequestByMessageAndPathParams{
		WorkspaceID: uuid.UUID(workspaceID),
		MessageID:   msgID,
		Path:        path,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.FileFetchRequest{}, db.ErrNotFound
		}
		return files.FileFetchRequest{}, err
	}
	return toDomainFetchRequest(row), nil
}

// FileFetchRequestByAgentAndPath loads the most recent request for an agent and path.
func (s *Store) FileFetchRequestByAgentAndPath(ctx context.Context, workspaceID, agentID ids.ID, path string) (files.FileFetchRequest, error) {
	row, err := s.q(ctx).FileFetchRequestByAgentAndPath(ctx, sqlcgen.FileFetchRequestByAgentAndPathParams{
		WorkspaceID: uuid.UUID(workspaceID),
		AgentID:     uuid.UUID(agentID),
		Path:        path,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.FileFetchRequest{}, db.ErrNotFound
		}
		return files.FileFetchRequest{}, err
	}
	return toDomainFetchRequest(row), nil
}

// ListFileFetchRequestsByAgent lists requests for an agent with a given state.
func (s *Store) ListFileFetchRequestsByAgent(ctx context.Context, workspaceID, agentID ids.ID, state files.FileFetchState) ([]files.FileFetchRequest, error) {
	rows, err := s.q(ctx).ListFileFetchRequestsByAgent(ctx, sqlcgen.ListFileFetchRequestsByAgentParams{
		WorkspaceID: uuid.UUID(workspaceID),
		AgentID:     uuid.UUID(agentID),
		State:       string(state),
	})
	if err != nil {
		return nil, err
	}
	out := make([]files.FileFetchRequest, len(rows))
	for i, r := range rows {
		out[i] = toDomainFetchRequest(r)
	}
	return out, nil
}

// ListPendingFileFetchRequestsBefore lists pending requests created on or before a given time.
func (s *Store) ListPendingFileFetchRequestsBefore(ctx context.Context, before time.Time) ([]files.FileFetchRequest, error) {
	rows, err := s.q(ctx).ListPendingFileFetchRequestsBefore(ctx, before)
	if err != nil {
		return nil, err
	}
	out := make([]files.FileFetchRequest, len(rows))
	for i, r := range rows {
		out[i] = toDomainFetchRequest(r)
	}
	return out, nil
}

// UpdateFileFetchRequestCompleted marks a request as done with its stored file ID.
func (s *Store) UpdateFileFetchRequestCompleted(ctx context.Context, id ids.ID, fileID ids.ID, completedAt time.Time) error {
	u := uuid.UUID(fileID)
	n, err := s.q(ctx).UpdateFileFetchRequestCompleted(ctx, sqlcgen.UpdateFileFetchRequestCompletedParams{
		ID:          uuid.UUID(id),
		FileID:      &u,
		CompletedAt: &completedAt,
	})
	return notPending(n, err)
}

// notPending turns an update that matched no pending request into a conflict.
func notPending(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return db.ErrConflict
	}
	return nil
}

// UpdateFileFetchRequestFailed marks a request as failed with an error code.
func (s *Store) UpdateFileFetchRequestFailed(ctx context.Context, id ids.ID, errorCode files.FileFetchErrorCode, completedAt time.Time) error {
	codeStr := string(errorCode)
	n, err := s.q(ctx).UpdateFileFetchRequestFailed(ctx, sqlcgen.UpdateFileFetchRequestFailedParams{
		ID:          uuid.UUID(id),
		ErrorCode:   &codeStr,
		CompletedAt: &completedAt,
	})
	return notPending(n, err)
}

// GetFileFetchSettings loads workspace file fetch settings.
func (s *Store) GetFileFetchSettings(ctx context.Context, workspaceID ids.ID) (files.FileFetchSettings, error) {
	row, err := s.q(ctx).GetFileFetchSettings(ctx, uuid.UUID(workspaceID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return files.FileFetchSettings{
				WorkspaceID: workspaceID,
				MaxImageMB:  files.DefaultMaxImageMB,
				MaxFileMB:   files.DefaultMaxFileMB,
			}, nil
		}
		return files.FileFetchSettings{}, err
	}
	return files.FileFetchSettings{
		WorkspaceID: ids.ID(row.WorkspaceID),
		MaxImageMB:  int(row.MaxImageMb),
		MaxFileMB:   int(row.MaxFileMb),
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

// UpsertFileFetchSettings creates or updates workspace file fetch settings.
func (s *Store) UpsertFileFetchSettings(ctx context.Context, st files.FileFetchSettings) error {
	return s.q(ctx).UpsertFileFetchSettings(ctx, sqlcgen.UpsertFileFetchSettingsParams{
		WorkspaceID: uuid.UUID(st.WorkspaceID),
		MaxImageMb:  int32(st.MaxImageMB),
		MaxFileMb:   int32(st.MaxFileMB),
		UpdatedAt:   st.UpdatedAt,
	})
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
		HumanOnly:   r.HumanOnly,
	}
}

func toDomainFetchRequest(r sqlcgen.FileFetchRequest) files.FileFetchRequest {
	var msgID ids.ID
	if r.MessageID != nil {
		msgID = ids.ID(*r.MessageID)
	}
	req := files.FileFetchRequest{
		ID:          ids.ID(r.ID),
		WorkspaceID: ids.ID(r.WorkspaceID),
		MessageID:   msgID,
		ChannelID:   ids.ID(r.ChannelID),
		AgentID:     ids.ID(r.AgentID),
		RequestedBy: ids.ID(r.RequestedBy),
		Path:        r.Path,
		State:       files.FileFetchState(r.State),
		CreatedAt:   r.CreatedAt,
		CompletedAt: r.CompletedAt,
	}
	if r.FileID != nil {
		req.FileID = ids.ID(*r.FileID)
	}
	if r.ErrorCode != nil {
		req.ErrorCode = files.FileFetchErrorCode(*r.ErrorCode)
	}
	return req
}

var _ files.Store = (*Store)(nil)
