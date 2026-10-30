// Package schedule owns scheduled messages. At the given time the server
// posts the message in the creator's name; the mentioned agent then wakes
// up exactly as if the person had written it at that moment, so every rule
// about who may give whom a task still applies.
package schedule

import (
	"errors"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"colonyhall/internal/platform/ids"
)

// States.
const (
	StateActive   = "active"
	StateDone     = "done"
	StateCanceled = "canceled"
)

// Catch-up policies for runs missed while the server was down.
const (
	CatchUpOnce = "once" // run once if not later than MaxLate
	CatchUpSkip = "skip"
)

// MaxLate is how late a missed run may still be made up.
const MaxLate = 12 * time.Hour

// DefaultTimezone is used when none is given.
const DefaultTimezone = "Europe/Budapest"

// Schedule is one scheduled message.
type Schedule struct {
	ID            ids.ID
	WorkspaceID   ids.ID
	CreatedBy     ids.ID
	ChannelID     ids.ID
	ThreadRootID  ids.ID
	Body          string
	Cron          string // empty: one-off
	Timezone      string
	CatchUp       string
	State         string
	NextRunAt     time.Time
	LastRunAt     *time.Time
	LastMessageID ids.ID
	LastError     string
	RunCount      int
	CreatedAt     time.Time
}

// Errors.
var (
	ErrForbidden  = errors.New("not allowed")
	ErrBadTime    = errors.New("give either a time in the future (at) or a cron expression, not both")
	ErrBadCron    = errors.New("invalid cron expression (5 fields: minute hour day-of-month month day-of-week)")
	ErrBadZone    = errors.New("unknown time zone")
	ErrBadCatchUp = errors.New("catch_up must be once or skip")
	ErrEmptyBody  = errors.New("message is empty")
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// NextRuns returns the first n runs of a cron expression after t, in zone.
func NextRuns(expr, zone string, t time.Time, n int) ([]time.Time, error) {
	out := make([]time.Time, 0, n)
	for len(out) < n {
		next, err := Next(expr, zone, t)
		if err != nil {
			return nil, err
		}
		if next.IsZero() {
			break // an expression that never fires again
		}
		out = append(out, next)
		t = next
	}
	return out, nil
}

// Next returns the first run of a cron expression after t, in zone.
func Next(expr, zone string, t time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, ErrBadZone
	}
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s", ErrBadCron, err.Error())
	}
	return sched.Next(t.In(loc)).UTC(), nil
}
