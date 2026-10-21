package sidebar

import (
	"context"
	"strings"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

// ChannelReader checks read permission on channels.
type ChannelReader interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
}

// Service provides sidebar section operations.
type Service struct {
	Store        Store
	Conversation ChannelReader
	Clock        clock.Clock
	IDs          ids.Generator
}

// NewService returns a new sidebar service.
func NewService(store Store, conv ChannelReader, clk clock.Clock, gen ids.Generator) *Service {
	return &Service{
		Store:        store,
		Conversation: conv,
		Clock:        clk,
		IDs:          gen,
	}
}

func (s *Service) checkHuman(by identity.Actor) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	if by.Halted {
		return identity.ErrHalted
	}
	return nil
}

// List returns all active sections and items for the calling user,
// filtering out any channels the user cannot read.
func (s *Service) List(ctx context.Context, by identity.Actor) ([]Section, error) {
	if err := s.checkHuman(by); err != nil {
		return nil, err
	}
	sections, err := s.Store.ListSections(ctx, by.WorkspaceID, by.ID)
	if err != nil {
		return nil, err
	}
	for i := range sections {
		var visibleItems []Item
		for _, item := range sections[i].Items {
			canRead, err := s.Conversation.CanRead(ctx, by, item.ChannelID)
			if err == nil && canRead {
				visibleItems = append(visibleItems, item)
			}
		}
		sections[i].Items = visibleItems
	}
	return sections, nil
}

// Create adds a new section to the end of the user's sidebar.
func (s *Service) Create(ctx context.Context, by identity.Actor, name string) (Section, error) {
	if err := s.checkHuman(by); err != nil {
		return Section{}, err
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" || len(cleanName) > 40 {
		return Section{}, ErrInvalidName
	}
	existing, err := s.Store.ListSections(ctx, by.WorkspaceID, by.ID)
	if err != nil {
		return Section{}, err
	}
	now := s.Clock.Now()
	sec := Section{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		OwnerID:     by.ID,
		Name:        cleanName,
		Position:    len(existing),
		Collapsed:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
		Items:       []Item{},
	}
	if err := s.Store.CreateSection(ctx, sec); err != nil {
		return Section{}, err
	}
	return sec, nil
}

// Update updates a section's name, position, or collapsed state.
func (s *Service) Update(ctx context.Context, by identity.Actor, id ids.ID, name *string, position *int, collapsed *bool) (Section, error) {
	if err := s.checkHuman(by); err != nil {
		return Section{}, err
	}
	sec, err := s.Store.GetSection(ctx, id)
	if err != nil {
		return Section{}, err
	}
	if sec.OwnerID != by.ID || sec.WorkspaceID != by.WorkspaceID {
		return Section{}, ErrSectionNotFound
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" || len(trimmed) > 40 {
			return Section{}, ErrInvalidName
		}
		name = &trimmed
	}
	return s.Store.UpdateSection(ctx, id, name, position, collapsed, s.Clock.Now())
}

// Delete soft-deletes a section, causing its items to fall back to the default channel/DM lists.
func (s *Service) Delete(ctx context.Context, by identity.Actor, id ids.ID) error {
	if err := s.checkHuman(by); err != nil {
		return err
	}
	sec, err := s.Store.GetSection(ctx, id)
	if err != nil {
		return err
	}
	if sec.OwnerID != by.ID || sec.WorkspaceID != by.WorkspaceID {
		return ErrSectionNotFound
	}
	return s.Store.DeleteSection(ctx, id, s.Clock.Now())
}

// ReorderSections changes the order of the user's sections.
func (s *Service) ReorderSections(ctx context.Context, by identity.Actor, sectionIDs []ids.ID) error {
	if err := s.checkHuman(by); err != nil {
		return err
	}
	return s.Store.ReorderSections(ctx, by.ID, sectionIDs)
}

// SetItem puts a channel or DM into a section (and removes it from any other section of this user).
func (s *Service) SetItem(ctx context.Context, by identity.Actor, sectionID, channelID ids.ID, position *int) error {
	if err := s.checkHuman(by); err != nil {
		return err
	}
	sec, err := s.Store.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if sec.OwnerID != by.ID || sec.WorkspaceID != by.WorkspaceID {
		return ErrSectionNotFound
	}
	canRead, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil {
		return err
	}
	if !canRead {
		return ErrForbidden
	}
	pos := len(sec.Items)
	if position != nil {
		pos = *position
	}
	return s.Store.SetItem(ctx, by.ID, sectionID, channelID, pos)
}

// RemoveItem removes a channel or DM from a section.
func (s *Service) RemoveItem(ctx context.Context, by identity.Actor, sectionID, channelID ids.ID) error {
	if err := s.checkHuman(by); err != nil {
		return err
	}
	sec, err := s.Store.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if sec.OwnerID != by.ID || sec.WorkspaceID != by.WorkspaceID {
		return ErrSectionNotFound
	}
	return s.Store.RemoveItem(ctx, sectionID, channelID)
}

// ReorderItems updates the ordering of items in a section.
func (s *Service) ReorderItems(ctx context.Context, by identity.Actor, sectionID ids.ID, channelIDs []ids.ID) error {
	if err := s.checkHuman(by); err != nil {
		return err
	}
	sec, err := s.Store.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if sec.OwnerID != by.ID || sec.WorkspaceID != by.WorkspaceID {
		return ErrSectionNotFound
	}
	return s.Store.ReorderItems(ctx, sectionID, channelIDs)
}
