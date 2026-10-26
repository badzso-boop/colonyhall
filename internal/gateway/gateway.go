// Package gateway serves live events to browsers over WebSocket.
//
// Protocol (JSON text frames):
//
//	client → {"type":"hello","token":"…","last_seq":123}
//	server → {"type":"welcome","protocol":1,"head":456}
//	server → {"type":"event","seq":124,"event_type":"message.created",…}
//	server → {"type":"replay_done","seq":456}
//
// Every connection keeps its own cursor and reads events from the event log
// itself; the bus only wakes it up (Dendrite notifier pattern). A client
// that reconnects with last_seq gets everything it missed, on any server
// instance. A connection that falls behind is closed; the client then
// reconnects and replays from its last sequence, so nothing is lost.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"colonyhall/internal/apispec"
	"colonyhall/internal/eventlog"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

// Protocol is the WebSocket protocol version.
const Protocol = 1

const (
	helloTimeout = 10 * time.Second
	writeTimeout = 10 * time.Second
	readBatch    = 200
	// maxReplay bounds how far back a client may replay; older cursors must
	// reload state over REST.
	maxReplay = 5000
)

// Gateway accepts WebSocket connections.
type Gateway struct {
	Events       eventlog.Store
	Bus          bus.Bus
	Auth         identity.RequestAuthenticator
	Conversation conversation.Reader
	Clock        clock.Clock
	Logger       *slog.Logger
	// OriginPatterns allows cross-origin browsers (e.g. a Vite dev server).
	OriginPatterns []string

	mu         sync.Mutex
	conns      map[*connection]struct{}
	activities map[activityKey]time.Time
	progress   map[progressKey]*runProgress
}

type activityKey struct {
	agentID      ids.ID
	channelID    ids.ID
	threadRootID ids.ID
}

// progressKey scopes a run to its workspace and agent: run keys come from
// the guards, so one agent must not be able to add steps to another's run.
type progressKey struct {
	workspaceID ids.ID
	agentID     ids.ID
	runKey      string
}

type runProgress struct {
	AgentID      ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID
	RunKey       string
	Steps        []apispec.AgentStep
	Done         bool
	DoneAt       time.Time
	UpdatedAt    time.Time
}

// ActivityFrame is pushed to WebSocket clients when an agent begins or finishes work.
type ActivityFrame struct {
	Type         string  `json:"type"`
	AgentID      string  `json:"agent_id"`
	ChannelID    string  `json:"channel_id"`
	ThreadRootID *string `json:"thread_root_id,omitempty"`
	State        string  `json:"state"`
}

// ProgressFrame is pushed to WebSocket clients when an agent reports progress steps.
type ProgressFrame struct {
	Type         string              `json:"type"`
	AgentID      string              `json:"agent_id"`
	ChannelID    string              `json:"channel_id"`
	ThreadRootID *string             `json:"thread_root_id,omitempty"`
	RunKey       string              `json:"run_key"`
	Steps        []apispec.AgentStep `json:"steps"`
	Done         bool                `json:"done"`
}

// FsRequestFrame is pushed to WebSocket clients when a filesystem request arrives for an agent.
type FsRequestFrame struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

type hello struct {
	Type    string `json:"type"`
	Token   string `json:"token"`
	LastSeq *int64 `json:"last_seq"`
}

type outEvent struct {
	Type      string          `json:"type"`
	Seq       int64           `json:"seq"`
	EventType string          `json:"event_type"`
	ChannelID string          `json:"channel_id,omitempty"`
	ActorID   string          `json:"actor_id"`
	At        time.Time       `json:"at"`
	Data      json.RawMessage `json:"data"`
}

func (g *Gateway) now() time.Time {
	if g.Clock != nil {
		return g.Clock.Now()
	}
	return time.Now()
}

func (g *Gateway) register(c *connection) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conns == nil {
		g.conns = make(map[*connection]struct{})
	}
	g.conns[c] = struct{}{}
}

func (g *Gateway) unregister(c *connection) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.conns, c)
}

// IsWorking reports whether the agent currently has an active, unexpired working activity.
func (g *Gateway) IsWorking(agentID ids.ID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	for k, exp := range g.activities {
		if exp.Before(now) {
			delete(g.activities, k)
			continue
		}
		if k.agentID == agentID {
			return true
		}
	}
	return false
}

