package files

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store persists file metadata and message attachments.
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
}
