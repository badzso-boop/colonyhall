package hosts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
)

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
		h = Host{ID: s.IDs.New(), WorkspaceID: pc.WorkspaceID, Name: name, TokenHash: Hash(secret), State: StateApproved,
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
func (s *Service) ListHosts(ctx context.Context, by identity.Actor) ([]HostView, error) {
	if err := admin(by); err != nil {
		return nil, err
	}
	hs, err := s.Store.ListHosts(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]HostView, 0, len(hs))
	for _, h := range hs {
		agents, err := s.agentConfigs(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, HostView{Host: h, Agents: agents})
	}
	return out, nil
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
}

// Assign puts an agent on a host (moving it if it was elsewhere; grants on
// the old host are revoked, as are its tokens there).
func (s *Service) Assign(ctx context.Context, by identity.Actor, agentID ids.ID, in AssignInput) (AgentConfig, error) {
	if err := admin(by); err != nil {
		return AgentConfig{}, err
	}
	agent, err := s.agent(ctx, by, agentID)
	if err != nil {
		return AgentConfig{}, err
	}
	if !Clients[in.Client] {
		return AgentConfig{}, ErrBadClient
	}
	h, err := s.hostIn(ctx, by.WorkspaceID, in.HostID)
	if err != nil {
		return AgentConfig{}, err
	}
	if h.State != StateApproved {
		return AgentConfig{}, ErrHostRevoked
	}
	if in.Mode == "" {
		in.Mode = AgentModeDirect
	}
	if in.Mode != AgentModeDirect && in.Mode != AgentModeCompact {
		return AgentConfig{}, ErrBadAgentMode
	}
	if in.RootDir != "" {
		if err := ValidatePath(in.RootDir); err != nil {
			return AgentConfig{}, err
		}
	}
	sandbox := false
	if in.Sandbox != nil {
		sandbox = *in.Sandbox
	} else if in.Mode == AgentModeCompact {
		sandbox = true
	}
	if (in.Mode == AgentModeDirect || !sandbox) && in.RootDir == "" {
		return AgentConfig{}, ErrBadRootDir
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
		return AgentConfig{}, ErrBadApprovals
	}
	if approvals == ApprovalsChat && in.Client != "claude-code" {
		return AgentConfig{}, ErrApprovalsChatClient
	}
	maxSessionTokens := 150000
	if in.MaxSessionTokens != nil {
		maxSessionTokens = *in.MaxSessionTokens
	}
	if maxSessionTokens < 0 {
		return AgentConfig{}, ErrBadMaxSessionTokens
	}
	var dailyTokenLimit int64
	if in.DailyTokenLimit != nil {
		dailyTokenLimit = *in.DailyTokenLimit
	}
	if dailyTokenLimit < 0 {
		return AgentConfig{}, ErrBadDailyTokenLimit
	}
	a := Assignment{
		AgentID: agent.ID, HostID: h.ID, WorkspaceID: by.WorkspaceID, Client: in.Client, Model: clip(in.Model, 100),
		WorkModel: clip(in.WorkModel, 100), AllowWork: in.AllowWork, PermissionMode: clip(in.PermissionMode, 40),
		Mode: in.Mode, RootDir: clip(in.RootDir, 1024), Sandbox: sandbox, Approvals: approvals,
		MaxSessionTokens: maxSessionTokens, DailyTokenLimit: dailyTokenLimit,
		CreatedBy: by.ID, CreatedAt: s.Clock.Now(),
	}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventAgentAssigned, h.ID,
		AssignmentEvent{
			AgentID: agent.ID.String(), HostID: h.ID.String(), Client: a.Client, Model: a.Model,
			AllowWork: a.AllowWork, Mode: a.Mode, RootDir: a.RootDir, Sandbox: a.Sandbox, Approvals: a.Approvals,
			MaxSessionTokens: a.MaxSessionTokens, DailyTokenLimit: a.DailyTokenLimit,
		},
		func(ctx context.Context) error {
			old, err := s.Store.AssignmentByAgent(ctx, agent.ID)
			if err == nil && old.HostID != h.ID {
				if err := s.Store.RevokeGrantsForAgent(ctx, agent.ID, s.Clock.Now()); err != nil {
					return err
				}
				if err := s.Tokens.RevokeHostTokens(ctx, old.HostID, agent.ID); err != nil {
					return err
				}
			} else if err != nil && !errors.Is(err, db.ErrNotFound) {
				return err
			}
			return s.Store.SetAssignment(ctx, a)
		})
	return AgentConfig{Assignment: a, Handle: agent.Handle, Grants: []Grant{}}, err
}

// AssignmentByAgent returns an agent's current host assignment, or db.ErrNotFound if not assigned.
func (s *Service) AssignmentByAgent(ctx context.Context, agentID ids.ID) (Assignment, error) {
	return s.Store.AssignmentByAgent(ctx, agentID)
}

