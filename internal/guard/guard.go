// Package guard is the "őr": a small client process next to the AI clients.
// It watches each agent's inbox over the REST API and, when something
// arrives, runs the agent's client once to decide, then posts the replies,
// reactions and acks itself. It stops a running client at once when the
// agent is paused or an emergency stop is on.
package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

// Agent runs the guard loop for one agent.
type Agent struct {
	Cfg     AgentConfig
	Client  *Client
	Adapter Adapter
	Log     *slog.Logger
	// Policy, when set (host mode), decides whether and where work runs:
	// the server's grants checked against this machine's roots. Without
	// it, Cfg.AllowWork and Cfg.Dir apply (single-machine config).
	Policy func() WorkPolicy
	// DecideWrap, when set (host mode), runs decision runs in the sandbox
	// too: Antigravity cannot switch its tools off.
	DecideWrap Wrapper

	// Mode is "direct" or "compact" (empty defaults to compact).
	Mode           string
	RootDir        string
	StateDir       string
	WorktreeRepo   string
	WorktreeBranch string
	WorktreePath   string

	ExchangeDir string
	ExchangeOK  bool

	// WebSocket options and test hooks
	DisableWS          bool
	WSTokenHook        func(ctx context.Context) (string, error)
	WSBackoff          func(attempt int) time.Duration
	TokenCheckInterval time.Duration
	WSDialHook         func(ctx context.Context, u string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error)

	// Now returns the current time. If nil, time.Now is used.
	Now func() time.Time

	budgetExhaustedUntil time.Time
	budgetNoticeDay      string

	workMu     sync.Mutex
	workCancel context.CancelFunc

	currentRunItems       []Item
	currentRunChannelID   string
	currentRunThreadID    string
	currentRunRetracted   bool
	currentRetractedMsgID string

	// HostConfigVersion, when set (host mode), returns the host config version.
	HostConfigVersion func() string

	// File fetch options and scope
	Sandbox          bool
	AllowWebRead     bool
	WebReadAllow     []string
	WebReadDeny      []string
	Roots            []string
	GrantsFunc       func() []Grant
	DenyFiles        []string
	FileFetchLimiter chan struct{}

	Terminals *TerminalManager

	ClaudeLimiter         *ClaudeLimiter
	notedRateLimitThreads map[string]bool

	fileFetchMu       sync.Mutex
	fileFetchInFlight map[string]bool

	activityMu     sync.Mutex
	activeActivity map[string]*activityEntry

	rulesMu             sync.Mutex
	cachedRulesVer      int
	cachedRulesText     string
	cachedRulesAt       time.Time
	cachedHostConfigVer string

	halted   atomic.Bool
	meMu     sync.RWMutex
	me       Me
	handles  map[string]string
	failures int
	attempts map[string]int // item id → failed decision runs

	wakeMu sync.Mutex
	wakeCh chan struct{}
}

// Wake wakes the agent's main loop immediately.
func (a *Agent) Wake() {
	a.wakeMu.Lock()
	defer a.wakeMu.Unlock()
	if a.wakeCh != nil {
		select {
		case a.wakeCh <- struct{}{}:
		default:
		}
	}
}

// IsHalted reports whether the agent is known to be paused/halted.
func (a *Agent) IsHalted() bool {
	return a.halted.Load()
}

func (a *Agent) getMe() Me {
	a.meMu.RLock()
	m := a.me
	a.meMu.RUnlock()
	return m
}

func (a *Agent) setMe(m Me) {
	a.meMu.Lock()
	a.me = m
	a.meMu.Unlock()
}

// maxAttempts bounds retries of the same items, so a broken client does not
// burn tokens forever. After that the items are acknowledged and logged.
const maxAttempts = 3

var (
	// errHalted means the agent must stop writing (paused, stopped, emergency).
	errHalted = errors.New("agent is halted")
	// errBudgetExceeded means the agent reached its daily token budget limit.
	errBudgetExceeded = errors.New("daily token budget exceeded")
)

// Run loops until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	if a.attempts == nil {
		a.attempts = map[string]int{}
	}
	me, err := a.Client.Me(ctx)
	if err != nil {
		return fmt.Errorf("guard %s: cannot reach the server: %w", a.Cfg.Name, err)
	}
	a.setMe(me)
	a.halted.Store(me.Halted)
	a.Log.Info("guard: watching", "agent", "@"+me.Handle, "client", a.Cfg.Client, "model", a.Cfg.Model)

	wakeCh := make(chan struct{}, 1)
	a.wakeMu.Lock()
	a.wakeCh = wakeCh
	a.wakeMu.Unlock()
	defer func() {
		a.wakeMu.Lock()
		a.wakeCh = nil
		a.wakeMu.Unlock()
	}()
	fileFetchWakeCh := make(chan struct{}, 1)
	fileFetchWakeCh <- struct{}{} // initial check on startup
	go a.runFileFetchLoop(ctx, fileFetchWakeCh)

	if !a.DisableWS && !a.Cfg.DisableWS {
		wakeCh <- struct{}{} // initial inbox check on startup
		go a.watchWS(ctx, wakeCh, fileFetchWakeCh)
	}

	timer := time.NewTimer(a.waitInterval())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			select {
			case fileFetchWakeCh <- struct{}{}:
			default:
			}
		}
		if err := a.Tick(ctx); err != nil && ctx.Err() == nil {
			a.failures++
			a.Log.Warn("guard: tick failed", "agent", a.logName(), "err", err, "failures", a.failures)
		}
		timer.Reset(a.waitInterval())
	}
}

