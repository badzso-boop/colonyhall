package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/lease"
	"colonyhall/internal/modules/lease/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements lease.Store on Postgres.
type Store struct{ DB *db.Postgres }

var _ lease.Store = (*Store)(nil)

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func toModel(l sqlcgen.Lease, holderHandle string) lease.Lease {
	var note string
	if l.Note != nil {
		note = *l.Note
	}
	return lease.Lease{
		ID:           l.ID,
		WorkspaceID:  l.WorkspaceID,
		Resource:     l.Resource,
		HolderID:     l.HolderID,
		HolderHandle: holderHandle,
		Note:         note,
		AcquiredAt:   l.AcquiredAt,
		ExpiresAt:    l.ExpiresAt,
		ReleasedAt:   l.ReleasedAt,
		ReleasedBy:   l.ReleasedBy,
	}
}

func (s *Store) LockResource(ctx context.Context, workspaceID ids.ID, resource string) error {
	return s.q(ctx).AdvisoryLockResource(ctx, sqlcgen.AdvisoryLockResourceParams{
		WorkspaceID: workspaceID.String(),
		Resource:    resource,
	})
}

func (s *Store) GetActiveByResource(ctx context.Context, workspaceID ids.ID, resource string, now time.Time) (lease.Lease, error) {
	row, err := s.q(ctx).GetActiveLeaseByResource(ctx, sqlcgen.GetActiveLeaseByResourceParams{
		WorkspaceID: workspaceID,
		Resource:    resource,
		ExpiresAt:   now,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lease.Lease{}, db.ErrNotFound
		}
		return lease.Lease{}, err
	}
	var note string
	if row.Note != nil {
		note = *row.Note
	}
	return lease.Lease{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		Resource:     row.Resource,
		HolderID:     row.HolderID,
		HolderHandle: row.HolderHandle,
		Note:         note,
		AcquiredAt:   row.AcquiredAt,
		ExpiresAt:    row.ExpiresAt,
		ReleasedAt:   row.ReleasedAt,
		ReleasedBy:   row.ReleasedBy,
	}, nil
}

func (s *Store) GetByID(ctx context.Context, id ids.ID) (lease.Lease, error) {
	row, err := s.q(ctx).GetLeaseByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lease.Lease{}, db.ErrNotFound
		}
		return lease.Lease{}, err
	}
	var note string
	if row.Note != nil {
		note = *row.Note
	}
	return lease.Lease{
		ID:           row.ID,
		WorkspaceID:  row.WorkspaceID,
		Resource:     row.Resource,
		HolderID:     row.HolderID,
		HolderHandle: row.HolderHandle,
		Note:         note,
		AcquiredAt:   row.AcquiredAt,
		ExpiresAt:    row.ExpiresAt,
		ReleasedAt:   row.ReleasedAt,
		ReleasedBy:   row.ReleasedBy,
	}, nil
}

func (s *Store) Create(ctx context.Context, l lease.Lease) error {
	var note *string
	if l.Note != "" {
		note = &l.Note
	}
	_, err := s.q(ctx).InsertLease(ctx, sqlcgen.InsertLeaseParams{
		ID:          l.ID,
		WorkspaceID: l.WorkspaceID,
		Resource:    l.Resource,
		HolderID:    l.HolderID,
		Note:        note,
		AcquiredAt:  l.AcquiredAt,
		ExpiresAt:   l.ExpiresAt,
	})
	return err
}

func (s *Store) Extend(ctx context.Context, id ids.ID, newExpiresAt time.Time, note *string) error {
	_, err := s.q(ctx).ExtendLease(ctx, sqlcgen.ExtendLeaseParams{
		ID:        id,
		ExpiresAt: newExpiresAt,
		Note:      note,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) Release(ctx context.Context, id ids.ID, releasedBy ids.ID, now time.Time) error {
	var relBy *ids.ID
	if releasedBy != ids.Nil {
		relBy = &releasedBy
	}
	_, err := s.q(ctx).ReleaseLease(ctx, sqlcgen.ReleaseLeaseParams{
		ID:         id,
		ReleasedAt: &now,
		ReleasedBy: relBy,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) ListActive(ctx context.Context, workspaceID ids.ID, now time.Time) ([]lease.Lease, error) {
	rows, err := s.q(ctx).ListActiveLeases(ctx, sqlcgen.ListActiveLeasesParams{
		WorkspaceID: workspaceID,
		ExpiresAt:   now,
	})
	if err != nil {
		return nil, err
	}
	res := make([]lease.Lease, len(rows))
	for i, r := range rows {
		var note string
		if r.Note != nil {
			note = *r.Note
		}
		res[i] = lease.Lease{
			ID:           r.ID,
			WorkspaceID:  r.WorkspaceID,
			Resource:     r.Resource,
			HolderID:     r.HolderID,
			HolderHandle: r.HolderHandle,
			Note:         note,
			AcquiredAt:   r.AcquiredAt,
			ExpiresAt:    r.ExpiresAt,
			ReleasedAt:   r.ReleasedAt,
			ReleasedBy:   r.ReleasedBy,
		}
	}
	return res, nil
}

func (s *Store) ReleaseActiveByWorkspace(ctx context.Context, workspaceID ids.ID, releasedBy ids.ID, now time.Time) ([]lease.Lease, error) {
	var relBy *ids.ID
	if releasedBy != ids.Nil {
		relBy = &releasedBy
	}
	rows, err := s.q(ctx).ReleaseActiveLeasesByWorkspace(ctx, sqlcgen.ReleaseActiveLeasesByWorkspaceParams{
		WorkspaceID: workspaceID,
		ReleasedAt:  &now,
		ReleasedBy:  relBy,
	})
	if err != nil {
		return nil, err
	}
	res := make([]lease.Lease, len(rows))
	for i, r := range rows {
		res[i] = toModel(r, "")
	}
	return res, nil
}

func (s *Store) ReleaseActiveByHolder(ctx context.Context, workspaceID ids.ID, holderID ids.ID, releasedBy ids.ID, now time.Time) ([]lease.Lease, error) {
	var relBy *ids.ID
	if releasedBy != ids.Nil {
		relBy = &releasedBy
	}
	rows, err := s.q(ctx).ReleaseActiveLeasesByHolder(ctx, sqlcgen.ReleaseActiveLeasesByHolderParams{
		WorkspaceID: workspaceID,
		HolderID:    holderID,
		ReleasedAt:  &now,
		ReleasedBy:  relBy,
	})
	if err != nil {
		return nil, err
	}
	res := make([]lease.Lease, len(rows))
	for i, r := range rows {
		res[i] = toModel(r, "")
	}
	return res, nil
}
