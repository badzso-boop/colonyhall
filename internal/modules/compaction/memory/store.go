// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type unitKey struct {
	channelID    ids.ID
	threadRootID ids.ID
}

// Store is an in-memory implementation of compaction.Store.
type Store struct {
	mu                sync.Mutex
	settings          map[ids.ID]compaction.Settings
	channelThresholds map[ids.ID]*int
	units             map[unitKey]compaction.CompactionUnit
	compactions       map[ids.ID]compaction.Compaction
}

func NewStore() *Store {
	return &Store{
		settings:          make(map[ids.ID]compaction.Settings),
		channelThresholds: make(map[ids.ID]*int),
		units:             make(map[unitKey]compaction.CompactionUnit),
		compactions:       make(map[ids.ID]compaction.Compaction),
	}
}

func (s *Store) GetSettings(_ context.Context, workspaceID ids.ID) (compaction.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.settings[workspaceID]
	if !ok {
		return compaction.Settings{
			WorkspaceID:            workspaceID,
			CompactThresholdTokens: compaction.DefaultThresholdTokens,
		}, nil
	}
	return st, nil
}

func (s *Store) UpsertSettings(_ context.Context, st compaction.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings[st.WorkspaceID] = st
	return nil
}

func (s *Store) GetChannelThreshold(_ context.Context, channelID ids.ID) (*int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.channelThresholds[channelID]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (s *Store) SetChannelThreshold(_ context.Context, channelID ids.ID, threshold *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelThresholds[channelID] = threshold
	return nil
}

func (s *Store) GetUnit(_ context.Context, channelID, threadRootID ids.ID) (compaction.CompactionUnit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := unitKey{channelID: channelID, threadRootID: threadRootID}
	u, ok := s.units[k]
	if !ok {
		return compaction.CompactionUnit{}, db.ErrNotFound
	}
	return u, nil
}

func (s *Store) UpsertUnit(_ context.Context, u compaction.CompactionUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.ID == ids.Nil {
		u.ID = ids.ID(uuid.New())
	}
	k := unitKey{channelID: u.ChannelID, threadRootID: u.ThreadRootID}
	s.units[k] = u
	return nil
}

func (s *Store) AddTokensToUnit(_ context.Context, workspaceID, channelID, threadRootID ids.ID, tokens, msgs int, now time.Time) (compaction.CompactionUnit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := unitKey{channelID: channelID, threadRootID: threadRootID}
	u, ok := s.units[k]
	if !ok {
		u = compaction.CompactionUnit{
			ID:                ids.ID(uuid.New()),
			WorkspaceID:       workspaceID,
			ChannelID:         channelID,
			ThreadRootID:      threadRootID,
			ApproxTokensSince: tokens,
			MessagesSince:     msgs,
			UpdatedAt:         now,
		}
	} else {
		u.ApproxTokensSince += tokens
		u.MessagesSince += msgs
		u.UpdatedAt = now
	}
	s.units[k] = u
	return u, nil
}

func (s *Store) ResetUnit(_ context.Context, channelID, threadRootID, compactionID ids.ID, remainingTokens, remainingMsgs int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := unitKey{channelID: channelID, threadRootID: threadRootID}
	u, ok := s.units[k]
	if !ok {
		return db.ErrNotFound
	}
	u.ApproxTokensSince = remainingTokens
	u.MessagesSince = remainingMsgs
	u.LastCompactionID = compactionID
	u.UpdatedAt = now
	s.units[k] = u
	return nil
}

func (s *Store) InsertCompaction(_ context.Context, c compaction.Compaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.compactions[c.ID] = c
	return nil
}

func (s *Store) CompactionByID(_ context.Context, id ids.ID) (compaction.Compaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.compactions[id]
	if !ok {
		return compaction.Compaction{}, db.ErrNotFound
	}
	return c, nil
}

func (s *Store) LatestDoneCompaction(_ context.Context, channelID, threadRootID ids.ID) (compaction.Compaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest compaction.Compaction
	var found bool
	for _, c := range s.compactions {
		if c.ChannelID == channelID && c.ThreadRootID == threadRootID && c.State == compaction.StateDone {
			if !found || c.CreatedAt.After(latest.CreatedAt) || (c.CreatedAt.Equal(latest.CreatedAt) && c.ID.String() > latest.ID.String()) {
				latest = c
				found = true
			}
		}
	}
	if !found {
		return compaction.Compaction{}, db.ErrNotFound
	}
	return latest, nil
}

func (s *Store) PendingCompaction(_ context.Context, channelID, threadRootID ids.ID) (compaction.Compaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest compaction.Compaction
	var found bool
	for _, c := range s.compactions {
		if c.ChannelID == channelID && c.ThreadRootID == threadRootID && c.State == compaction.StatePending {
			if !found || c.CreatedAt.After(latest.CreatedAt) || (c.CreatedAt.Equal(latest.CreatedAt) && c.ID.String() > latest.ID.String()) {
				latest = c
				found = true
			}
		}
	}
	if !found {
		return compaction.Compaction{}, db.ErrNotFound
	}
	return latest, nil
}

func (s *Store) CompactionCounts(_ context.Context, workspaceID ids.ID, since time.Time) (running, failed int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.compactions {
		switch {
		case c.WorkspaceID != workspaceID:
		case c.State == compaction.StatePending:
			running++
		case c.State == compaction.StateFailed && c.CompletedAt != nil && !c.CompletedAt.Before(since):
			failed++
		}
	}
	return running, failed, nil
}

func (s *Store) FailPendingCompaction(_ context.Context, id ids.ID, errorCode string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.compactions[id]
	if !ok || c.State != compaction.StatePending {
		return nil
	}
	c.State = compaction.StateFailed
	c.ErrorCode = errorCode
	c.CompletedAt = &completedAt
	s.compactions[id] = c
	return nil
}

func (s *Store) CompleteCompaction(_ context.Context, id ids.ID, fileID, sysMsgID ids.ID, messagesCount, approxTokensIn, summaryTokens int, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.compactions[id]
	if !ok || c.State != compaction.StatePending {
		return nil
	}
	c.State = compaction.StateDone
	c.SummaryFileID = fileID
	c.SystemMessageID = sysMsgID
	c.MessagesCount = messagesCount
	c.ApproxTokensIn = approxTokensIn
	c.SummaryTokens = summaryTokens
	c.CompletedAt = &completedAt
	s.compactions[id] = c
	return nil
}

func (s *Store) CompactionIDsBySystemMessageIDs(_ context.Context, msgIDs []ids.ID) (map[ids.ID]ids.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[ids.ID]ids.ID)
	wanted := make(map[ids.ID]bool, len(msgIDs))
	for _, id := range msgIDs {
		wanted[id] = true
	}
	for _, c := range s.compactions {
		if c.SystemMessageID != ids.Nil && wanted[c.SystemMessageID] {
			out[c.SystemMessageID] = c.ID
		}
	}
	return out, nil
}

var _ compaction.Store = (*Store)(nil)