func (a *Agent) waitInterval() time.Duration {
	if a.isClaudeClient() && a.ClaudeLimiter != nil && a.ClaudeLimiter.IsPaused(a.now()) {
		until := a.ClaudeLimiter.PauseUntil()
		if !until.IsZero() && a.now().Before(until) {
			d := a.until(until)
			if d > 0 {
				return d
			}
		}
	}
	if !a.budgetExhaustedUntil.IsZero() && a.now().Before(a.budgetExhaustedUntil) {
		d := a.until(a.budgetExhaustedUntil)
		if d > 0 {
			return d
		}
	}
	if a.DisableWS || a.Cfg.DisableWS {
		return a.pause()
	}
	return a.fallbackPause()
}

func (a *Agent) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Agent) until(t time.Time) time.Duration {
	return t.Sub(a.now())
}

func (a *Agent) checkBudget(ctx context.Context, sample Item) (bool, error) {
	b, err := a.Client.Budget(ctx)
	if err != nil {
		return false, err
	}
	if b.Limit > 0 && b.Remaining <= 0 {
		a.budgetExhaustedUntil = b.ResetsAt
		dayKey := b.ResetsAt.Format(time.RFC3339)
		if a.budgetNoticeDay != dayKey {
			resetTime := "tomorrow"
			if !b.ResetsAt.IsZero() {
				resetTime = b.ResetsAt.Local().Format("15:04")
			}
			msg := fmt.Sprintf("⏳ My daily token budget is used up; I continue after %s.", resetTime)
			_ = a.reply(ctx, sample, msg, "budget-"+sample.ID)
			a.budgetNoticeDay = dayKey
		}
		return false, nil
	}
	a.budgetExhaustedUntil = time.Time{}
	return true, nil
}

// pause is the poll interval, backing off after failures (max 5 minutes).
func (a *Agent) pause() time.Duration {
	d := a.Cfg.Poll
	for i := 0; i < a.failures && d < 5*time.Minute; i++ {
		d *= 2
	}
	return min(d, 5*time.Minute)
}

// fallbackPause is the fallback poll interval when WebSocket is active, backing off after failures.
func (a *Agent) fallbackPause() time.Duration {
	d := a.Cfg.FallbackPoll
	for i := 0; i < a.failures && d < 5*time.Minute; i++ {
		d *= 2
	}
	return min(d, 5*time.Minute)
}

// Tick checks the inbox once and handles what is there.
func (a *Agent) Tick(ctx context.Context) error {
	me, err := a.Client.Me(ctx)
	if err != nil {
		return err
	}
	a.setMe(me)
	a.halted.Store(me.Halted)
	if me.Halted {
		return nil
	}
	_, total, err := a.Client.Inbox(ctx, 1)
	if err != nil || total == 0 {
		return err
	}
	// Let a burst of messages arrive, so one run answers all of them.
	if err := sleep(ctx, a.Cfg.Debounce); err != nil {
		return nil
	}
	items, _, err := a.Client.Inbox(ctx, a.Cfg.Batch)
	if err != nil || len(items) == 0 {
		return err
	}
	var chatItems []Item
	for _, it := range items {
		if it.Kind == "compact" {
			if err := a.handleCompact(ctx, it); err != nil {
				a.Log.Error("guard: handle compact failed", "item", it.ID, "err", err)
			}
		} else {
			chatItems = append(chatItems, it)
		}
	}
	if len(chatItems) == 0 {
		a.failures = 0
		return nil
	}
	items = chatItems

	if a.isClaudeClient() && a.ClaudeLimiter != nil {
		if a.ClaudeLimiter.IsPaused(a.now()) {
			a.postRateLimitNotes(ctx, items)
			return nil
		}
		a.notedRateLimitThreads = nil
	}

	if a.Mode == "direct" || (a.Mode == "" && a.Cfg.Mode == "direct") {
		if err := a.handleDirect(ctx, items); err != nil {
			return err
		}
	} else {
		if err := a.handle(ctx, items); err != nil {
			return err
		}
	}
	a.failures = 0
	return nil
}

func (a *Agent) handleCompact(ctx context.Context, it Item) error {
	task, err := a.Client.CompactionTask(ctx, it.ItemID)
	if err != nil {
		_ = a.Client.Ack(ctx, []string{it.ID})
		return fmt.Errorf("fetch compaction task %s: %w", it.ItemID, err)
	}

	prompt := ScribePrompt(task)
	req := SummarizeRequest{
		Prompt: prompt,
		Model:  a.Cfg.Model,
		Dir:    a.Cfg.Dir,
		Wrap:   a.DecideWrap,
	}

	res, err := a.Adapter.Summarize(ctx, req)
	if err != nil {
		_ = a.Client.ReportCompactionResult(ctx, task.ID, "", "adapter_error")
		_ = a.Client.Ack(ctx, []string{it.ID})
		return fmt.Errorf("summarize %s: %w", task.ID, err)
	}

	trimmed := strings.TrimSpace(res.Text)
	if trimmed == "" {
		_ = a.Client.ReportCompactionResult(ctx, task.ID, "", "empty_summary")
		_ = a.Client.Ack(ctx, []string{it.ID})
		return nil
	}

	if err := a.Client.ReportCompactionResult(ctx, task.ID, trimmed, ""); err != nil {
		_ = a.Client.Ack(ctx, []string{it.ID})
		return fmt.Errorf("report compaction result %s: %w", task.ID, err)
	}

	_ = a.Client.Ack(ctx, []string{it.ID})
	return nil
}

