package guard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"colonyhall/internal/apispec"
)

// AgyRunError is a failed agy run: its status and the reason agy gave, from
// the result event's error field or, when that is empty, from agy's own log.
type AgyRunError struct {
	Status string
	// Reason is agy's error text. Only this ever decides a quota pause: a
	// response text can quote anything, including "quota reached".
	Reason string
	// Response is the response field of the failed run (no answer: a run
	// with an answer is not an error), kept for the hidden error details.
	Response string
}

func (e *AgyRunError) Error() string {
	text := e.Reason
	if text == "" {
		text = e.Response
	}
	return fmt.Sprintf("agy: run status %q: %s", e.Status, clip(strings.TrimSpace(text), 300))
}

// agyQuota is a recognised quota failure.
type agyQuota struct {
	ResetAt    time.Time // zero: agy said nothing about the reset
	ResetKnown bool
	Message    string
}

var (
	agyQuotaRe = regexp.MustCompile(`(?i)resource_exhausted|code 429|quota reached|quota exceeded`)
	agyResetRe = regexp.MustCompile(`(?i)resets in\s+((?:\d+\s*[hms]\s*)+)`)
	agyPartRe  = regexp.MustCompile(`(\d+)\s*([hms])`)
)

// asAgyQuota recognises a quota failure in the reason of a failed agy run.
func asAgyQuota(err error, now time.Time) (agyQuota, bool) {
	var re *AgyRunError
	if !errors.As(err, &re) || !agyQuotaRe.MatchString(re.Reason) {
		return agyQuota{}, false
	}
	q := agyQuota{Message: clip(re.Reason, 300)}
	if d, ok := parseAgyReset(re.Reason); ok {
		q.ResetAt, q.ResetKnown = now.Add(d), true
	}
	return q, true
}

// parseAgyReset reads "Resets in 6h43m54s"; any part may be missing.
func parseAgyReset(msg string) (time.Duration, bool) {
	m := agyResetRe.FindStringSubmatch(msg)
	if m == nil {
		return 0, false
	}
	var d time.Duration
	for _, p := range agyPartRe.FindAllStringSubmatch(m[1], -1) {
		n, err := strconv.Atoi(p[1])
		if err != nil {
			return 0, false
		}
		switch p[2] {
		case "h", "H":
			d += time.Duration(n) * time.Hour
		case "m", "M":
			d += time.Duration(n) * time.Minute
		default:
			d += time.Duration(n) * time.Second
		}
	}
	return d, d > 0
}

const (
	agyLogTail = 256 << 10 // the end of the log is enough: the reason is among the last lines
	// agyLogSlack allows for the log being created a moment before the guard
	// noted the start.
	agyLogSlack = 5 * time.Second
)

var agyLogName = regexp.MustCompile(`^cli-(\d{8}_\d{6})\.log$`)

// agyLogReason fills an empty reason from agy's log in dir: only from a log
// that agy started during this run, only its tail, and only the line that
// carries the run's error. Nothing else from the log (it holds account
// details) is ever returned. Several matching logs with different reasons
// (parallel runs) give no reason rather than a wrong one.
func agyLogReason(dir string, started time.Time) string {
	if dir == "" {
		return ""
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	reasons := map[string]bool{}
	for _, e := range entries {
		m := agyLogName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.ParseInLocation("20060102_150405", m[1], time.Local)
		if err != nil || at.Before(started.Add(-agyLogSlack)) {
			continue
		}
		if r := logReason(root, e.Name()); r != "" {
			reasons[r] = true
		}
	}
	if len(reasons) != 1 {
		return ""
	}
	for r := range reasons {
		return r
	}
	return ""
}

func logReason(root *os.Root, name string) string {
	f, err := root.Open(name)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	if fi.Size() > agyLogTail {
		if _, err := f.Seek(-agyLogTail, io.SeekEnd); err != nil {
			return ""
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, agyLogTail))
	if err != nil {
		return ""
	}
	var session, exhausted string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if _, after, ok := strings.Cut(line, "run ended with error and no response: "); ok {
			session = after
		} else if i := strings.Index(line, "RESOURCE_EXHAUSTED"); i >= 0 && exhausted == "" {
			exhausted = line[i:]
		}
	}
	if session != "" {
		return clip(strings.TrimSpace(session), 300)
	}
	return clip(strings.TrimSpace(exhausted), 300)
}