// Unassign takes an agent off its host: its grants and host tokens end.
func (s *Service) Unassign(ctx context.Context, by identity.Actor, agentID ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	agent, err := s.agent(ctx, by, agentID)
	if err != nil {
		return err
	}
	a, err := s.Store.AssignmentByAgent(ctx, agent.ID)
	if err != nil {
		return err
	}
	return s.record(ctx, by.WorkspaceID, by.ID, EventAgentUnassigned, a.HostID, AssignmentEvent{AgentID: agent.ID.String(), HostID: a.HostID.String()},
		func(ctx context.Context) error {
			if err := s.Store.RevokeGrantsForAgent(ctx, agent.ID, s.Clock.Now()); err != nil {
				return err
			}
			if err := s.Tokens.RevokeHostTokens(ctx, a.HostID, agent.ID); err != nil {
				return err
			}
			return s.Store.DeleteAssignment(ctx, agent.ID)
		})
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

// GrantResult is a new grant plus whether its path lies outside the roots
// the host reported (the guard will refuse it until the roots change).
type GrantResult struct {
	Grant
	OutsideRoots bool
}

// AddGrant lets an agent use a folder on the host it is assigned to.
func (s *Service) AddGrant(ctx context.Context, by identity.Actor, agentID ids.ID, path, mode string) (GrantResult, error) {
	if err := admin(by); err != nil {
		return GrantResult{}, err
	}
	if err := ValidatePath(path); err != nil {
		return GrantResult{}, err
	}
	if mode != ModeRead && mode != ModeWrite {
		return GrantResult{}, ErrBadMode
	}
	agent, err := s.agent(ctx, by, agentID)
	if err != nil {
		return GrantResult{}, err
	}
	a, err := s.Store.AssignmentByAgent(ctx, agent.ID)
	if errors.Is(err, db.ErrNotFound) {
		return GrantResult{}, ErrNotAssigned
	}
	if err != nil {
		return GrantResult{}, err
	}
	h, err := s.Store.HostByID(ctx, a.HostID)
	if err != nil {
		return GrantResult{}, err
	}
	g := Grant{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, AgentID: agent.ID, HostID: a.HostID, Path: path, Mode: mode,
		CreatedBy: by.ID, CreatedAt: s.Clock.Now()}
	err = s.record(ctx, by.WorkspaceID, by.ID, EventGrantAdded, a.HostID,
		GrantEvent{GrantID: g.ID.String(), AgentID: agent.ID.String(), HostID: a.HostID.String(), Path: path, Mode: mode},
		func(ctx context.Context) error { return s.Store.AddGrant(ctx, g) })
	if err != nil {
		return GrantResult{}, err
	}
	outside := true
	for _, r := range h.Report.Roots {
		if Within(path, r) {
			outside = false
		}
	}
	return GrantResult{Grant: g, OutsideRoots: outside}, nil
}

// RevokeGrant ends a grant (kept, marked revoked).
func (s *Service) RevokeGrant(ctx context.Context, by identity.Actor, grantID ids.ID) error {
	if err := admin(by); err != nil {
		return err
	}
	g, err := s.Store.GrantByID(ctx, grantID)
	if err != nil {
		return err
	}
	if g.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	if _, err := s.agent(ctx, by, g.AgentID); err != nil {
		return err
	}
	return s.record(ctx, by.WorkspaceID, by.ID, EventGrantRevoked, g.HostID,
		GrantEvent{GrantID: g.ID.String(), AgentID: g.AgentID.String(), HostID: g.HostID.String(), Path: g.Path, Mode: g.Mode},
		func(ctx context.Context) error { return s.Store.RevokeGrant(ctx, g.ID, s.Clock.Now()) })
}

// SaveReport stores what the guard says about its machine.
func (s *Service) SaveReport(ctx context.Context, h Host, r Report) error {
	r.GuardVersion = clip(r.GuardVersion, 40)
	r.OS = clip(r.OS, 40)
	r.Sandbox = clip(r.Sandbox, 80)
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
	Version string
	Agents  []AgentConfig
}

// Config returns the host's current config.
func (s *Service) Config(ctx context.Context, h Host) (Config, error) {
	agents, err := s.agentConfigs(ctx, h.ID)
	if err != nil {
		return Config{}, err
	}
	raw, err := json.Marshal(agents)
	if err != nil {
		return Config{}, err
	}
	sum := sha256.Sum256(raw)
	return Config{Version: hex.EncodeToString(sum[:8]), Agents: agents}, nil
}

func (s *Service) agentConfigs(ctx context.Context, hostID ids.ID) ([]AgentConfig, error) {
	as, err := s.Store.AssignmentsForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	agentIDs := make([]ids.ID, 0, len(as))
	for _, a := range as {
		agentIDs = append(agentIDs, a.AgentID)
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
		out = append(out, AgentConfig{Assignment: a, Handle: handles[a.AgentID], Grants: gs})
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
