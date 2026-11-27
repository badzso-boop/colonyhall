// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/eventlog/memory"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// countingStore counts event log reads and checks that each one runs in a
// single workspace's scope. With rls it returns only that workspace's events,
// as row-level security does in Postgres. hold, when set, is waited on after
// the read took its snapshot, so a test can keep a read "in flight".
type countingStore struct {
	*memory.Store
	rls      bool
	reads    atomic.Int64
	badScope atomic.Int64
	delay    time.Duration
	hold     chan struct{}
	entered  chan struct{}
}

func (s *countingStore) Since(ctx context.Context, after int64, limit int) ([]eventlog.Event, error) {
	s.reads.Add(1)
	scope := db.ScopeOf(ctx)
	if scope.System || scope.Operator || scope.Workspace == ids.Nil {
		s.badScope.Add(1)
	}
	var out []eventlog.Event
	var err error
	if s.rls {
		all, _ := s.Store.Since(ctx, after, 1<<30)
		for _, e := range all {
			if e.WorkspaceID == scope.Workspace && len(out) < limit {
				out = append(out, e)
			}
		}
	} else {
		out, err = s.Store.Since(ctx, after, limit)
	}
	if s.entered != nil {
		s.entered <- struct{}{}
	}
	if s.hold != nil {
		<-s.hold
	}
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return out, err
}

type feedFixture struct {
	t     *testing.T
	store *countingStore
	gw    *Gateway
	at    time.Time
}

func newFeedFixture(t *testing.T, size int) *feedFixture {
	store := &countingStore{Store: memory.New()}
	t.Cleanup(func() {
		if n := store.badScope.Load(); n > 0 {
			t.Errorf("%d event log reads ran outside a workspace scope", n)
		}
	})
	return &feedFixture{t: t, store: store, gw: &Gateway{Events: store, feedSize: size},
		at: time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC)}
}

func (fx *feedFixture) append(ws ids.ID) int64 {
	fx.t.Helper()
	e, err := eventlog.NewEvent(newID(), ws, "workspace.test", newID(), ids.Nil, "test", newID(), map[string]string{}, fx.at)
	if err != nil {
		fx.t.Fatal(err)
	}
	if err := fx.store.Append(context.Background(), &e); err != nil {
		fx.t.Fatal(err)
	}
	return e.Seq
}

// conn registers an admin connection (it sees every event of its workspace)
// at the given cursor.
func (fx *feedFixture) conn(ws ids.ID, cursor int64) *connection {
	c := &connection{g: fx.gw, cursor: cursor, canRead: map[ids.ID]bool{},
		actor: identity.Actor{ID: newID(), WorkspaceID: ws, Kind: identity.KindHuman, Role: identity.RoleAdmin}}
	fx.gw.register(c)
	return c
}

