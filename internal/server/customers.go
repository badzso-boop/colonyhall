// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"context"

	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// customers is the operator's view of the workspaces (round 51,
// operator.Customers). Every read goes through operator.Overview in the
// operator scope itself (Postgres: colony_operator_overview(), account data
// and numbers only), so this code never switches system mode or a
// workspace's scope to look (depguard keeps db/system out of this file).
// The writes go through the identity module's narrow methods, each in the
// one workspace it changes.
type customers struct {
	identity *identity.Service
	overview operator.Overview
	clock    clock.Clock
	// closeWorkspace drops the suspended workspace's open sockets.
	closeWorkspace func(id ids.ID)
}

// read returns the overview of one workspace (only) or all (ids.Nil), with
// the storage's used bytes and state after round 54's rules.
func (c *customers) read(ctx context.Context, only ids.ID) ([]operator.Customer, error) {
	list, err := c.overview.Customers(ctx, only, c.clock.Now().Add(-hosts.OnlineWindow))
	if err != nil {
		return nil, err
	}
	for i := range list {
		st := &list[i].Storage
		u := files.Usage{QuotaBytes: st.QuotaBytes, FilesBytes: st.FilesBytes, DBBytes: st.DBBytes}
		st.UsedBytes, st.State = u.Used(), string(u.State())
	}
	return list, nil
}

func (c *customers) List(ctx context.Context) ([]operator.Customer, error) {
	return c.read(ctx, ids.Nil)
}

func (c *customers) Get(ctx context.Context, id ids.ID) (operator.Customer, error) {
	if id == ids.Nil {
		return operator.Customer{}, db.ErrNotFound
	}
	list, err := c.read(ctx, id)
	if err != nil {
		return operator.Customer{}, err
	}
	if len(list) != 1 {
		return operator.Customer{}, db.ErrNotFound
	}
	return list[0], nil
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