func (a *Agent) handle(ctx context.Context, items []Item) error {
	runCtx, stop := a.watchHalt(ctx)
	defer stop()

	if a.isClaudeClient() && a.ClaudeLimiter != nil && a.ClaudeLimiter.IsPaused(a.now()) {
		a.postRateLimitNotes(ctx, items)
		return nil
	}

	if len(items) > 0 {
		ok, err := a.checkBudget(runCtx, items[0])
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	decideStops := make(map[string]func())
	for _, it := range items {
		ch, th := a.activityTarget(it)
		k := ch + ":" + th
		if _, ok := decideStops[k]; !ok {
			decideStops[k] = a.startActivity(ch, th)
		}
	}
	defer func() {
		for _, s := range decideStops {
			s()
		}
	}()

	pitems := make([]PromptItem, 0, len(items))
	for _, it := range items {
		pitems = append(pitems, PromptItem{Item: it, Context: a.threadContext(runCtx, it)})
	}
	dctx, cancel := context.WithTimeout(runCtx, a.Cfg.DecideTimeout)
	res, err := a.Adapter.Decide(dctx, DecideRequest{Handle: a.getMe().Handle, Items: pitems, Model: a.Cfg.Model, Dir: a.Cfg.Dir, Wrap: a.DecideWrap})
	cancel()
	a.logRun("decide", a.Cfg.Model, len(items), res, err)
	decideStatus := "ok"
	if err != nil {
		if runCtx.Err() != nil && ctx.Err() == nil {
			decideStatus = "halted"
		} else {
			decideStatus = "error"
		}
	}
	a.reportRun(ctx, "decide", a.Cfg.Model, "", "", "", len(items), res, decideStatus, "", nil, "")
	if a.isClaudeClient() && a.ClaudeLimiter != nil && res.RateLimit != nil {
		a.ClaudeLimiter.Update(*res.RateLimit)
	}
	if err != nil {
		if a.isClaudeClient() && a.ClaudeLimiter != nil && isClaudeLimitMessage(err.Error()) {
			a.ClaudeLimiter.RecordLimitFailure(err.Error(), a.now())
			a.postRateLimitNotes(ctx, items)
			return nil
		}
		if runCtx.Err() != nil && ctx.Err() == nil {
			a.Log.Warn("guard: run stopped, agent halted", "agent", a.logName())
			return nil
		}
		return a.giveUpAfterRetries(ctx, items, err)
	}
	for _, it := range items {
		delete(a.attempts, it.ID)
	}

	byRef := map[string]Action{}
	for _, act := range res.Decision.Actions {
		byRef[act.Ref] = act
	}
	var acked []string
	for i := range items {
		ref := strconv.Itoa(i + 1) // refs follow item order (BuildPrompt)
		it, ok := res.Refs[ref]
		if !ok {
			it = pitems[i]
		}
		act, ok := byRef[ref]
		if !ok {
			act = Action{Kind: ActNone}
		}
		if err := a.apply(runCtx, it, act); err != nil {
			_ = a.Client.Ack(ctx, acked)
			if errors.Is(err, errHalted) || runCtx.Err() != nil {
				a.Log.Warn("guard: stopped mid-run, agent halted", "agent", a.logName())
				return nil
			}
			if errors.Is(err, errBudgetExceeded) {
				return nil
			}
			return err
		}
		acked = append(acked, it.ID)
	}

	for k, s := range decideStops {
		delete(decideStops, k)
		s()
	}

	return a.Client.Ack(ctx, acked)
}

// giveUpAfterRetries keeps failed items for a retry, but acknowledges them
// after maxAttempts so a broken client cannot loop on them.
func (a *Agent) giveUpAfterRetries(ctx context.Context, items []Item, cause error) error {
	if a.isClaudeClient() && a.ClaudeLimiter != nil && isClaudeLimitMessage(cause.Error()) {
		a.ClaudeLimiter.RecordLimitFailure(cause.Error(), a.now())
		a.postRateLimitNotes(ctx, items)
		return nil
	}
	if a.attempts == nil {
		a.attempts = map[string]int{}
	}
	var drop []string
	for _, it := range items {
		a.attempts[it.ID]++
		if a.attempts[it.ID] >= maxAttempts {
			drop = append(drop, it.ID)
			delete(a.attempts, it.ID)
		}
	}
	if len(drop) > 0 {
		a.Log.Error("guard: giving up on items after repeated failures", "agent", a.logName(), "items", len(drop), "err", cause)
		// Say so in the thread: a silent drop looks like the agent never
		// got the message.
		dropped := map[string]bool{}
		for _, id := range drop {
			dropped[id] = true
		}
		for _, it := range items {
			if dropped[it.ID] {
				summary := fmt.Sprintf("gave up after %d attempts", maxAttempts)
				if _, err := a.reportError(ctx, it, summary, cause.Error(), "gx-"+it.ID); err != nil {
					a.Log.Warn("guard: could not report the failure", "err", err)
				}
			}
		}
		if err := a.Client.Ack(ctx, drop); err != nil {
			return err
		}
	}
	return cause
}

func (a *Agent) apply(ctx context.Context, it PromptItem, act Action) error {
	if ctx.Err() != nil {
		return errHalted
	}
	switch act.Kind {
	case ActReply:
		if strings.TrimSpace(act.Text) == "" {
			return nil
		}
		return a.reply(ctx, it.Item, act.Text, "g-"+it.ID)
	case ActReact:
		emoji := act.Emoji
		if emoji != "👎" {
			emoji = "👍"
		}
		return halted(a.Client.React(ctx, it.ItemID, emoji))
	case ActWork:
		return a.work(ctx, it, act.Task)
	}
	return nil
}

// WorkPolicy says whether and where a work run may happen.
type WorkPolicy struct {
	Allowed      bool
	Reason       string // why not, shown in the chat
	Dir          string
	AddDirs      []string
	Grants       []Grant
	Wrap         Wrapper
	NoExec       bool
	AgentDir     string
	AllowWebRead bool
	AllowPlanner bool
}

func (a *Agent) policy() WorkPolicy {
	if a.Policy != nil {
		return a.Policy()
	}
	if !a.Cfg.AllowWork {
		return WorkPolicy{Reason: "I can only chat here; ask a human to run this task."}
	}
	return WorkPolicy{Allowed: true, Dir: a.Cfg.Dir}
}

// StopWork cancels a running work run, e.g. because its grants changed.
func (a *Agent) StopWork() {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	if a.workCancel != nil {
		a.workCancel()
	}
}

// OnMessageRetracted cancels a running run if it includes the retracted message.
func (a *Agent) OnMessageRetracted(messageID string) {
	a.workMu.Lock()
	defer a.workMu.Unlock()
	contains := false
	for _, it := range a.currentRunItems {
		if it.ItemID == messageID {
			contains = true
			break
		}
	}
	if !contains {
		return
	}
	a.currentRunRetracted = true
	a.currentRetractedMsgID = messageID
	a.Log.Info("guard: stopping run because message was retracted", "agent", a.logName(), "message_id", messageID)
	if a.workCancel != nil {
		a.workCancel()
	}
}

// fetchRules fetches the agent's instructions (rendered rules) from the server,
// caching it and refetching when the host config version changes or every 10 minutes.
// On error, the last good text is kept; if none, it runs without rules and logs a warning.
func (a *Agent) fetchRules(ctx context.Context) (int, string) {
	a.rulesMu.Lock()
	defer a.rulesMu.Unlock()

	var curHostVer string
	if a.HostConfigVersion != nil {
		curHostVer = a.HostConfigVersion()
	}

	needsFetch := a.cachedRulesAt.IsZero() ||
		a.now().Sub(a.cachedRulesAt) >= 10*time.Minute ||
		(curHostVer != "" && curHostVer != a.cachedHostConfigVer)

	if !needsFetch {
		return a.cachedRulesVer, a.cachedRulesText
	}

	if a.Client == nil {
		return a.cachedRulesVer, a.cachedRulesText
	}

	inst, err := a.Client.Instructions(ctx)
	if err != nil {
		if a.cachedRulesText != "" || a.cachedRulesVer > 0 {
			if a.Log != nil {
				a.Log.Warn("guard: cannot fetch agent instructions, using cached rules", "agent", a.logName(), "err", err)
			}
			return a.cachedRulesVer, a.cachedRulesText
		}
		if a.Log != nil {
			a.Log.Warn("guard: cannot fetch agent instructions, running without rules", "agent", a.logName(), "err", err)
		}
		return 0, ""
	}

	a.cachedRulesVer = inst.Version
	a.cachedRulesText = inst.Text
	a.cachedRulesAt = a.now()
	a.cachedHostConfigVer = curHostVer
	return a.cachedRulesVer, a.cachedRulesText
}

type activityEntry struct {
	channelID     string
	threadRootID  string
	refs          int
	stopHeartbeat chan struct{}
}

func (a *Agent) activityTarget(it Item) (string, string) {
	if it.ReplyOrigin != nil && it.ReplyOrigin.ChannelID != "" {
		return it.ReplyOrigin.ChannelID, it.ReplyOrigin.ThreadRootID
	}
	chID := it.ChannelID
	thID := it.ThreadRootID
	if it.DM && thID == "" {
		thID = ThreadRoot(it)
	}
	return chID, thID
}

func (a *Agent) startActivity(channelID, threadRootID string) func() {
	key := channelID + ":" + threadRootID
	a.activityMu.Lock()
	if a.activeActivity == nil {
		a.activeActivity = make(map[string]*activityEntry)
	}
	entry, exists := a.activeActivity[key]
	if !exists {
		entry = &activityEntry{
			channelID:     channelID,
			threadRootID:  threadRootID,
			refs:          1,
			stopHeartbeat: make(chan struct{}),
		}
		a.activeActivity[key] = entry
		a.activityMu.Unlock()

		actCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := a.Client.SetActivity(actCtx, channelID, threadRootID, "working"); err != nil {
			a.Log.Warn("guard: activity report failed", "state", "working", "err", err)
		}
		cancel()

		go func(e *activityEntry) {
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-e.stopHeartbeat:
					return
				case <-ticker.C:
					hCtx, hCancel := context.WithTimeout(context.Background(), 5*time.Second)
					if err := a.Client.SetActivity(hCtx, e.channelID, e.threadRootID, "working"); err != nil {
						a.Log.Warn("guard: activity report failed", "state", "working", "err", err)
					}
					hCancel()
				}
			}
		}(entry)
	} else {
		entry.refs++
		a.activityMu.Unlock()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			a.activityMu.Lock()
			entry.refs--
			if entry.refs <= 0 {
				delete(a.activeActivity, key)
				close(entry.stopHeartbeat)
				a.activityMu.Unlock()

				actCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := a.Client.SetActivity(actCtx, channelID, threadRootID, "idle"); err != nil {
					a.Log.Warn("guard: activity report failed", "state", "idle", "err", err)
				}
				cancel()
			} else {
				a.activityMu.Unlock()
			}
		})
	}
}

