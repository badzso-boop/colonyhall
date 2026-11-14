// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/compaction/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements compaction.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func uuidPtr(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromUUIDPtr(u *uuid.UUID) ids.ID {
	if u == nil {
		return ids.Nil
	}
	return ids.ID(*u)
}

func int32Ptr(n *int) *int32 {
	if n == nil {
		return nil
	}
	v := int32(*n)
	return &v
}

func fromInt32Ptr(n *int32) *int {
	if n == nil {
		return nil
	}
	v := int(*n)
	return &v
}

func (s *Store) GetSettings(ctx context.Context, workspaceID ids.ID) (compaction.Settings, error) {
	row, err := s.q(ctx).GetSettings(ctx, uuid.UUID(workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return compaction.Settings{
			WorkspaceID:            workspaceID,
			CompactThresholdTokens: compaction.DefaultThresholdTokens,
		}, nil
	}
	if err != nil {
		return compaction.Settings{}, err
	}
	return compaction.Settings{
		WorkspaceID:            ids.ID(row.WorkspaceID),
		CompactThresholdTokens: int(row.CompactThresholdTokens),
		CompactorAgentID:       fromUUIDPtr(row.CompactorAgentID),
		UpdatedAt:              row.UpdatedAt,
	}, nil
}

func (s *Store) UpsertSettings(ctx context.Context, st compaction.Settings) error {
	return s.q(ctx).UpsertSettings(ctx, sqlcgen.UpsertSettingsParams{
		WorkspaceID:            uuid.UUID(st.WorkspaceID),
		CompactThresholdTokens: int32(st.CompactThresholdTokens),
		CompactorAgentID:       uuidPtr(st.CompactorAgentID),
		UpdatedAt:              st.UpdatedAt,
	})
}

func (s *Store) GetChannelThreshold(ctx context.Context, channelID ids.ID) (*int, error) {
	val, err := s.q(ctx).GetChannelThreshold(ctx, uuid.UUID(channelID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, db.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromInt32Ptr(val), nil
}

func (s *Store) SetChannelThreshold(ctx context.Context, channelID ids.ID, threshold *int) error {
	return s.q(ctx).SetChannelThreshold(ctx, sqlcgen.SetChannelThresholdParams{
		ID:                     uuid.UUID(channelID),
		CompactThresholdTokens: int32Ptr(threshold),
	})
}

func (s *Store) GetUnit(ctx context.Context, channelID, threadRootID ids.ID) (compaction.CompactionUnit, error) {
	row, err := s.q(ctx).GetUnit(ctx, sqlcgen.GetUnitParams{
		ChannelID:    uuid.UUID(channelID),
		ThreadRootID: uuidPtr(threadRootID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return compaction.CompactionUnit{}, db.ErrNotFound
	}
	if err != nil {
		return compaction.CompactionUnit{}, err
	}
	return compaction.CompactionUnit{
		ID:                ids.ID(row.ID),
		WorkspaceID:       ids.ID(row.WorkspaceID),
		ChannelID:         ids.ID(row.ChannelID),
		ThreadRootID:      fromUUIDPtr(row.ThreadRootID),
		ApproxTokensSince: int(row.ApproxTokensSince),
		MessagesSince:     int(row.MessagesSince),
		LastCompactionID:  fromUUIDPtr(row.LastCompactionID),
		UpdatedAt:         row.UpdatedAt,
	}, nil
}

func (s *Store) UpsertUnit(ctx context.Context, u compaction.CompactionUnit) error {
	existing, err := s.GetUnit(ctx, u.ChannelID, u.ThreadRootID)
	if errors.Is(err, db.ErrNotFound) {
		id := u.ID
		if id == ids.Nil {
			id = ids.ID(uuid.New())
		}
		return s.q(ctx).InsertUnit(ctx, sqlcgen.InsertUnitParams{
			ID:                uuid.UUID(id),
			WorkspaceID:       uuid.UUID(u.WorkspaceID),
			ChannelID:         uuid.UUID(u.ChannelID),
			ThreadRootID:      uuidPtr(u.ThreadRootID),
			ApproxTokensSince: int32(u.ApproxTokensSince),
			MessagesSince:     int32(u.MessagesSince),
			LastCompactionID:  uuidPtr(u.LastCompactionID),
			UpdatedAt:         u.UpdatedAt,
		})
	}
	if err != nil {
		return err
	}
	return s.q(ctx).UpdateUnit(ctx, sqlcgen.UpdateUnitParams{
		ID:                uuid.UUID(existing.ID),
		ApproxTokensSince: int32(u.ApproxTokensSince),
		MessagesSince:     int32(u.MessagesSince),
		LastCompactionID:  uuidPtr(u.LastCompactionID),
		UpdatedAt:         u.UpdatedAt,
	})
}

func (s *Store) AddTokensToUnit(ctx context.Context, workspaceID, channelID, threadRootID ids.ID, tokens, msgs int, now time.Time) (compaction.CompactionUnit, error) {
	existing, err := s.GetUnit(ctx, channelID, threadRootID)
	if errors.Is(err, db.ErrNotFound) {
		unit := compaction.CompactionUnit{
			ID:                ids.ID(uuid.New()),
			WorkspaceID:       workspaceID,
			ChannelID:         channelID,
			ThreadRootID:      threadRootID,
			ApproxTokensSince: tokens,
			MessagesSince:     msgs,
			UpdatedAt:         now,
		}
		if err := s.q(ctx).InsertUnit(ctx, sqlcgen.InsertUnitParams{
			ID:                uuid.UUID(unit.ID),
			WorkspaceID:       uuid.UUID(workspaceID),
			ChannelID:         uuid.UUID(channelID),
			ThreadRootID:      uuidPtr(threadRootID),
			ApproxTokensSince: int32(tokens),
			MessagesSince:     int32(msgs),
			LastCompactionID:  nil,
			UpdatedAt:         now,
		}); err != nil {
			return compaction.CompactionUnit{}, err
		}
		return unit, nil
	}
	if err != nil {
		return compaction.CompactionUnit{}, err
	}
	existing.ApproxTokensSince += tokens
	existing.MessagesSince += msgs
	existing.UpdatedAt = now
	if err := s.q(ctx).UpdateUnit(ctx, sqlcgen.UpdateUnitParams{
		ID:                uuid.UUID(existing.ID),
		ApproxTokensSince: int32(existing.ApproxTokensSince),
		MessagesSince:     int32(existing.MessagesSince),
		LastCompactionID:  uuidPtr(existing.LastCompactionID),
		UpdatedAt:         now,
	}); err != nil {
		return compaction.CompactionUnit{}, err
	}
	return existing, nil
}

func (s *Store) ResetUnit(ctx context.Context, channelID, threadRootID, compactionID ids.ID, remainingTokens, remainingMsgs int, now time.Time) error {
	existing, err := s.GetUnit(ctx, channelID, threadRootID)
	if err != nil {
		return err
	}
	return s.q(ctx).UpdateUnit(ctx, sqlcgen.UpdateUnitParams{
		ID:                uuid.UUID(existing.ID),
		ApproxTokensSince: int32(remainingTokens),
		MessagesSince:     int32(remainingMsgs),
		LastCompactionID:  uuidPtr(compactionID),
		UpdatedAt:         now,
	})
}

func toDomainCompaction(row sqlcgen.Compaction) compaction.Compaction {
	return compaction.Compaction{
		ID:                   ids.ID(row.ID),
		WorkspaceID:          ids.ID(row.WorkspaceID),
		ChannelID:            ids.ID(row.ChannelID),
		ThreadRootID:         fromUUIDPtr(row.ThreadRootID),
		UpToMessageID:        fromUUIDPtr(row.UpToMessageID),
		PreviousCompactionID: fromUUIDPtr(row.PreviousCompactionID),
		Trigger:              compaction.Trigger(row.Trigger),
		RequestedBy:          fromUUIDPtr(row.RequestedBy),
		CompactorAgentID:     fromUUIDPtr(row.CompactorAgentID),
		State:                compaction.State(row.State),
		SummaryFileID:        fromUUIDPtr(row.SummaryFileID),
		SystemMessageID:      fromUUIDPtr(row.SystemMessageID),
		MessagesCount:        int(row.MessagesCount),
		ApproxTokensIn:       int(row.ApproxTokensIn),
		SummaryTokens:        int(row.SummaryTokens),
		ErrorCode:            stringFromPtr(row.ErrorCode),
		CreatedAt:            row.CreatedAt,
		CompletedAt:          row.CompletedAt,
	}
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringFromPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Store) InsertCompaction(ctx context.Context, c compaction.Compaction) error {
	return s.q(ctx).InsertCompaction(ctx, sqlcgen.InsertCompactionParams{
		ID:                   uuid.UUID(c.ID),
		WorkspaceID:          uuid.UUID(c.WorkspaceID),
		ChannelID:            uuid.UUID(c.ChannelID),
		ThreadRootID:         uuidPtr(c.ThreadRootID),
		UpToMessageID:        uuidPtr(c.UpToMessageID),
		PreviousCompactionID: uuidPtr(c.PreviousCompactionID),
		Trigger:              string(c.Trigger),
		RequestedBy:          uuidPtr(c.RequestedBy),
		CompactorAgentID:     uuidPtr(c.CompactorAgentID),
		State:                string(c.State),
		SummaryFileID:        uuidPtr(c.SummaryFileID),
		SystemMessageID:      uuidPtr(c.SystemMessageID),
		MessagesCount:        int32(c.MessagesCount),
		ApproxTokensIn:       int32(c.ApproxTokensIn),
		SummaryTokens:        int32(c.SummaryTokens),
		ErrorCode:            stringPtr(c.ErrorCode),
		CreatedAt:            c.CreatedAt,
		CompletedAt:          c.CompletedAt,
	})
}

func (s *Store) CompactionByID(ctx context.Context, id ids.ID) (compaction.Compaction, error) {
	row, err := s.q(ctx).CompactionByID(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return compaction.Compaction{}, db.ErrNotFound
	}
	if err != nil {
		return compaction.Compaction{}, err
	}
	return toDomainCompaction(row), nil
}

func (s *Store) LatestDoneCompaction(ctx context.Context, channelID, threadRootID ids.ID) (compaction.Compaction, error) {
	row, err := s.q(ctx).LatestDoneCompaction(ctx, sqlcgen.LatestDoneCompactionParams{
		ChannelID:    uuid.UUID(channelID),
		ThreadRootID: uuidPtr(threadRootID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return compaction.Compaction{}, db.ErrNotFound
	}
	if err != nil {
		return compaction.Compaction{}, err
	}
	return toDomainCompaction(row), nil
}

func (s *Store) PendingCompaction(ctx context.Context, channelID, threadRootID ids.ID) (compaction.Compaction, error) {
	row, err := s.q(ctx).PendingCompaction(ctx, sqlcgen.PendingCompactionParams{
		ChannelID:    uuid.UUID(channelID),
		ThreadRootID: uuidPtr(threadRootID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return compaction.Compaction{}, db.ErrNotFound
	}
	if err != nil {
		return compaction.Compaction{}, err
	}
	return toDomainCompaction(row), nil
}

func (s *Store) CompactionCounts(ctx context.Context, workspaceID ids.ID, since time.Time) (running, failed int64, err error) {
	row, err := s.q(ctx).CompactionCounts(ctx, sqlcgen.CompactionCountsParams{WorkspaceID: workspaceID, Since: &since})
	if err != nil {
		return 0, 0, err
	}
	return row.Running, row.Failed, nil
}

func (s *Store) FailPendingCompaction(ctx context.Context, id ids.ID, errorCode string, completedAt time.Time) error {
	return s.q(ctx).FailPendingCompaction(ctx, sqlcgen.FailPendingCompactionParams{
		ID:          uuid.UUID(id),
		ErrorCode:   stringPtr(errorCode),
		CompletedAt: &completedAt,
	})
}

func (s *Store) CompleteCompaction(ctx context.Context, id ids.ID, fileID, sysMsgID ids.ID, messagesCount, approxTokensIn, summaryTokens int, completedAt time.Time) error {
	return s.q(ctx).CompleteCompaction(ctx, sqlcgen.CompleteCompactionParams{
		ID:              uuid.UUID(id),
		SummaryFileID:   uuidPtr(fileID),
		SystemMessageID: uuidPtr(sysMsgID),
		MessagesCount:   int32(messagesCount),
		ApproxTokensIn:  int32(approxTokensIn),
		SummaryTokens:   int32(summaryTokens),
		CompletedAt:     &completedAt,
	})
}

func (s *Store) CompactionIDsBySystemMessageIDs(ctx context.Context, msgIDs []ids.ID) (map[ids.ID]ids.ID, error) {
	if len(msgIDs) == 0 {
		return map[ids.ID]ids.ID{}, nil
	}
	uuids := make([]uuid.UUID, len(msgIDs))
	for i, id := range msgIDs {
		uuids[i] = uuid.UUID(id)
	}
	rows, err := s.q(ctx).CompactionIDsBySystemMessageIDs(ctx, uuids)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ID]ids.ID, len(rows))
	for _, r := range rows {
		if r.SystemMessageID != nil {
			out[ids.ID(*r.SystemMessageID)] = ids.ID(r.ID)
		}
	}
	return out, nil
}

var _ compaction.Store = (*Store)(nil)
