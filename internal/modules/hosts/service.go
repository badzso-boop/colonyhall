package hosts

import (
	"colonyhall/internal/apispec"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/palette"
	"colonyhall/internal/platform/tenant"
)

// PreviewProvider supplies previews for a host config.
type PreviewProvider interface {
	HostPreviews(ctx context.Context, hostID ids.ID) ([]HostPreview, error)
}

// Service holds the hosts use cases.
type Service struct {
	Store     Store
	Directory identity.Directory
	Tokens    identity.HostTokens
	Events    eventlog.Store
	Bus       bus.Bus
	Tx        db.TxRunner
	IDs       ids.Generator
	Clock     clock.Clock
	Previews  PreviewProvider
}

// admin checks that by may manage hosts: a human admin who is not halted.
func admin(by identity.Actor) error {
	if by.Kind != identity.KindHuman || !by.IsAdmin() || by.Halted {
		return ErrForbidden
	}
	return nil
}

// record runs change and appends the event in one transaction.
func (s *Service) record(ctx context.Context, workspaceID, actorID ids.ID, typ string, aggID ids.ID, payload any, change func(ctx context.Context) error) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := change(ctx); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, typ, actorID, ids.Nil, "host", aggID, payload, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err == nil {
		s.Bus.Notify(seq)
	}
	return err
}

// CreatePairingCode returns a one-time code a guard uses to register
// (`colony guard enroll`). It is shown once and expires in PairingTTL.
func (s *Service) CreatePairingCode(ctx context.Context, by identity.Actor) (string, time.Time, error) {
	if err := admin(by); err != nil {
		return "", time.Time{}, err
	}
	code, err := NewPairingCode()
	if err != nil {
		return "", time.Time{}, err
	}
	now := s.Clock.Now()
	pc := PairingCode{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, CodeHash: Hash(NormalizeCode(code)),
		CreatedBy: by.ID, CreatedAt: now, ExpiresAt: now.Add(PairingTTL)}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventPairingCreated, pc.ID, HostEvent{},
		func(ctx context.Context) error { return s.Store.CreatePairingCode(ctx, pc) })
	return code, pc.ExpiresAt, err
}

// Enroll registers a guard with a pairing code and returns the host and
// its token secret (shown once). The admin who created the code approved
// the machine by doing so.
func (s *Service) Enroll(ctx context.Context, code, name string) (Host, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return Host{}, "", ErrBadName
	}
	secret, err := NewHostSecret()
	if err != nil {
		return Host{}, "", err
	}
	// Round 45: the code's workspace first; the host is created there.
	ws, err := s.Store.WorkspaceOfPairingCode(ctx, Hash(NormalizeCode(code)))
	if t, ok := tenant.From(ctx); errors.Is(err, db.ErrNotFound) || ok && err == nil && t.Workspace != ws {
		return Host{}, "", ErrBadCode // another subdomain's code is just a wrong code here
	}
	if err != nil {
		return Host{}, "", err
	}
	ctx = db.WithWorkspace(ctx, ws)
	var h Host
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		pc, err := s.Store.UsePairingCode(ctx, Hash(NormalizeCode(code)), s.Clock.Now())
		if errors.Is(err, db.ErrNotFound) {
			return ErrBadCode
		}
		if err != nil {
			return err
		}
		existing, err := s.Store.ListHosts(ctx, pc.WorkspaceID)
		if err != nil {
			return err
		}
		h = Host{ID: s.IDs.New(), WorkspaceID: pc.WorkspaceID, Name: name, Color: palette.Default(len(existing)), TokenHash: Hash(secret), State: StateApproved,
			CreatedBy: pc.CreatedBy, CreatedAt: s.Clock.Now(), Report: Report{Roots: []string{}, Clients: []Client{}, Rejected: []Rejected{}}}
		if err := s.Store.CreateHost(ctx, h); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), h.WorkspaceID, EventHostEnrolled, pc.CreatedBy, ids.Nil, "host", h.ID,
			HostEvent{HostID: h.ID.String(), Name: h.Name}, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err != nil {
		return Host{}, "", err
	}
	s.Bus.Notify(seq)
	return h, secret, nil
}

