// Package memory is the in-memory reference implementation of hosts.Store.
package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements hosts.Store in memory.
type Store struct {
	mu      sync.RWMutex
	codes   map[string]hosts.PairingCode // key: string(hash)
	hosts   map[ids.ID]hosts.Host
	assigns map[ids.ID]hosts.Assignment // key: agent id
	grants  map[ids.ID]hosts.Grant
}

// New returns an empty store.
func New() *Store {
	return &Store{codes: map[string]hosts.PairingCode{}, hosts: map[ids.ID]hosts.Host{},
		assigns: map[ids.ID]hosts.Assignment{}, grants: map[ids.ID]hosts.Grant{}}
}

// CreatePairingCode stores a code.
func (s *Store) CreatePairingCode(_ context.Context, c hosts.PairingCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[string(c.CodeHash)] = c
	return nil
}

// UsePairingCode marks an unused, unexpired code as used.
func (s *Store) UsePairingCode(_ context.Context, hash []byte, now time.Time) (hosts.PairingCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[string(hash)]
	if !ok || c.UsedAt != nil || !c.ExpiresAt.After(now) {
		return hosts.PairingCode{}, db.ErrNotFound
	}
	c.UsedAt = &now
	s.codes[string(hash)] = c
	return c, nil
}

// CreateHost stores a host.
func (s *Store) CreateHost(_ context.Context, h hosts.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hosts[h.ID] = h
	return nil
}

// HostByID loads a host.
func (s *Store) HostByID(_ context.Context, id ids.ID) (hosts.Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.hosts[id]
	if !ok {
		return h, db.ErrNotFound
	}
	return h, nil
}

// HostByTokenHash loads a host by its token.
func (s *Store) HostByTokenHash(_ context.Context, hash []byte) (hosts.Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range s.hosts {
		if bytes.Equal(h.TokenHash, hash) {
			return h, nil
		}
	}
	return hosts.Host{}, db.ErrNotFound
}

// ListHosts lists a workspace's hosts by name.
func (s *Store) ListHosts(_ context.Context, workspaceID ids.ID) ([]hosts.Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []hosts.Host
	for _, h := range s.hosts {
		if h.WorkspaceID == workspaceID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) < 0
	})
	return out, nil
}

func (s *Store) update(id ids.ID, f func(*hosts.Host)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.hosts[id]
	if !ok {
		return db.ErrNotFound
	}
	f(&h)
	s.hosts[id] = h
	return nil
}

// SetHostState changes a host's state.
func (s *Store) SetHostState(_ context.Context, id ids.ID, state string) error {
	return s.update(id, func(h *hosts.Host) { h.State = state })
}

// UpdateHost sets the host's name and colour.
func (s *Store) UpdateHost(_ context.Context, id ids.ID, name, color string) error {
	return s.update(id, func(h *hosts.Host) { h.Name, h.Color = name, color })
}

// AgentHostColors maps assigned agents to their host's colour.
func (s *Store) AgentHostColors(_ context.Context, workspaceID ids.ID) (map[ids.ID]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[ids.ID]string{}
	for _, a := range s.assigns {
		if h, ok := s.hosts[a.HostID]; ok && h.WorkspaceID == workspaceID {
			out[a.AgentID] = h.Color
		}
	}
	return out, nil
}

// SaveReport stores the guard's report.
func (s *Store) SaveReport(_ context.Context, id ids.ID, r hosts.Report, at time.Time) error {
	return s.update(id, func(h *hosts.Host) { h.Report, h.LastSeenAt = r, &at })
}

// Touch records that the host was seen.
func (s *Store) Touch(_ context.Context, id ids.ID, at time.Time) error {
	return s.update(id, func(h *hosts.Host) { h.LastSeenAt = &at })
}

// SetAssignment creates or replaces an agent's assignment.
func (s *Store) SetAssignment(_ context.Context, a hosts.Assignment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Approvals == "" {
		a.Approvals = hosts.ApprovalsNone
	}
	if a.WriteScope == "" {
		a.WriteScope = hosts.WriteScopeAll
	}
	s.assigns[a.AgentID] = a
	return nil
}

// DeleteAssignment removes an agent's assignment.
func (s *Store) DeleteAssignment(_ context.Context, agentID ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.assigns, agentID)
	return nil
}

// AssignmentByAgent loads an agent's assignment.
func (s *Store) AssignmentByAgent(_ context.Context, agentID ids.ID) (hosts.Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.assigns[agentID]
	if !ok {
		return a, db.ErrNotFound
	}
	return a, nil
}

// AssignmentsForHost lists a host's assignments.
func (s *Store) AssignmentsForHost(_ context.Context, hostID ids.ID) ([]hosts.Assignment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []hosts.Assignment
	for _, a := range s.assigns {
		if a.HostID == hostID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i].AgentID[:], out[j].AgentID[:]) < 0 })
	return out, nil
}

// AddGrant stores a grant.
func (s *Store) AddGrant(_ context.Context, g hosts.Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[g.ID] = g
	return nil
}

// GrantByID loads a grant.
func (s *Store) GrantByID(_ context.Context, id ids.ID) (hosts.Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.grants[id]
	if !ok {
		return g, db.ErrNotFound
	}
	return g, nil
}

// RevokeGrant marks a grant revoked.
func (s *Store) RevokeGrant(_ context.Context, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if ok && g.RevokedAt == nil {
		g.RevokedAt = &at
		s.grants[id] = g
	}
	return nil
}

// ActiveGrants lists the agent's active grants on the host, by path.
func (s *Store) ActiveGrants(_ context.Context, hostID, agentID ids.ID) ([]hosts.Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []hosts.Grant
	for _, g := range s.grants {
		if g.HostID == hostID && g.AgentID == agentID && g.RevokedAt == nil {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) < 0
	})
	return out, nil
}

// RevokeGrantsForAgent revokes every active grant of the agent.
func (s *Store) RevokeGrantsForAgent(_ context.Context, agentID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, g := range s.grants {
		if g.AgentID == agentID && g.RevokedAt == nil {
			g.RevokedAt = &at
			s.grants[id] = g
		}
	}
	return nil
}