// collect replays a connection and returns the sequence numbers it sent.
func collect(t *testing.T, c *connection, obs uint64) []int64 {
	t.Helper()
	var got []int64
	err := c.replay(db.WithWorkspace(context.Background(), c.actor.WorkspaceID), obs, func(_ context.Context, v any) error {
		got = append(got, v.(outEvent).Seq)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func seqs(from, to int64) []int64 {
	var out []int64
	for s := from; s <= to; s++ {
		out = append(out, s)
	}
	return out
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// catchUpAll wakes every connection at once, as a bus signal does, and
// returns what each one sent.
func catchUpAll(t *testing.T, conns []*connection, obsAtStart bool) [][]int64 {
	t.Helper()
	got := make([][]int64, len(conns))
	var obs uint64
	if obsAtStart {
		obs = conns[0].feed.observe()
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, c := range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			o := obs
			if !obsAtStart {
				o = c.feed.observe() // what catchUp does right after the wake-up
			}
			got[i] = collect(t, c, o)
		}()
	}
	close(start)
	wg.Wait()
	return got
}

// One event must cost one event log read for all the connections of a
// workspace, not one per connection (before the feed: 2 per connection,
// the read that finds the event and the one that finds nothing more).
func TestFeedOneReadForAllConnectionsOfAWorkspace(t *testing.T) {
	const n = 50
	fx := newFeedFixture(t, 0)
	ws := newID()
	for range 10 {
		fx.append(ws)
	}
	head, _ := fx.store.Head(context.Background())
	conns := make([]*connection, n)
	for i := range conns {
		conns[i] = fx.conn(ws, head)
	}
	catchUpAll(t, conns, true) // the initial catch-up after the welcome

	fx.store.reads.Store(0)
	seq := fx.append(ws)
	for i, got := range catchUpAll(t, conns, true) {
		if !equal(got, []int64{seq}) {
			t.Fatalf("connection %d sent %v, want [%d]", i, got, seq)
		}
	}
	if r := fx.store.reads.Load(); r != 1 {
		t.Fatalf("%d connections caught up with %d event log reads, want 1", n, r)
	}
}

// With a realistic read latency and every connection observing its own
// wake-up, the reads still coalesce into a handful, far fewer than n.
func TestFeedCoalescesConcurrentWakeUps(t *testing.T) {
	const n = 50
	fx := newFeedFixture(t, 0)
	fx.store.delay = 20 * time.Millisecond
	ws := newID()
	fx.append(ws)
	conns := make([]*connection, n)
	for i := range conns {
		conns[i] = fx.conn(ws, 1)
	}
	catchUpAll(t, conns, false)

	for round := range 3 {
		fx.store.reads.Store(0)
		seq := fx.append(ws)
		for i, got := range catchUpAll(t, conns, false) {
			if !equal(got, []int64{seq}) {
				t.Fatalf("round %d: connection %d sent %v, want [%d]", round, i, got, seq)
			}
		}
		r := fx.store.reads.Load()
		t.Logf("round %d: %d connections, %d event log reads (before the feed: %d)", round, n, r, 2*n)
		if r > n/10 {
			t.Fatalf("round %d: %d event log reads for %d connections", round, r, n)
		}
	}
}

// A read that took its snapshot before a connection woke up must not answer
// for it: the event that woke it may be missing from that snapshot.
func TestFeedDoesNotTrustAReadOlderThanTheWakeUp(t *testing.T) {
	fx := newFeedFixture(t, 0)
	ws := newID()
	fx.append(ws)
	a, b := fx.conn(ws, 1), fx.conn(ws, 1)

	fx.store.hold = make(chan struct{})
	fx.store.entered = make(chan struct{}, 4)
	gotA := make(chan []int64)
	obsA := a.feed.observe()
	go func() { gotA <- collect(t, a, obsA) }()
	<-fx.store.entered // a's read has its (empty) snapshot

	seq := fx.append(ws)
	obsB := b.feed.observe() // b wakes up for seq while a's read is in flight
	gotB := make(chan []int64)
	go func() { gotB <- collect(t, b, obsB) }()

	fx.store.hold <- struct{}{} // a's read returns nothing
	<-fx.store.entered          // b's own read
	fx.store.hold <- struct{}{}
	if got := <-gotB; !equal(got, []int64{seq}) {
		t.Fatalf("b sent %v, want [%d]", got, seq)
	}
	close(fx.store.hold)
	<-gotA
}

// A cursor older than the buffer is served from the event log, without gaps.
func TestFeedCursorOlderThanTheBufferReadsTheLog(t *testing.T) {
	fx := newFeedFixture(t, 4)
	ws := newID()
	for range 20 {
		fx.append(ws)
	}
	first := fx.conn(ws, 0)
	if got := collect(t, first, first.feed.observe()); !equal(got, seqs(1, 20)) {
		t.Fatalf("first connection sent %v", got)
	}
	if first.feed.lo != 16 || first.feed.hi != 20 {
		t.Fatalf("buffer covers (%d, %d], want (16, 20]", first.feed.lo, first.feed.hi)
	}

	fx.store.reads.Store(0)
	late := fx.conn(ws, 3)
	if got := collect(t, late, late.feed.observe()); !equal(got, seqs(4, 20)) {
		t.Fatalf("late connection sent %v, want 4..20", got)
	}
	if fx.store.reads.Load() == 0 {
		t.Fatal("the late connection never read the event log")
	}
}

// A cursor older than a buffer that holds nothing yet still moves onto it
// when nothing of the workspace lies in between.
func TestFeedOlderCursorJoinsAnEmptyBuffer(t *testing.T) {
	fx := newFeedFixture(t, 0)
	fx.store.rls = true
	ws, other := newID(), newID()
	for range 5 {
		fx.append(other)
	}
	head := fx.conn(ws, 5)
	collect(t, head, head.feed.observe())
	behind := fx.conn(ws, 2)
	collect(t, behind, behind.feed.observe())
	if behind.cursor != 5 {
		t.Fatalf("cursor %d, want 5", behind.cursor)
	}
}

// Two workspaces never share a feed or see each other's events, with
// row-level security and even when the store returns every workspace's events.
func TestFeedKeepsWorkspacesApart(t *testing.T) {
	for _, rls := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-rls", true: "rls"}[rls], func(t *testing.T) {
			testFeedKeepsWorkspacesApart(t, rls)
		})
	}
}

func testFeedKeepsWorkspacesApart(t *testing.T, rls bool) {
	fx := newFeedFixture(t, 0)
	fx.store.rls = rls
	wsA, wsB := newID(), newID()
	var wantA, wantB []int64
	for i := range 12 {
		if i%3 == 0 {
			wantB = append(wantB, fx.append(wsB))
		} else {
			wantA = append(wantA, fx.append(wsA))
		}
	}
	a, b := fx.conn(wsA, 0), fx.conn(wsB, 0)
	if a.feed == b.feed {
		t.Fatal("two workspaces share a feed")
	}
	if got := collect(t, a, a.feed.observe()); !equal(got, wantA) {
		t.Fatalf("workspace A sent %v, want %v", got, wantA)
	}
	if got := collect(t, b, b.feed.observe()); !equal(got, wantB) {
		t.Fatalf("workspace B sent %v, want %v", got, wantB)
	}
	for _, f := range []*feed{a.feed, b.feed} {
		for i := range f.n {
			if e := f.at(i); e.WorkspaceID != f.ws {
				t.Fatalf("feed of %s buffered event %d of %s", f.ws, e.Seq, e.WorkspaceID)
			}
		}
	}
}

// The last connection of a workspace releases its feed.
func TestFeedReleasedWithTheLastConnection(t *testing.T) {
	fx := newFeedFixture(t, 0)
	ws := newID()
	c1, c2 := fx.conn(ws, 0), fx.conn(ws, 0)
	if c1.feed != c2.feed {
		t.Fatal("connections of one workspace do not share a feed")
	}
	fx.gw.unregister(c1)
	if len(fx.gw.feeds) != 1 {
		t.Fatal("the feed was released while a connection still uses it")
	}
	fx.gw.unregister(c2)
	fx.gw.unregister(c2) // twice is harmless
	if len(fx.gw.feeds) != 0 {
		t.Fatalf("%d feeds left after the last connection", len(fx.gw.feeds))
	}
}
