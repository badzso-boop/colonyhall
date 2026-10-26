package question

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists questions. Implementations: postgres, memory. Lookups
// return db.ErrNotFound when nothing matches.
type Store interface {
	Insert(ctx context.Context, q Question) error
	NextNumber(ctx context.Context, workspaceID ids.ID) (int, error)
	CountOpenByThreadAsker(ctx context.Context, threadRootID, askedBy ids.ID) (int, error)
	GetByID(ctx context.Context, id ids.ID) (Question, error)
	GetByNumber(ctx context.Context, workspaceID ids.ID, number int) (Question, error)
	List(ctx context.Context, workspaceID ids.ID, state *string) ([]Question, error)
	// Answer moves an open/escalated question to answered. Returns (updated, true, nil) or (Question{}, false, nil) if not in open/escalated.
	Answer(ctx context.Context, id ids.ID, answer string, answeredBy *ids.ID, answeredAt time.Time) (Question, bool, error)
	// Withdraw moves an open/escalated question to withdrawn. Returns (updated, true, nil) or (Question{}, false, nil) if not in open/escalated.
	Withdraw(ctx context.Context, id ids.ID, withdrawnAt time.Time) (Question, bool, error)
	// Escalate moves an open question to escalated. Returns (updated, true, nil) or (Question{}, false, nil) if not open.
	Escalate(ctx context.Context, id ids.ID, escalatedAt time.Time) (Question, bool, error)
	// Expire moves an open question to expired. Returns (updated, true, nil) or (Question{}, false, nil) if not open.
	Expire(ctx context.Context, id ids.ID, expiredAt time.Time) (Question, bool, error)
	// Reopen sets an answered question back to open with nil answer fields.
	Reopen(ctx context.Context, id ids.ID) error
	// Due lists open questions to agents that are due for escalation (created_at <= escalateBefore) or deadline (deadline <= deadlineBefore).
	Due(ctx context.Context, escalateBefore, deadlineBefore time.Time, limit int) ([]Question, error)
}