// AuthenticateHost resolves a host token to an approved host.
func (s *Service) AuthenticateHost(ctx context.Context, secret string) (Host, error) {
	if !strings.HasPrefix(secret, hostTokenPrefix) {
		return Host{}, ErrBadHostToken
	}
	// Round 45: find the host's workspace first, then read the host there.
	ws, err := s.Store.WorkspaceOfHostToken(ctx, Hash(secret))
	if errors.Is(err, db.ErrNotFound) {
		return Host{}, ErrBadHostToken
	}
	if err != nil {
		return Host{}, err
	}
	ctx = db.WithWorkspace(ctx, ws)
	if t, ok := tenant.From(ctx); ok && t.Workspace != ws {
		s.crossTenant(ctx, t, Hash(secret))
		return Host{}, ErrBadHostToken
	}
	h, err := s.Store.HostByTokenHash(ctx, Hash(secret))
	if errors.Is(err, db.ErrNotFound) || (err == nil && h.State != StateApproved) {
		return Host{}, ErrBadHostToken
	}
	if err != nil {
		return Host{}, err
	}
	_ = s.Store.Touch(ctx, h.ID, s.Clock.Now())
	return h, nil
}

// HostView is a host with its agents, for admins.
type HostView struct {
	Host
	Agents []AgentConfig
}

// ListHosts lists the workspace's hosts with their agents and grants.
// Admins see every machine; another person only the machines they own
// (round 38), with only the agents they own.
func (s *Service) ListHosts(ctx context.Context, by identity.Actor) ([]HostView, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	all := admin(by) == nil
	hs, err := s.Store.ListHosts(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]HostView, 0, len(hs))
	for _, h := range hs {
		if !all && (h.OwnerID == ids.Nil || h.OwnerID != by.ID) {
			continue
		}
		agents, err := s.agentConfigs(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		if !all {
			agents, err = s.ownAgents(ctx, by, agents)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, HostView{Host: h, Agents: agents})
	}
	return out, nil
}

// ownAgents keeps the configurations of the agents by may see.
func (s *Service) ownAgents(ctx context.Context, by identity.Actor, list []AgentConfig) ([]AgentConfig, error) {
	if len(list) == 0 {
		return list, nil
	}
	idList := make([]ids.ID, 0, len(list))
	for _, a := range list {
		idList = append(idList, a.AgentID)
	}
	actors, err := s.Directory.ActorsByIDs(ctx, idList)
	if err != nil {
		return nil, err
	}
	sees := map[ids.ID]bool{}
	for _, a := range actors {
		sees[a.ID] = by.SeesAgent(a)
	}
	out := list[:0:0]
	for _, a := range list {
		if sees[a.AgentID] {
			out = append(out, a)
		}
	}
	return out, nil
}

// SetHostOwner gives the machine to a person, or to nobody (ids.Nil). Only
// the workspace owner may (audited, host.owner_changed).
func (s *Service) SetHostOwner(ctx context.Context, by identity.Actor, hostID, ownerID ids.ID) (Host, error) {
	if by.Kind != identity.KindHuman || by.Role != identity.RoleOwner || by.Halted {
		return Host{}, ErrForbidden
	}
	h, err := s.hostIn(ctx, by.WorkspaceID, hostID)
	if err != nil {
		return Host{}, err
	}
	if ownerID != ids.Nil {
		o, err := s.Directory.ActorByID(ctx, ownerID)
		if errors.Is(err, db.ErrNotFound) || (err == nil && (o.WorkspaceID != by.WorkspaceID || o.Kind != identity.KindHuman || o.ArchivedAt != nil)) {
			return Host{}, identity.ErrInvalidOwner
		}
		if err != nil {
			return Host{}, err
		}
	}
	if h.OwnerID == ownerID {
		return h, nil
	}
	owner := ""
	if ownerID != ids.Nil {
		owner = ownerID.String()
	}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventHostOwner, h.ID, HostEvent{HostID: h.ID.String(), Name: h.Name, Owner: owner},
		func(ctx context.Context) error { return s.Store.SetHostOwner(ctx, h.ID, ownerID) })
	h.OwnerID = ownerID
	return h, err
}

