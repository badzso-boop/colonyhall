// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/extrunner"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/secrets"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Endpoint is where a connector's request goes, with its authentication
// headers already set (they hold the secret: never log them).
type Endpoint struct {
	URL    string
	Header map[string]string
}

// Result is a tool's answer.
type Result struct {
	Text    string
	IsError bool
}

// Remote talks to external MCP servers (adapter: mcpremote).
type Remote interface {
	ListTools(ctx context.Context, ep Endpoint) ([]ToolInfo, error)
	CallTool(ctx context.Context, ep Endpoint, name string, args json.RawMessage) (Result, error)
}

// Approvals asks a human before a write or external call (the approval
// module).
type Approvals interface {
	CreateRequest(ctx context.Context, by identity.Actor, in approval.CreateInput) (approval.ApprovalRequest, error)
	WaitForDecision(ctx context.Context, req approval.ApprovalRequest, current func(ctx context.Context) (identity.Actor, error)) (approval.ApprovalRequest, error)
}

// Service holds the Workshop's use cases.
type Service struct {
	Store     Store
	Secrets   *secrets.Service
	Remote    Remote
	Approvals Approvals
	Directory interface {
		ActorByID(ctx context.Context, id ids.ID) (identity.Actor, error)
	}
	Events eventlog.Store
	Bus    bus.Bus
	Tx     db.TxRunner
	IDs    ids.Generator
	Clock  clock.Clock
	Logger *slog.Logger

	// Round 56b: the extensions. Engine runs code tools (nil: no runner);
	// Compiler builds them; Fetcher is their outbound HTTP; Bots creates
	// their bots; Core calls Colonyhall's own tools; Budget is the plan's
	// monthly call budget; Limits are COLONY_EXT_*.
	Engine   extrunner.Engine
	Compiler Compiler
	Fetcher  Fetcher
	Bots     Bots
	Core     CoreTool
	Budget   CallBudget
	Limits   Limits
	// Channels: the routines' posts and the bots' channels (round 56c).
	Channels Channels

	slotsMu sync.Mutex
	slots   map[ids.ID]chan struct{}
}

func admin(by identity.Actor) error {
	if by.Kind != identity.KindHuman || !by.IsAdmin() || by.Halted {
		return ErrForbidden
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (s *Service) emit(ctx context.Context, by identity.Actor, typ, aggregate string, id ids.ID, payload any) (int64, error) {
	ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, ids.Nil, aggregate, id, payload, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}

func (s *Service) notify(seq int64) {
	if s.Bus != nil && seq > 0 {
		s.Bus.Notify(seq)
	}
}

// ConnectorInput creates or changes a connector. On update, nil fields stay.
type ConnectorInput struct {
	Slug       string
	Title      *string
	Kind       string
	BaseURL    *string
	Auth       *string
	HeaderName *string
	// Secret: a new value (stored encrypted); never returned.
	Secret *string
}

func secretKind(auth string) string {
	switch auth {
	case AuthHeader:
		return secrets.KindConnectorHeader
	case AuthBasic:
		return secrets.KindConnectorBasic
	}
	return secrets.KindConnectorToken
}

func (c *Connector) apply(in ConnectorInput) error {
	if in.Title != nil {
		c.Title = strings.TrimSpace(*in.Title)
	}
	if in.BaseURL != nil {
		c.BaseURL = strings.TrimSpace(*in.BaseURL)
	}
	if in.Auth != nil {
		c.Auth = *in.Auth
	}
	if in.HeaderName != nil {
		c.HeaderName = strings.TrimSpace(*in.HeaderName)
	}
	if c.Title == "" || utf8.RuneCountInString(c.Title) > 80 {
		return ErrBadTitle
	}
	if err := checkBaseURL(c.BaseURL); err != nil {
		return err
	}
	if !validAuth(c.Auth) {
		return ErrBadAuth
	}
	if c.Auth == AuthHeader {
		if !headerRe.MatchString(c.HeaderName) || strings.EqualFold(c.HeaderName, "host") {
			return ErrBadHeader
		}
	} else {
		c.HeaderName = ""
	}
	return nil
}

// CreateConnector adds a connector (and its secret) and, for an MCP
// server, fetches its tools. A failed fetch keeps the connector in the
// error state; it can be refreshed later.
func (s *Service) CreateConnector(ctx context.Context, by identity.Actor, in ConnectorInput) (Connector, error) {
	if err := admin(by); err != nil {
		return Connector{}, err
	}
	if !ValidSlug(in.Slug) {
		return Connector{}, ErrBadSlug
	}
	if in.Kind != KindMCP && in.Kind != KindHTTP {
		return Connector{}, ErrBadKind
	}
	now := s.Clock.Now()
	c := Connector{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, Slug: in.Slug, Kind: in.Kind, Auth: AuthNone,
		RiskOverrides: map[string]string{}, State: "new", CreatedBy: by.ID, CreatedAt: now, UpdatedAt: now}
	if err := c.apply(in); err != nil {
		return Connector{}, err
	}
	hasSecret := in.Secret != nil && *in.Secret != ""
	if c.Auth != AuthNone && !hasSecret {
		return Connector{}, ErrSecretNeeded
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if hasSecret && c.Auth != AuthNone {
			sec, err := s.Secrets.Create(ctx, by, secretKind(c.Auth), "connector "+c.Slug, *in.Secret)
			if err != nil {
				return err
			}
			c.SecretID = sec.ID
		}
		if err := s.Store.InsertConnector(ctx, c); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrSlugTaken
			}
			return err
		}
		var err error
		seq, err = s.emit(ctx, by, EventConnectorCreated, "connector", c.ID, ConnectorChanged{ConnectorID: c.ID.String(), Slug: c.Slug, Kind: c.Kind})
		return err
	})
	if err != nil {
		return Connector{}, err
	}
	s.notify(seq)
	if c.Kind == KindMCP {
		if fresh, err := s.refresh(ctx, c); err == nil {
			c = fresh
		}
	}
	return c, nil
}

