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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
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

	// WebSocket options and test hooks
	DisableWS          bool
	WSTokenHook        func(ctx context.Context) (string, error)
	WSBackoff          func(attempt int) time.Duration
	TokenCheckInterval time.Duration
	WSDialHook         func(ctx context.Context, u string, opts *websocket.DialOptions) (*websocket.Conn, *http.Response, error)

	workMu     sync.Mutex
	workCancel context.CancelFunc

	me       Me
	handles  map[string]string
	failures int
	attempts map[string]int // item id → failed decision runs
}

// maxAttempts bounds retries of the same items, so a broken client does not
// burn tokens forever. After that the items are acknowledged and logged.
const maxAttempts = 3

// errHalted means the agent must stop writing (paused, stopped, emergency).
var errHalted = errors.New("agent is halted")

// Run loops until ctx ends.
func (a *Agent) Run(ctx context.Context) error {
	if a.attempts == nil {
		a.attempts = map[string]int{}
	}
	me, err := a.Client.Me(ctx)
	if err != nil {
		return fmt.Errorf("guard %s: cannot reach the server: %w", a.Cfg.Name, err)
	}
	a.me = me
	a.Log.Info("guard: watching", "agent", "@"+me.Handle, "client", a.Cfg.Client, "model", a.Cfg.Model)

	wakeCh := make(chan struct{}, 1)
	if !a.DisableWS && !a.Cfg.DisableWS {
		wakeCh <- struct{}{} // initial inbox check on startup
		go a.watchWS(ctx, wakeCh)
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
		}
		if err := a.Tick(ctx); err != nil && ctx.Err() == nil {
			a.failures++
			a.Log.Warn("guard: tick failed", "agent", "@"+a.me.Handle, "err", err, "failures", a.failures)
		}
		timer.Reset(a.waitInterval())
	}
}

func (a *Agent) waitInterval() time.Duration {
	if a.DisableWS || a.Cfg.DisableWS {
		return a.pause()
	}
	return a.fallbackPause()
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
	a.me = me
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

func (a *Agent) handle(ctx context.Context, items []Item) error {
	runCtx, stop := a.watchHalt(ctx)
	defer stop()

	pitems := make([]PromptItem, 0, len(items))
	for _, it := range items {
		pitems = append(pitems, PromptItem{Item: it, Context: a.threadContext(runCtx, it)})
	}
	dctx, cancel := context.WithTimeout(runCtx, a.Cfg.DecideTimeout)
	res, err := a.Adapter.Decide(dctx, DecideRequest{Handle: a.me.Handle, Items: pitems, Model: a.Cfg.Model, Dir: a.Cfg.Dir, Wrap: a.DecideWrap})
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
	a.reportRun(ctx, "decide", a.Cfg.Model, "", "", "", len(items), res, decideStatus, "")
	if err != nil {
		if runCtx.Err() != nil && ctx.Err() == nil {
			a.Log.Warn("guard: run stopped, agent halted", "agent", "@"+a.me.Handle)
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
				a.Log.Warn("guard: stopped mid-run, agent halted", "agent", "@"+a.me.Handle)
				return nil
			}
			return err
		}
		acked = append(acked, it.ID)
	}
	return a.Client.Ack(ctx, acked)
}