// UpdateHostInput changes a host's name or colour; nil leaves it.
type UpdateHostInput struct {
	Name  *string
	Color *string
}

// UpdateHost renames or recolours a host (operational state, audited;
// round 33). Admins only.
func (s *Service) UpdateHost(ctx context.Context, by identity.Actor, hostID ids.ID, in UpdateHostInput) (Host, error) {
	if err := admin(by); err != nil {
		return Host{}, err
	}
	h, err := s.hostIn(ctx, by.WorkspaceID, hostID)
	if err != nil {
		return Host{}, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || utf8.RuneCountInString(name) > 64 {
			return Host{}, ErrBadName
		}
		h.Name = name
	}
	if in.Color != nil {
		if !palette.Valid(*in.Color) {
			return Host{}, ErrBadColor
		}
		h.Color = *in.Color
	}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventHostUpdated, h.ID, HostEvent{HostID: h.ID.String(), Name: h.Name, Color: h.Color},
		func(ctx context.Context) error { return s.Store.UpdateHost(ctx, h.ID, h.Name, h.Color) })
	return h, err
}

// AgentHostColors maps the workspace's assigned agents to their host's
// colour (an agent without its own colour shows its machine's).
func (s *Service) AgentHostColors(ctx context.Context, workspaceID ids.ID) (map[ids.ID]string, error) {
	return s.Store.AgentHostColors(ctx, workspaceID)
}

// SetHostState approves or revokes a host. Revoking also revokes every
// agent token the host was given.
func (s *Service) SetHostState(ctx context.Context, by identity.Actor, hostID ids.ID, state string) error {
	if err := admin(by); err != nil {
		return err
	}
	if state != StateApproved && state != StateRevoked {
		return ErrBadState
	}
	h, err := s.hostIn(ctx, by.WorkspaceID, hostID)
	if err != nil {
		return err
	}
	return s.record(ctx, by.WorkspaceID, by.ID, EventHostState, h.ID, HostEvent{HostID: h.ID.String(), Name: h.Name, State: state},
		func(ctx context.Context) error {
			if err := s.Store.SetHostState(ctx, h.ID, state); err != nil {
				return err
			}
			if state == StateRevoked {
				return s.Tokens.RevokeHostTokens(ctx, h.ID, ids.Nil)
			}
			return nil
		})
}

func (s *Service) hostIn(ctx context.Context, workspaceID, hostID ids.ID) (Host, error) {
	h, err := s.Store.HostByID(ctx, hostID)
	if err != nil {
		return Host{}, err
	}
	if h.WorkspaceID != workspaceID {
		return Host{}, db.ErrNotFound
	}
	return h, nil
}

// AssignInput configures how an agent runs on a host.
type AssignInput struct {
	HostID           ids.ID
	Client           string
	Model            string
	WorkModel        string
	AllowWork        bool
	PermissionMode   string
	Mode             string
	RootDir          string
	Sandbox          *bool
	Approvals        string
	MaxSessionTokens *int
	DailyTokenLimit  *int64
	WorktreeRepo     string
	AllowPreview     bool
	AllowWebRead     bool
	WriteScope       string
	PartnerID        ids.ID
}

// GrantInput is a folder an agent gets at creation.
type GrantInput struct {
	Path string
	Mode string
}

