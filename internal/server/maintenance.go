// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"context"
	"time"

	"colonyhall/internal/platform/db/system"
)

// maintenanceInterval is how often the housekeeping runs (also once at start).
const maintenanceInterval = time.Hour

// maintenanceLoop runs Maintain at start-up and then every interval until
// ctx ends. Every step is idempotent, so several servers may run it at once.
func (a *App) maintenanceLoop(ctx context.Context, interval time.Duration) {
	a.Maintain(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Maintain(ctx)
		}
	}
}

// Maintain deletes ended credentials and old login failures (round 40) and samples the database size (round
// 50). It logs counts only, never token data. It runs in system mode: the
// cleanup covers every workspace (round 45).
func (a *App) Maintain(ctx context.Context) {
	ctx = system.Context(ctx)
	if err := a.DBStats.SampleToday(ctx); err != nil && ctx.Err() == nil {
		a.logger.Warn("database size sample failed", "err", err)
	}
	if n, err := a.Identity.CleanupAuthFailures(ctx); err != nil {
		if ctx.Err() == nil {
			a.logger.Warn("login failure cleanup failed", "err", err)
		}
	} else if n > 0 {
		a.logger.Info("deleted old login failures", "rows", n)
	}
	res, err := a.Identity.CleanupCredentials(ctx)
	if err != nil {
		if ctx.Err() == nil {
			a.logger.Warn("credential cleanup failed", "err", err)
		}
		return
	}
	if res.HostTokens > 0 || res.Sessions > 0 {
		a.logger.Info("deleted ended credentials", "host_tokens", res.HostTokens, "sessions", res.Sessions)
	}
}
