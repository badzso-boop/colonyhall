// Package postgres implements hosts.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/modules/hosts/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements hosts.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	return err
}

// CreatePairingCode stores a code.
func (s *Store) CreatePairingCode(ctx context.Context, c hosts.PairingCode) error {
	return s.q(ctx).CreatePairingCode(ctx, sqlcgen.CreatePairingCodeParams{ID: c.ID, WorkspaceID: c.WorkspaceID,
		CodeHash: c.CodeHash, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, ExpiresAt: c.ExpiresAt})
}

// UsePairingCode marks an unused, unexpired code as used.
func (s *Store) UsePairingCode(ctx context.Context, hash []byte, now time.Time) (hosts.PairingCode, error) {
	r, err := s.q(ctx).UsePairingCode(ctx, sqlcgen.UsePairingCodeParams{Now: &now, CodeHash: hash})
	if err != nil {
		return hosts.PairingCode{}, notFound(err)
	}
	return hosts.PairingCode{ID: r.ID, WorkspaceID: r.WorkspaceID, CodeHash: r.CodeHash, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, UsedAt: r.UsedAt}, nil
}

// CreateHost stores a host.
func (s *Store) CreateHost(ctx context.Context, h hosts.Host) error {
	report, err := json.Marshal(h.Report)
	if err != nil {
		return err
	}
	return s.q(ctx).CreateHost(ctx, sqlcgen.CreateHostParams{ID: h.ID, WorkspaceID: h.WorkspaceID, Name: h.Name,
		TokenHash: h.TokenHash, State: h.State, CreatedBy: h.CreatedBy, CreatedAt: h.CreatedAt, Report: report})
}

func toHost(r sqlcgen.Host) (hosts.Host, error) {
	h := hosts.Host{ID: r.ID, WorkspaceID: r.WorkspaceID, Name: r.Name, TokenHash: r.TokenHash, State: r.State,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt}
	if len(r.Report) > 0 {
		if err := json.Unmarshal(r.Report, &h.Report); err != nil {
			return h, err
		}
	}
	return h, nil
}

// HostByID loads a host.
func (s *Store) HostByID(ctx context.Context, id ids.ID) (hosts.Host, error) {
	r, err := s.q(ctx).HostByID(ctx, id)
	if err != nil {
		return hosts.Host{}, notFound(err)
	}
	return toHost(r)
}

// HostByTokenHash loads a host by its token.
func (s *Store) HostByTokenHash(ctx context.Context, hash []byte) (hosts.Host, error) {
	r, err := s.q(ctx).HostByTokenHash(ctx, hash)
	if err != nil {
		return hosts.Host{}, notFound(err)
	}
	return toHost(r)
}

// ListHosts lists a workspace's hosts by name.
func (s *Store) ListHosts(ctx context.Context, workspaceID ids.ID) ([]hosts.Host, error) {
	rows, err := s.q(ctx).ListHosts(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]hosts.Host, 0, len(rows))
	for _, r := range rows {
		h, err := toHost(r)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// SetHostState changes a host's state.
func (s *Store) SetHostState(ctx context.Context, id ids.ID, state string) error {
	return s.q(ctx).SetHostState(ctx, sqlcgen.SetHostStateParams{ID: id, State: state})
}

// SaveReport stores the guard's report.
func (s *Store) SaveReport(ctx context.Context, id ids.ID, r hosts.Report, at time.Time) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return s.q(ctx).SaveReport(ctx, sqlcgen.SaveReportParams{ID: id, Report: raw, LastSeenAt: &at})
}

// Touch records that the host was seen.
func (s *Store) Touch(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).Touch(ctx, sqlcgen.TouchParams{ID: id, LastSeenAt: &at})
}

// SetAssignment creates or replaces an agent's assignment.
func (s *Store) SetAssignment(ctx context.Context, a hosts.Assignment) error {
	approvals := a.Approvals
	if approvals == "" {
		approvals = hosts.ApprovalsNone
	}
	return s.q(ctx).SetAssignment(ctx, sqlcgen.SetAssignmentParams{
		AgentID: a.AgentID, HostID: a.HostID, WorkspaceID: a.WorkspaceID, Client: a.Client,
		Model: a.Model, WorkModel: a.WorkModel, AllowWork: a.AllowWork, PermissionMode: a.PermissionMode,
		Mode: a.Mode, RootDir: a.RootDir, Sandbox: a.Sandbox, Approvals: approvals,
		CreatedBy: a.CreatedBy, CreatedAt: a.CreatedAt,
	})
}