// UpdateConnector changes a connector's title, address, authentication or
// secret. A changed address or secret fetches the tools again.
func (s *Service) UpdateConnector(ctx context.Context, by identity.Actor, id ids.ID, in ConnectorInput) (Connector, error) {
	if err := admin(by); err != nil {
		return Connector{}, err
	}
	var c Connector
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		c, err = s.Store.GetConnector(ctx, by.WorkspaceID, id)
		if err != nil {
			return notFound(err)
		}
		oldAuth, oldURL := c.Auth, c.BaseURL
		if err := c.apply(in); err != nil {
			return err
		}
		newSecret := in.Secret != nil && *in.Secret != ""
		// The stored secret was given for one server: a new origin
		// (scheme and host) needs it again, so an address change never
		// sends it somewhere else.
		if c.SecretID != ids.Nil && c.Auth != AuthNone && !newSecret && origin(oldURL) != origin(c.BaseURL) {
			return ErrSecretAgain
		}
		switch {
		case c.Auth == AuthNone:
			if c.SecretID != ids.Nil {
				if err := s.Secrets.Delete(ctx, by, c.SecretID); err != nil && !errors.Is(err, secrets.ErrNotFound) {
					return err
				}
				c.SecretID = ids.Nil
			}
		case newSecret && c.SecretID != ids.Nil && oldAuth == c.Auth:
			if _, err := s.Secrets.Replace(ctx, by, c.SecretID, *in.Secret); err != nil {
				return err
			}
		case newSecret:
			if c.SecretID != ids.Nil {
				if err := s.Secrets.Delete(ctx, by, c.SecretID); err != nil && !errors.Is(err, secrets.ErrNotFound) {
					return err
				}
			}
			sec, err := s.Secrets.Create(ctx, by, secretKind(c.Auth), "connector "+c.Slug, *in.Secret)
			if err != nil {
				return err
			}
			c.SecretID = sec.ID
		case c.SecretID == ids.Nil || oldAuth != c.Auth:
			return ErrSecretNeeded
		}
		c.UpdatedAt = s.Clock.Now()
		if err := s.Store.UpdateConnector(ctx, c); err != nil {
			return notFound(err)
		}
		seq, err = s.emit(ctx, by, EventConnectorUpdated, "connector", c.ID, ConnectorChanged{ConnectorID: c.ID.String(), Slug: c.Slug})
		return err
	})
	if err != nil {
		return Connector{}, err
	}
	s.notify(seq)
	if c.Kind == KindMCP && (in.BaseURL != nil || in.Secret != nil || in.Auth != nil) {
		if fresh, err := s.refresh(ctx, c); err == nil {
			c = fresh
		}
	}
	return c, nil
}

