package saved

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists saved items. Implementations: postgres, memory.
type Store interface {
	// Insert stores a new item; db.ErrConflict when the actor saved the
	// message already.
	Insert(ctx context.Context, it Item) error
	// Update stores the item's note, reminder and done fields.
	Update(ctx context.Context, it Item) error
	Get(ctx context.Context, id ids.ID) (Item, error)
	ByMessage(ctx context.Context, actorID, messageID ids.ID) (Item, error)
	Delete(ctx context.Context, id ids.ID) error
	// List returns the actor's items newest first, after the cursor (nil:
	// from the newest), at most limit.
	List(ctx context.Context, workspaceID, actorID ids.ID, f Filter, after *Cursor, limit int) ([]Item, error)
	Count(ctx context.Context, workspaceID, actorID ids.ID) (int, error)
	// Due lists reminders due at now and not yet sent, for every workspace
	// (system mode), oldest first.
	Due(ctx context.Context, now time.Time, limit int) ([]Item, error)
	// ClaimReminder marks a due reminder sent; false when another instance
	// (or a done/changed item) got there first.
	ClaimReminder(ctx context.Context, id ids.ID, at time.Time) (bool, error)
}