// ReportActivity updates the in-memory activity state for an agent and pushes
// an ephemeral WebSocket frame to viewers who can read that channel.
func (g *Gateway) ReportActivity(ctx context.Context, actor identity.Actor, channelID, threadRootID ids.ID, state string) error {
	if actor.Kind != identity.KindAgent {
		return identity.ErrForbidden
	}
	if actor.Halted {
		return identity.ErrHalted
	}
	if state != "working" && state != "idle" {
		return &httpx.BadRequest{Detail: "state must be working or idle"}
	}
	if channelID == ids.Nil {
		return &httpx.BadRequest{Detail: "channel_id is required"}
	}
	canRead, err := g.Conversation.CanRead(ctx, actor, channelID)
	if err != nil {
		return err
	}
	if !canRead {
		return identity.ErrForbidden
	}

	now := g.now()
	key := activityKey{agentID: actor.ID, channelID: channelID, threadRootID: threadRootID}
	g.mu.Lock()
	if g.activities == nil {
		g.activities = make(map[activityKey]time.Time)
	}
	if state == "working" {
		g.activities[key] = now.Add(60 * time.Second)
	} else {
		delete(g.activities, key)
	}
	recipients := make([]*connection, 0, len(g.conns))
	for c := range g.conns {
		if c.actor.WorkspaceID == actor.WorkspaceID {
			recipients = append(recipients, c)
		}
	}
	g.mu.Unlock()

	frame := ActivityFrame{
		Type:      "agent.activity",
		AgentID:   actor.ID.String(),
		ChannelID: channelID.String(),
		State:     state,
	}
	if threadRootID != ids.Nil {
		tr := threadRootID.String()
		frame.ThreadRootID = &tr
	}

	for _, c := range recipients {
		visible, err := c.visibleChannel(ctx, channelID)
		if err == nil && visible {
			_ = c.writeJSON(ctx, frame)
		}
	}
	return nil
}

