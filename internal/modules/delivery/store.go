package delivery

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists deliveries. Implementations: postgres, memory.
type Store interface {
	// Insert stores d unless a row for (recipient, item kind, item id)
	// already exists; it reports whether a row was inserted.
	Insert(ctx context.Context, d Delivery) (bool, error)
	// Pending returns unacknowledged deliveries for the recipient, oldest
	// first, and the total number pending.
	Pending(ctx context.Context, recipientID ids.ID, limit int) ([]Delivery, int, error)
	// Ack marks the recipient's deliveries as handled and returns how many changed.
	Ack(ctx context.Context, recipientID ids.ID, idList []ids.ID, at time.Time) (int, error)
}
