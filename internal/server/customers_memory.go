// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"time"

	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// memoryOverview is operator.Overview for the in-memory stores (development
// and tests). They have no row-level security to stay inside, so it counts
// from the modules' stores, by the rules colony_operator_overview() follows
// on Postgres (migrations/00069): actors not archived, hosts not revoked,
// the newest audit entry, the newest invite. Postgres never uses it.
type memoryOverview struct {
	identity *identity.Service
	idStore  identity.Store
	hosts    hosts.Store
	files    *files.Service
	audit    audit.Store
}

func (m *memoryOverview) Customers(ctx context.Context, only ids.ID, onlineSince time.Time) ([]operator.Customer, error) {
	list, err := m.idStore.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(list, func(i, j int) bool {
		if !list[i].CreatedAt.Equal(list[j].CreatedAt) {
			return list[i].CreatedAt.Before(list[j].CreatedAt)
		}
		return bytes.Compare(list[i].ID[:], list[j].ID[:]) < 0
	})
	var out []operator.Customer
	for _, ws := range list {
		if only != ids.Nil && ws.ID != only {
			continue
		}
		cu := operator.Customer{ID: ws.ID, Name: ws.Name, Slug: ws.Slug, State: ws.State, SuspendedAt: ws.SuspendedAt,
			SuspendedReason: ws.SuspendedReason, CreatedAt: ws.CreatedAt}
		actors, err := m.idStore.ListActors(ctx, ws.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range actors {
			switch a.Kind {
			case identity.KindHuman:
				cu.Users++
				if a.Role == identity.RoleOwner {
					cu.HasOwner = true
				}
			case identity.KindAgent:
				cu.Agents++
			}
		}
		hs, err := m.hosts.ListHosts(ctx, ws.ID)
		if err != nil {
			return nil, err
		}
		for _, h := range hs {
			if h.State == hosts.StateRevoked {
				continue
			}
			cu.Hosts++
			if h.LastSeenAt != nil && !h.LastSeenAt.Before(onlineSince) {
				cu.HostsOnline++
			}
		}
		if last, err := m.audit.Last(ctx, ws.ID); err == nil {
			at := last.At
			cu.LastActivity = &at
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		u, err := m.files.StorageState(ctx, ws.ID)
		if err != nil {
			return nil, err
		}
		cu.Storage = operator.Storage{OwnerQuotaBytes: u.QuotaBytes, FilesBytes: u.FilesBytes, DBBytes: u.DBBytes}
		if u.Plan != nil {
			cu.PlanID, cu.Storage.PlanStorageBytes = u.Plan.ID, u.Plan.StorageBytes
		}
		if inv, err := m.identity.LatestInvite(ctx, ws.ID); err == nil {
			cu.Invite = &operator.InviteState{CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt, Used: inv.UsedAt != nil, Revoked: inv.RevokedAt != nil}
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		out = append(out, cu)
	}
	return out, nil
}