// PushFsRequest delivers an ephemeral fs.request frame only to the open connections of the given agent.
func (g *Gateway) PushFsRequest(agentID ids.ID, requestID string) bool {
	g.mu.Lock()
	var recipients []*connection
	for c := range g.conns {
		if c.actor.Kind == identity.KindAgent && c.actor.ID == agentID {
			recipients = append(recipients, c)
		}
	}
	g.mu.Unlock()

	if len(recipients) == 0 {
		return false
	}

	frame := FsRequestFrame{
		Type:      "fs.request",
		RequestID: requestID,
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	pushed := false
	for _, c := range recipients {
		if err := c.writeJSON(ctx, frame); err == nil {
			pushed = true
		}
	}
	return pushed
}

// IsAgentWorking returns whether the agent is currently working in any channel.
func (g *Gateway) IsAgentWorking(agentID ids.ID) bool {
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, exp := range g.activities {
		if exp.Before(now) {
			delete(g.activities, k)
			continue
		}
		if k.agentID == agentID {
			return true
		}
	}
	return false
}

func (g *Gateway) pruneProgressLocked(now time.Time) {
	for k, rp := range g.progress {
		if rp.Done && now.Sub(rp.DoneAt) >= 2*time.Minute {
			delete(g.progress, k)
			continue
		}
		if now.Sub(rp.UpdatedAt) >= 15*time.Minute {
			delete(g.progress, k)
			continue
		}
	}
}

// ReportProgress updates the in-memory progress for a run and pushes an ephemeral
// agent.progress WebSocket frame to viewers who can read that channel.
func (g *Gateway) ReportProgress(ctx context.Context, actor identity.Actor, in apispec.AgentProgressRequest) error {
	if actor.Kind != identity.KindAgent {
		return identity.ErrForbidden
	}
	if actor.Halted {
		return identity.ErrHalted
	}
	if in.ChannelId == uuid.Nil {
		return &httpx.BadRequest{Detail: "channel_id is required"}
	}
	if in.RunKey == "" {
		return &httpx.BadRequest{Detail: "run_key is required"}
	}
	canRead, err := g.Conversation.CanRead(ctx, actor, ids.ID(in.ChannelId))
	if err != nil {
		return err
	}
	if !canRead {
		return identity.ErrForbidden
	}

	now := g.now()
	g.mu.Lock()
	if g.progress == nil {
		g.progress = make(map[progressKey]*runProgress)
	}
	g.pruneProgressLocked(now)

	var trID ids.ID
	if in.ThreadRootId != nil {
		trID = ids.ID(*in.ThreadRootId)
	}

	key := progressKey{workspaceID: actor.WorkspaceID, agentID: actor.ID, runKey: in.RunKey}
	rp, ok := g.progress[key]
	if ok && rp.ChannelID != ids.ID(in.ChannelId) {
		g.mu.Unlock()
		return &httpx.BadRequest{Detail: "run_key belongs to another channel"}
	}
	if !ok {
		rp = &runProgress{
			AgentID:      actor.ID,
			ChannelID:    ids.ID(in.ChannelId),
			ThreadRootID: trID,
			RunKey:       in.RunKey,
			UpdatedAt:    now,
		}
		g.progress[key] = rp
	}
	rp.Steps = append(rp.Steps, in.Steps...)
	if len(rp.Steps) > 200 {
		rp.Steps = rp.Steps[:200]
	}
	rp.UpdatedAt = now
	isDone := in.Done != nil && *in.Done
	if isDone {
		rp.Done = true
		rp.DoneAt = now
	}

	recipients := make([]*connection, 0, len(g.conns))
	for c := range g.conns {
		if c.actor.WorkspaceID == actor.WorkspaceID {
			recipients = append(recipients, c)
		}
	}
	g.mu.Unlock()

	frame := ProgressFrame{
		Type:      "agent.progress",
		AgentID:   actor.ID.String(),
		ChannelID: in.ChannelId.String(),
		RunKey:    in.RunKey,
		Steps:     in.Steps,
		Done:      isDone,
	}
	if in.ThreadRootId != nil && *in.ThreadRootId != uuid.Nil {
		tr := in.ThreadRootId.String()
		frame.ThreadRootID = &tr
	}

	for _, c := range recipients {
		// Steps are for humans only: other agents' guards never get them.
		if c.actor.Kind != identity.KindHuman {
			continue
		}
		visible, err := c.visibleChannel(ctx, ids.ID(in.ChannelId))
		if err == nil && visible {
			_ = c.writeJSON(ctx, frame)
		}
	}
	return nil
}

// ChannelProgress returns active runs with their steps in the channel for humans who can read it.
func (g *Gateway) ChannelProgress(ctx context.Context, actor identity.Actor, channelID ids.ID) ([]apispec.ActiveRunProgress, error) {
	if actor.Kind != identity.KindHuman {
		return nil, identity.ErrForbidden
	}
	canRead, err := g.Conversation.CanRead(ctx, actor, channelID)
	if err != nil {
		return nil, err
	}
	if !canRead {
		return nil, identity.ErrForbidden
	}

	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneProgressLocked(now)

	out := make([]apispec.ActiveRunProgress, 0)
	for _, rp := range g.progress {
		if rp.ChannelID == channelID {
			var tr *openapi_types.UUID
			if rp.ThreadRootID != ids.Nil {
				u := openapi_types.UUID(rp.ThreadRootID)
				tr = &u
			}
			done := rp.Done
			stepsCopy := make([]apispec.AgentStep, len(rp.Steps))
			copy(stepsCopy, rp.Steps)
			out = append(out, apispec.ActiveRunProgress{
				AgentId:      openapi_types.UUID(rp.AgentID),
				ChannelId:    openapi_types.UUID(rp.ChannelID),
				ThreadRootId: tr,
				RunKey:       rp.RunKey,
				Steps:        stepsCopy,
				Done:         &done,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].RunKey < out[j].RunKey
	})
	return out, nil
}

// ServeHTTP upgrades the request and runs the connection until it closes.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Browsers authenticate with the session cookie sent on the upgrade
	// request (the origin check below blocks cross-site use of it); agents
	// and scripts send a token in the hello frame instead.
	var cookie string
	if c, err := r.Cookie(httpx.SessionCookie); err == nil {
		cookie = c.Value
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: g.OriginPatterns})
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	ctx := r.Context()

	helloCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	var h hello
	err = wsjson.Read(helloCtx, c, &h)
	cancel()
	if err != nil || h.Type != "hello" {
		_ = c.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	var actor identity.Actor
	if h.Token != "" {
		actor, err = g.Auth.Authenticate(ctx, h.Token)
	} else {
		actor, _, err = g.Auth.AuthenticateSession(ctx, cookie)
	}
	if err != nil {
		_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}

	signals, unsubscribe := g.Bus.Subscribe()
	defer unsubscribe()

	head, err := g.Events.Head(ctx)
	if err != nil {
		_ = c.Close(websocket.StatusInternalError, "unavailable")
		return
	}
	cursor := head
	if h.LastSeq != nil {
		cursor = *h.LastSeq
		if head-cursor > maxReplay || cursor > head {
			if err := g.writeRaw(ctx, c, map[string]any{"type": "resync_required", "head": head}); err != nil {
				return
			}
			cursor = head
		}
	}
	if err := g.writeRaw(ctx, c, map[string]any{"type": "welcome", "protocol": Protocol, "head": head}); err != nil {
		return
	}

	conn := &connection{g: g, ws: c, actor: actor, cursor: cursor, canRead: map[ids.ID]bool{}}
	g.register(conn)
	defer g.unregister(conn)

	if err := conn.catchUp(ctx); err != nil {
		return
	}
	if err := conn.writeJSON(ctx, map[string]any{"type": "replay_done", "seq": conn.cursor}); err != nil {
		return
	}

	// Reading is only needed to notice the client closing.
	ctx = c.CloseRead(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-signals:
			if err := conn.catchUp(ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					g.logger().Debug("websocket closed", "actor", actor.Handle, "err", err)
				}
				return
			}
		}
	}
}

