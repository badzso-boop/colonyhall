// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package gateway

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

const (
	// feedSize is how many recent events a workspace feed keeps in memory.
	feedSize = 1024
	// feedReadTimeout bounds a shared read: it outlives the connection that
	// started it, because other connections wait for its result.
	feedReadTimeout = 30 * time.Second
)

// feed is the shared read path of one workspace's connections. Without it
// every connection reads the event log on every bus signal, so one event
// costs one query per open socket. The feed coalesces concurrent reads at the
// same cursor into one query and keeps the newest events in a ring buffer, so
// connections that are up to date catch up from memory.
//
// Correctness rests on the event log's guarantee that sequence numbers become
// visible in commit order: a read of Since(hi) that returns events up to X
// has seen every event of the workspace up to X, forever.
//
// Freshness uses tickets: every read takes the next ticket before it queries.
// A connection notes the current ticket when it wakes (observe); a read with
// a greater ticket began after the wake-up, so it saw every event the wake-up
// announced. Only such a read may answer "nothing new" for that connection.
//
// Events handed out are shared between connections and must not be modified.
type feed struct {
	store eventlog.Store
	ws    ids.ID
	batch int

	// refs counts the connections using the feed; guarded by Gateway.mu.
	refs int

	tickets atomic.Uint64

	mu sync.Mutex
	// valid is false until the first read sets the buffer's start.
	valid bool
	// The ring holds every event of the workspace with lo < Seq <= hi.
	lo, hi int64
	ring   []eventlog.Event
	start  int
	n      int
	// fresh is the ticket of the newest read that found nothing after hi.
	fresh uint64
	// frontier is the read in flight that extends the buffer from hi.
	frontier *feedCall
	// older are the reads in flight for cursors before the buffer, by cursor.
	older map[int64]*feedCall
}

// feedCall is one shared event log read.
type feedCall struct {
	ticket uint64
	after  int64
	// lo is the buffer's start when the read began (reads before the buffer).
	lo   int64
	done chan struct{}

	// Set before done is closed, read-only afterwards.
	events    []eventlog.Event // this workspace's events only
	next      int64            // the position the read covers up to
	truncated bool             // the read hit the batch limit
	err       error
}

func newFeed(store eventlog.Store, ws ids.ID, size, batch int) *feed {
	if size <= 0 {
		size = feedSize
	}
	if batch <= 0 {
		batch = readBatch
	}
	return &feed{store: store, ws: ws, batch: batch, ring: make([]eventlog.Event, size), older: map[int64]*feedCall{}}
}

// observe returns the ticket a connection records when it wakes up.
func (f *feed) observe() uint64 { return f.tickets.Load() }

// since returns the workspace's events after the cursor, at most one batch,
// and the position the answer covers up to (at least the last event's Seq).
// An empty answer whose position equals after means nothing was committed
// after the cursor before the caller observed obs. ctx must carry the
// connection's workspace scope.
func (f *feed) since(ctx context.Context, after int64, obs uint64) ([]eventlog.Event, int64, error) {
	for {
		f.mu.Lock()
		if !f.valid {
			f.valid, f.lo, f.hi = true, after, after
		}
		switch {
		case after < f.lo:
			events, next, done, err := f.readOlder(ctx, after, obs) // unlocks
			if done || err != nil {
				return events, next, err
			}
			continue
		case after < f.hi:
			events, next := f.bufferedLocked(after)
			f.mu.Unlock()
			return events, next, nil
		case f.fresh > obs:
			// A read that began after the caller woke found nothing after
			// hi, and after >= hi.
			f.mu.Unlock()
			return nil, after, nil
		}
		if c := f.frontier; c != nil {
			f.mu.Unlock()
			if err := wait(ctx, c); err != nil {
				return nil, after, err
			}
			continue
		}
		c := f.newCallLocked(f.hi)
		f.frontier = c
		f.mu.Unlock()
		f.run(ctx, c, func() {
			if c.err == nil {
				f.extendLocked(c)
			}
			f.frontier = nil
		})
		if c.err != nil {
			return nil, after, c.err
		}
	}
}

