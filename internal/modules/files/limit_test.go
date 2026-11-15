// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files_test

import (
	"testing"

	"colonyhall/internal/modules/files"
)

// Round 51b: the effective quota is the plan's storage, or the owner's
// quota when that is lower and set; the state follows it.
func TestLimitIsThePlanOrTheLowerOwnerQuota(t *testing.T) {
	plan := func(n int64) *files.Plan { return &files.Plan{StorageBytes: n} }
	for _, c := range []struct {
		quota int64
		plan  *files.Plan
		want  int64
	}{
		{0, nil, 0},            // nothing: unlimited
		{500, nil, 500},        // the owner's quota alone
		{0, plan(1000), 1000},  // the plan alone
		{500, plan(1000), 500}, // the owner lowered it
		{2000, plan(1000), 1000},
		{0, plan(0), 0}, // a plan without storage changes nothing
	} {
		u := files.Usage{QuotaBytes: c.quota, Plan: c.plan}
		if got := u.Limit(); got != c.want {
			t.Errorf("quota %d plan %+v: limit %d, want %d", c.quota, c.plan, got, c.want)
		}
	}
	u := files.Usage{FilesBytes: 960, Plan: plan(1000)}
	if u.State() != files.StorageWarning95 {
		t.Fatalf("960 of a 1000-byte plan: %s", u.State())
	}
	u.QuotaBytes = 900
	if u.State() != files.StorageFull {
		t.Fatalf("960 of the owner's 900: %s", u.State())
	}
}
