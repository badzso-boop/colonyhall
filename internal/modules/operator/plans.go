// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Plan is a subscription plan (round 51b, docs/24 7. pont). The operator
// edits them; a plan limits the workspace's storage, and states the CPU
// cores and memory a hosted guard will get (stored and shown, enforced
// later).
type Plan struct {
	ID           ids.ID
	Name         string
	StorageBytes int64
	CPUCores     int
	MemoryBytes  int64
	ArchivedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PlanPatch changes a plan; nil fields stay.
type PlanPatch struct {
	Name         *string
	StorageBytes *int64
	CPUCores     *int
	MemoryBytes  *int64
	Archived     *bool
	// ConfirmOverQuota: go ahead although a workspace on the plan would be
	// full with the new storage.
	ConfirmOverQuota bool
}

// PlanCheck is what a plan would mean for a workspace's storage.
type PlanCheck struct {
	UsedBytes     int64
	NewQuotaBytes int64
	Fits          bool
	// StateAfter: ok, warning_80, warning_95 or full (round 54).
	StateAfter string
}

// Plan limits.
const (
	MaxPlanBytes = int64(1) << 50 // 1 PiB, as the quota of round 54
	MaxCPUCores  = 1024
)

var (
	// ErrInvalidPlan: a missing name or a number out of range.
	ErrInvalidPlan = errors.New("a plan needs a name (1–60 characters), storage, CPU cores (1–1024) and memory")
	// ErrPlanArchived: an archived plan cannot be assigned.
	ErrPlanArchived = errors.New("the plan is archived")
	// ErrPlanNameTaken: plan names are unique.
	ErrPlanNameTaken = errors.New("a plan with this name exists")
)

func validPlan(p Plan) error {
	n := utf8.RuneCountInString(p.Name)
	if strings.TrimSpace(p.Name) == "" || n > 60 || p.StorageBytes <= 0 || p.StorageBytes > MaxPlanBytes ||
		p.CPUCores <= 0 || p.CPUCores > MaxCPUCores || p.MemoryBytes <= 0 || p.MemoryBytes > MaxPlanBytes {
		return ErrInvalidPlan
	}
	return nil
}

// PlanWithUse is a plan and the number of workspaces on it.
type PlanWithUse struct {
	Plan
	Workspaces int
}

// ListPlans lists every plan with the number of workspaces on it.
func (s *Service) ListPlans(ctx context.Context) ([]PlanWithUse, error) {
	ctx = scope(ctx)
	plans, err := s.Store.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	use, err := s.planUse(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PlanWithUse, len(plans))
	for i, p := range plans {
		out[i] = PlanWithUse{Plan: p, Workspaces: len(use[p.ID])}
	}
	return out, nil
}

// planUse maps each plan to the workspaces on it.
func (s *Service) planUse(ctx context.Context) (map[ids.ID][]Customer, error) {
	list, err := s.Customers.List(ctx)
	if err != nil {
		return nil, err
	}
	use := map[ids.ID][]Customer{}
	for _, c := range list {
		if c.PlanID != ids.Nil {
			use[c.PlanID] = append(use[c.PlanID], c)
		}
	}
	return use, nil
}

// CreatePlan adds a plan.
func (s *Service) CreatePlan(ctx context.Context, o Operator, in Plan, remote string) (PlanWithUse, error) {
	ctx = scope(ctx)
	now := s.Clock.Now()
	p := Plan{ID: s.IDs.New(), Name: strings.TrimSpace(in.Name), StorageBytes: in.StorageBytes, CPUCores: in.CPUCores,
		MemoryBytes: in.MemoryBytes, CreatedAt: now, UpdatedAt: now}
	if err := validPlan(p); err != nil {
		return PlanWithUse{}, err
	}
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreatePlan(ctx, p); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrPlanNameTaken
			}
			return err
		}
		return s.audit(ctx, o.ID, ActionPlanCreated, ids.Nil, planPayload(p), remote)
	})
	return PlanWithUse{Plan: p}, err
}

