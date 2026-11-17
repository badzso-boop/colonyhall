// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"sort"
	"time"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// CountCallsSince implements workshop.Store.
func (s *Store) CountCallsSince(_ context.Context, workspaceID ids.ID, since time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c.WorkspaceID == workspaceID && !c.StartedAt.Before(since) && c.Outcome != workshop.OutcomeDenied {
			n++
		}
	}
	return n, nil
}

func (s *Store) liveExt(workspaceID, id ids.ID) (workshop.Extension, bool) {
	e, ok := s.exts[id]
	return e, ok && e.WorkspaceID == workspaceID && e.DeletedAt == nil
}

func copyExt(e workshop.Extension) workshop.Extension {
	e.RecentErrors = append([]time.Time(nil), e.RecentErrors...)
	return e
}

// InsertExtension implements workshop.Store.
func (s *Store) InsertExtension(_ context.Context, e workshop.Extension) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.exts {
		if x.WorkspaceID == e.WorkspaceID && x.Slug == e.Slug && x.DeletedAt == nil {
			return db.ErrConflict
		}
	}
	s.exts[e.ID] = copyExt(e)
	return nil
}

// UpdateExtension implements workshop.Store.
func (s *Store) UpdateExtension(_ context.Context, e workshop.Extension) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.liveExt(e.WorkspaceID, e.ID)
	if !ok {
		return db.ErrNotFound
	}
	cur.Title, cur.Description, cur.State, cur.LiveVersion = e.Title, e.Description, e.State, e.LiveVersion
	cur.ApprovalOff, cur.DisabledReason, cur.MaintainerID, cur.UpdatedAt = e.ApprovalOff, e.DisabledReason, e.MaintainerID, e.UpdatedAt
	s.exts[e.ID] = cur
	return nil
}

// SetExtensionErrors implements workshop.Store.
func (s *Store) SetExtensionErrors(_ context.Context, workspaceID, id ids.ID, consecutive int, recent []time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.liveExt(workspaceID, id)
	if !ok {
		return db.ErrNotFound
	}
	cur.ConsecutiveErrors, cur.RecentErrors = consecutive, append([]time.Time(nil), recent...)
	s.exts[id] = cur
	return nil
}

// GetExtension implements workshop.Store.
func (s *Store) GetExtension(_ context.Context, workspaceID, id ids.ID) (workshop.Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveExt(workspaceID, id)
	if !ok {
		return workshop.Extension{}, db.ErrNotFound
	}
	return copyExt(e), nil
}

// ListExtensions implements workshop.Store.
func (s *Store) ListExtensions(_ context.Context, workspaceID ids.ID) ([]workshop.Extension, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.Extension
	for _, e := range s.exts {
		if e.WorkspaceID == workspaceID && e.DeletedAt == nil {
			out = append(out, copyExt(e))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// DeleteExtension implements workshop.Store.
func (s *Store) DeleteExtension(_ context.Context, workspaceID, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.liveExt(workspaceID, id)
	if !ok {
		return db.ErrNotFound
	}
	e.DeletedAt, e.UpdatedAt, e.State = &at, at, workshop.StateDisabled
	s.exts[id] = e
	return nil
}

// InsertVersion implements workshop.Store.
func (s *Store) InsertVersion(_ context.Context, v workshop.Version) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.versions[v.ExtensionID] {
		if x.Version == v.Version {
			return db.ErrConflict
		}
	}
	s.versions[v.ExtensionID] = append(s.versions[v.ExtensionID], v)
	return nil
}

// GetVersion implements workshop.Store.
func (s *Store) GetVersion(_ context.Context, workspaceID, extensionID ids.ID, version int) (workshop.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.versions[extensionID] {
		if v.WorkspaceID == workspaceID && v.Version == version {
			return v, nil
		}
	}
	return workshop.Version{}, db.ErrNotFound
}

// ListVersions implements workshop.Store.
func (s *Store) ListVersions(_ context.Context, workspaceID, extensionID ids.ID) ([]workshop.Version, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.Version
	for _, v := range s.versions[extensionID] {
		if v.WorkspaceID == workspaceID {
			v.Source, v.Bundle = "", ""
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

// InsertApproval implements workshop.Store.
func (s *Store) InsertApproval(_ context.Context, a workshop.Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals = append(s.approvals, a)
	return nil
}

// ListApprovals implements workshop.Store.
func (s *Store) ListApprovals(_ context.Context, workspaceID, extensionID ids.ID) ([]workshop.Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.Approval
	for i := len(s.approvals) - 1; i >= 0; i-- {
		a := s.approvals[i]
		if a.WorkspaceID == workspaceID && a.ExtensionID == extensionID {
			out = append(out, a)
		}
	}
	return out, nil
}

func docKey(ext ids.ID, collection, id string) string {
	return ext.String() + "/" + collection + "/" + id
}

// GetDoc implements workshop.Store.
func (s *Store) GetDoc(_ context.Context, workspaceID, extensionID ids.ID, collection, docID string) (workshop.StoreDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[docKey(extensionID, collection, docID)]
	if !ok || d.WorkspaceID != workspaceID {
		return workshop.StoreDoc{}, db.ErrNotFound
	}
	return d, nil
}

// PutDoc implements workshop.Store.
func (s *Store) PutDoc(_ context.Context, d workshop.StoreDoc) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[docKey(d.ExtensionID, d.Collection, d.DocID)] = d
	return nil
}

// DeleteDoc implements workshop.Store.
func (s *Store) DeleteDoc(_ context.Context, workspaceID, extensionID ids.ID, collection, docID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := docKey(extensionID, collection, docID)
	if d, ok := s.docs[k]; ok && d.WorkspaceID == workspaceID {
		delete(s.docs, k)
	}
	return nil
}

// ListDocs implements workshop.Store.
func (s *Store) ListDocs(_ context.Context, workspaceID, extensionID ids.ID, collection, after string, limit int) ([]workshop.StoreDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.StoreDoc
	for _, d := range s.docs {
		if d.WorkspaceID == workspaceID && d.ExtensionID == extensionID && d.Collection == collection && d.DocID > after {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DocID < out[j].DocID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// StoreUsage implements workshop.Store.
func (s *Store) StoreUsage(_ context.Context, workspaceID, extensionID ids.ID) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, b := 0, 0
	for _, d := range s.docs {
		if d.WorkspaceID == workspaceID && d.ExtensionID == extensionID {
			n++
			b += d.SizeBytes
		}
	}
	return n, b, nil
}
