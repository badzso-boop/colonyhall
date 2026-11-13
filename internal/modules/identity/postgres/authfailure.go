package postgres

import (
	"context"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/identity/postgres/sqlcgen"
	"colonyhall/internal/platform/ids"
)

// RecordAuthFailure stores one counted attempt (in any scope).
func (s *Store) RecordAuthFailure(ctx context.Context, f identity.AuthFailure) error {
	var ws *ids.ID
	if f.WorkspaceID != ids.Nil {
		ws = &f.WorkspaceID
	}
	return s.q(ctx).RecordAuthFailure(ctx, sqlcgen.RecordAuthFailureParams{ID: f.ID, WorkspaceID: ws, Scope: f.Scope,
		Handle: f.Handle, Ip: f.IP, FailedAt: f.At, LockedUntil: f.LockedUntil})
}

// AuthFailures returns the counted attempts after since (in any scope).
func (s *Store) AuthFailures(ctx context.Context, scope, handle, ip string, since time.Time) (identity.AuthFailureTimes, error) {
	rows, err := s.q(ctx).AuthFailures(ctx, sqlcgen.AuthFailuresParams{Scope: scope, Handle: handle, Ip: ip, Since: since})
	var out identity.AuthFailureTimes
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		if r.ByIp {
			out.ByIP = append(out.ByIP, r.FailedAt)
		} else {
			out.ByHandle = append(out.ByHandle, r.FailedAt)
		}
	}
	return out, nil
}

// ClearAuthFailures marks the handle's attempts as cleared (in any scope).
func (s *Store) ClearAuthFailures(ctx context.Context, scope, handle string, at time.Time) error {
	return s.q(ctx).ClearAuthFailures(ctx, sqlcgen.ClearAuthFailuresParams{Scope: scope, Handle: handle, At: at})
}

// WorkspaceLockouts lists the workspace's lockouts (in its scope).
func (s *Store) WorkspaceLockouts(ctx context.Context, workspaceID ids.ID, since time.Time) ([]identity.Lockout, error) {
	rows, err := s.q(ctx).WorkspaceLockouts(ctx, sqlcgen.WorkspaceLockoutsParams{WorkspaceID: workspaceID, Since: since})
	if err != nil {
		return nil, err
	}
	out := make([]identity.Lockout, 0, len(rows))
	for _, r := range rows {
		out = append(out, identity.Lockout{Scope: r.Scope, Handle: r.Handle, IP: r.Ip, At: r.FailedAt, Until: r.LockedUntil})
	}
	return out, nil
}

// UnattributedLockouts lists the lockouts of no workspace (in any scope).
func (s *Store) UnattributedLockouts(ctx context.Context, since time.Time) ([]identity.Lockout, error) {
	rows, err := s.q(ctx).UnattributedLockouts(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]identity.Lockout, 0, len(rows))
	for _, r := range rows {
		out = append(out, identity.Lockout{Scope: r.Scope, Handle: r.Handle, IP: r.Ip, At: r.FailedAt, Until: r.LockedUntil})
	}
	return out, nil
}

// DeleteAuthFailures deletes old attempts, one batch per statement.
func (s *Store) DeleteAuthFailures(ctx context.Context, before time.Time) (int64, error) {
	return deleteInBatches(func() (int64, error) {
		return s.q(ctx).DeleteAuthFailures(ctx, sqlcgen.DeleteAuthFailuresParams{Before: before, Batch: deleteBatch})
	})
}