// ValidateNewAgent checks an agent's machine configuration before anything
// is written (POST /agents, round 33): the host is approved and has
// reported the client; the root folder, the worktree repo and every grant
// lie inside the roots the host reported. It returns the assignment to
// write (without the agent id) and the normalised grants.
func (s *Service) ValidateNewAgent(ctx context.Context, by identity.Actor, in AssignInput, grants []GrantInput) (Assignment, []GrantInput, error) {
	if err := admin(by); err != nil {
		return Assignment{}, nil, err
	}
	if !Clients[in.Client] {
		return Assignment{}, nil, ErrBadClient
	}
	h, err := s.hostIn(ctx, by.WorkspaceID, in.HostID)
	if err != nil {
		return Assignment{}, nil, err
	}
	if h.State != StateApproved {
		return Assignment{}, nil, ErrHostRevoked
	}
	reported := false
	for _, c := range h.Report.Clients {
		if c.Name == in.Client {
			reported = true
		}
	}
	if !reported {
		return Assignment{}, nil, ErrClientNotReported
	}
	if in.Mode == "" {
		in.Mode = AgentModeDirect
	}
	if in.Mode != AgentModeDirect && in.Mode != AgentModeCompact {
		return Assignment{}, nil, ErrBadAgentMode
	}
	sandbox := false
	if in.Sandbox != nil {
		sandbox = *in.Sandbox
	} else if in.Mode == AgentModeCompact {
		sandbox = true
	}
	// A machine whose guard reported its system but no sandbox (macOS, or
	// Linux without bubblewrap) cannot run a sandboxed agent (round 41).
	if sandbox && h.Report.OS != "" && h.Report.Sandbox == "" {
		return Assignment{}, nil, ErrNoSandboxOnHost
	}
	inRoots := func(p string) error {
		if err := ValidatePath(p); err != nil {
			return err
		}
		for _, r := range h.Report.Roots {
			if Within(p, r) {
				return nil
			}
		}
		return ErrOutsideRoots
	}
	if in.WorktreeRepo != "" {
		if (in.Mode == AgentModeDirect && sandbox) || (in.Mode == AgentModeCompact && !sandbox) {
			return Assignment{}, nil, ErrWorktreeDirectOnly
		}
		if err := inRoots(in.WorktreeRepo); err != nil {
			return Assignment{}, nil, err
		}
		if in.RootDir == "" {
			in.RootDir = in.WorktreeRepo
		}
	}
	if in.RootDir != "" {
		if err := inRoots(in.RootDir); err != nil {
			return Assignment{}, nil, err
		}
	}
	if (in.Mode == AgentModeDirect || !sandbox) && in.RootDir == "" {
		return Assignment{}, nil, ErrBadRootDir
	}
	// A preview runs docker compose (image builds run any command): a
	// sandboxed agent, which may not run commands, must not get it.
	if in.AllowPreview && (in.Mode != AgentModeDirect || sandbox) {
		return Assignment{}, nil, ErrPreviewFullAccess
	}
	if in.AllowWebRead && !sandbox {
		return Assignment{}, nil, ErrWebReadSandboxedOnly
	}
	if in.PermissionMode == "" {
		in.PermissionMode = "auto" // claude: --permission-mode auto; antigravity: --mode accept-edits
	}
	if in.WorkModel == "" {
		in.WorkModel = in.Model
	}
	approvals := in.Approvals
	if approvals == "" {
		approvals = ApprovalsNone
	}
	if approvals != ApprovalsNone && approvals != ApprovalsChat {
		return Assignment{}, nil, ErrBadApprovals
	}
	if approvals == ApprovalsChat && in.Client != "claude-code" {
		return Assignment{}, nil, ErrApprovalsChatClient
	}
	maxSessionTokens := 150000
	if in.MaxSessionTokens != nil {
		maxSessionTokens = *in.MaxSessionTokens
	}
	if maxSessionTokens < 0 {
		return Assignment{}, nil, ErrBadMaxSessionTokens
	}
	var dailyTokenLimit int64
	if in.DailyTokenLimit != nil {
		dailyTokenLimit = *in.DailyTokenLimit
	}
	if dailyTokenLimit < 0 {
		return Assignment{}, nil, ErrBadDailyTokenLimit
	}
	writeScope, err := s.validateWriteScope(ctx, by, in, sandbox, approvals)
	if err != nil {
		return Assignment{}, nil, err
	}
	out := make([]GrantInput, 0, len(grants))
	seen := map[string]bool{}
	for _, g := range grants {
		if g.Mode != ModeRead && g.Mode != ModeWrite {
			return Assignment{}, nil, ErrBadMode
		}
		if err := inRoots(g.Path); err != nil {
			return Assignment{}, nil, err
		}
		if seen[g.Path] {
			continue
		}
		seen[g.Path] = true
		out = append(out, g)
	}
	a := Assignment{
		HostID: h.ID, WorkspaceID: by.WorkspaceID, Client: in.Client, Model: clip(in.Model, 100),
		WorkModel: clip(in.WorkModel, 100), AllowWork: in.AllowWork, PermissionMode: clip(in.PermissionMode, 40),
		Mode: in.Mode, RootDir: clip(in.RootDir, 1024), WorktreeRepo: clip(in.WorktreeRepo, 1024), Sandbox: sandbox, Approvals: approvals, AllowPreview: in.AllowPreview,
		AllowWebRead:     in.AllowWebRead,
		MaxSessionTokens: maxSessionTokens, DailyTokenLimit: dailyTokenLimit,
		WriteScope: writeScope, PartnerID: in.PartnerID,
		CreatedBy: by.ID, CreatedAt: s.Clock.Now(),
	}
	return a, out, nil
}

