// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/modules/delegation/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements delegation.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func uuidPtr(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromUUIDPtr(u *uuid.UUID) *ids.ID {
	if u == nil {
		return nil
	}
	id := ids.ID(*u)
	return &id
}

func toModel(r sqlcgen.Delegation) delegation.Delegation {
	d := delegation.Delegation{
		ID:          ids.ID(r.ID),
		WorkspaceID: ids.ID(r.WorkspaceID),
		MessageID:   ids.ID(r.MessageID),
		ChannelID:   ids.ID(r.ChannelID),
		FromAgentID: ids.ID(r.FromAgentID),
		ToAgentID:   ids.ID(r.ToAgentID),
		QuestionID:  ids.ID(r.QuestionID),
		State:       r.State,
		DecidedBy:   fromUUIDPtr(r.DecidedBy),
		DecidedAt:   r.DecidedAt,
		ViaGrant:    r.ViaGrant,
		CreatedAt:   r.CreatedAt,
	}
	if r.ThreadRootID != nil {
		d.ThreadRootID = ids.ID(*r.ThreadRootID)
	}
	return d
}

func toGrant(r sqlcgen.DelegationGrant) delegation.Grant {
	g := delegation.Grant{
		ID:           ids.ID(r.ID),
		WorkspaceID:  ids.ID(r.WorkspaceID),
		ChannelID:    ids.ID(r.ChannelID),
		AgentAID:     ids.ID(r.AgentAID),
		AgentBID:     ids.ID(r.AgentBID),
		DelegationID: ids.ID(r.DelegationID),
		ApprovedBy:   ids.ID(r.ApprovedBy),
		ApprovedAt:   r.ApprovedAt,
		LastUsedAt:   r.LastUsedAt,
		ExpiresAt:    r.ExpiresAt,
		RevokedBy:    fromUUIDPtr(r.RevokedBy),
		RevokedAt:    r.RevokedAt,
	}
	if r.ThreadRootID != nil {
		g.ThreadRootID = ids.ID(*r.ThreadRootID)
	}
	return g
}

func (s *Store) Insert(ctx context.Context, d delegation.Delegation) error {
	_, err := s.q(ctx).InsertDelegation(ctx, sqlcgen.InsertDelegationParams{
		ID:           uuid.UUID(d.ID),
		WorkspaceID:  uuid.UUID(d.WorkspaceID),
		MessageID:    uuid.UUID(d.MessageID),
		ChannelID:    uuid.UUID(d.ChannelID),
		ThreadRootID: uuidPtr(d.ThreadRootID),
		FromAgentID:  uuid.UUID(d.FromAgentID),
		ToAgentID:    uuid.UUID(d.ToAgentID),
		QuestionID:   uuid.UUID(d.QuestionID),
		State:        d.State,
		CreatedAt:    d.CreatedAt,
	})
	return err
}

