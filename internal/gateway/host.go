// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// HostAuthenticator resolves a guard's host token (hosts.Service).
type HostAuthenticator interface {
	AuthenticateHost(ctx context.Context, secret string) (hosts.Host, error)
}

// HostFsRequestFrame wakes a guard for a pending host fs request.
type HostFsRequestFrame struct {
	Type      string `json:"type"`
	RequestID string `json:"request_id"`
}

// HostGateway serves /ws/host: the guard's own connection, authenticated
// with its host token (not an agent token). Protocol:
//
//	guard  → {"type":"hello","token":"colh_…"}
//	server → {"type":"welcome","protocol":1,"host_id":"…"}
//	server → {"type":"host.fs.request","request_id":"…"}
//
// It carries nothing else: no events and no agent frames, so a host token
// never sees chat content. The guard sends nothing after the hello; reading
// only notices the close.
type HostGateway struct {
	Auth   HostAuthenticator
	Logger *slog.Logger

	mu    sync.Mutex
	conns map[*hostConn]struct{}
}

// Connections counts the open guard connections.
func (g *HostGateway) Connections() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.conns)
}

type hostConn struct {
	ws          *websocket.Conn
	hostID      ids.ID
	workspaceID ids.ID
	writeMu     sync.Mutex
}

// ServeHTTP upgrades the request and holds the connection until it closes.
func (g *HostGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// No origin patterns: guards are not browsers, and a browser page on
	// another origin must not open this socket.
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	ctx := r.Context()

	helloCtx, cancel := context.WithTimeout(ctx, helloTimeout)
	var h hello
	err = wsjson.Read(helloCtx, c, &h)
	cancel()
	if err != nil || h.Type != "hello" || h.Token == "" {
		_ = c.Close(websocket.StatusPolicyViolation, "expected hello")
		return
	}
	host, err := g.Auth.AuthenticateHost(ctx, h.Token)
	if err != nil {
		_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
	ctx = db.WithWorkspace(ctx, host.WorkspaceID)
	conn := &hostConn{ws: c, hostID: host.ID, workspaceID: host.WorkspaceID}
	if err := conn.write(ctx, map[string]any{"type": "welcome", "protocol": Protocol, "host_id": host.ID.String()}); err != nil {
		return
	}
	g.mu.Lock()
	if g.conns == nil {
		g.conns = map[*hostConn]struct{}{}
	}
	g.conns[conn] = struct{}{}
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.conns, conn)
		g.mu.Unlock()
	}()
	g.logger().Debug("host websocket connected", "host", host.Name)

	// CloseRead discards anything the guard sends and answers pings.
	<-c.CloseRead(ctx).Done()
}

// CloseWorkspace closes every guard connection of a workspace (round 51:
// the operator suspended it). It returns how many it closed.
func (g *HostGateway) CloseWorkspace(workspaceID ids.ID) int {
	g.mu.Lock()
	var list []*hostConn
	for c := range g.conns {
		if c.workspaceID == workspaceID {
			list = append(list, c)
		}
	}
	g.mu.Unlock()
	for _, c := range list {
		go func() { _ = c.ws.Close(websocket.StatusPolicyViolation, "workspace suspended") }()
	}
	return len(list)
}

// Connected reports whether the host holds at least one connection.
func (g *HostGateway) Connected(hostID ids.ID) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for c := range g.conns {
		if c.hostID == hostID {
			return true
		}
	}
	return false
}

// PushHostLoginRequest sends a host.claude_login.request frame (round 38)
// to the host's connections only; the frame carries only the request id.
func (g *HostGateway) PushHostLoginRequest(hostID ids.ID, requestID string) bool {
	return g.push(hostID, HostFsRequestFrame{Type: "host.claude_login.request", RequestID: requestID})
}

// PushHostFsRequest sends a host.fs.request frame to the host's
// connections only. It reports whether any connection took it.
func (g *HostGateway) PushHostFsRequest(hostID ids.ID, requestID string) bool {
	return g.push(hostID, HostFsRequestFrame{Type: "host.fs.request", RequestID: requestID})
}

func (g *HostGateway) push(hostID ids.ID, frame HostFsRequestFrame) bool {
	g.mu.Lock()
	var recipients []*hostConn
	for c := range g.conns {
		if c.hostID == hostID {
			recipients = append(recipients, c)
		}
	}
	g.mu.Unlock()

	pushed := false
	for _, c := range recipients {
		if err := c.write(context.Background(), frame); err == nil {
			pushed = true
		}
	}
	return pushed
}

func (c *hostConn) write(ctx context.Context, v any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsjson.Write(ctx, c.ws, v)
}

func (g *HostGateway) logger() *slog.Logger {
	if g.Logger != nil {
		return g.Logger
	}
	return slog.Default()
}