// validateWriteScope checks a docs-only planner (round 38): only a
// full-access Claude Code agent can be one, since the restriction is
// enforced by Claude Code's permission rules on the guard; chat approvals
// would turn a refusal into a question and previews run commands, so
// both are refused. Its partner must be a sandboxed agent of the
// workspace (an archived agent has no assignment).
func (s *Service) validateWriteScope(ctx context.Context, by identity.Actor, in AssignInput, sandbox bool, approvals string) (string, error) {
	scope := in.WriteScope
	if scope == "" {
		scope = WriteScopeAll
	}
	switch scope {
	case WriteScopeAll:
		if in.PartnerID != ids.Nil {
			return "", ErrPartnerDocsOnly
		}
		return scope, nil
	case WriteScopeDocs:
	default:
		return "", ErrBadWriteScope
	}
	if in.Client != "claude-code" || sandbox || in.Mode != AgentModeDirect {
		return "", ErrDocsOnlyClient
	}
	if approvals == ApprovalsChat {
		return "", ErrDocsOnlyApprovals
	}
	if in.AllowPreview {
		return "", ErrDocsOnlyPreview
	}
	if in.PartnerID == ids.Nil {
		return "", ErrPartnerRequired
	}
	pa, err := s.Store.AssignmentByAgent(ctx, in.PartnerID)
	if errors.Is(err, db.ErrNotFound) {
		return "", ErrBadPartner
	}
	if err != nil {
		return "", err
	}
	if pa.WorkspaceID != by.WorkspaceID || !pa.Sandbox {
		return "", ErrBadPartner
	}
	return scope, nil
}

// CreateAgentConfig writes a validated assignment and its grants for a new
// agent (inside the caller's transaction; audited).
func (s *Service) CreateAgentConfig(ctx context.Context, by identity.Actor, agent identity.Actor, a Assignment, grants []GrantInput) (AgentConfig, error) {
	if err := admin(by); err != nil {
		return AgentConfig{}, err
	}
	a.AgentID = agent.ID
	cfg := AgentConfig{Assignment: a, Handle: agent.Handle, Grants: []Grant{}}
	if a.PartnerID != ids.Nil {
		p, err := s.Directory.ActorByID(ctx, a.PartnerID)
		if err != nil {
			return AgentConfig{}, err
		}
		cfg.PartnerHandle = p.Handle
	}
	var partnerID string
	if a.PartnerID != ids.Nil {
		partnerID = a.PartnerID.String()
	}
	err := s.record(ctx, by.WorkspaceID, by.ID, EventAgentAssigned, a.HostID,
		AssignmentEvent{
			AgentID: agent.ID.String(), HostID: a.HostID.String(), Client: a.Client, Model: a.Model,
			AllowWork: a.AllowWork, Mode: a.Mode, RootDir: a.RootDir, WorktreeRepo: a.WorktreeRepo, Sandbox: a.Sandbox, Approvals: a.Approvals,
			MaxSessionTokens: a.MaxSessionTokens, DailyTokenLimit: a.DailyTokenLimit,
			AllowPreview: a.AllowPreview, AllowWebRead: a.AllowWebRead,
			WriteScope: a.WriteScope, PartnerID: partnerID,
		},
		func(ctx context.Context) error { return s.Store.SetAssignment(ctx, a) })
	if err != nil {
		return AgentConfig{}, err
	}
	for _, gi := range grants {
		g := Grant{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, AgentID: agent.ID, HostID: a.HostID, Path: gi.Path, Mode: gi.Mode,
			CreatedBy: by.ID, CreatedAt: s.Clock.Now()}
		err := s.record(ctx, by.WorkspaceID, by.ID, EventGrantAdded, a.HostID,
			GrantEvent{GrantID: g.ID.String(), AgentID: agent.ID.String(), HostID: a.HostID.String(), Path: g.Path, Mode: g.Mode},
			func(ctx context.Context) error { return s.Store.AddGrant(ctx, g) })
		if err != nil {
			return AgentConfig{}, err
		}
		cfg.Grants = append(cfg.Grants, g)
	}
	return cfg, nil
}