// SetApprovalOff switches the approval of write and external calls off
// (or on again) for one connector. Only the owner (decision, 2026-11-17).
func (s *Service) SetApprovalOff(ctx context.Context, by identity.Actor, id ids.ID, off bool) (Connector, error) {
	if by.Kind != identity.KindHuman || by.Role != identity.RoleOwner || by.Halted {
		return Connector{}, ErrForbidden
	}
	var c Connector
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if c, err = s.Store.GetConnector(ctx, by.WorkspaceID, id); err != nil {
			return notFound(err)
		}
		if c.ApprovalOff == off {
			return nil
		}
		c.ApprovalOff, c.UpdatedAt = off, s.Clock.Now()
		if err := s.Store.UpdateConnector(ctx, c); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventApprovalOffChanged, "connector", c.ID, ConnectorChanged{ConnectorID: c.ID.String(), Slug: c.Slug, ApprovalOff: &off})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return c, err
}

func riskRank(r string) int {
	switch r {
	case RiskRead:
		return 0
	case RiskWrite:
		return 1
	}
	return 2
}

// SetRisk overrides a tool's risk ("" removes the override). Raising a
// risk is for the owner and the admins; lowering it skips the approval
// like switching the approval off, so it is the owner's alone.
func (s *Service) SetRisk(ctx context.Context, by identity.Actor, id ids.ID, tool, risk string) (Connector, error) {
	if err := admin(by); err != nil {
		return Connector{}, err
	}
	if risk != "" && !validRisk(risk) {
		return Connector{}, ErrBadRisk
	}
	var c Connector
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if c, err = s.Store.GetConnector(ctx, by.WorkspaceID, id); err != nil {
			return notFound(err)
		}
		info, ok := c.Tool(tool)
		if !ok {
			return ErrUnknownTool
		}
		before := c.EffectiveRisk(info)
		after := risk
		if after == "" {
			after = info.Risk
			if after == "" {
				after = RiskWrite
			}
		}
		if riskRank(after) < riskRank(before) && by.Role != identity.RoleOwner {
			return ErrOwnerOnly
		}
		if risk == "" {
			delete(c.RiskOverrides, tool)
		} else {
			c.RiskOverrides[tool] = risk
		}
		c.UpdatedAt = s.Clock.Now()
		if err := s.Store.UpdateConnector(ctx, c); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventRiskOverridden, "connector", c.ID, ConnectorChanged{ConnectorID: c.ID.String(), Slug: c.Slug, Tool: tool, Risk: risk})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return c, err
}

// DeleteConnector removes a connector, its secret and every grant to it.
func (s *Service) DeleteConnector(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.GetConnector(ctx, by.WorkspaceID, id)
		if err != nil {
			return notFound(err)
		}
		now := s.Clock.Now()
		if c.SecretID != ids.Nil {
			if err := s.Secrets.Delete(ctx, by, c.SecretID); err != nil && !errors.Is(err, secrets.ErrNotFound) {
				return err
			}
		}
		if err := s.Store.RevokeGrantsForTarget(ctx, by.WorkspaceID, TargetConnector, c.ID, now); err != nil {
			return err
		}
		if err := s.Store.DeleteConnector(ctx, by.WorkspaceID, c.ID, now); err != nil {
			return err
		}
		seq, err = s.emit(ctx, by, EventConnectorDeleted, "connector", c.ID, ConnectorChanged{ConnectorID: c.ID.String(), Slug: c.Slug})
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return err
}

// ConnectorView is a connector as the Workshop shows it: the secret only
// by its hint.
type ConnectorView struct {
	Connector
	SecretHint string
	HasSecret  bool
}