// UpdatePlan changes a plan. Every workspace on it gets the new numbers;
// when the new storage would leave one of them full, nothing changes
// unless the patch confirms it (ErrPlanOverQuota from the storage module).
func (s *Service) UpdatePlan(ctx context.Context, o Operator, id ids.ID, patch PlanPatch, remote string) (PlanWithUse, error) {
	ctx = scope(ctx)
	p, err := s.Store.PlanByID(ctx, id)
	if err != nil {
		return PlanWithUse{}, err
	}
	if patch.Name != nil {
		p.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.StorageBytes != nil {
		p.StorageBytes = *patch.StorageBytes
	}
	if patch.CPUCores != nil {
		p.CPUCores = *patch.CPUCores
	}
	if patch.MemoryBytes != nil {
		p.MemoryBytes = *patch.MemoryBytes
	}
	if patch.Archived != nil {
		p.ArchivedAt = nil
		if *patch.Archived {
			now := s.Clock.Now()
			p.ArchivedAt = &now
		}
	}
	if err := validPlan(p); err != nil {
		return PlanWithUse{}, err
	}
	use, err := s.planUse(ctx)
	if err != nil {
		return PlanWithUse{}, err
	}
	on := use[p.ID]
	// All or nothing: check every workspace before any changes.
	if !patch.ConfirmOverQuota {
		for _, c := range on {
			chk, err := s.Customers.CheckPlan(ctx, c.ID, p)
			if err != nil {
				return PlanWithUse{}, err
			}
			// Only a change that makes it full needs the confirmation.
			fullBefore := c.Storage.QuotaBytes > 0 && c.Storage.UsedBytes >= c.Storage.QuotaBytes
			if !chk.Fits && !fullBefore {
				return PlanWithUse{}, &PlanOverQuotaError{Workspace: c.Slug, Used: chk.UsedBytes, Quota: chk.NewQuotaBytes}
			}
		}
	}
	p.UpdatedAt = s.Clock.Now()
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.UpdatePlan(ctx, p); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrPlanNameTaken
			}
			return err
		}
		return s.audit(ctx, o.ID, ActionPlanUpdated, ids.Nil, planPayload(p), remote)
	})
	if err != nil {
		return PlanWithUse{}, err
	}
	for _, c := range on {
		if _, err := s.Customers.SetPlan(ctx, c.ID, &p, true); err != nil {
			return PlanWithUse{}, err
		}
	}
	return PlanWithUse{Plan: p, Workspaces: len(on)}, nil
}

// AssignPlan sets a workspace's plan (nil: none). A plan below the usage
// needs confirmOverQuota (ErrPlanOverQuota otherwise).
func (s *Service) AssignPlan(ctx context.Context, o Operator, workspaceID ids.ID, planID *ids.ID, confirmOverQuota bool, remote string) (Customer, error) {
	ctx = scope(ctx)
	var p *Plan
	if planID != nil {
		got, err := s.Store.PlanByID(ctx, *planID)
		if errors.Is(err, db.ErrNotFound) {
			return Customer{}, ErrInvalidPlan
		}
		if err != nil {
			return Customer{}, err
		}
		if got.ArchivedAt != nil {
			return Customer{}, ErrPlanArchived
		}
		p = &got
	}
	c, err := s.Customers.SetPlan(ctx, workspaceID, p, confirmOverQuota)
	if err != nil {
		return Customer{}, err
	}
	payload := map[string]any{"plan_id": nil, "confirmed_over_quota": confirmOverQuota}
	if p != nil {
		payload["plan_id"], payload["plan_name"] = p.ID.String(), p.Name
	}
	return c, s.record(ctx, o, ActionPlanAssigned, workspaceID, payload, remote)
}

// CheckPlan tells what a plan would mean for a workspace's storage.
func (s *Service) CheckPlan(ctx context.Context, workspaceID, planID ids.ID) (PlanCheck, error) {
	ctx = scope(ctx)
	p, err := s.Store.PlanByID(ctx, planID)
	if err != nil {
		return PlanCheck{}, err
	}
	return s.Customers.CheckPlan(ctx, workspaceID, p)
}

// PlanOverQuotaError: a plan change would leave a workspace full; the
// operator must confirm it.
type PlanOverQuotaError struct {
	Workspace   string
	Used, Quota int64
}

func (e *PlanOverQuotaError) Error() string {
	return fmt.Sprintf("Workspace %s uses %d bytes, the plan would give %d: it would be full and its agents would pause. Confirm to go ahead.", e.Workspace, e.Used, e.Quota)
}

// Is makes it match the storage module's ErrPlanOverQuota answer.
func (e *PlanOverQuotaError) Is(target error) bool { return target == ErrPlanOverQuota }

// ErrPlanOverQuota is the operator's view of the storage module's error
// (the composition root maps both to 409 plan_over_quota).
var ErrPlanOverQuota = errors.New("the plan is below a workspace's current usage")

func planPayload(p Plan) map[string]any {
	return map[string]any{"plan_id": p.ID.String(), "name": p.Name, "storage_bytes": p.StorageBytes,
		"cpu_cores": p.CPUCores, "memory_bytes": p.MemoryBytes, "archived": p.ArchivedAt != nil}
}
