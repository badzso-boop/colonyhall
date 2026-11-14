// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists file metadata, message attachments, and file fetch requests.
type Store interface {
	// Insert saves a new file's metadata.
	Insert(ctx context.Context, f File) error

	// ByID loads a file by ID.
	ByID(ctx context.Context, id ids.ID) (File, error)

	// ByIDs loads multiple files by IDs.
	ByIDs(ctx context.Context, idList []ids.ID) ([]File, error)

	// ListByChannel returns files uploaded in channel, newest first.
	ListByChannel(ctx context.Context, channelID ids.ID) ([]File, error)

	// AttachToMessage links a file to a message with a position index.
	AttachToMessage(ctx context.Context, messageID, fileID ids.ID, position int) error

	// AttachmentsForMessage returns attachments for a single message.
	AttachmentsForMessage(ctx context.Context, messageID ids.ID) ([]Attachment, error)

	// AttachmentsForMessages returns attachments grouped by message ID.
	AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]Attachment, error)

	// InsertFileFetchRequest creates a new file fetch request.
	InsertFileFetchRequest(ctx context.Context, req FileFetchRequest) error

	// FileFetchRequestByID loads a file fetch request by ID.
	FileFetchRequestByID(ctx context.Context, id ids.ID) (FileFetchRequest, error)

	// FileFetchRequestByMessageAndPath loads the most recent request for a message and path.
	FileFetchRequestByMessageAndPath(ctx context.Context, workspaceID, messageID ids.ID, path string) (FileFetchRequest, error)

	// FileFetchRequestByAgentAndPath loads the most recent request for an agent and path.
	FileFetchRequestByAgentAndPath(ctx context.Context, workspaceID, agentID ids.ID, path string) (FileFetchRequest, error)

	// ListFileFetchRequestsByAgent lists requests for an agent with a given state.
	ListFileFetchRequestsByAgent(ctx context.Context, workspaceID, agentID ids.ID, state FileFetchState) ([]FileFetchRequest, error)

	// ListPendingFileFetchRequestsBefore lists pending requests created on or before a given time.
	ListPendingFileFetchRequestsBefore(ctx context.Context, before time.Time) ([]FileFetchRequest, error)

	// UpdateFileFetchRequestCompleted marks a pending request as done with its stored file ID; db.ErrConflict if it is not pending.
	UpdateFileFetchRequestCompleted(ctx context.Context, id ids.ID, fileID ids.ID, completedAt time.Time) error

	// UpdateFileFetchRequestFailed marks a pending request as failed; db.ErrConflict if it is not pending.
	UpdateFileFetchRequestFailed(ctx context.Context, id ids.ID, errorCode FileFetchErrorCode, completedAt time.Time) error

	// GetFileFetchSettings loads workspace file fetch settings.
	GetFileFetchSettings(ctx context.Context, workspaceID ids.ID) (FileFetchSettings, error)

	// UpsertFileFetchSettings creates or updates workspace file fetch settings.
	UpsertFileFetchSettings(ctx context.Context, s FileFetchSettings) error
}