// AssignmentByAgent returns an agent's current host assignment, or db.ErrNotFound if not assigned.
func (s *Service) AssignmentByAgent(ctx context.Context, agentID ids.ID) (Assignment, error) {
	return s.Store.AssignmentByAgent(ctx, agentID)
}

// EndAssignment takes an archived agent off its host: its grants and host
// tokens end, and the guard drops it on its next config fetch. No
// assignment (an API-key agent) is not an error.
func (s *Service) EndAssignment(ctx context.Context, by identity.Actor, agentID ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	a, err := s.Store.AssignmentByAgent(ctx, agentID)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	return s.record(ctx, by.WorkspaceID, by.ID, EventAgentUnassigned, a.HostID, AssignmentEvent{AgentID: agentID.String(), HostID: a.HostID.String()},
		func(ctx context.Context) error {
			if err := s.Store.RevokeGrantsForAgent(ctx, agentID, s.Clock.Now()); err != nil {
				return err
			}
			if err := s.Tokens.RevokeHostTokens(ctx, a.HostID, agentID); err != nil {
				return err
			}
			return s.Store.DeleteAssignment(ctx, agentID)
		})
}

// SetLimits changes an agent's limits, the plan's changeable "keretek"
// (round 33): the daily token limit and the session limit. nil leaves one.
func (s *Service) SetLimits(ctx context.Context, by identity.Actor, agentID ids.ID, daily *int64, maxSession *int) (Assignment, error) {
	if err := admin(by); err != nil {
		return Assignment{}, err
	}
	agent, err := s.agent(ctx, by, agentID)
	if err != nil {
		return Assignment{}, err
	}
	if agent.ArchivedAt != nil {
		return Assignment{}, identity.ErrAgentConfigImmutable
	}
	a, err := s.Store.AssignmentByAgent(ctx, agent.ID)
	if errors.Is(err, db.ErrNotFound) {
		return Assignment{}, ErrNotAssigned
	}
	if err != nil {
		return Assignment{}, err
	}
	if daily != nil {
		if *daily < 0 {
			return Assignment{}, ErrBadDailyTokenLimit
		}
		a.DailyTokenLimit = *daily
	}
	if maxSession != nil {
		if *maxSession < 0 {
			return Assignment{}, ErrBadMaxSessionTokens
		}
		a.MaxSessionTokens = *maxSession
	}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventAgentLimits, a.HostID,
		AssignmentEvent{AgentID: agent.ID.String(), HostID: a.HostID.String(), MaxSessionTokens: a.MaxSessionTokens, DailyTokenLimit: a.DailyTokenLimit},
		func(ctx context.Context) error { return s.Store.SetAssignment(ctx, a) })
	return a, err
}

func (s *Service) agent(ctx context.Context, by identity.Actor, agentID ids.ID) (identity.Actor, error) {
	a, err := s.Directory.ActorByID(ctx, agentID)
	if err != nil {
		return identity.Actor{}, err
	}
	if a.WorkspaceID != by.WorkspaceID {
		return identity.Actor{}, db.ErrNotFound
	}
	if a.Kind != identity.KindAgent {
		return identity.Actor{}, ErrNotAgent
	}
	if !by.CanManage(a) {
		return identity.Actor{}, ErrForbidden
	}
	return a, nil
}