// ListConnectors returns the workspace's connectors (owner and admins).
func (s *Service) ListConnectors(ctx context.Context, by identity.Actor) ([]ConnectorView, error) {
	if err := admin(by); err != nil {
		return nil, err
	}
	cs, err := s.Store.ListConnectors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorView, len(cs))
	for i, c := range cs {
		out[i] = s.view(ctx, c)
	}
	return out, nil
}

// GetConnector returns one connector (owner and admins).
func (s *Service) GetConnector(ctx context.Context, by identity.Actor, id ids.ID) (ConnectorView, error) {
	if err := admin(by); err != nil {
		return ConnectorView{}, err
	}
	c, err := s.Store.GetConnector(ctx, by.WorkspaceID, id)
	if err != nil {
		return ConnectorView{}, notFound(err)
	}
	return s.view(ctx, c), nil
}

func (s *Service) view(ctx context.Context, c Connector) ConnectorView {
	v := ConnectorView{Connector: c}
	if c.SecretID != ids.Nil {
		v.HasSecret = true
		if info, err := s.Secrets.Info(ctx, c.WorkspaceID, c.SecretID); err == nil {
			v.SecretHint = info.Hint
		}
	}
	return v
}

// RefreshTools fetches an MCP connector's tool list again.
func (s *Service) RefreshTools(ctx context.Context, by identity.Actor, id ids.ID) (ConnectorView, error) {
	if err := admin(by); err != nil {
		return ConnectorView{}, err
	}
	c, err := s.Store.GetConnector(ctx, by.WorkspaceID, id)
	if err != nil {
		return ConnectorView{}, notFound(err)
	}
	if c.Kind != KindMCP {
		return s.view(ctx, c), nil
	}
	c, err = s.refresh(ctx, c)
	if err != nil {
		return ConnectorView{}, err
	}
	return s.view(ctx, c), nil
}

// refresh fetches the tools and stores them with the state. Tools whose
// names do not fit the gateway are left out.
func (s *Service) refresh(ctx context.Context, c Connector) (Connector, error) {
	ep, err := s.endpoint(ctx, c)
	if err != nil {
		return c, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	now := s.Clock.Now()
	tools, rerr := s.Remote.ListTools(cctx, ep)
	state, lastErr := "ok", ""
	if rerr != nil {
		state, lastErr, tools = "error", errText(rerr), c.Tools
	} else {
		kept := tools[:0]
		for _, t := range tools {
			if ValidToolName(t.Name) && validRisk(t.Risk) {
				kept = append(kept, t)
			}
		}
		tools = kept
	}
	if err := s.Store.SetConnectorTools(ctx, c.WorkspaceID, c.ID, tools, now, state, lastErr); err != nil {
		return c, err
	}
	c.Tools, c.ToolsCachedAt, c.State, c.LastError = tools, &now, state, lastErr
	return c, nil
}

func errText(err error) string {
	msg := err.Error()
	if utf8.RuneCountInString(msg) > 300 {
		msg = string([]rune(msg)[:300])
	}
	return msg
}

// endpoint builds the request target with the authentication header.
func (s *Service) endpoint(ctx context.Context, c Connector) (Endpoint, error) {
	ep := Endpoint{URL: c.BaseURL, Header: map[string]string{}}
	if c.Auth == AuthNone {
		return ep, nil
	}
	if c.SecretID == ids.Nil {
		return ep, ErrSecretNeeded
	}
	v, err := s.Secrets.Reveal(ctx, c.WorkspaceID, c.SecretID)
	if err != nil {
		return ep, err
	}
	switch c.Auth {
	case AuthBearer:
		ep.Header["Authorization"] = "Bearer " + v
	case AuthHeader:
		ep.Header[c.HeaderName] = v
	case AuthBasic:
		ep.Header["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(v))
	}
	return ep, nil
}

// GrantView is a grant with the names the Workshop shows.
type GrantView struct {
	Grant
	AgentHandle string
	TargetName  string
}

// AddGrant lets an agent use a connector (or the Workshop's meta-tools).
func (s *Service) AddGrant(ctx context.Context, by identity.Actor, agentID ids.ID, kind string, targetID ids.ID) (Grant, error) {
	if err := admin(by); err != nil {
		return Grant{}, err
	}
	agent, err := s.Directory.ActorByID(ctx, agentID)
	if err != nil || agent.WorkspaceID != by.WorkspaceID {
		return Grant{}, ErrNotFound
	}
	if agent.Kind != identity.KindAgent {
		return Grant{}, ErrNotAgent
	}
	switch kind {
	case TargetConnector:
		if _, err := s.Store.GetConnector(ctx, by.WorkspaceID, targetID); err != nil {
			return Grant{}, notFound(err)
		}
	case TargetExtension:
		if _, err := s.Store.GetExtension(ctx, by.WorkspaceID, targetID); err != nil {
			return Grant{}, notFound(err)
		}
	case TargetWorkshop:
		targetID = ids.Nil
	default:
		return Grant{}, ErrNotFound
	}
	g := Grant{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, AgentID: agentID, TargetKind: kind, TargetID: targetID,
		GrantedBy: by.ID, CreatedAt: s.Clock.Now()}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.InsertGrant(ctx, g); err != nil {
			return err
		}
		var err error
		seq, err = s.emit(ctx, by, EventGrantAdded, "grant", g.ID, grantPayload(g))
		return err
	})
	if errors.Is(err, db.ErrConflict) {
		return s.existingGrant(ctx, g)
	}
	if err != nil {
		return Grant{}, err
	}
	s.notify(seq)
	return g, nil
}

