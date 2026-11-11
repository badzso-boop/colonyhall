package identity

import (
	"context"
	"time"
)

// How long ended credentials are kept before the cleanup deletes them: a
// recent run's guard token can still be looked up while debugging.
const (
	HostTokenRetention = 7 * 24 * time.Hour
	SessionRetention   = 30 * 24 * time.Hour
)

// CleanupResult counts what one cleanup deleted.
type CleanupResult struct {
	HostTokens int64
	Sessions   int64
}

// CleanupCredentials deletes short-lived guard tokens that ended more than
// HostTokenRetention ago and sessions that ended more than SessionRetention
// ago. Tokens made by hand (no host) are never deleted, even when revoked.
// Credentials are not append-only data: who used what stays in the run log
// and the audit log. The deletes are idempotent, so several servers may run
// it at the same time.
func (s *Service) CleanupCredentials(ctx context.Context) (CleanupResult, error) {
	now := s.Clock.Now()
	var res CleanupResult
	var err error
	if res.HostTokens, err = s.Store.DeleteExpiredHostTokens(ctx, now.Add(-HostTokenRetention)); err != nil {
		return res, err
	}
	res.Sessions, err = s.Store.DeleteEndedSessions(ctx, now.Add(-SessionRetention))
	return res, err
}