// readOlder serves a cursor that the buffer no longer covers straight from
// the event log, sharing the read with connections at the same cursor. It
// is called with f.mu held and releases it. done is false when the caller
// must try again: the read it waited for began before the caller woke, so
// it may have missed what woke it.
func (f *feed) readOlder(ctx context.Context, after int64, obs uint64) (events []eventlog.Event, next int64, done bool, err error) {
	c := f.older[after]
	if c == nil {
		c = f.newCallLocked(after)
		c.lo = f.lo
		f.older[after] = c
		f.mu.Unlock()
		f.run(ctx, c, func() {
			if f.older[after] == c {
				delete(f.older, after)
			}
		})
		return c.events, c.next, true, c.err
	}
	f.mu.Unlock()
	if err := wait(ctx, c); err != nil {
		return nil, after, true, err
	}
	if c.ticket > obs {
		return c.events, c.next, true, nil
	}
	return nil, after, false, nil
}

// newCallLocked starts a read: its ticket is taken before the query runs.
func (f *feed) newCallLocked(after int64) *feedCall {
	return &feedCall{ticket: f.tickets.Add(1), after: after, done: make(chan struct{})}
}

// run performs the read, then applies it under f.mu and wakes the waiters.
// The query keeps the caller's values (its workspace scope, telemetry) but
// not its cancellation: other connections wait for the same answer.
func (f *feed) run(ctx context.Context, c *feedCall, apply func()) {
	rctx, cancel := context.WithTimeout(db.WithWorkspace(context.WithoutCancel(ctx), f.ws), feedReadTimeout)
	raw, err := f.store.Since(rctx, c.after, f.batch)
	cancel()
	c.err = err
	if err == nil {
		c.next = c.after
		c.truncated = len(raw) >= f.batch
		c.events = make([]eventlog.Event, 0, len(raw))
		for _, e := range raw {
			c.next = e.Seq
			if e.WorkspaceID == f.ws {
				c.events = append(c.events, e)
			}
		}
		if !c.truncated && c.lo > c.next {
			// Nothing of the workspace lies between the end of this read
			// and the buffer's start (that start was committed before the
			// read began), so the cursor may move on to the buffer.
			c.next = c.lo
		}
	}
	f.mu.Lock()
	apply()
	f.mu.Unlock()
	close(c.done)
}

// extendLocked appends a frontier read to the buffer.
func (f *feed) extendLocked(c *feedCall) {
	if c.after != f.hi {
		return
	}
	for _, e := range c.events {
		f.pushLocked(e)
	}
	f.hi = c.next
	if !c.truncated && c.ticket > f.fresh {
		f.fresh = c.ticket
	}
}

// pushLocked adds an event to the ring, evicting the oldest when it is full.
func (f *feed) pushLocked(e eventlog.Event) {
	if f.n == len(f.ring) {
		f.lo = f.ring[f.start].Seq
		f.ring[f.start] = e
		f.start = (f.start + 1) % len(f.ring)
		return
	}
	f.ring[(f.start+f.n)%len(f.ring)] = e
	f.n++
}

func (f *feed) at(i int) eventlog.Event { return f.ring[(f.start+i)%len(f.ring)] }

// bufferedLocked returns up to one batch of buffered events after the
// cursor (lo <= after < hi) and the position they cover up to.
func (f *feed) bufferedLocked(after int64) ([]eventlog.Event, int64) {
	i := sort.Search(f.n, func(i int) bool { return f.at(i).Seq > after })
	end := min(f.n, i+f.batch)
	out := make([]eventlog.Event, 0, end-i)
	for j := i; j < end; j++ {
		out = append(out, f.at(j))
	}
	if end < f.n {
		return out, out[len(out)-1].Seq
	}
	return out, f.hi
}

// wait blocks until the read is done or the caller gives up.
func wait(ctx context.Context, c *feedCall) error {
	select {
	case <-c.done:
		return c.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