// giveUpAfterRetries keeps failed items for a retry, but acknowledges them
// after maxAttempts so a broken client cannot loop on them.
func (a *Agent) giveUpAfterRetries(ctx context.Context, items []Item, cause error) error {
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
		a.Log.Error("guard: giving up on items after repeated failures", "agent", "@"+a.me.Handle, "items", len(drop), "err", cause)
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
	Allowed bool
	Reason  string // why not, shown in the chat
	Dir     string
	AddDirs []string
	Wrap    Wrapper
	NoExec  bool
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

func (a *Agent) work(ctx context.Context, it PromptItem, task string) error {
	p := a.policy()
	if !p.Allowed {
		return a.reply(ctx, it.Item, p.Reason, "g-"+it.ID)
	}
	if strings.TrimSpace(task) == "" {
		task = it.Body
	}
	if err := halted(a.Client.React(ctx, it.ItemID, "👀")); err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, a.Cfg.WorkTimeout)
	a.workMu.Lock()
	a.workCancel = cancel
	a.workMu.Unlock()
	res, err := a.Adapter.Work(wctx, WorkRequest{
		Prompt: WorkPrompt(a.me.Handle, it, task), Model: a.Cfg.WorkModel, Dir: p.Dir, PermissionMode: a.Cfg.PermissionMode,
		AddDirs: p.AddDirs, Wrap: p.Wrap, NoExec: p.NoExec,
	})
	stopped := errors.Is(wctx.Err(), context.Canceled)
	cancel()
	a.workMu.Lock()
	a.workCancel = nil
	a.workMu.Unlock()
	a.logRun("work", a.Cfg.WorkModel, 1, res, err)
	if ctx.Err() != nil {
		a.reportRun(context.Background(), "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "halted", "")
		return errHalted
	}
	text := res.Text
	if err != nil && !stopped {
		errID, rerr := a.reportError(ctx, it.Item, "work run failed", err.Error(), "gw-"+it.ID)
		a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "error", errID)
		return rerr
	}
	if stopped {
		a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "halted", "")
		text = "⚠️ Stopped: my folder access changed while I was working. Ask again if it is still needed."
	} else {
		a.reportRun(ctx, "work", a.Cfg.WorkModel, res.SessionID, it.ChannelID, it.ThreadRootID, 1, res, "ok", "")
	}
	if strings.TrimSpace(text) == "" {
		text = "Done."
	}
	return a.reply(ctx, it.Item, text, "gw-"+it.ID)
}

// reportError keeps an error's details out of the chat: they go to the
// server (humans only) and the guard's log; the thread gets a short note
// with a reference the web page turns into a "details" button.
func (a *Agent) reportError(ctx context.Context, it Item, summary, detail, clientID string) (string, error) {
	a.Log.Error("guard: run error", "agent", "@"+a.me.Handle, "summary", summary, "detail", detail)
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
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
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
	}
	return halted(a.Client.Send(ctx, it.ChannelID, thread, clip(strings.TrimSpace(text), MaxReplyLen), clientID))
}

// MaxLongReply bounds a direct-mode answer (a report someone asked for).
const MaxLongReply = 8000

// replyLong answers in the thread like reply, but a text longer than the
// usual limit is sent as an elaborate message instead of being cut. Used
// for direct-mode answers: a human asked, and a report may be long.
func (a *Agent) replyLong(ctx context.Context, it Item, text, clientID string) error {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= MaxReplyLen {
		return a.reply(ctx, it, text, clientID)
	}
	thread := it.ThreadRootID
	if thread == "" && !it.DM {
		thread = it.ItemID
	}
	return halted(a.Client.SendLong(ctx, it.ChannelID, thread, clip(text, MaxLongReply), clientID, true))
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
	go func() {
		defer wg.Done()
		t := time.NewTicker(a.Cfg.HaltCheck)
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
					a.Log.Warn("guard: halt detected, stopping the client", "agent", "@"+a.me.Handle, "state", me.State)
					cancel()
					return
				}
			}
		}
	}()
	return runCtx, func() { cancel(); wg.Wait() }
}

func (a *Agent) logRun(phase, model string, items int, res RunResult, err error) {
	attrs := []any{"agent", "@" + a.me.Handle, "phase", phase, "client", a.Cfg.Client, "model", model, "items", items,
		"tokens", res.Usage.Total(), "in", res.Usage.Input, "out", res.Usage.Output,
		"cache_read", res.Usage.CacheRead, "cache_write", res.Usage.CacheWrite, "ms", res.Duration.Milliseconds()}
	if phase == "decide" && err == nil {
		attrs = append(attrs, "actions", summarize(res.Decision))
	}
	if err != nil {
		a.Log.Warn("guard: run failed", append(attrs, "err", err)...)
		return
	}
	a.Log.Info("guard: run", attrs...)
}

func (a *Agent) reportRun(ctx context.Context, phase, model, sessionID, channelID, threadRootID string, items int, res RunResult, status, errorID string) {
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
	})
	if err != nil {
		a.Log.Warn("guard: could not report run", "phase", phase, "err", err)
	}
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
	var wg sync.WaitGroup
	errs := make(chan error, len(cfg.Agents))
	for _, ac := range cfg.Agents {
		token, err := ac.Token()
		if err != nil {
			return err
		}
		agent := &Agent{
			Cfg:     ac,
			Client:  &Client{BaseURL: cfg.Server, Token: token, HTTP: &http.Client{Timeout: 30 * time.Second}},
			Adapter: Adapters[ac.Client](ac, run),
			Log:     log,
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
