package hosts

import (
	"context"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// OnlineWindow: a machine counts as online when its guard was seen within
// it (the same window as the PWA's Machines page).
const OnlineWindow = 90 * time.Second

// Placement is where an agent runs, as any actor of the workspace may know
// it: the machine, whether it is online, the client and the mode. It has
// no folders, grants, models or limits.
type Placement struct {
	AgentID       ids.ID
	Machine       string
	MachineOnline bool
	Client        string
	Sandbox       bool
	AllowWork     bool
	DocsOnly      bool
}

// Placements lists where the workspace's agents run, for by (any actor of
// the workspace, agents included: they need it to know whom to ask).
func (s *Service) Placements(ctx context.Context, by identity.Actor) ([]Placement, error) {
	hs, err := s.Store.ListHosts(ctx, by.WorkspaceID)
	if err != nil {
		return nil, err
	}
	now := s.Clock.Now()
	var out []Placement
	for _, h := range hs {
		as, err := s.Store.AssignmentsForHost(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		online := h.LastSeenAt != nil && now.Sub(*h.LastSeenAt) <= OnlineWindow
		for _, a := range as {
			out = append(out, Placement{AgentID: a.AgentID, Machine: h.Name, MachineOnline: online, Client: a.Client,
				Sandbox: a.Sandbox, AllowWork: a.AllowWork, DocsOnly: a.WriteScope == WriteScopeDocs})
		}
	}
	return out, nil
}
