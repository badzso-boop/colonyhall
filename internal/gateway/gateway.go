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
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
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
	Logger       *slog.Logger
	// OriginPatterns allows cross-origin browsers (e.g. a Vite dev server).
	OriginPatterns []string
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
			if err := g.write(ctx, c, map[string]any{"type": "resync_required", "head": head}); err != nil {
				return
			}
			cursor = head
		}
	}
	if err := g.write(ctx, c, map[string]any{"type": "welcome", "protocol": Protocol, "head": head}); err != nil {
		return
	}

	conn := &connection{g: g, ws: c, actor: actor, cursor: cursor, canRead: map[ids.ID]bool{}}
	if err := conn.catchUp(ctx); err != nil {
		return
	}
	if err := g.write(ctx, c, map[string]any{"type": "replay_done", "seq": conn.cursor}); err != nil {
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

func (g *Gateway) write(ctx context.Context, c *websocket.Conn, v any) error {
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

	mu      sync.Mutex
	canRead map[ids.ID]bool
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
				if err := c.g.write(ctx, c.ws, out); err != nil {
					return err
				}
			}
			c.cursor = e.Seq
		}
	}
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
		return false, nil
	}
	// A membership change for this actor invalidates the cached answer.
	if e.Type == conversation.EventMemberAdded {
		c.mu.Lock()
		delete(c.canRead, e.ChannelID)
		c.mu.Unlock()
	}
	c.mu.Lock()
	ok, cached := c.canRead[e.ChannelID]
	c.mu.Unlock()
	if cached {
		return ok, nil
	}
	ok, err := c.g.Conversation.CanRead(ctx, c.actor, e.ChannelID)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.canRead[e.ChannelID] = ok
	c.mu.Unlock()
	return ok, nil
}
