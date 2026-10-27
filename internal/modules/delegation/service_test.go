package delegation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	evmem "colonyhall/internal/eventlog/memory"
	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// grantStore holds one grant; other Store methods are not used here.
type grantStore struct {
	delegation.Store
	g delegation.Grant
}

func (s *grantStore) GetSettings(context.Context, ids.ID) (delegation.Settings, error) {
	return delegation.Settings{}, db.ErrNotFound
}

func (s *grantStore) GetGrantByID(context.Context, ids.ID) (delegation.Grant, error) { return s.g, nil }

func (s *grantStore) RevokeGrantConditional(_ context.Context, _ ids.ID, by ids.ID, at time.Time) (int64, error) {
	s.g.RevokedBy, s.g.RevokedAt = &by, &at
	return 1, nil
}

// failingEvents refuses every append, like a database error would.
type failingEvents struct{ eventlog.Store }

func (failingEvents) Append(context.Context, *eventlog.Event) error {
	return errors.New("event log unavailable")
}

// A revoke whose event cannot be written must fail, not report success
// with no event and no audit trail.
func TestRevokeGrantFailsWhenEventFails(t *testing.T) {
	ctx := context.Background()
	gen := &ids.Sequential{}
	clk := clock.NewManual(time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC))
	ws := gen.New()
	a, b := delegation.CanonicalPair(gen.New(), gen.New())
	admin := identity.Actor{ID: gen.New(), WorkspaceID: ws, Kind: identity.KindHuman, Role: identity.RoleAdmin, Handle: "norbi"}
	g := delegation.Grant{
		ID: gen.New(), WorkspaceID: ws, ChannelID: gen.New(), AgentAID: a, AgentBID: b,
		DelegationID: gen.New(), ApprovedBy: admin.ID, ApprovedAt: clk.Now(), LastUsedAt: clk.Now(),
		ExpiresAt: clk.Now().Add(12 * time.Hour),
	}
	svc := &delegation.Service{
		Store:  &grantStore{g: g},
		Events: failingEvents{evmem.New()},
		Tx:     &db.Memory{},
		IDs:    gen,
		Clock:  clk,
	}
	if _, err := svc.RevokeGrant(ctx, admin, g.ID); err == nil {
		t.Fatal("RevokeGrant succeeded although its event could not be written")
	}
}