func (g *Gateway) writeRaw(ctx context.Context, c *websocket.Conn, v any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, c, v)
}

func (g *Gateway) logger() *slog.Logger {
	if g.Logger != nil {
		return g.Logger
	}
	return slog.Default()
}

type connection struct {
	g      *Gateway
	ws     *websocket.Conn
	actor  identity.Actor
	cursor int64

	writeMu sync.Mutex
	mu      sync.Mutex
	canRead map[ids.ID]bool
}

func (c *connection) writeJSON(ctx context.Context, v any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsjson.Write(ctx, c.ws, v)
}

// catchUp sends every visible event after the cursor.
func (c *connection) catchUp(ctx context.Context) error {
	for {
		events, err := c.g.Events.Since(ctx, c.cursor, readBatch)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}
		for _, e := range events {
			visible, err := c.visible(ctx, e)
			if err != nil {
				return err
			}
			if visible {
				out := outEvent{Type: "event", Seq: e.Seq, EventType: e.Type, ActorID: e.ActorID.String(), At: e.At, Data: e.Payload}
				if e.ChannelID != ids.Nil {
					out.ChannelID = e.ChannelID.String()
				}
				if err := c.writeJSON(ctx, out); err != nil {
					return err
				}
			}
			c.cursor = e.Seq
		}
	}
}

// visibleChannel decides whether the connection actor may read the channel.
func (c *connection) visibleChannel(ctx context.Context, channelID ids.ID) (bool, error) {
	if c.actor.IsAdmin() {
		return true, nil
	}
	c.mu.Lock()
	ok, cached := c.canRead[channelID]
	c.mu.Unlock()
	if cached {
		return ok, nil
	}
	ok, err := c.g.Conversation.CanRead(ctx, c.actor, channelID)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.canRead[channelID] = ok
	c.mu.Unlock()
	return ok, nil
}

// visible decides whether the actor may see an event.
func (c *connection) visible(ctx context.Context, e eventlog.Event) (bool, error) {
	if e.WorkspaceID != c.actor.WorkspaceID {
		return false, nil
	}
	if c.actor.IsAdmin() {
		return true, nil
	}
	if e.ChannelID == ids.Nil {
		// Workspace-wide announcements (e.g. the emergency stop) reach
		// everyone; other channel-less events are admin-only.
		return strings.HasPrefix(e.Type, "workspace."), nil
	}
	// A membership change for this actor invalidates the cached answer.
	if e.Type == conversation.EventMemberAdded || e.Type == conversation.EventMemberRemoved {
		c.mu.Lock()
		delete(c.canRead, e.ChannelID)
		c.mu.Unlock()
	}
	return c.visibleChannel(ctx, e.ChannelID)
}