func (a *Agent) withActivity(ctx context.Context, channelID, threadRootID string, run func() error) error {
	stop := a.startActivity(channelID, threadRootID)
	defer stop()
	return run()
}

func (a *Agent) work(ctx context.Context, it PromptItem, task string) error {
	chID, thID := a.activityTarget(it.Item)
	return a.withActivity(ctx, chID, thID, func() error {
		p := a.policy()
		if !p.Allowed {
			return a.reply(ctx, it.Item, p.Reason, "g-"+it.ID)
		}
		ok, err := a.checkBudget(ctx, it.Item)
		if err != nil {
			return err
		}
		if !ok {
			return errBudgetExceeded
		}
		if strings.TrimSpace(task) == "" {
			task = it.Body
		}
		if err := halted(a.Client.React(ctx, it.ItemID, "👀")); err != nil {
			return err
		}
		rulesVer, rulesText := a.fetchRules(ctx)
		rtc := newRunTimeoutContext(ctx, a.Cfg.IdleTimeout, a.Cfg.MaxRun)
		a.workMu.Lock()
		a.workCancel = rtc.Cancel
		a.currentRunItems = []Item{it.Item}
		a.currentRunChannelID = it.ChannelID
		a.currentRunThreadID = it.ThreadRootID
		a.currentRunRetracted = false
		a.currentRetractedMsgID = ""
		a.workMu.Unlock()
		promptBranch := ""
		if a.Sandbox && a.WorktreeBranch != "" {
			promptBranch = a.WorktreeBranch
		}
		var tracker *RunWebReadTracker
		var mcpSrv *WebReadServer
		var plannerSrv *PlannerServer
		var mcpConfigPath string
		if p.AllowWebRead && p.AgentDir != "" {
			tracker = &RunWebReadTracker{}
			sockPath := filepath.Join(p.AgentDir, "webread.sock")
			srv, err := StartWebReadMCPServer(sockPath, FetchOptions{
				Allow:   a.WebReadAllow,
				Deny:    a.WebReadDeny,
				Tracker: tracker,
			}, a.logName(), a.Log)
			if err != nil {
				a.Log.Error("guard: start web_read mcp server", "err", err)
			} else {
				mcpSrv = srv
				defer func() { _ = mcpSrv.Close() }()
			}
		}
		allowPlanner := (p.AllowPlanner || a.Sandbox) && p.AgentDir != ""
		if allowPlanner {
			sockPath := filepath.Join(p.AgentDir, "planner.sock")
			ps, err := StartPlannerMCPServer(sockPath, a.Client, "", a.logName(), a.Log)
			if err != nil {
				a.Log.Error("guard: start planner mcp server", "err", err)
			} else {
				plannerSrv = ps
				defer func() { _ = plannerSrv.Close() }()
			}
		}
		if (p.AllowWebRead || allowPlanner) && p.AgentDir != "" {
			mcpConfigPath = filepath.Join(p.AgentDir+".config", "claude", "mcp.json")
		}
		runKey := uuid.New().String()
		pr := newProgressReporter(a.Client, it.ChannelID, it.ThreadRootID, runKey, a.Log)
		origOnStep := pr.OnStep
		onStep := func(s Step) {
			rtc.Touch()
			if origOnStep != nil {
				origOnStep(s)
			}
		}
		res, err := a.Adapter.Work(rtc, WorkRequest{
			Prompt:         WorkPromptWithTools(a.getMe().Handle, promptBranch, p.AllowWebRead, allowPlanner, it, task, p.Grants...),
			Model:          a.Cfg.WorkModel,
			Dir:            p.Dir,
			PermissionMode: a.Cfg.PermissionMode,
			AddDirs:        p.AddDirs,
			Wrap:           p.Wrap,
			NoExec:         p.NoExec,
			AllowWebRead:   p.AllowWebRead,
			AllowPlanner:   allowPlanner,
			MCPConfigPath:  mcpConfigPath,
			Rules:          rulesText,
			RulesVersion:   rulesVer,
			OnStep:         onStep,
			OnActivity:     rtc.Touch,
			OnRateLimit: func(rl RateLimit) {
				if a.ClaudeLimiter != nil {
					a.ClaudeLimiter.Update(rl)
				}
			},
		})
		steps := pr.Finish()
		wasCanceled := errors.Is(rtc.Err(), context.Canceled)
		timedOut := rtc.Finish()
		stopped := wasCanceled && timedOut == timeoutNone
		a.workMu.Lock()
		wasRetracted := a.currentRunRetracted
		retractedMsgID := a.currentRetractedMsgID
		a.workCancel = nil
		a.currentRunItems = nil
		a.currentRunRetracted = false
		a.workMu.Unlock()
		var visitedURLs []string
		if tracker != nil {
			visitedURLs = tracker.VisitedURLs()
		}
		a.logRun("work", a.Cfg.WorkModel, 1, res, err)
		if wasRetracted {
			a.Log.Info("guard: work run stopped because message was retracted", "agent", a.logName(), "message_id", retractedMsgID)
			_ = a.Client.CreateRetractNote(context.Background(), it.ChannelID, it.ThreadRootID)
			a.reportRun(context.Background(), "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "stopped", "", steps, "", visitedURLs)
			return nil
		}
		if ctx.Err() != nil {
			a.reportRun(context.Background(), "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "halted", "", steps, "", visitedURLs)
			return errHalted
		}
		if timedOut != timeoutNone {
			var timeoutMsg string
			if timedOut == timeoutIdle {
				if a.Cfg.IdleTimeout >= time.Minute {
					timeoutMsg = fmt.Sprintf("⏱ Leállítottam: %d perce nem jött semmi a klienstől.", int(a.Cfg.IdleTimeout.Minutes()))
				} else {
					timeoutMsg = fmt.Sprintf("⏱ Leállítottam: %d másodperce nem jött semmi a klienstől.", int(a.Cfg.IdleTimeout.Seconds()))
				}
			} else {
				if a.Cfg.MaxRun >= time.Hour {
					timeoutMsg = fmt.Sprintf("⏱ Leállítottam: elérte a(z) %d órás korlátot.", int(a.Cfg.MaxRun.Hours()))
				} else if a.Cfg.MaxRun >= time.Minute {
					timeoutMsg = fmt.Sprintf("⏱ Leállítottam: elérte a(z) %d perces korlátot.", int(a.Cfg.MaxRun.Minutes()))
				} else {
					timeoutMsg = fmt.Sprintf("⏱ Leállítottam: elérte a(z) %d másodperces korlátot.", int(a.Cfg.MaxRun.Seconds()))
				}
			}
			a.Log.Warn("guard: work run killed due to timeout", "agent", a.logName(), "reason", timedOut.String(), "session", res.SessionID)
			replyMsgID, replyErr := a.replyLongWithID(ctx, it.Item, timeoutMsg, "gw-"+it.ID)
			a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "error", "", steps, replyMsgID, visitedURLs)
			return replyErr
		}
		text := res.Text
		if a.isClaudeClient() && a.ClaudeLimiter != nil {
			if res.RateLimit != nil {
				a.ClaudeLimiter.Update(*res.RateLimit)
			}
			if err != nil && isClaudeLimitMessage(err.Error()) {
				a.ClaudeLimiter.RecordLimitFailure(err.Error(), a.now())
				a.postRateLimitNotes(ctx, []Item{it.Item})
				return nil
			}
		}
		if err != nil && !stopped {
			errID, rerr := a.reportError(ctx, it.Item, "work run failed", err.Error(), "gw-"+it.ID)
			a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "error", errID, steps, "", visitedURLs)
			return rerr
		}
		var commitLine string
		if a.Sandbox && a.WorktreeRepo != "" {
			commitLine = a.checkAndCommitWorktree(ctx, it, task, p.Dir)
		}
		var webReadFooter string
		if len(visitedURLs) > 0 {
			webReadFooter = formatWebReadFooter(visitedURLs)
		}
		if stopped {
			text = "⚠️ Stopped: my folder access changed while I was working. Ask again if it is still needed."
			replyMsgID, replyErr := a.replyLongWithID(ctx, it.Item, text, "gw-"+it.ID)
			a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "halted", "", steps, replyMsgID, visitedURLs)
			return replyErr
		}
		if strings.TrimSpace(text) == "" {
			text = "Done."
		}
		if commitLine != "" {
			text = strings.TrimSpace(text) + "\n\n" + commitLine
		}
		if webReadFooter != "" {
			text = strings.TrimSpace(text) + "\n\n" + webReadFooter
		}
		replyMsgID, replyErr := a.replyLongWithID(ctx, it.Item, text, "gw-"+it.ID)
		a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "ok", "", steps, replyMsgID, visitedURLs)
		return replyErr
	})
}