func (s *Store) GetByID(ctx context.Context, id ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByID(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) GetByQuestionID(ctx context.Context, questionID ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByQuestionID(ctx, uuid.UUID(questionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) GetByMessageAndTarget(ctx context.Context, messageID, toAgentID ids.ID) (delegation.Delegation, error) {
	r, err := s.q(ctx).GetDelegationByMessageAndTarget(ctx, sqlcgen.GetDelegationByMessageAndTargetParams{
		MessageID: uuid.UUID(messageID),
		ToAgentID: uuid.UUID(toAgentID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Delegation{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Delegation{}, err
	}
	return toModel(r), nil
}

func (s *Store) ListByMessageID(ctx context.Context, messageID ids.ID) ([]delegation.Delegation, error) {
	rows, err := s.q(ctx).ListDelegationsByMessageID(ctx, uuid.UUID(messageID))
	if err != nil {
		return nil, err
	}
	out := make([]delegation.Delegation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

func (s *Store) UpdateStateConditional(ctx context.Context, id ids.ID, expectedState, newState string, decidedBy *ids.ID, decidedAt time.Time) (int64, error) {
	var decBy *uuid.UUID
	if decidedBy != nil && *decidedBy != ids.Nil {
		u := uuid.UUID(*decidedBy)
		decBy = &u
	}
	return s.q(ctx).UpdateDelegationStateConditional(ctx, sqlcgen.UpdateDelegationStateConditionalParams{
		ID:        uuid.UUID(id),
		State:     expectedState,
		State_2:   newState,
		DecidedBy: decBy,
		DecidedAt: &decidedAt,
	})
}

func (s *Store) ApproveViaGrant(ctx context.Context, id ids.ID, decidedBy ids.ID, decidedAt time.Time) (int64, error) {
	by := uuid.UUID(decidedBy)
	return s.q(ctx).ApproveDelegationViaGrant(ctx, sqlcgen.ApproveDelegationViaGrantParams{
		ID:        uuid.UUID(id),
		DecidedBy: &by,
		DecidedAt: &decidedAt,
	})
}

func (s *Store) ListByWorkspace(ctx context.Context, workspaceID ids.ID, state *string) ([]delegation.Delegation, error) {
	rows, err := s.q(ctx).ListDelegationsByWorkspace(ctx, sqlcgen.ListDelegationsByWorkspaceParams{
		WorkspaceID: uuid.UUID(workspaceID),
		State:       state,
	})
	if err != nil {
		return nil, err
	}
	out := make([]delegation.Delegation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

func (s *Store) ListPendingByPairAndConversation(ctx context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID) ([]delegation.Delegation, error) {
	rows, err := s.q(ctx).ListPendingByPairAndConversation(ctx, sqlcgen.ListPendingByPairAndConversationParams{
		WorkspaceID:  uuid.UUID(workspaceID),
		FromAgentID:  uuid.UUID(agentA),
		ToAgentID:    uuid.UUID(agentB),
		ChannelID:    uuid.UUID(channelID),
		ThreadRootID: uuidPtr(threadRootID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]delegation.Delegation, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

func (s *Store) InsertGrant(ctx context.Context, g delegation.Grant) error {
	return s.q(ctx).InsertGrant(ctx, sqlcgen.InsertGrantParams{
		ID:           uuid.UUID(g.ID),
		WorkspaceID:  uuid.UUID(g.WorkspaceID),
		ChannelID:    uuid.UUID(g.ChannelID),
		ThreadRootID: uuidPtr(g.ThreadRootID),
		AgentAID:     uuid.UUID(g.AgentAID),
		AgentBID:     uuid.UUID(g.AgentBID),
		DelegationID: uuid.UUID(g.DelegationID),
		ApprovedBy:   uuid.UUID(g.ApprovedBy),
		ApprovedAt:   g.ApprovedAt,
		LastUsedAt:   g.LastUsedAt,
		ExpiresAt:    g.ExpiresAt,
	})
}

func (s *Store) GetGrantByID(ctx context.Context, id ids.ID) (delegation.Grant, error) {
	r, err := s.q(ctx).GetGrantByID(ctx, uuid.UUID(id))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Grant{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Grant{}, err
	}
	return toGrant(r), nil
}

func (s *Store) GetActiveGrant(ctx context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID, now time.Time) (delegation.Grant, error) {
	canonicalA, canonicalB := delegation.CanonicalPair(agentA, agentB)
	r, err := s.q(ctx).GetActiveGrantByPairAndConversation(ctx, sqlcgen.GetActiveGrantByPairAndConversationParams{
		WorkspaceID:  uuid.UUID(workspaceID),
		AgentAID:     uuid.UUID(canonicalA),
		AgentBID:     uuid.UUID(canonicalB),
		ChannelID:    uuid.UUID(channelID),
		ThreadRootID: uuidPtr(threadRootID),
		Now:          now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Grant{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Grant{}, err
	}
	return toGrant(r), nil
}

func (s *Store) UpdateGrantLastUsed(ctx context.Context, id ids.ID, lastUsedAt time.Time) error {
	_, err := s.q(ctx).UpdateGrantLastUsed(ctx, sqlcgen.UpdateGrantLastUsedParams{
		ID:         uuid.UUID(id),
		LastUsedAt: lastUsedAt,
	})
	return err
}

func (s *Store) RevokeGrantConditional(ctx context.Context, id ids.ID, revokedBy ids.ID, revokedAt time.Time) (int64, error) {
	return s.q(ctx).RevokeGrantConditional(ctx, sqlcgen.RevokeGrantConditionalParams{
		ID:        uuid.UUID(id),
		RevokedBy: uuidPtr(revokedBy),
		RevokedAt: revokedAt,
	})
}

func (s *Store) ListGrants(ctx context.Context, workspaceID ids.ID, activeOnly bool, now time.Time) ([]delegation.Grant, error) {
	rows, err := s.q(ctx).ListGrantsByWorkspace(ctx, sqlcgen.ListGrantsByWorkspaceParams{
		WorkspaceID: uuid.UUID(workspaceID),
		ActiveOnly:  activeOnly,
		Now:         now,
	})
	if err != nil {
		return nil, err
	}
	out := make([]delegation.Grant, 0, len(rows))
	for _, r := range rows {
		out = append(out, toGrant(r))
	}
	return out, nil
}

func (s *Store) GetSettings(ctx context.Context, workspaceID ids.ID) (delegation.Settings, error) {
	r, err := s.q(ctx).GetSettings(ctx, uuid.UUID(workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return delegation.Settings{}, db.ErrNotFound
	}
	if err != nil {
		return delegation.Settings{}, err
	}
	return delegation.Settings{
		WorkspaceID:      ids.ID(r.WorkspaceID),
		GrantIdleMinutes: int(r.GrantIdleMinutes),
		GrantMaxHours:    int(r.GrantMaxHours),
		UpdatedAt:        r.UpdatedAt,
	}, nil
}

func (s *Store) UpsertSettings(ctx context.Context, st delegation.Settings) error {
	return s.q(ctx).UpsertSettings(ctx, sqlcgen.UpsertSettingsParams{
		WorkspaceID:      uuid.UUID(st.WorkspaceID),
		GrantIdleMinutes: int32(st.GrantIdleMinutes),
		GrantMaxHours:    int32(st.GrantMaxHours),
		UpdatedAt:        st.UpdatedAt,
	})
}
