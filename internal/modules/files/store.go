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

	// Round 54: deleting files and the workspace storage.

	// MarkDeleted soft-deletes a live file; db.ErrConflict if it is already
	// deleted.
	MarkDeleted(ctx context.Context, id ids.ID, at time.Time) error
	// ListMine lists live files of the query's uploaders, newest first.
	ListMine(ctx context.Context, q MyFilesQuery) ([]MyFile, error)
	// LiveFilesBytes is the size of the workspace's live files, each
	// content (sha256) counted once.
	LiveFilesBytes(ctx context.Context, workspaceID ids.ID) (int64, error)
	// WorkspaceHasLiveContent reports whether a live file of the workspace
	// already has the content (a second copy adds nothing to the usage).
	WorkspaceHasLiveContent(ctx context.Context, workspaceID ids.ID, sha256Hex string) (bool, error)
	// LockUsage creates the workspace's storage row if missing and locks
	// it until the transaction ends (memory: returns it).
	LockUsage(ctx context.Context, workspaceID ids.ID, now time.Time) (Usage, error)
	// UsageOf reads the workspace's storage row; a missing row is a zero
	// usage without a quota.
	UsageOf(ctx context.Context, workspaceID ids.ID) (Usage, error)
	// SaveUsage writes the workspace's storage row.
	SaveUsage(ctx context.Context, u Usage) error
	// LockContent serializes the writers and the remover of one content
	// across servers until the transaction ends (memory: a no-op, its
	// transactions are serial).
	LockContent(ctx context.Context, sha256Hex string) error
	// ContentLive reports whether any live file in the whole store (every
	// workspace) references the content. System mode.
	ContentLive(ctx context.Context, sha256Hex string) (bool, error)
	// RemovableContents lists hashes of deleted files whose content was not
	// removed yet. System mode.
	RemovableContents(ctx context.Context, limit int) ([]string, error)
	// MarkContentRemoved records on the deleted rows of the hash that the
	// content is gone (or still in use: the sweep then skips them too until
	// another delete). System mode.
	MarkContentRemoved(ctx context.Context, sha256Hex string, at time.Time) error
}
