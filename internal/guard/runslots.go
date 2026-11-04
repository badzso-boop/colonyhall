package guard

import "sync"

// RunSlots caps the client runs of all agents on a machine at once
// (guard.yaml max_concurrent_runs). An agent that finds no free slot keeps
// its items pending and is woken when a slot frees. A nil *RunSlots means no
// limit.
type RunSlots struct {
	slots   chan struct{}
	mu      sync.Mutex
	waiters []func()
}

// NewRunSlots returns a limit of n concurrent runs, or nil for n <= 0.
func NewRunSlots(n int) *RunSlots {
	if n <= 0 {
		return nil
	}
	return &RunSlots{slots: make(chan struct{}, n)}
}

// TryAcquire takes a slot without waiting. When none is free it remembers
// wake, which is called once a slot frees, and returns false.
func (r *RunSlots) TryAcquire(wake func()) bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.slots <- struct{}{}:
		return true
	default:
		if wake != nil {
			r.waiters = append(r.waiters, wake)
		}
		return false
	}
}

// Release frees a slot and wakes the agents that waited for one. They race
// for it; the losers register again.
func (r *RunSlots) Release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	<-r.slots
	waiters := r.waiters
	r.waiters = nil
	r.mu.Unlock()
	for _, w := range waiters {
		w()
	}
}

// InUse is the number of runs holding a slot.
func (r *RunSlots) InUse() int {
	if r == nil {
		return 0
	}
	return len(r.slots)
}