// SaveReport stores what the guard says about its machine.
func (s *Service) SaveReport(ctx context.Context, h Host, r Report) error {
	r.GuardVersion = clip(r.GuardVersion, 40)
	r.OS = clip(r.OS, 40)
	r.Sandbox = clip(r.Sandbox, 80)
	if len(r.Doctor) > 40 {
		r.Doctor = r.Doctor[:40]
	}
	for i := range r.Doctor {
		r.Doctor[i].Id = clip(r.Doctor[i].Id, 40)
		if d := r.Doctor[i].Detail; d != nil {
			c := clip(*d, 200)
			r.Doctor[i].Detail = &c
		}
	}
	if cg := r.Codegraph; cg != nil {
		cg.Version = clip(cg.Version, 40)
		if len(cg.Repos) > 50 {
			cg.Repos = cg.Repos[:50]
		}
		if cg.Repos == nil {
			cg.Repos = []apispec.CodegraphRepo{}
		}
		for i := range cg.Repos {
			rp := &cg.Repos[i]
			rp.Path = clip(rp.Path, 1024)
			switch rp.Status {
			case apispec.CodegraphRepoStatusIndexed, apispec.CodegraphRepoStatusFailed, apispec.CodegraphRepoStatusSkipped:
			default:
				rp.Status = apispec.CodegraphRepoStatusFailed
			}
			if d := rp.Detail; d != nil {
				c := clip(*d, 200)
				rp.Detail = &c
			}
		}
	}
	if len(r.AgyLimits) > 20 {
		r.AgyLimits = r.AgyLimits[:20]
	}
	for i := range r.AgyLimits {
		r.AgyLimits[i].Model = clip(r.AgyLimits[i].Model, 80)
	}
	if len(r.AgyUsage) > 10 {
		r.AgyUsage = r.AgyUsage[:10]
	}
	for i := range r.AgyUsage {
		g := &r.AgyUsage[i]
		g.Name = clip(g.Name, 80)
		if g.Models != nil {
			m := clip(*g.Models, 200)
			g.Models = &m
		}
		for _, w := range []*apispec.AgyUsageWindow{g.FiveHour, g.Weekly} {
			if w != nil {
				w.Remaining = min(max(w.Remaining, 0), 1)
			}
		}
	}
	r.Roots = clipList(r.Roots, 32, 1024)
	if len(r.Clients) > 16 {
		r.Clients = r.Clients[:16]
	}
	if len(r.Rejected) > 100 {
		r.Rejected = r.Rejected[:100]
	}
	if r.Clients == nil {
		r.Clients = []Client{}
	}
	if r.Rejected == nil {
		r.Rejected = []Rejected{}
	}
	return s.Store.SaveReport(ctx, h.ID, r, s.Clock.Now())
}

// Config is what a guard needs to run its agents. Version changes whenever
// anything in it changes, so the guard can poll cheaply (ETag).
type Config struct {
	Version  string
	Agents   []AgentConfig
	Previews []HostPreview
}

// Config returns the host's current config.
func (s *Service) Config(ctx context.Context, h Host) (Config, error) {
	agents, err := s.agentConfigs(ctx, h.ID)
	if err != nil {
		return Config{}, err
	}
	var previews []HostPreview
	if s.Previews != nil {
		ps, err := s.Previews.HostPreviews(ctx, h.ID)
		if err != nil {
			return Config{}, err
		}
		previews = ps
	}
	if previews == nil {
		previews = []HostPreview{}
	}
	raw, err := json.Marshal(struct {
		Agents   []AgentConfig `json:"agents"`
		Previews []HostPreview `json:"previews"`
	}{Agents: agents, Previews: previews})
	if err != nil {
		return Config{}, err
	}
	sum := sha256.Sum256(raw)
	return Config{Version: hex.EncodeToString(sum[:8]), Agents: agents, Previews: previews}, nil
}

