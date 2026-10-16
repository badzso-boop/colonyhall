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
	links        map[string]git.LinkCard
	repoChannels map[string]git.RepoChannel // key: workspaceID.String() + ":" + repo
	deliveries   map[string]time.Time
}

// New creates an in-memory git store.
func New() *Store {
	return &Store{
		links:        make(map[string]git.LinkCard),
		repoChannels: make(map[string]git.RepoChannel),
		deliveries:   make(map[string]time.Time),
	}
}

var _ git.Store = (*Store)(nil)

func (s *Store) GetLink(_ context.Context, url string) (git.LinkCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	card, ok := s.links[url]
	if !ok {
		return git.LinkCard{}, git.ErrLinkNotFound
	}
	return card, nil
}

func (s *Store) GetLinks(_ context.Context, urls []string) ([]git.LinkCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]git.LinkCard, 0, len(urls))
	for _, u := range urls {
		if card, ok := s.links[u]; ok {
			out = append(out, card)
		}
	}
	return out, nil
}

func (s *Store) SaveLink(_ context.Context, card git.LinkCard) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.links[card.URL] = card
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