// agyRetryAfter is the next try when agy names no reset time.
const agyRetryAfter = 30 * time.Minute

// AgyPause is one paused model.
type AgyPause struct {
	Until      time.Time `json:"until"`
	ResetKnown bool      `json:"reset_known"`
	Reason     string    `json:"reason,omitempty"`
}

// AgyLimiter pauses Antigravity models on this machine after a quota error.
// It is separate from ClaudeLimiter: Claude reports its usage windows on
// every run and is paused per account at a reserve, while agy reports
// nothing until a run fails, and the quota is per model. All this one keeps
// is a reset time per model.
type AgyLimiter struct {
	mu        sync.Mutex
	stateFile string
	pauses    map[string]AgyPause
	log       *slog.Logger
	now       func() time.Time
	onResume  func()
}

// AgyModelKey names the model of an agent that sets none.
func AgyModelKey(model string) string {
	if strings.TrimSpace(model) == "" {
		return "default"
	}
	return model
}

// NewAgyLimiter loads the pauses from stateFile (survives a restart).
func NewAgyLimiter(stateFile string, log *slog.Logger, now func() time.Time) *AgyLimiter {
	if now == nil {
		now = time.Now
	}
	if log == nil {
		log = slog.Default()
	}
	l := &AgyLimiter{stateFile: stateFile, pauses: map[string]AgyPause{}, log: log, now: now}
	if data, err := os.ReadFile(stateFile); err == nil {
		_ = json.Unmarshal(data, &l.pauses)
	}
	return l
}

// SetOnResume is called (in a goroutine) when a pause ends.
func (l *AgyLimiter) SetOnResume(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onResume = fn
}

func (l *AgyLimiter) saveLocked() {
	if l.stateFile == "" {
		return
	}
	data, err := json.MarshalIndent(l.pauses, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.stateFile), 0o700)
	_ = os.WriteFile(l.stateFile, data, 0o600)
}

// Pause stops runs on model until the reset plus a minute, or for
// agyRetryAfter when the reset is unknown.
func (l *AgyLimiter) Pause(model string, q agyQuota) AgyPause {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	p := AgyPause{Until: now.Add(agyRetryAfter), Reason: q.Message}
	if q.ResetKnown && q.ResetAt.After(now) {
		p.Until, p.ResetKnown = q.ResetAt.Add(time.Minute), true
	}
	key := AgyModelKey(model)
	l.pauses[key] = p
	l.saveLocked()
	l.log.Warn("guard: antigravity quota reached", "model", key, "until", p.Until, "reset_known", p.ResetKnown, "reason", p.Reason)
	return p
}

// Paused returns the pause of model, ending it once its time has passed.
func (l *AgyLimiter) Paused(model string) (AgyPause, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := AgyModelKey(model)
	p, ok := l.pauses[key]
	if !ok {
		return AgyPause{}, false
	}
	if !l.now().Before(p.Until) {
		delete(l.pauses, key)
		l.saveLocked()
		l.log.Info("guard: antigravity quota pause over", "model", key)
		if l.onResume != nil {
			go l.onResume()
		}
		return AgyPause{}, false
	}
	return p, true
}

// Report lists the paused models for the host heartbeat.
func (l *AgyLimiter) Report() []apispec.AgyLimit {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	out := make([]apispec.AgyLimit, 0, len(l.pauses))
	for model, p := range l.pauses {
		if now.Before(p.Until) {
			out = append(out, apispec.AgyLimit{Model: model, PausedUntil: p.Until, ResetKnown: p.ResetKnown})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}