func (s *Service) agentConfigs(ctx context.Context, hostID ids.ID) ([]AgentConfig, error) {
	as, err := s.Store.AssignmentsForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	agentIDs := make([]ids.ID, 0, len(as))
	for _, a := range as {
		agentIDs = append(agentIDs, a.AgentID)
		if a.PartnerID != ids.Nil {
			agentIDs = append(agentIDs, a.PartnerID)
		}
	}
	actors, err := s.Directory.ActorsByIDs(ctx, agentIDs)
	if err != nil {
		return nil, err
	}
	handles := map[ids.ID]string{}
	for _, a := range actors {
		handles[a.ID] = a.Handle
	}
	out := make([]AgentConfig, 0, len(as))
	for _, a := range as {
		gs, err := s.Store.ActiveGrants(ctx, hostID, a.AgentID)
		if err != nil {
			return nil, err
		}
		out = append(out, AgentConfig{Assignment: a, Handle: handles[a.AgentID], PartnerHandle: handles[a.PartnerID], Grants: gs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out, nil
}

// AgentToken gives the host a short-lived token for one of its agents.
func (s *Service) AgentToken(ctx context.Context, h Host, agentID ids.ID) (string, time.Time, error) {
	a, err := s.Store.AssignmentByAgent(ctx, agentID)
	if errors.Is(err, db.ErrNotFound) || (err == nil && a.HostID != h.ID) {
		return "", time.Time{}, ErrForbidden // only agents assigned to this host
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return s.Tokens.IssueHostToken(ctx, agentID, h.ID, AgentTokenTTL)
}

// Assignment returns an agent's current assignment, or db.ErrNotFound.
func (s *Service) Assignment(ctx context.Context, agentID ids.ID) (Assignment, error) {
	return s.Store.AssignmentByAgent(ctx, agentID)
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func clipList(list []string, maxItems, maxLen int) []string {
	if list == nil {
		return []string{}
	}
	if len(list) > maxItems {
		list = list[:maxItems]
	}
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = clip(s, maxLen)
	}
	return out
}

// HostByID returns a host by its id.
func (s *Service) HostByID(ctx context.Context, id ids.ID) (Host, error) {
	return s.Store.HostByID(ctx, id)
}

// ActiveGrants returns non-revoked grants for the agent on the host.
func (s *Service) ActiveGrants(ctx context.Context, hostID, agentID ids.ID) ([]Grant, error) {
	return s.Store.ActiveGrants(ctx, hostID, agentID)
}

// IsFullAccess reports whether the agent is assigned in direct mode without sandbox.
func (s *Service) IsFullAccess(ctx context.Context, agentID ids.ID) (bool, error) {
	a, err := s.Store.AssignmentByAgent(ctx, agentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return a.Mode == AgentModeDirect && !a.Sandbox, nil
}

// crossTenant revokes a host whose token was used on another workspace's
// subdomain (round 45): the token has leaked. ctx is narrowed to the
// host's workspace; the event lands in its audit log.
func (s *Service) crossTenant(ctx context.Context, t tenant.Request, hash []byte) {
	log := slog.Default()
	h, err := s.Store.HostByTokenHash(ctx, hash)
	log.Error("SECURITY: cross-tenant credential use; the host is revoked",
		"credential", "host", "host_id", h.ID, "credential_workspace", h.WorkspaceID,
		"request_workspace", t.Workspace, "subdomain", t.Slug, "remote_addr", t.RemoteAddr, "user_agent", t.UserAgent)
	if err != nil {
		log.Error("cross-tenant: finding the host failed", "err", err)
		return
	}
	err = s.record(ctx, h.WorkspaceID, ids.Nil, EventHostCrossTenant, h.ID, HostEvent{HostID: h.ID.String(), Name: h.Name, State: StateRevoked},
		func(ctx context.Context) error {
			if err := s.Store.SetHostState(ctx, h.ID, StateRevoked); err != nil {
				return err
			}
			return s.Tokens.RevokeHostTokens(ctx, h.ID, ids.Nil)
		})
	if err != nil {
		log.Error("cross-tenant: revoking the host failed", "err", err)
	}
}
