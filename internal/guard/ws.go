package guard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

var errTokenChanged = errors.New("guard: agent token changed")

// wsBackoff computes exponential backoff (1s -> 30s) for WebSocket reconnects.
func wsBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 1 * time.Second
	}
	d := (1 * time.Second) * time.Duration(1<<attempt)
	if d > 30*time.Second || d <= 0 {
		return 30 * time.Second
	}
	return d
}

// wsURL converts a base server URL (http:// or https://) into a WebSocket URL for /ws/ui.
func wsURL(baseURL string) (string, error) { return wsURLPath(baseURL, "/ws/ui") }

// wsURLPath converts a base server URL into a WebSocket URL for path.
func wsURLPath(baseURL, path string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("guard: invalid server URL: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already ws scheme
	default:
		return "", fmt.Errorf("guard: unsupported URL scheme %q for websocket", u.Scheme)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	return u.String(), nil
}

func (a *Agent) logName() string {
	me := a.getMe()
	if me.Handle != "" {
		return "@" + me.Handle
	}
	if a.Cfg.Name != "" {
		return a.Cfg.Name
	}
	return "agent"
}

// wsDialClient returns an http.Client without Timeout, reusing Transport and Jar.
func (a *Agent) wsDialClient() *http.Client {
	if a.Client != nil && a.Client.HTTP != nil {
		return &http.Client{
			Transport: a.Client.HTTP.Transport,
			Jar:       a.Client.HTTP.Jar,
		}
	}
	return nil
}

func (a *Agent) currentToken(ctx context.Context) (string, error) {
	if a.WSTokenHook != nil {
		return a.WSTokenHook(ctx)
	}
	if a.Client != nil && a.Client.WSTokenHook != nil {
		return a.Client.WSTokenHook(ctx)
	}
	if a.Client != nil {
		return a.Client.CurrentToken(ctx)
	}
	return "", errors.New("guard: no client configured")
}

// watchWS maintains a persistent WebSocket connection to /ws/ui, reconnecting
// with backoff when the socket closes, or immediately when the token changes.
func (a *Agent) watchWS(ctx context.Context, wakeCh chan<- struct{}, extra ...chan<- struct{}) {
	var fileFetchWakeCh chan<- struct{}
	if len(extra) > 0 {
		fileFetchWakeCh = extra[0]
	}
	agentName := a.logName()
	var attempt int
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		connected, err := a.runWSConn(ctx, agentName, wakeCh, fileFetchWakeCh)
		if ctx.Err() != nil {
			return
		}

		if errors.Is(err, errTokenChanged) {
			attempt = 0
			continue
		}

		if connected {
			// Once a connection was established and lasted until disconnect,
			// reset the backoff sequence for subsequent retries.
			attempt = 0
		}

		backoffFn := a.WSBackoff
		if backoffFn == nil {
			backoffFn = wsBackoff
		}
		d := backoffFn(attempt)
		attempt++

		a.Log.Debug("guard: websocket reconnecting", "agent", agentName, "backoff", d)

		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

// runWSConn dials /ws/ui, sends the hello handshake, starts the token rotation
// monitor, and reads event frames until disconnection.
func (a *Agent) runWSConn(ctx context.Context, agentName string, wakeCh chan<- struct{}, fileFetchWakeCh chan<- struct{}) (bool, error) {
	token, err := a.currentToken(ctx)
	if err != nil {
		a.Log.Debug("guard: cannot get agent token for websocket", "agent", agentName, "err", err)
		return false, err
	}

	u, err := wsURL(a.Client.BaseURL)
	if err != nil {
		return false, err
	}

	opts := &websocket.DialOptions{
		HTTPClient: a.wsDialClient(),
	}

	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	var conn *websocket.Conn
	var resp *http.Response
	if a.WSDialHook != nil {
		conn, resp, err = a.WSDialHook(connCtx, u, opts)
	} else {
		conn, resp, err = websocket.Dial(connCtx, u, opts)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		a.Log.Debug("guard: websocket dial failed", "agent", agentName, "err", err)
		return false, err
	}
	defer func() {
		_ = conn.CloseNow()
	}()

	helloCtx, helloCancel := context.WithTimeout(connCtx, 10*time.Second)
	err = wsjson.Write(helloCtx, conn, map[string]any{
		"type":  "hello",
		"token": token,
	})
	helloCancel()
	if err != nil {
		a.Log.Debug("guard: websocket hello failed", "agent", agentName, "err", err)
		return false, err
	}

	a.Log.Debug("guard: websocket connected", "agent", agentName)

	tokenInterval := a.TokenCheckInterval
	if tokenInterval <= 0 {
		tokenInterval = 5 * time.Second
	}

	var tokenChanged atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(tokenInterval)
		defer t.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case <-t.C:
				current, err := a.currentToken(connCtx)
				if err == nil && current != "" && current != token {
					tokenChanged.Store(true)
					a.Log.Debug("guard: agent token changed, reconnecting websocket", "agent", agentName)
					_ = conn.Close(websocket.StatusNormalClosure, "token changed")
					return
				}
			}
		}
	}()

	for {
		var frame struct {
			Type      string          `json:"type"`
			EventType string          `json:"event_type"`
			RequestID string          `json:"request_id"`
			Data      json.RawMessage `json:"data"`
		}
		err := wsjson.Read(connCtx, conn, &frame)
		if err != nil {
			connCancel()
			wg.Wait()
			if tokenChanged.Load() {
				return true, errTokenChanged
			}
			if connCtx.Err() == nil {
				a.Log.Debug("guard: websocket disconnected", "agent", agentName, "err", err)
			}
			return true, err
		}

		switch frame.Type {
		case "fs.request":
			reqID := frame.RequestID
			if reqID == "" && len(frame.Data) > 0 {
				var p struct {
					RequestID string `json:"request_id"`
				}
				_ = json.Unmarshal(frame.Data, &p)
				reqID = p.RequestID
			}
			if reqID != "" {
				go a.serveFsRequest(connCtx, reqID)
			}
		case "event":
			if frame.EventType == "message.retracted" {
				var p struct {
					MessageID string `json:"message_id"`
				}
				_ = json.Unmarshal(frame.Data, &p)
				if p.MessageID != "" {
					a.OnMessageRetracted(p.MessageID)
				}
			}
			a.onChannelEvent(frame.EventType, frame.Data)
			if frame.EventType == "file.fetch_requested" {
				var p struct {
					AgentID string `json:"agent_id"`
				}
				_ = json.Unmarshal(frame.Data, &p)
				me := a.getMe()
				if p.AgentID == "" || me.ID == "" || p.AgentID == me.ID {
					if fileFetchWakeCh != nil {
						select {
						case fileFetchWakeCh <- struct{}{}:
						default:
						}
					}
				}
			} else {
				select {
				case wakeCh <- struct{}{}:
				default:
				}
			}
		case "resync_required":
			select {
			case wakeCh <- struct{}{}:
			default:
			}
			if fileFetchWakeCh != nil {
				select {
				case fileFetchWakeCh <- struct{}{}:
				default:
				}
			}
		case "welcome", "replay_done":
			// Ignored as per spec
		default:
			// Ignored
		}
	}
}

// onChannelEvent stops work in a channel the agent was removed from, or
// that was archived (round 34).
func (a *Agent) onChannelEvent(eventType string, data json.RawMessage) {
	if eventType == "workspace.storage_state_changed" {
		if a.OnStorageEvent != nil {
			a.OnStorageEvent()
		}
		return
	}
	if eventType != "channel.member_removed" && eventType != "channel.archived" && eventType != "channel.locked" {
		return
	}
	var p struct {
		ChannelID string `json:"channel_id"`
		ActorID   string `json:"actor_id"`
	}
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if eventType == "channel.archived" {
		a.OnChannelLeft(p.ChannelID, "archived")
	} else if eventType == "channel.locked" {
		a.OnChannelLeft(p.ChannelID, "locked")
	} else if me := a.getMe(); me.ID != "" && p.ActorID == me.ID {
		a.OnChannelLeft(p.ChannelID, "removed")
	}
}