// reportError keeps an error's details out of the chat: they go to the
// server (humans only) and the guard's log; the thread gets a short note
// with a reference the web page turns into a "details" button.
func (a *Agent) reportError(ctx context.Context, it Item, summary, detail, clientID string) (string, error) {
	a.Log.Error("guard: run error", "agent", a.logName(), "summary", summary, "detail", detail)
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
		if thread == "" {
			thread = it.ID
		}
	}
	msg := "⚠️ I ran into an error."
	id, err := a.Client.ReportError(ctx, it.ChannelID, thread, summary, detail)
	if err == nil && id != "" {
		msg += " [error:" + id + "]"
	} else if err != nil {
		a.Log.Warn("guard: could not store the error details", "err", err)
	}
	return id, a.reply(ctx, it, msg, clientID)
}

// reply answers in the item's thread (top level in a DM without a thread).
func (a *Agent) reply(ctx context.Context, it Item, text, clientID string) error {
	_, err := a.replyWithID(ctx, it, text, clientID)
	return err
}

func (a *Agent) replyWithID(ctx context.Context, it Item, text, clientID string) (string, error) {
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
		if thread == "" {
			thread = it.ID
		}
	}
	text = strings.TrimSpace(text)
	fullLinks := FileLinks(text)
	clipped := clip(text, MaxReplyLen)
	msgID, err := a.Client.SendAnswer(ctx, it.ChannelID, thread, clipped, clientID, false, it.AnswersMessageID)
	if err := halted(err); err != nil {
		return "", err
	}
	a.sendLostLinks(ctx, it, thread, fullLinks, clipped, clientID)
	return msgID, nil
}

