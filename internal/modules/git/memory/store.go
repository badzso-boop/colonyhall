// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/git"
	"colonyhall/internal/platform/ids"
)

// Store is an in-memory implementation of git.Store.
type Store struct {
	mu           sync.RWMutex
	links        map[linkKey]git.LinkCard
	repoChannels map[string]git.RepoChannel // key: workspaceID.String() + ":" + repo
	deliveries   map[string]time.Time
	giteas       map[ids.ID]git.WorkspaceGitea
}

// New creates an in-memory git store.
func New() *Store {
	return &Store{
		links:        make(map[linkKey]git.LinkCard),
		repoChannels: make(map[string]git.RepoChannel),
		deliveries:   make(map[string]time.Time),
		giteas:       make(map[ids.ID]git.WorkspaceGitea),
	}
}

var _ git.Store = (*Store)(nil)

type linkKey struct {
	ws  ids.ID
	url string
}

func (s *Store) GetLink(_ context.Context, workspaceID ids.ID, url string) (git.LinkCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	card, ok := s.links[linkKey{workspaceID, url}]
	if !ok {
		return git.LinkCard{}, git.ErrLinkNotFound
	}
	return card, nil
}

func (s *Store) GetLinks(_ context.Context, workspaceID ids.ID, urls []string) ([]git.LinkCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]git.LinkCard, 0, len(urls))
	for _, u := range urls {
		if card, ok := s.links[linkKey{workspaceID, u}]; ok {
			out = append(out, card)
		}
	}
	return out, nil
}

func (s *Store) SaveLink(_ context.Context, card git.LinkCard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[linkKey{card.WorkspaceID, card.URL}] = card
	return nil
}

func (s *Store) ListRepoChannels(_ context.Context, workspaceID ids.ID) ([]git.RepoChannel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []git.RepoChannel
	for _, m := range s.repoChannels {
		if m.WorkspaceID == workspaceID {
			list = append(list, m)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Repo < list[j].Repo
	})
	return list, nil
}

func (s *Store) GetRepoChannel(_ context.Context, workspaceID ids.ID, repo string) (git.RepoChannel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := workspaceID.String() + ":" + repo
	m, ok := s.repoChannels[key]
	if !ok {
		return git.RepoChannel{}, git.ErrMappingNotFound
	}
	return m, nil
}

func (s *Store) FindRepoChannels(_ context.Context, repo string) ([]git.RepoChannel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []git.RepoChannel
	for _, m := range s.repoChannels {
		if m.Repo == repo {
			list = append(list, m)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].WorkspaceID.String() < list[j].WorkspaceID.String()
	})
	return list, nil
}

func (s *Store) SetRepoChannel(_ context.Context, mapping git.RepoChannel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := mapping.WorkspaceID.String() + ":" + mapping.Repo
	s.repoChannels[key] = mapping
	return nil
}

func (s *Store) DeleteRepoChannel(_ context.Context, workspaceID ids.ID, repo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := workspaceID.String() + ":" + repo
	if _, ok := s.repoChannels[key]; !ok {
		return git.ErrMappingNotFound
	}
	delete(s.repoChannels, key)
	return nil
}

func (s *Store) RecordDelivery(_ context.Context, deliveryID string, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.deliveries[deliveryID]; exists {
		return false, nil
	}
	s.deliveries[deliveryID] = now
	return true, nil
}

// GetWorkspaceGitea returns the workspace's own Gitea.
func (s *Store) GetWorkspaceGitea(_ context.Context, workspaceID ids.ID) (git.WorkspaceGitea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.giteas[workspaceID]
	if !ok {
		return git.WorkspaceGitea{}, git.ErrNoOwnGitea
	}
	return c, nil
}

// SetWorkspaceGitea stores the workspace's own Gitea.
func (s *Store) SetWorkspaceGitea(_ context.Context, c git.WorkspaceGitea) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.giteas[c.WorkspaceID] = c
	return nil
}