func (s *Service) existingGrant(ctx context.Context, g Grant) (Grant, error) {
	gs, err := s.Store.GrantsForAgent(ctx, g.WorkspaceID, g.AgentID)
	if err != nil {
		return Grant{}, err
	}
	for _, x := range gs {
		if x.TargetKind == g.TargetKind && x.TargetID == g.TargetID {
			return x, nil
		}
	}
	return Grant{}, db.ErrConflict
}

func grantPayload(g Grant) GrantChanged {
	p := GrantChanged{GrantID: g.ID.String(), AgentID: g.AgentID.String(), TargetKind: g.TargetKind}
	if g.TargetID != ids.Nil {
		p.TargetID = g.TargetID.String()
	}
	return p
}

// RevokeGrant takes a grant back.
func (s *Service) RevokeGrant(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		gs, err := s.Store.ListGrants(ctx, by.WorkspaceID)
		if err != nil {
			return err
		}
		var g *Grant
		for i := range gs {
			if gs[i].ID == id {
				g = &gs[i]
			}
		}
		if g == nil {
			return ErrNotFound
		}
		if err := s.Store.RevokeGrant(ctx, by.WorkspaceID, id, s.Clock.Now()); err != nil {
			return notFound(err)
		}
		seq, err = s.emit(ctx, by, EventGrantRevoked, "grant", id, grantPayload(*g))
		return err
	})
	if err == nil {
		s.notify(seq)
	}
	return err
}