// MaxLongReply bounds a direct-mode answer (a report someone asked for).
const MaxLongReply = 8000

// replyLong answers in the thread like reply, but a text longer than the
// usual limit is sent as an elaborate message instead of being cut. Used
// for direct-mode answers: a human asked, and a report may be long.
func (a *Agent) replyLong(ctx context.Context, it Item, text, clientID string) error {
	_, err := a.replyLongWithID(ctx, it, text, clientID)
	return err
}

func (a *Agent) replyLongWithID(ctx context.Context, it Item, text, clientID string) (string, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= MaxReplyLen {
		return a.replyWithID(ctx, it, text, clientID)
	}
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
		if thread == "" {
			thread = it.ID
		}
	}
	fullLinks := FileLinks(text)
	clipped := clip(text, MaxLongReply)
	msgID, err := a.Client.SendAnswer(ctx, it.ChannelID, thread, clipped, clientID, true, it.AnswersMessageID)
	if err := halted(err); err != nil {
		return "", err
	}
	a.sendLostLinks(ctx, it, thread, fullLinks, clipped, clientID)
	return msgID, nil
}

func (a *Agent) sendLostLinks(ctx context.Context, it Item, thread string, fullLinks []string, clipped, clientID string) {
	if len(fullLinks) == 0 {
		return
	}
	clippedLinks := FileLinks(clipped)
	clippedSet := make(map[string]bool, len(clippedLinks))
	for _, l := range clippedLinks {
		clippedSet[l] = true
	}
	var lost []string
	for _, l := range fullLinks {
		if !clippedSet[l] {
			lost = append(lost, l)
		}
	}
	if len(lost) == 0 {
		return
	}
	body := "📎\n" + strings.Join(lost, "\n")
	filesClientID := clientID + "-files"
	var err error
	if utf8.RuneCountInString(body) <= MaxReplyLen {
		err = a.Client.Send(ctx, it.ChannelID, thread, body, filesClientID)
	} else {
		_, err = a.Client.SendLong(ctx, it.ChannelID, thread, body, filesClientID, true)
	}
	if err != nil && a.Log != nil {
		agentName := a.logName()
		a.Log.Warn("guard: follow-up file links failed", "err", err, "agent", agentName, "channel", it.ChannelID)
	}
}

