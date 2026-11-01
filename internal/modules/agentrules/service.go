package agentrules

import (
	"context"
	"errors"
	"sort"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Directory provides actor and workspace lookups needed by agentrules.
type Directory interface {
	Admins(ctx context.Context, workspaceID ids.ID) ([]identity.Actor, error)
	WorkspaceByID(ctx context.Context, id ids.ID) (identity.Workspace, error)
}

// Service implements the agent rules use cases and rendering.
type Service struct {
	Store     Store
	Directory Directory
	Events    eventlog.Store
	Bus       bus.Bus
	Tx        db.TxRunner
	IDs       ids.Generator
	Clock     clock.Clock
	// OwnInstructions returns an agent's own instructions (round 33; the
	// agents module), appended after the shared rules. nil: none.
	OwnInstructions func(ctx context.Context, agentID ids.ID) (string, error)
}

// GetLatest returns the current agent rules for a workspace.
// If no rules have been set yet, version 0 with the built-in default is returned.
func (s *Service) GetLatest(ctx context.Context, workspaceID ids.ID) (Rules, error) {
	r, err := s.Store.GetLatest(ctx, workspaceID)
	if err == nil {
		return r, nil
	}
	if errors.Is(err, db.ErrNotFound) {
		return Rules{
			WorkspaceID: workspaceID,
			Version:     0,
			Text:        DefaultRulesText,
		}, nil
	}
	return Rules{}, err
}

// GetVersion returns a specific version of rules for a workspace.
// Version 0 returns the built-in default.
func (s *Service) GetVersion(ctx context.Context, workspaceID ids.ID, version int) (Rules, error) {
	if version == 0 {
		return Rules{
			WorkspaceID: workspaceID,
			Version:     0,
			Text:        DefaultRulesText,
		}, nil
	}
	r, err := s.Store.GetVersion(ctx, workspaceID, version)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return Rules{}, ErrNotFound
		}
		return Rules{}, err
	}
	return r, nil
}

// ListHistory returns version metadata ordered newest to oldest.
func (s *Service) ListHistory(ctx context.Context, workspaceID ids.ID) ([]RulesMeta, error) {
	return s.Store.ListHistory(ctx, workspaceID)
}

// Update creates a new version of the agent rules. Humans only (admin or owner).
func (s *Service) Update(ctx context.Context, by identity.Actor, text string) (Rules, error) {
	if by.Kind != identity.KindHuman || !by.IsAdmin() {
		return Rules{}, ErrForbidden
	}
	if by.Halted {
		return Rules{}, identity.ErrHalted
	}
	if len([]rune(text)) > MaxRulesLen {
		return Rules{}, ErrRulesTooLong
	}

	latest, err := s.GetLatest(ctx, by.WorkspaceID)
	if err != nil {
		return Rules{}, err
	}
	newVersion := latest.Version + 1
	now := s.Clock.Now()

	r := Rules{
		WorkspaceID: by.WorkspaceID,
		Version:     newVersion,
		Text:        text,
		CreatedBy:   by.ID,
		CreatedAt:   now,
	}

	payload := RulesUpdatedEvent{
		Version:   newVersion,
		CreatedBy: by.ID.String(),
	}

	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.Insert(ctx, r); err != nil {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventRulesUpdated, by.ID, ids.Nil,
			"agent_rules", ids.Nil, payload, now)
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
		return Rules{}, err
	}
	s.Bus.Notify(seq)

	return r, nil
}

// RenderInstructions renders the current rules for the calling actor.
func (s *Service) RenderInstructions(ctx context.Context, actor identity.Actor) (int, string, error) {
	version, shared, err := s.RenderShared(ctx, actor.WorkspaceID, actor.Handle)
	if err != nil {
		return 0, "", err
	}
	if s.OwnInstructions == nil || actor.Kind != identity.KindAgent {
		return version, shared, nil
	}
	own, err := s.OwnInstructions(ctx, actor.ID)
	if err != nil {
		return 0, "", err
	}
	return version, WithOwn(shared, s.renderFor(ctx, actor.WorkspaceID, own, actor.Handle), actor.Handle), nil
}

// WithOwn puts an agent's own instructions after the shared rules, under a
// clear heading (round 33).
func WithOwn(shared, own, handle string) string {
	if own == "" {
		return shared
	}
	return shared + "\n\n## Instructions for @" + handle + "\n\n" + own
}

// RenderShared renders the workspace's shared rules for a handle (also for
// an agent about to be created: the creation dialog's effective preview).
func (s *Service) RenderShared(ctx context.Context, workspaceID ids.ID, handle string) (int, string, error) {
	rules, err := s.GetLatest(ctx, workspaceID)
	if err != nil {
		return 0, "", err
	}
	return rules.Version, s.renderFor(ctx, workspaceID, rules.Text, handle), nil
}

// RenderText fills the placeholders of any text for a handle.
func (s *Service) RenderText(ctx context.Context, workspaceID ids.ID, text, handle string) string {
	return s.renderFor(ctx, workspaceID, text, handle)
}

func (s *Service) renderFor(ctx context.Context, workspaceID ids.ID, text, handle string) string {
	actor := identity.Actor{WorkspaceID: workspaceID, Handle: handle}

	var adminHandles []string
	if s.Directory != nil {
		admins, err := s.Directory.Admins(ctx, actor.WorkspaceID)
		if err == nil {
			for _, a := range admins {
				if a.Kind == identity.KindHuman {
					adminHandles = append(adminHandles, a.Handle)
				}
			}
			sort.Strings(adminHandles)
		}
	}

	wsName := "Colonyhall"
	if s.Directory != nil {
		if ws, err := s.Directory.WorkspaceByID(ctx, actor.WorkspaceID); err == nil && ws.Name != "" {
			wsName = ws.Name
		}
	}

	return Render(text, actor.Handle, adminHandles, wsName)
}

// InstructionsForActor returns rendered instructions for agents, or empty string for humans.
func (s *Service) InstructionsForActor(ctx context.Context, actor identity.Actor) (string, error) {
	if actor.Kind != identity.KindAgent {
		return "", nil
	}
	_, text, err := s.RenderInstructions(ctx, actor)
	return text, err
}
