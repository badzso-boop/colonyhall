// Package clock provides the Clock port. Domain and service code must take
// time from a Clock instead of calling time.Now directly, so tests can run
// with a deterministic, manually advanced clock.
package clock

import (
	"sync"
	"time"
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the real wall clock, always in UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

// Manual is a clock for tests: it only moves when told to.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

// NewManual returns a Manual clock set to start.
func NewManual(start time.Time) *Manual { return &Manual{now: start.UTC()} }

// Now returns the clock's current time.
func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Advance moves the clock forward by d.
func (m *Manual) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
