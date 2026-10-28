package guard

import (
	"context"
	"errors"
	"sync"
	"time"
)

type runTimeoutType int

const (
	timeoutNone runTimeoutType = iota
	timeoutIdle
	timeoutMaxRun
)

func (t runTimeoutType) String() string {
	switch t {
	case timeoutIdle:
		return "idle"
	case timeoutMaxRun:
		return "max_run"
	default:
		return "none"
	}
}

var (
	errTimeoutIdle   = errors.New("inactivity timeout exceeded")
	errTimeoutMaxRun = errors.New("max run limit exceeded")
)

type runTimeoutContext struct {
	context.Context
	cancel    context.CancelCauseFunc
	idleDur   time.Duration
	maxDur    time.Duration
	mu        sync.Mutex
	idleTimer *time.Timer
	maxTimer  *time.Timer
	timedOut  runTimeoutType
	done      bool
}

func newRunTimeoutContext(parent context.Context, idleDur, maxDur time.Duration) *runTimeoutContext {
	ctx, cancel := context.WithCancelCause(parent)
	rtc := &runTimeoutContext{
		Context: ctx,
		cancel:  cancel,
		idleDur: idleDur,
		maxDur:  maxDur,
	}

	if idleDur > 0 {
		rtc.idleTimer = time.AfterFunc(idleDur, func() {
			rtc.mu.Lock()
			if !rtc.done {
				rtc.timedOut = timeoutIdle
				rtc.cancel(errTimeoutIdle)
			}
			rtc.mu.Unlock()
		})
	}

	if maxDur > 0 {
		rtc.maxTimer = time.AfterFunc(maxDur, func() {
			rtc.mu.Lock()
			if !rtc.done {
				rtc.timedOut = timeoutMaxRun
				rtc.cancel(errTimeoutMaxRun)
			}
			rtc.mu.Unlock()
		})
	}

	return rtc
}

// Touch resets the inactivity timer if it is running.
func (rtc *runTimeoutContext) Touch() {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()
	if rtc.done {
		return
	}
	if rtc.idleTimer != nil && rtc.idleDur > 0 {
		rtc.idleTimer.Stop()
		rtc.idleTimer.Reset(rtc.idleDur)
	}
}

// Cancel cancels the context manually (e.g. on external cancel/retraction).
func (rtc *runTimeoutContext) Cancel() {
	rtc.cancel(nil)
}

// Finish stops the timers and returns the timeout reason (timeoutNone if completed or canceled externally).
func (rtc *runTimeoutContext) Finish() runTimeoutType {
	rtc.mu.Lock()
	defer rtc.mu.Unlock()
	rtc.done = true
	if rtc.idleTimer != nil {
		rtc.idleTimer.Stop()
	}
	if rtc.maxTimer != nil {
		rtc.maxTimer.Stop()
	}
	rtc.cancel(nil)
	return rtc.timedOut
}