// DeleteAssignment removes an agent's assignment.
func (s *Store) DeleteAssignment(ctx context.Context, agentID ids.ID) error {
	return s.q(ctx).DeleteAssignment(ctx, agentID)
}

func toAssignment(r sqlcgen.AgentHost) hosts.Assignment {
	approvals := r.Approvals
	if approvals == "" {
		approvals = hosts.ApprovalsNone
	}
	return hosts.Assignment{
		AgentID: r.AgentID, HostID: r.HostID, WorkspaceID: r.WorkspaceID, Client: r.Client,
		Model: r.Model, WorkModel: r.WorkModel, AllowWork: r.AllowWork, PermissionMode: r.PermissionMode,
		Mode: r.Mode, RootDir: r.RootDir, Sandbox: r.Sandbox, Approvals: approvals,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt,
	}
}

// AssignmentByAgent loads an agent's assignment.
func (s *Store) AssignmentByAgent(ctx context.Context, agentID ids.ID) (hosts.Assignment, error) {
	r, err := s.q(ctx).AssignmentByAgent(ctx, agentID)
	if err != nil {
		return hosts.Assignment{}, notFound(err)
	}
	return toAssignment(r), nil
}

// AssignmentsForHost lists a host's assignments.
func (s *Store) AssignmentsForHost(ctx context.Context, hostID ids.ID) ([]hosts.Assignment, error) {
	rows, err := s.q(ctx).AssignmentsForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := make([]hosts.Assignment, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAssignment(r))
	}
	return out, nil
}

// AddGrant stores a grant.
func (s *Store) AddGrant(ctx context.Context, g hosts.Grant) error {
	return s.q(ctx).AddGrant(ctx, sqlcgen.AddGrantParams{ID: g.ID, WorkspaceID: g.WorkspaceID, AgentID: g.AgentID,
		HostID: g.HostID, Path: g.Path, Mode: g.Mode, CreatedBy: g.CreatedBy, CreatedAt: g.CreatedAt})
}

func toGrant(r sqlcgen.FolderGrant) hosts.Grant {
	return hosts.Grant{ID: r.ID, WorkspaceID: r.WorkspaceID, AgentID: r.AgentID, HostID: r.HostID, Path: r.Path,
		Mode: r.Mode, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, RevokedAt: r.RevokedAt}
}

// GrantByID loads a grant.
func (s *Store) GrantByID(ctx context.Context, id ids.ID) (hosts.Grant, error) {
	r, err := s.q(ctx).GrantByID(ctx, id)
	if err != nil {
		return hosts.Grant{}, notFound(err)
	}
	return toGrant(r), nil
}

// RevokeGrant marks a grant revoked.
func (s *Store) RevokeGrant(ctx context.Context, id ids.ID, at time.Time) error {
	return s.q(ctx).RevokeGrant(ctx, sqlcgen.RevokeGrantParams{ID: id, RevokedAt: &at})
}

// ActiveGrants lists the agent's active grants on the host, by path.
func (s *Store) ActiveGrants(ctx context.Context, hostID, agentID ids.ID) ([]hosts.Grant, error) {
	rows, err := s.q(ctx).ActiveGrants(ctx, sqlcgen.ActiveGrantsParams{HostID: hostID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	out := make([]hosts.Grant, 0, len(rows))
	for _, r := range rows {
		out = append(out, toGrant(r))
	}
	return out, nil
}

// RevokeGrantsForAgent revokes every active grant of the agent.
func (s *Store) RevokeGrantsForAgent(ctx context.Context, agentID ids.ID, at time.Time) error {
	return s.q(ctx).RevokeGrantsForAgent(ctx, sqlcgen.RevokeGrantsForAgentParams{AgentID: agentID, RevokedAt: &at})
}
