package identity

import (
	"context"
	"testing"
	"time"

	"colonyhall/internal/platform/clock"
)

// cutoffStore records the cut-offs the cleanup asks for.
type cutoffStore struct {
	Store
	tokensBefore, sessionsBefore []time.Time
}

func (s *cutoffStore) DeleteExpiredHostTokens(_ context.Context, before time.Time) (int64, error) {
	s.tokensBefore = append(s.tokensBefore, before)
	return 3, nil
}

func (s *cutoffStore) DeleteEndedSessions(_ context.Context, before time.Time) (int64, error) {
	s.sessionsBefore = append(s.sessionsBefore, before)
	return 1, nil
}

func TestCleanupCredentialsCutoffsFollowTheClock(t *testing.T) {
	t0 := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	clk := clock.NewManual(t0)
	st := &cutoffStore{}
	svc := &Service{Store: st, Clock: clk}

	res, err := svc.CleanupCredentials(context.Background())
	if err != nil || res.HostTokens != 3 || res.Sessions != 1 {
		t.Fatalf("result %+v, %v", res, err)
	}
	clk.Advance(time.Hour)
	if _, err := svc.CleanupCredentials(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantTok := []time.Time{t0.Add(-7 * 24 * time.Hour), t0.Add(time.Hour - 7*24*time.Hour)}
	wantSess := []time.Time{t0.Add(-30 * 24 * time.Hour), t0.Add(time.Hour - 30*24*time.Hour)}
	for i := range 2 {
		if !st.tokensBefore[i].Equal(wantTok[i]) || !st.sessionsBefore[i].Equal(wantSess[i]) {
			t.Fatalf("run %d: cut-offs %v / %v, want %v / %v", i, st.tokensBefore[i], st.sessionsBefore[i], wantTok[i], wantSess[i])
		}
	}
}

type failureCutoffStore struct {
	Store
	before []time.Time
}

func (s *failureCutoffStore) DeleteAuthFailures(_ context.Context, before time.Time) (int64, error) {
	s.before = append(s.before, before)
	return 2, nil
}

func TestCleanupAuthFailuresKeepsAWeek(t *testing.T) {
	t0 := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	st := &failureCutoffStore{}
	svc := &Service{Store: st, Clock: clock.NewManual(t0)}
	if n, err := svc.CleanupAuthFailures(context.Background()); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if len(st.before) != 1 || !st.before[0].Equal(t0.Add(-7*24*time.Hour)) {
		t.Fatalf("cut-off %v", st.before)
	}
}
