// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"context"
	"errors"

	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
)

// customers is the operator's view of the workspaces (round 51,
// operator.Customers). It reads each workspace in its own narrowed scope
// and returns account data and numbers only: no channel, message, file or
// person names. The operator scope itself never sees a workspace's rows.
type customers struct {
	identity *identity.Service
	idStore  identity.Store
	hosts    hosts.Store
	files    *files.Service
	audit    audit.Store
	clock    clock.Clock
	// closeWorkspace drops the suspended workspace's open sockets.
	closeWorkspace func(id ids.ID)
}

func (c *customers) List(ctx context.Context) ([]operator.Customer, error) {
	// The list of workspace ids is the one system-mode read.
	list, err := c.idStore.ListWorkspaces(system.Context(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]operator.Customer, 0, len(list))
	for _, w := range list {
		cu, err := c.Get(ctx, w.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, cu)
	}
	return out, nil
}

func (c *customers) Get(ctx context.Context, id ids.ID) (operator.Customer, error) {
	ctx = db.WithWorkspace(ctx, id)
	ws, err := c.idStore.WorkspaceByID(ctx, id)
	if err != nil {
		return operator.Customer{}, err
	}
	out := operator.Customer{ID: ws.ID, Name: ws.Name, Slug: ws.Slug, State: ws.State, SuspendedAt: ws.SuspendedAt,
		SuspendedReason: ws.SuspendedReason, CreatedAt: ws.CreatedAt}
	actors, err := c.idStore.ListActors(ctx, id)
	if err != nil {
		return operator.Customer{}, err
	}
	for _, a := range actors {
		switch a.Kind {
		case identity.KindHuman:
			out.Users++
			if a.Role == identity.RoleOwner {
				out.HasOwner = true
			}
		case identity.KindAgent:
			out.Agents++
		}
	}
	hs, err := c.hosts.ListHosts(ctx, id)
	if err != nil {
		return operator.Customer{}, err
	}
	now := c.clock.Now()
	for _, h := range hs {
		if h.State == hosts.StateRevoked {
			continue
		}
		out.Hosts++
		if h.LastSeenAt != nil && now.Sub(*h.LastSeenAt) <= hosts.OnlineWindow {
			out.HostsOnline++
		}
	}
	// Every event reaches the audit chain: its last entry is the last activity.
	if last, err := c.audit.Last(ctx, id); err == nil {
		at := last.At
		out.LastActivity = &at
	} else if !errors.Is(err, db.ErrNotFound) {
		return operator.Customer{}, err
	}
	u, err := c.files.StorageState(ctx, id)
	if err != nil {
		return operator.Customer{}, err
	}
	out.Storage = operator.Storage{UsedBytes: u.Used(), QuotaBytes: u.QuotaBytes, FilesBytes: u.FilesBytes, DBBytes: u.DBBytes, State: string(u.State())}
	if inv, err := c.identity.LatestInvite(ctx, id); err == nil {
		out.Invite = &operator.InviteState{CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt, Used: inv.UsedAt != nil, Revoked: inv.RevokedAt != nil}
	} else if !errors.Is(err, db.ErrNotFound) {
		return operator.Customer{}, err
	}
	return out, nil
}

func (c *customers) Create(ctx context.Context, name, slug string) (operator.Customer, error) {
	ws, err := c.identity.CreateWorkspace(ctx, name, slug)
	if err != nil {
		return operator.Customer{}, err
	}
	return c.Get(ctx, ws.ID)
}

func (c *customers) Invite(ctx context.Context, id, operatorID ids.ID) (operator.InviteLink, error) {
	inv, token, err := c.identity.IssueInvite(ctx, id, operatorID)
	if err != nil {
		return operator.InviteLink{}, err
	}
	return operator.InviteLink{Token: token, ExpiresAt: inv.ExpiresAt}, nil
}

func (c *customers) SetSuspended(ctx context.Context, id ids.ID, suspend bool, reason string) (operator.Customer, error) {
	if _, err := c.identity.SetWorkspaceSuspended(ctx, id, suspend, reason); err != nil {
		return operator.Customer{}, err
	}
	if suspend && c.closeWorkspace != nil {
		c.closeWorkspace(id)
	}
	return c.Get(ctx, id)
}