// ListGrants returns the live grants with names (owner and admins).
func (s *Service) ListGrants(ctx context.Context, by identity.Actor) ([]GrantView, error) {
	if err := admin(by); err != nil {
		return nil, err
	}
	gs, err := s.Store.ListGrants(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	cs, err := s.Store.ListConnectors(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	names := map[ids.ID]string{}
	for _, c := range cs {
		names[c.ID] = c.Slug
	}
	es, err := s.Store.ListExtensions(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	for _, e := range es {
		names[e.ID] = e.Slug
	}
	out := make([]GrantView, 0, len(gs))
	for _, g := range gs {
		v := GrantView{Grant: g, TargetName: names[g.TargetID]}
		if g.TargetKind == TargetWorkshop {
			v.TargetName = "workshop"
		}
		if a, err := s.Directory.ActorByID(ctx, g.AgentID); err == nil {
			v.AgentHandle = a.Handle
		}
		out = append(out, v)
	}
	return out, nil
}

// ListCalls returns the call log, newest first (owner and admins).
func (s *Service) ListCalls(ctx context.Context, by identity.Actor, targetID *ids.ID, before *time.Time, limit int) ([]ToolCall, error) {
	if err := admin(by); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.Store.ListToolCalls(ctx, by.WorkspaceID, targetID, before, limit)
}

// GatewayTool is one tool an agent sees in the gateway.
type GatewayTool struct {
	Name        string // <slug>__<tool>
	Description string
	InputSchema json.RawMessage
	Risk        string
}

// ToolsFor returns the connector tools the agent may use: the live
// connectors it has a grant for, with their cached tools.
func (s *Service) ToolsFor(ctx context.Context, agent identity.Actor) ([]GatewayTool, error) {
	if agent.Kind != identity.KindAgent {
		return nil, nil
	}
	gs, err := s.Store.GrantsForAgent(ctx, agent.WorkspaceID, agent.ID)
	if err != nil || len(gs) == 0 {
		return nil, err
	}
	var out []GatewayTool
	for _, g := range gs {
		if g.TargetKind == TargetExtension {
			out = append(out, s.extensionTools(ctx, agent, g.TargetID)...)
			continue
		}
		if g.TargetKind != TargetConnector {
			continue
		}
		c, err := s.Store.GetConnector(ctx, agent.WorkspaceID, g.TargetID)
		if err != nil {
			continue
		}
		if c.Kind == KindMCP && c.ToolsCachedAt == nil {
			if fresh, err := s.refresh(ctx, c); err == nil {
				c = fresh
			}
		}
		for _, t := range c.Tools {
			desc := t.Description
			risk := c.EffectiveRisk(t)
			if risk != RiskRead && !c.ApprovalOff {
				desc = strings.TrimSpace(desc + " (Needs a human's approval: the call waits for it.)")
			}
			out = append(out, GatewayTool{Name: GatewayName(c.Slug, t.Name), Description: fmt.Sprintf("[%s] %s", c.Title, desc),
				InputSchema: t.InputSchema, Risk: risk})
		}
	}
	return out, nil
}

// CallInput is one gateway call.
type CallInput struct {
	Name string // <slug>__<tool>
	Args json.RawMessage
	// ChannelID and ThreadRootID: the conversation of the agent's run (the
	// guard's X-Colonyhall-Channel / -Thread headers), where an approval
	// card goes.
	ChannelID    ids.ID
	ThreadRootID ids.ID
	// Current returns the caller's up-to-date state (emergency stop, pause).
	Current func(ctx context.Context) (identity.Actor, error)
}

// Call runs one connector tool for an agent: grant, risk, approval, the
// call itself with a time limit, and the call log.
func (s *Service) Call(ctx context.Context, in CallInput) (Result, error) {
	caller, err := in.Current(ctx)
	if err != nil {
		return Result{}, err
	}
	if caller.Halted || caller.State != identity.StateActive {
		return Result{}, identity.ErrHalted
	}
	if strings.HasPrefix(in.Name, GatewayPrefix) {
		return s.callExtension(ctx, caller, in)
	}
	slug, tool, ok := SplitGatewayName(in.Name)
	if !ok {
		return Result{}, ErrUnknownTool
	}
	release, err := s.acquire(caller.WorkspaceID)
	if err != nil {
		return Result{}, err
	}
	defer release()
	if err := s.checkBudget(ctx, caller.WorkspaceID); err != nil {
		return Result{}, err
	}
	c, err := s.connectorBySlug(ctx, caller.WorkspaceID, slug)
	if err != nil {
		return Result{}, ErrUnknownTool
	}
	if !s.granted(ctx, caller, TargetConnector, c.ID) {
		return Result{}, ErrNoGrant
	}
	info, ok := c.Tool(tool)
	if !ok {
		return Result{}, ErrUnknownTool
	}
	risk := c.EffectiveRisk(info)
	start := s.Clock.Now()
	call := ToolCall{ID: s.IDs.New(), WorkspaceID: caller.WorkspaceID, ActorID: caller.ID, TargetKind: TargetConnector,
		TargetID: c.ID, Tool: tool, Risk: risk, StartedAt: start, InBytes: len(in.Args)}
	logCall := func(outcome, msg string, out int) {
		call.Outcome, call.Error, call.OutBytes = outcome, msg, out
		call.DurationMS = int(s.Clock.Now().Sub(start) / time.Millisecond)
		if call.DurationMS < 0 {
			call.DurationMS = 0
		}
		if err := s.Store.InsertToolCall(ctx, call); err != nil && s.Logger != nil {
			s.Logger.Warn("workshop: call log", "err", err)
		}
	}

	if risk != RiskRead && !c.ApprovalOff {
		if in.ChannelID == ids.Nil {
			logCall(OutcomeDenied, "no conversation for the approval", 0)
			return Result{}, ErrNoChannel
		}
		req, err := s.Approvals.CreateRequest(ctx, caller, approval.CreateInput{ChannelID: in.ChannelID,
			ThreadRootID: in.ThreadRootID, ToolName: in.Name, Input: string(in.Args)})
		if err != nil {
			logCall(OutcomeDenied, errText(err), 0)
			return Result{}, err
		}
		call.ApprovalID = req.ID
		decided, err := s.Approvals.WaitForDecision(ctx, req, in.Current)
		if err != nil {
			logCall(OutcomeDenied, errText(err), 0)
			return Result{}, err
		}
		if decided.State != approval.StateAllowed {
			logCall(OutcomeDenied, decided.State, 0)
			return Result{Text: "The call was not approved (" + decided.State + "). Do not retry it unless a person asks you to.", IsError: true}, nil
		}
		// Re-check after the wait: the grant and the connector may have
		// gone, the agent may have been stopped (security review).
		if _, err := s.Store.GetConnector(ctx, caller.WorkspaceID, c.ID); err != nil || !s.granted(ctx, caller, TargetConnector, c.ID) {
			logCall(OutcomeDenied, "changed while waiting for the approval", 0)
			return Result{}, ErrNoGrant
		}
		if fresh, err := in.Current(ctx); err != nil || fresh.Halted || fresh.State != identity.StateActive {
			logCall(OutcomeDenied, "halted while waiting for the approval", 0)
			return Result{}, identity.ErrHalted
		}
	}

	ep, err := s.endpoint(ctx, c)
	if err != nil {
		logCall(OutcomeError, errText(err), 0)
		return Result{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	res, err := s.Remote.CallTool(cctx, ep, tool, in.Args)
	if err != nil {
		outcome := OutcomeError
		if errors.Is(err, context.DeadlineExceeded) {
			outcome = OutcomeTimeout
		}
		logCall(outcome, errText(err), 0)
		return Result{}, fmt.Errorf("%s: %s", in.Name, errText(err))
	}
	size := len(res.Text)
	res = cutResult(res)
	outcome := OutcomeOK
	if res.IsError {
		outcome = OutcomeError
	}
	logCall(outcome, "", size)
	return res, nil
}

func (s *Service) connectorBySlug(ctx context.Context, workspaceID ids.ID, slug string) (Connector, error) {
	cs, err := s.Store.ListConnectors(ctx, workspaceID)
	if err != nil {
		return Connector{}, err
	}
	for _, c := range cs {
		if c.Slug == slug {
			return c, nil
		}
	}
	return Connector{}, ErrNotFound
}

func (s *Service) granted(ctx context.Context, agent identity.Actor, kind string, targetID ids.ID) bool {
	gs, err := s.Store.GrantsForAgent(ctx, agent.WorkspaceID, agent.ID)
	if err != nil {
		return false
	}
	for _, g := range gs {
		if g.TargetKind == kind && g.TargetID == targetID {
			return true
		}
	}
	return false
}

// origin returns scheme://host of a URL ("" if it does not parse).
func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// cutResult keeps a tool's answer within what the model gets.
func cutResult(res Result) Result {
	size := len(res.Text)
	if size <= MaxResult {
		return res
	}
	cut := MaxResult
	for cut > 0 && !utf8.RuneStart(res.Text[cut]) {
		cut--
	}
	res.Text = res.Text[:cut] + fmt.Sprintf("\n… +%d bytes cut", size-cut)
	return res
}
