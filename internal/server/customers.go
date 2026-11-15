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
	files    *files.Service
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
		u := usageOf(*st)
		st.UsedBytes, st.QuotaBytes, st.State = u.Used(), u.Limit(), string(u.State())
	}
	return list, nil
}

// usageOf rebuilds round 54's usage from the overview's numbers, so the
// effective quota and the state follow the storage module's own rules.
func usageOf(st operator.Storage) files.Usage {
	u := files.Usage{QuotaBytes: st.OwnerQuotaBytes, FilesBytes: st.FilesBytes, DBBytes: st.DBBytes}
	if st.PlanStorageBytes > 0 {
		u.Plan = &files.Plan{StorageBytes: st.PlanStorageBytes}
	}
	return u
}

// SetPlan writes the plan's numbers into the workspace's storage (in that
// workspace only), through the storage module, which keeps round 54's
// states and warnings.
func (c *customers) SetPlan(ctx context.Context, id ids.ID, plan *operator.Plan, confirmOverQuota bool) (operator.Customer, error) {
	if _, err := c.Get(ctx, id); err != nil {
		return operator.Customer{}, err
	}
	var p *files.Plan
	if plan != nil {
		p = &files.Plan{ID: plan.ID, Name: plan.Name, StorageBytes: plan.StorageBytes, CPUCores: plan.CPUCores, MemoryBytes: plan.MemoryBytes}
	}
	if _, err := c.files.SetPlan(db.WithWorkspace(ctx, id), id, p, confirmOverQuota); err != nil {
		return operator.Customer{}, err
	}
	return c.Get(ctx, id)
}

// CheckPlan tells, from the overview's numbers, what the plan would do.
func (c *customers) CheckPlan(ctx context.Context, id ids.ID, plan operator.Plan) (operator.PlanCheck, error) {
	cu, err := c.Get(ctx, id)
	if err != nil {
		return operator.PlanCheck{}, err
	}
	u := usageOf(cu.Storage)
	u.Plan = &files.Plan{StorageBytes: plan.StorageBytes}
	lim := u.Limit()
	return operator.PlanCheck{UsedBytes: u.Used(), NewQuotaBytes: lim, Fits: lim == 0 || u.Used() < lim, StateAfter: string(u.State())}, nil
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
