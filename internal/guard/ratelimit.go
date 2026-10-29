package guard

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"colonyhall/internal/apispec"
)

// RateLimitWindow represents one rate limit window (e.g. 5-hour or 7-day).
type RateLimitWindow struct {
	Utilization float64   `json:"utilization"` // 0.0 to 1.0 (e.g. 0.05 = 5%, 0.73 = 73%)
	ResetsAt    time.Time `json:"resets_at"`
	Status      string    `json:"status"` // "allowed", etc.
}

// RateLimit contains the parsed rate limit event from Claude Code.
type RateLimit struct {
	Status   string           `json:"status"`
	ResetsAt time.Time        `json:"resets_at"`
	FiveHour *RateLimitWindow `json:"five_hour,omitempty"`
	SevenDay *RateLimitWindow `json:"seven_day,omitempty"`
	Raw      string           `json:"raw,omitempty"`
}

// ClaudeUsageState is persisted in claude_usage.json in the guard state dir.
type ClaudeUsageState struct {
	FiveHour    *RateLimitWindow `json:"five_hour,omitempty"`
	SevenDay    *RateLimitWindow `json:"seven_day,omitempty"`
	Paused      bool             `json:"paused"`
	PauseReason string           `json:"pause_reason,omitempty"` // "five_hour", "seven_day", "limit_message", "status"
	PauseUntil  *time.Time       `json:"pause_until,omitempty"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// PauseInfo holds the details needed for posting the silent note when paused.
type PauseInfo struct {
	Window      string // "five_hour" | "seven_day"
	Utilization int    // e.g. 91
	ResetsAt    time.Time
}

// ClaudeLimiter tracks Claude rate limits across all agents on a guard.
type ClaudeLimiter struct {
	mu            sync.Mutex
	stateFile     string
	reserve       int
	warn          int
	state         ClaudeUsageState
	log           *slog.Logger
	now           func() time.Time
	onPauseChange func(paused bool)
}

// NewClaudeLimiter creates or loads a ClaudeLimiter backed by stateFile.
func NewClaudeLimiter(stateFile string, reserve, warn int, log *slog.Logger, now func() time.Time) *ClaudeLimiter {
	if reserve <= 0 {
		reserve = 90
	}
	if warn <= 0 {
		warn = 80
	}
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}

	l := &ClaudeLimiter{
		stateFile: stateFile,
		reserve:   reserve,
		warn:      warn,
		log:       log,
		now:       now,
	}

	l.load()
	// Re-evaluate pause state on startup
	curr := l.currentTime()
	if l.state.Paused {
		if l.state.PauseUntil != nil && !curr.Before(*l.state.PauseUntil) {
			shouldPause, reason, until := l.evaluatePause(curr)
			if shouldPause {
				l.state.PauseReason = reason
				l.state.PauseUntil = &until
				l.saveLocked()
			} else {
				l.state.Paused = false
				l.state.PauseReason = ""
				l.state.PauseUntil = nil
				l.saveLocked()
				l.log.Info("guard: claude usage pause expired on startup", "stateFile", stateFile)
			}
		}
	} else {
		shouldPause, reason, until := l.evaluatePause(curr)
		if shouldPause {
			l.state.Paused = true
			l.state.PauseReason = reason
			l.state.PauseUntil = &until
			l.saveLocked()
			l.logUsageLocked()
		}
	}

	return l
}

// SetOnPauseChange sets a callback triggered when the paused state transitions.
func (l *ClaudeLimiter) SetOnPauseChange(fn func(paused bool)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onPauseChange = fn
}

func (l *ClaudeLimiter) currentTime() time.Time {
	if l.now != nil {
		return l.now()
	}
	return time.Now()
}

func (l *ClaudeLimiter) load() {
	if l.stateFile == "" {
		return
	}
	data, err := os.ReadFile(l.stateFile)
	if err != nil {
		return
	}
	var s ClaudeUsageState
	if err := json.Unmarshal(data, &s); err == nil {
		l.state = s
	}
}

func (l *ClaudeLimiter) saveLocked() {
	if l.stateFile == "" {
		return
	}
	data, err := json.MarshalIndent(l.state, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.stateFile), 0o700)
	_ = os.WriteFile(l.stateFile, data, 0o600)
}

func (l *ClaudeLimiter) evaluatePause(now time.Time) (bool, string, time.Time) {
	reservePct := float64(l.reserve)

	var pauseFH, pauseSD bool
	var fhUntil, sdUntil time.Time

	// Check 5-hour window
	if fh := l.state.FiveHour; fh != nil {
		resetPassed := !fh.ResetsAt.IsZero() && !now.Before(fh.ResetsAt.Add(1*time.Minute))
		if !resetPassed {
			if (fh.Status != "" && fh.Status != "allowed") || fh.Utilization*100 >= reservePct-1e-6 {
				pauseFH = true
				fhUntil = fh.ResetsAt.Add(1 * time.Minute)
				if fhUntil.Before(now) {
					fhUntil = now.Add(15 * time.Minute)
				}
			}
		}
	}

	// Check 7-day window
	if sd := l.state.SevenDay; sd != nil {
		resetPassed := !sd.ResetsAt.IsZero() && !now.Before(sd.ResetsAt.Add(1*time.Minute))
		if !resetPassed {
			if (sd.Status != "" && sd.Status != "allowed") || sd.Utilization*100 >= reservePct-1e-6 {
				pauseSD = true
				sdUntil = sd.ResetsAt.Add(1 * time.Minute)
				if sdUntil.Before(now) {
					sdUntil = now.Add(15 * time.Minute)
				}
			}
		}
	}

	if pauseSD && (!pauseFH || sdUntil.After(fhUntil)) {
		return true, "seven_day", sdUntil
	}
	if pauseFH {
		return true, "five_hour", fhUntil
	}
	if pauseSD {
		return true, "seven_day", sdUntil
	}

	return false, "", time.Time{}
}

// Update updates the rate limit state from a parsed event and persists it.
func (l *ClaudeLimiter) Update(rl RateLimit) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.currentTime()
	l.state.UpdatedAt = now

	if rl.FiveHour != nil {
		l.state.FiveHour = rl.FiveHour
	}
	if rl.SevenDay != nil {
		l.state.SevenDay = rl.SevenDay
	}

	wasPaused := l.state.Paused
	shouldPause, reason, until := l.evaluatePause(now)
	if shouldPause {
		l.state.Paused = true
		l.state.PauseReason = reason
		l.state.PauseUntil = &until
	} else if wasPaused && l.state.PauseReason != "limit_message" {
		// Both windows are under reserve and status is allowed
		l.state.Paused = false
		l.state.PauseReason = ""
		l.state.PauseUntil = nil
	}

	l.saveLocked()
	l.logUsageLocked()

	if l.onPauseChange != nil && wasPaused != l.state.Paused {
		fn := l.onPauseChange
		paused := l.state.Paused
		go fn(paused)
	}
}

// RecordLimitFailure pauses the limiter when Claude Code exits with a limit message.
func (l *ClaudeLimiter) RecordLimitFailure(msg string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var resetAt time.Time
	if l.state.FiveHour != nil && !l.state.FiveHour.ResetsAt.IsZero() && l.state.FiveHour.ResetsAt.After(now) {
		resetAt = l.state.FiveHour.ResetsAt
	} else if parsed, ok := parseResetTimeFromMessage(msg, now); ok {
		resetAt = parsed
	} else {
		resetAt = now.Add(15 * time.Minute)
	}

	pauseUntil := resetAt.Add(1 * time.Minute)
	wasPaused := l.state.Paused
	l.state.Paused = true
	l.state.PauseReason = "limit_message"
	l.state.PauseUntil = &pauseUntil
	l.state.UpdatedAt = now

	l.saveLocked()
	l.log.Warn("guard: claude limit hit", "resets_at", resetAt, "pause_until", pauseUntil, "msg", msg)

	if l.onPauseChange != nil && !wasPaused {
		fn := l.onPauseChange
		go fn(true)
	}
}

// IsPaused reports whether Claude Code runs are currently paused.
func (l *ClaudeLimiter) IsPaused(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.state.Paused {
		shouldPause, reason, until := l.evaluatePause(now)
		if shouldPause {
			l.state.Paused = true
			l.state.PauseReason = reason
			l.state.PauseUntil = &until
			l.saveLocked()
			l.logUsageLocked()
			return true
		}
		return false
	}

	// Currently paused. Check if PauseUntil has expired
	if l.state.PauseUntil != nil && !now.Before(*l.state.PauseUntil) {
		shouldPause, reason, until := l.evaluatePause(now)
		if shouldPause {
			l.state.PauseReason = reason
			l.state.PauseUntil = &until
			l.saveLocked()
			return true
		}
		l.state.Paused = false
		l.state.PauseReason = ""
		l.state.PauseUntil = nil
		l.saveLocked()
		l.log.Info("guard: claude usage resumed after reset", "five_hour", l.formatWindow(l.state.FiveHour), "seven_day", l.formatWindow(l.state.SevenDay))
		if l.onPauseChange != nil {
			fn := l.onPauseChange
			go fn(false)
		}
		return false
	}

	return true
}

// PauseUntil returns the time until which runs are paused, or zero time.
func (l *ClaudeLimiter) PauseUntil() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state.Paused && l.state.PauseUntil != nil {
		return *l.state.PauseUntil
	}
	return time.Time{}
}

// PauseInfo returns details of the active pause for generating notification messages.
func (l *ClaudeLimiter) PauseInfo() PauseInfo {
	l.mu.Lock()
	defer l.mu.Unlock()

	info := PauseInfo{
		Window:      "five_hour",
		Utilization: l.reserve,
	}
	if l.state.PauseUntil != nil {
		info.ResetsAt = *l.state.PauseUntil
	}
	if l.state.PauseReason == "seven_day" && l.state.SevenDay != nil {
		info.Window = "seven_day"
		info.Utilization = int(l.state.SevenDay.Utilization*100 + 0.5)
		if !l.state.SevenDay.ResetsAt.IsZero() {
			info.ResetsAt = l.state.SevenDay.ResetsAt
		}
	} else if l.state.FiveHour != nil {
		info.Window = "five_hour"
		info.Utilization = int(l.state.FiveHour.Utilization*100 + 0.5)
		if !l.state.FiveHour.ResetsAt.IsZero() {
			info.ResetsAt = l.state.FiveHour.ResetsAt
		}
	}
	return info
}

// ReportLimits returns the apispec.ClaudeLimits representation for host reports.
func (l *ClaudeLimiter) ReportLimits() *apispec.ClaudeLimits {
	l.mu.Lock()
	defer l.mu.Unlock()

	paused := l.state.Paused
	res := &apispec.ClaudeLimits{
		Paused:           &paused,
		RateLimitReserve: &l.reserve,
		RateLimitWarn:    &l.warn,
	}
	if fh := l.state.FiveHour; fh != nil {
		w := apispec.ClaudeRateLimitWindow{
			Utilization: float32(fh.Utilization),
			Status:      fh.Status,
		}
		if !fh.ResetsAt.IsZero() {
			t := fh.ResetsAt
			w.ResetsAt = &t
		}
		res.FiveHour = &w
	}
	if sd := l.state.SevenDay; sd != nil {
		w := apispec.ClaudeRateLimitWindow{
			Utilization: float32(sd.Utilization),
			Status:      sd.Status,
		}
		if !sd.ResetsAt.IsZero() {
			t := sd.ResetsAt
			w.ResetsAt = &t
		}
		res.SevenDay = &w
	}
	return res
}

func (l *ClaudeLimiter) formatWindow(w *RateLimitWindow) string {
	if w == nil {
		return "none"
	}
	return fmt.Sprintf("%.0f%% (status=%s, resets=%s)", w.Utilization*100, w.Status, w.ResetsAt.Format(time.RFC3339))
}

func (l *ClaudeLimiter) logUsageLocked() {
	if l.log == nil {
		return
	}
	l.log.Info("guard: claude usage",
		"five_hour", l.formatWindow(l.state.FiveHour),
		"seven_day", l.formatWindow(l.state.SevenDay),
		"paused", l.state.Paused,
		"pause_reason", l.state.PauseReason,
		"reserve", l.reserve,
		"warn", l.warn,
	)
}

var resetsRe = regexp.MustCompile(`(?i)resets\s+([0-9]{1,2})(?::([0-9]{2}))?\s*(am|pm)?`)

// parseResetTimeFromMessage extracts the reset time from a Claude session limit message.
// The time is interpreted in the Europe/Budapest location.
func parseResetTimeFromMessage(msg string, now time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation("Europe/Budapest")
	if err != nil {
		loc = time.UTC
	}
	m := resetsRe.FindStringSubmatch(msg)
	if len(m) < 2 {
		return time.Time{}, false
	}
	hour, err := strconv.Atoi(m[1])
	if err != nil {
		return time.Time{}, false
	}
	min := 0
	if m[2] != "" {
		min, _ = strconv.Atoi(m[2])
	}
	ampm := strings.ToLower(m[3])
	if ampm == "pm" && hour < 12 {
		hour += 12
	} else if ampm == "am" && hour == 12 {
		hour = 0
	}
	nowInLoc := now.In(loc)
	resetTime := time.Date(nowInLoc.Year(), nowInLoc.Month(), nowInLoc.Day(), hour, min, 0, 0, loc)
	if !resetTime.After(nowInLoc) {
		resetTime = resetTime.Add(24 * time.Hour)
	}
	return resetTime, true
}

// isClaudeLimitMessage checks if a failure string or output is a Claude rate/session limit error.
func isClaudeLimitMessage(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "hit your session limit") ||
		strings.Contains(lower, "usage limit") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "hit your limit")
}