func halted(err error) error {
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusForbidden {
		return fmt.Errorf("%w: %s", errHalted, ae.Detail)
	}
	return err
}

// threadContext returns up to maxContextMsgs earlier messages of the item's
// thread, so the model can answer a reply without a read tool call.
func (a *Agent) threadContext(ctx context.Context, it Item) []ContextMsg {
	if it.ThreadRootID == "" {
		return nil
	}
	root, replies, err := a.Client.Thread(ctx, it.ThreadRootID)
	if err != nil {
		return nil
	}
	msgs := append([]Message{root}, replies...)
	var earlier []Message
	for _, m := range msgs {
		if m.ID == it.ItemID {
			break
		}
		if m.DeletedAt != nil {
			continue
		}
		earlier = append(earlier, m)
	}
	if len(earlier) > maxContextMsgs {
		earlier = append(earlier[:1], earlier[len(earlier)-maxContextMsgs+1:]...) // keep the root
	}
	out := make([]ContextMsg, 0, len(earlier))
	for _, m := range earlier {
		out = append(out, ContextMsg{Author: a.handleOf(ctx, m.AuthorID), Body: m.Body})
	}
	return out
}

func (a *Agent) handleOf(ctx context.Context, id string) string {
	if h, ok := a.handles[id]; ok {
		return h
	}
	if hs, err := a.Client.Handles(ctx); err == nil {
		a.handles = hs
	}
	if h, ok := a.handles[id]; ok {
		return h
	}
	return "unknown"
}

// watchHalt cancels the returned context as soon as the agent may no longer
// write: the client process is killed, nothing more is posted.
func (a *Agent) watchHalt(ctx context.Context) (context.Context, func()) {
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	interval := a.Cfg.HaltCheck
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		defer wg.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-t.C:
				me, err := a.Client.Me(runCtx)
				var ae *APIError
				if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
					me.Halted, me.State = true, "stopped" // a stopped agent's token stops working
				}
				if me.Halted {
					a.halted.Store(true)
					a.Log.Warn("guard: halt detected, stopping the client", "agent", a.logName(), "state", me.State)
					if a.Terminals != nil {
						a.Terminals.KillAllForHandle(a.Cfg.Name, "halt")
					}
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, func() { cancel(); wg.Wait() }
}

func (a *Agent) logRun(phase, model string, items int, res RunResult, err error) {
	if a.Log == nil {
		return
	}
	attrs := []any{"agent", a.logName(), "phase", phase, "client", a.Cfg.Client, "model", model, "items", items,
		"tokens", res.Usage.Total(), "in", res.Usage.Input, "out", res.Usage.Output,
		"cache_read", res.Usage.CacheRead, "cache_write", res.Usage.CacheWrite, "ms", res.Duration.Milliseconds()}
	if phase == "decide" && err == nil {
		attrs = append(attrs, "actions", summarize(res.Decision))
	}
	if err != nil {
		a.Log.Warn("guard: run failed", append(attrs, "err", err)...)
		return
	}
	if res.ClientWarning != "" {
		a.Log.Warn("guard: run answered with a client warning", append(attrs, "warning", res.ClientWarning)...)
		return
	}
	a.Log.Info("guard: run", attrs...)
}

