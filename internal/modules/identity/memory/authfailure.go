package memory

import (
	"context"
	"slices"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// RecordAuthFailure stores one counted attempt.
func (s *Store) RecordAuthFailure(_ context.Context, f identity.AuthFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = append(s.failures, authFailure{AuthFailure: f})
	return nil
}

// AuthFailures returns the counted attempts after since, oldest first.
func (s *Store) AuthFailures(_ context.Context, scope, handle, ip string, since time.Time) (identity.AuthFailureTimes, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out identity.AuthFailureTimes
	for _, f := range s.failures {
		if f.Scope != scope || !f.At.After(since) {
			continue
		}
		if handle != "" && f.Handle == handle && f.cleared == nil {
			out.ByHandle = append(out.ByHandle, f.At)
		}
		if ip != "" && f.IP == ip {
			out.ByIP = append(out.ByIP, f.At)
		}
	}
	slices.SortFunc(out.ByHandle, time.Time.Compare)
	slices.SortFunc(out.ByIP, time.Time.Compare)
	return out, nil
}

// ClearAuthFailures marks the handle's attempts as cleared.
func (s *Store) ClearAuthFailures(_ context.Context, scope, handle string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.failures {
		if f.Scope == scope && f.Handle == handle && f.cleared == nil {
			s.failures[i].cleared = &at
		}
	}
	return nil
}

func (s *Store) lockouts(match func(identity.AuthFailure) bool, since time.Time) []identity.Lockout {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []identity.Lockout{}
	for _, f := range s.failures {
		if f.LockedUntil != nil && f.At.After(since) && match(f.AuthFailure) {
			out = append(out, identity.Lockout{Scope: f.Scope, Handle: f.Handle, IP: f.IP, At: f.At, Until: *f.LockedUntil})
		}
	}
	slices.SortFunc(out, func(a, b identity.Lockout) int { return b.At.Compare(a.At) })
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

// WorkspaceLockouts lists the workspace's lockouts, newest first. Like the
// tenant policy in Postgres, another workspace's scope sees none.
func (s *Store) WorkspaceLockouts(ctx context.Context, workspaceID ids.ID, since time.Time) ([]identity.Lockout, error) {
	if sc := db.ScopeOf(ctx); !sc.System && sc.Workspace != workspaceID {
		return []identity.Lockout{}, nil
	}
	return s.lockouts(func(f identity.AuthFailure) bool { return f.WorkspaceID == workspaceID && workspaceID != ids.Nil }, since), nil
}

// UnattributedLockouts lists the lockouts of no workspace, newest first.
func (s *Store) UnattributedLockouts(_ context.Context, since time.Time) ([]identity.Lockout, error) {
	return s.lockouts(func(f identity.AuthFailure) bool { return f.WorkspaceID == ids.Nil }, since), nil
}

// DeleteAuthFailures deletes attempts before a time.
func (s *Store) DeleteAuthFailures(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(s.failures)
	s.failures = slices.DeleteFunc(s.failures, func(f authFailure) bool { return f.At.Before(before) })
	return int64(n - len(s.failures)), nil
}

type authFailure struct {
	identity.AuthFailure
	cleared *time.Time
}
