// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package bus carries "something changed" signals between components.
// Following the Dendrite notifier pattern, a notification only carries the
// newest event sequence number; receivers always read the events themselves
// from the event log, so a lost or merged signal never loses data.
package bus

import "sync"

// Bus publishes the latest committed event sequence to subscribers.
type Bus interface {
	// Notify announces that events up to seq are committed.
	Notify(seq int64)
	// Subscribe returns a channel that receives sequence numbers. The channel
	// has capacity 1 and keeps only the newest value, so a slow receiver
	// never blocks publishers. Call the returned func to unsubscribe.
	Subscribe() (<-chan int64, func())
}

// InProc is a Bus for a single process (COLONY_ROLE=all).
type InProc struct {
	mu   sync.Mutex
	next int
	subs map[int]chan int64
}

// NewInProc returns an empty in-process bus.
func NewInProc() *InProc { return &InProc{subs: map[int]chan int64{}} }

// Notify delivers seq to every subscriber, replacing an unread older value.
func (b *InProc) Notify(seq int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		select {
		case ch <- seq:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- seq:
			default:
			}
		}
	}
}

// Subscribe registers a new subscriber.
func (b *InProc) Subscribe() (<-chan int64, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.next
	b.next++
	ch := make(chan int64, 1)
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs, id)
	}
}