func (a *Agent) reportRun(ctx context.Context, phase, model, sessionID, channelID, threadRootID string, items int, res RunResult, status, errorID string, steps []Step, replyMessageID string, webReads ...[]string) {
	rctx := ctx
	if rctx.Err() != nil {
		rctx = context.Background()
	}
	rctx, cancel := context.WithTimeout(rctx, 5*time.Second)
	defer cancel()

	dur := int(res.Duration.Milliseconds())
	if dur < 0 {
		dur = 0
	}
	var urls []string
	if len(webReads) > 0 {
		urls = webReads[0]
	}
	_, err := a.Client.ReportRun(rctx, ReportRunInput{
		Phase:            phase,
		Client:           a.Cfg.Client,
		Model:            model,
		SessionID:        sessionID,
		ChannelID:        channelID,
		ThreadRootID:     threadRootID,
		Items:            items,
		InputTokens:      int64(res.Usage.Input),
		OutputTokens:     int64(res.Usage.Output),
		CacheReadTokens:  int64(res.Usage.CacheRead),
		CacheWriteTokens: int64(res.Usage.CacheWrite),
		DurationMS:       dur,
		Status:           status,
		ErrorID:          errorID,
		WebReads:         urls,
		Steps:            steps,
		ReplyMessageID:   replyMessageID,
	})
	if err != nil && a.Log != nil {
		a.Log.Warn("guard: could not report run", "phase", phase, "err", err)
	}
}

func formatWebReadFooter(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	var hosts []string
	seen := make(map[string]bool)
	for _, raw := range urls {
		u, err := url.Parse(raw)
		host := raw
		if err == nil && u.Hostname() != "" {
			host = u.Hostname()
		}
		if !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	pageWord := "pages"
	if len(urls) == 1 {
		pageWord = "page"
	}
	return fmt.Sprintf("🌐 web_read: %s (%d %s)", strings.Join(hosts, ", "), len(urls), pageWord)
}

func summarize(d Decision) string {
	counts := map[string]int{}
	for _, a := range d.Actions {
		counts[a.Kind]++
	}
	var parts []string
	for _, k := range []string{ActReply, ActReact, ActWork, ActNone} {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
		}
	}
	return strings.Join(parts, " ")
}

func (a *Agent) isClaudeClient() bool {
	return a.Cfg.Client == "claude-code"
}

func (a *Agent) postRateLimitNotes(ctx context.Context, items []Item) {
	if a.ClaudeLimiter == nil || a.Client == nil {
		return
	}
	if a.notedRateLimitThreads == nil {
		a.notedRateLimitThreads = make(map[string]bool)
	}
	info := a.ClaudeLimiter.PauseInfo()

	for _, it := range items {
		targetChannelID := it.ChannelID
		targetThreadID := it.ThreadRootID
		if it.ReplyOrigin != nil {
			targetChannelID = it.ReplyOrigin.ChannelID
			targetThreadID = it.ReplyOrigin.ThreadRootID
		} else if targetThreadID == "" && !it.DM {
			targetThreadID = it.ItemID
			if targetThreadID == "" {
				targetThreadID = it.ID
			}
		}
		key := targetChannelID + "/" + targetThreadID
		if a.notedRateLimitThreads[key] {
			continue
		}
		if err := a.Client.PostRateLimitNote(ctx, targetChannelID, targetThreadID, info.Window, info.Utilization, info.ResetsAt); err != nil {
			a.Log.Warn("guard: post rate limit note failed", "channel_id", targetChannelID, "thread_root_id", targetThreadID, "err", err)
		} else {
			a.notedRateLimitThreads[key] = true
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RunAll starts one loop per configured agent and blocks until ctx ends.
func RunAll(ctx context.Context, cfg Config, run Runner, log *slog.Logger) error {
	var exchangeOK bool
	if cfg.ExchangeDir != "" {
		if err := CheckExchangeDir(cfg.ExchangeDir); err != nil {
			log.Warn("guard: secrets exchange folder not usable", "dir", cfg.ExchangeDir, "err", err)
		} else {
			exchangeOK = true
		}
	}
	var claudeLimiter *ClaudeLimiter
	for _, ac := range cfg.Agents {
		if ac.Client == "claude-code" {
			stateDir := cfg.SandboxDir
			if stateDir == "" {
				home, _ := os.UserHomeDir()
				stateDir = filepath.Join(home, ".local", "share", "colonyhall", "sandbox")
			}
			claudeLimiter = NewClaudeLimiter(filepath.Join(filepath.Dir(stateDir), "claude_usage.json"), cfg.RateLimitReserve, cfg.RateLimitWarn, log, nil)
			break
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(cfg.Agents))
	for _, ac := range cfg.Agents {
		token, err := ac.Token()
		if err != nil {
			return err
		}
		agent := &Agent{
			Cfg:           ac,
			Client:        &Client{BaseURL: cfg.Server, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}},
			Adapter:       Adapters[ac.Client](ac, run),
			Log:           log,
			ExchangeDir:   cfg.ExchangeDir,
			ExchangeOK:    exchangeOK,
			ClaudeLimiter: claudeLimiter,
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := agent.Run(ctx); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	return <-errs
}
