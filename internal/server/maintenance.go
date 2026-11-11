package server

import (
	"context"
	"time"
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

// Maintain deletes ended credentials (round 50). It logs counts only, never
// token data.
func (a *App) Maintain(ctx context.Context) {
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
