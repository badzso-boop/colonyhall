// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/modules/workshop/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

func optInt(n int) *int32 {
	if n == 0 {
		return nil
	}
	v := int32(n)
	return &v
}

func fromOptInt(p *int32) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	return err
}

// CountCallsSince implements workshop.Store.
func (s *Store) CountCallsSince(ctx context.Context, workspaceID ids.ID, since time.Time) (int, error) {
	n, err := s.q(ctx).CountCallsSince(ctx, sqlcgen.CountCallsSinceParams{WorkspaceID: workspaceID, StartedAt: since})
	return int(n), err
}

// InsertExtension implements workshop.Store.
func (s *Store) InsertExtension(ctx context.Context, e workshop.Extension) error {
	err := s.q(ctx).InsertExtension(ctx, sqlcgen.InsertExtensionParams{ID: e.ID, WorkspaceID: e.WorkspaceID, Slug: e.Slug,
		Title: e.Title, Description: e.Description, MaintainerID: e.MaintainerID, BotID: e.BotID, State: e.State,
		LiveVersion: optInt(e.LiveVersion), ApprovalOff: e.ApprovalOff, DisabledReason: e.DisabledReason,
		CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// UpdateExtension implements workshop.Store.
func (s *Store) UpdateExtension(ctx context.Context, e workshop.Extension) error {
	return rows(s.q(ctx).UpdateExtension(ctx, sqlcgen.UpdateExtensionParams{WorkspaceID: e.WorkspaceID, ID: e.ID, Title: e.Title,
		Description: e.Description, State: e.State, LiveVersion: optInt(e.LiveVersion), ApprovalOff: e.ApprovalOff,
		DisabledReason: e.DisabledReason, MaintainerID: e.MaintainerID, UpdatedAt: e.UpdatedAt}))
}

// SetExtensionErrors implements workshop.Store.
func (s *Store) SetExtensionErrors(ctx context.Context, workspaceID, id ids.ID, consecutive int, recent []time.Time) error {
	if recent == nil {
		recent = []time.Time{}
	}
	b, _ := json.Marshal(recent)
	return rows(s.q(ctx).SetExtensionErrors(ctx, sqlcgen.SetExtensionErrorsParams{WorkspaceID: workspaceID, ID: id,
		ConsecutiveErrors: int32(consecutive), RecentErrors: b}))
}

func extension(r sqlcgen.Extension) workshop.Extension {
	e := workshop.Extension{ID: r.ID, WorkspaceID: r.WorkspaceID, Slug: r.Slug, Title: r.Title, Description: r.Description,
		MaintainerID: r.MaintainerID, BotID: r.BotID, State: r.State, LiveVersion: fromOptInt(r.LiveVersion),
		ApprovalOff: r.ApprovalOff, DisabledReason: r.DisabledReason, ConsecutiveErrors: int(r.ConsecutiveErrors),
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt}
	_ = json.Unmarshal(r.RecentErrors, &e.RecentErrors)
	return e
}

// GetExtension implements workshop.Store.
func (s *Store) GetExtension(ctx context.Context, workspaceID, id ids.ID) (workshop.Extension, error) {
	r, err := s.q(ctx).GetExtension(ctx, sqlcgen.GetExtensionParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return workshop.Extension{}, notFound(err)
	}
	return extension(r), nil
}

// ListExtensions implements workshop.Store.
func (s *Store) ListExtensions(ctx context.Context, workspaceID ids.ID) ([]workshop.Extension, error) {
	rs, err := s.q(ctx).ListExtensions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Extension, len(rs))
	for i, r := range rs {
		out[i] = extension(r)
	}
	return out, nil
}

// DeleteExtension implements workshop.Store.
func (s *Store) DeleteExtension(ctx context.Context, workspaceID, id ids.ID, at time.Time) error {
	return rows(s.q(ctx).DeleteExtension(ctx, sqlcgen.DeleteExtensionParams{WorkspaceID: workspaceID, ID: id, DeletedAt: &at}))
}

// InsertVersion implements workshop.Store.
func (s *Store) InsertVersion(ctx context.Context, v workshop.Version) error {
	m, err := json.Marshal(v.Manifest)
	if err != nil {
		return err
	}
	err = s.q(ctx).InsertVersion(ctx, sqlcgen.InsertVersionParams{ID: v.ID, WorkspaceID: v.WorkspaceID, ExtensionID: v.ExtensionID,
		Version: int32(v.Version), Manifest: m, Source: v.Source, Bundle: v.Bundle, BundleSha256: v.BundleSHA256,
		CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// GetVersion implements workshop.Store.
func (s *Store) GetVersion(ctx context.Context, workspaceID, extensionID ids.ID, version int) (workshop.Version, error) {
	r, err := s.q(ctx).GetVersion(ctx, sqlcgen.GetVersionParams{WorkspaceID: workspaceID, ExtensionID: extensionID, Version: int32(version)})
	if err != nil {
		return workshop.Version{}, notFound(err)
	}
	v := workshop.Version{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Version: int(r.Version),
		Source: r.Source, Bundle: r.Bundle, BundleSHA256: r.BundleSha256, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}
	return v, json.Unmarshal(r.Manifest, &v.Manifest)
}

// ListVersions implements workshop.Store.
func (s *Store) ListVersions(ctx context.Context, workspaceID, extensionID ids.ID) ([]workshop.Version, error) {
	rs, err := s.q(ctx).ListVersions(ctx, sqlcgen.ListVersionsParams{WorkspaceID: workspaceID, ExtensionID: extensionID})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Version, len(rs))
	for i, r := range rs {
		out[i] = workshop.Version{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Version: int(r.Version),
			BundleSHA256: r.BundleSha256, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}
		if err := json.Unmarshal(r.Manifest, &out[i].Manifest); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// InsertApproval implements workshop.Store.
func (s *Store) InsertApproval(ctx context.Context, a workshop.Approval) error {
	d, err := json.Marshal(a.Diff)
	if err != nil {
		return err
	}
	return s.q(ctx).InsertApproval(ctx, sqlcgen.InsertApprovalParams{ID: a.ID, WorkspaceID: a.WorkspaceID, ExtensionID: a.ExtensionID,
		Version: int32(a.Version), DecidedBy: a.DecidedBy, Diff: d, CreatedAt: a.CreatedAt})
}

// ListApprovals implements workshop.Store.
func (s *Store) ListApprovals(ctx context.Context, workspaceID, extensionID ids.ID) ([]workshop.Approval, error) {
	rs, err := s.q(ctx).ListApprovals(ctx, sqlcgen.ListApprovalsParams{WorkspaceID: workspaceID, ExtensionID: extensionID})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Approval, len(rs))
	for i, r := range rs {
		out[i] = workshop.Approval{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Version: int(r.Version),
			DecidedBy: r.DecidedBy, CreatedAt: r.CreatedAt}
		if err := json.Unmarshal(r.Diff, &out[i].Diff); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func doc(r sqlcgen.ExtStoreDoc) workshop.StoreDoc {
	return workshop.StoreDoc{WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Collection: r.Collection, DocID: r.DocID,
		Data: json.RawMessage(r.Data), SizeBytes: int(r.SizeBytes), UpdatedAt: r.UpdatedAt}
}

// GetDoc implements workshop.Store.
func (s *Store) GetDoc(ctx context.Context, workspaceID, extensionID ids.ID, collection, docID string) (workshop.StoreDoc, error) {
	r, err := s.q(ctx).GetDoc(ctx, sqlcgen.GetDocParams{WorkspaceID: workspaceID, ExtensionID: extensionID, Collection: collection, DocID: docID})
	if err != nil {
		return workshop.StoreDoc{}, notFound(err)
	}
	return doc(r), nil
}

// PutDoc implements workshop.Store.
func (s *Store) PutDoc(ctx context.Context, d workshop.StoreDoc) error {
	return s.q(ctx).PutDoc(ctx, sqlcgen.PutDocParams{WorkspaceID: d.WorkspaceID, ExtensionID: d.ExtensionID, Collection: d.Collection,
		DocID: d.DocID, Data: d.Data, SizeBytes: int32(d.SizeBytes), UpdatedAt: d.UpdatedAt})
}

// DeleteDoc implements workshop.Store.
func (s *Store) DeleteDoc(ctx context.Context, workspaceID, extensionID ids.ID, collection, docID string) error {
	return s.q(ctx).DeleteDoc(ctx, sqlcgen.DeleteDocParams{WorkspaceID: workspaceID, ExtensionID: extensionID, Collection: collection, DocID: docID})
}

// ListDocs implements workshop.Store.
func (s *Store) ListDocs(ctx context.Context, workspaceID, extensionID ids.ID, collection, after string, limit int) ([]workshop.StoreDoc, error) {
	rs, err := s.q(ctx).ListDocs(ctx, sqlcgen.ListDocsParams{WorkspaceID: workspaceID, ExtensionID: extensionID, Collection: collection,
		DocID: after, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.StoreDoc, len(rs))
	for i, r := range rs {
		out[i] = doc(r)
	}
	return out, nil
}

// StoreUsage implements workshop.Store.
func (s *Store) StoreUsage(ctx context.Context, workspaceID, extensionID ids.ID) (int, int, error) {
	r, err := s.q(ctx).StoreUsage(ctx, sqlcgen.StoreUsageParams{WorkspaceID: workspaceID, ExtensionID: extensionID})
	return int(r.Docs), int(r.Bytes), err
}
