// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package gateway

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

type mockConvReader struct {
	canReadMap map[string]bool
}

func (m *mockConvReader) AgentLockedChannels(context.Context, ids.ID) ([]ids.ID, error) {
	return nil, nil
}

func (m *mockConvReader) MessageByID(_ context.Context, _ ids.ID) (conversation.Message, error) {
	return conversation.Message{}, nil
}
func (m *mockConvReader) MessagesByIDs(_ context.Context, _ []ids.ID) ([]conversation.Message, error) {
	return nil, nil
}
func (m *mockConvReader) MemberIDs(_ context.Context, _ ids.ID) ([]ids.ID, error) {
	return nil, nil
}
func (m *mockConvReader) ThreadAuthorIDs(_ context.Context, _ ids.ID) ([]ids.ID, error) {
	return nil, nil
}
func (m *mockConvReader) ChannelByID(_ context.Context, id ids.ID) (conversation.Channel, error) {
	return conversation.Channel{ID: id}, nil
}
func (m *mockConvReader) CanRead(_ context.Context, by identity.Actor, channelID ids.ID) (bool, error) {
	if m.canReadMap != nil {
		k := fmt.Sprintf("%s:%s", by.ID, channelID)
		if v, ok := m.canReadMap[k]; ok {
			return v, nil
		}
	}
	return true, nil
}
func (m *mockConvReader) CanWrite(_ context.Context, _ identity.Actor, _ ids.ID) (bool, error) {
	return true, nil
}
func (m *mockConvReader) MessagesInUnit(_ context.Context, _, _ ids.ID) ([]conversation.Message, error) {
	return nil, nil
}
func (m *mockConvReader) LatestTopLevelMessageByAuthor(_ context.Context, _, _ ids.ID) (conversation.Message, error) {
	return conversation.Message{}, nil
}
func (m *mockConvReader) PostSystemMessage(_ context.Context, _, _, _, _ ids.ID, _ string, _ []ids.ID) (conversation.Message, error) {
	return conversation.Message{}, nil
}

func newID() ids.ID {
	return ids.ID(uuid.New())
}

func TestGatewayProgressCapsAndExpiry(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	clk := clock.NewManual(now)
	reader := &mockConvReader{}

	gw := &Gateway{
		Conversation: reader,
		Clock:        clk,
	}

	wsID := newID()
	// The human owns the agent: only its owner (or an admin) sees its
	// steps (round 38).
	humanID := newID()
	agentActor := identity.Actor{
		ID:          newID(),
		WorkspaceID: wsID,
		Kind:        identity.KindAgent,
		OwnerID:     humanID,
	}
	humanActor := identity.Actor{
		ID:          humanID,
		WorkspaceID: wsID,
		Kind:        identity.KindHuman,
	}
	channelID := newID()

	// 1. Report 250 steps in batches of 100 -> capped at 200
	steps1 := make([]apispec.AgentStep, 150)
	for i := range steps1 {
		steps1[i] = apispec.AgentStep{
			Kind:      apispec.AgentStepKindRead,
			Label:     fmt.Sprintf("step %d", i),
			State:     apispec.AgentStepStateDone,
			StartedAt: now,
		}
	}
	err := gw.ReportProgress(ctx, agentActor, apispec.AgentProgressRequest{
		ChannelId: openapi_types.UUID(channelID),
		RunKey:    "run-1",
		Steps:     steps1,
	})
	if err != nil {
		t.Fatalf("first report failed: %v", err)
	}

	steps2 := make([]apispec.AgentStep, 100)
	for i := range steps2 {
		steps2[i] = apispec.AgentStep{
			Kind:      apispec.AgentStepKindCommand,
			Label:     fmt.Sprintf("step %d", i+150),
			State:     apispec.AgentStepStateDone,
			StartedAt: now,
		}
	}
	err = gw.ReportProgress(ctx, agentActor, apispec.AgentProgressRequest{
		ChannelId: openapi_types.UUID(channelID),
		RunKey:    "run-1",
		Steps:     steps2,
	})
	if err != nil {
		t.Fatalf("second report failed: %v", err)
	}

	// Verify steps capped at 200
	runs, err := gw.ChannelProgress(ctx, humanActor, channelID)
	if err != nil {
		t.Fatalf("channel progress failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 active run, got %d", len(runs))
	}
	if len(runs[0].Steps) != 200 {
		t.Fatalf("expected 200 steps capped, got %d", len(runs[0].Steps))
	}
	if runs[0].Done != nil && *runs[0].Done {
		t.Fatalf("expected run not done")
	}

	// 2. Mark run-1 as done
	isDone := true
	err = gw.ReportProgress(ctx, agentActor, apispec.AgentProgressRequest{
		ChannelId: openapi_types.UUID(channelID),
		RunKey:    "run-1",
		Steps:     nil,
		Done:      &isDone,
	})
	if err != nil {
		t.Fatalf("mark done failed: %v", err)
	}

	// Advance clock by 1 minute: still within 2 minutes after done
	clk.Advance(1 * time.Minute)
	runs, err = gw.ChannelProgress(ctx, humanActor, channelID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected run-1 retained at 1 min after done, got %d runs (err: %v)", len(runs), err)
	}

	// Advance clock past 2 minutes (e.g. 1m 1s more -> 2m 1s total after done)
	clk.Advance(61 * time.Second)
	runs, err = gw.ChannelProgress(ctx, humanActor, channelID)
	if err != nil || len(runs) != 0 {
		t.Fatalf("expected run-1 pruned 2 minutes after done, got %d runs", len(runs))
	}

	// 3. Test 15-minute idle expiry without done
	err = gw.ReportProgress(ctx, agentActor, apispec.AgentProgressRequest{
		ChannelId: openapi_types.UUID(channelID),
		RunKey:    "run-2",
		Steps: []apispec.AgentStep{{
			Kind:      apispec.AgentStepKindThink,
			Label:     "thinking",
			State:     apispec.AgentStepStateActive,
			StartedAt: clk.Now(),
		}},
	})
	if err != nil {
		t.Fatalf("run-2 report failed: %v", err)
	}

	// Advance clock by 14 minutes -> still present
	clk.Advance(14 * time.Minute)
	runs, err = gw.ChannelProgress(ctx, humanActor, channelID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected run-2 present at 14 minutes idle, got %d runs", len(runs))
	}

	// Advance clock past 15 minutes total -> pruned
	clk.Advance(2 * time.Minute)
	runs, err = gw.ChannelProgress(ctx, humanActor, channelID)
	if err != nil || len(runs) != 0 {
		t.Fatalf("expected run-2 pruned after 15 minutes idle, got %d runs", len(runs))
	}
}

func TestGatewayProgressChannelPermissions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	clk := clock.NewManual(now)

	wsID := newID()
	agentID := newID()
	channelID := newID()

	reader := &mockConvReader{
		canReadMap: map[string]bool{
			fmt.Sprintf("%s:%s", agentID, channelID): false, // Agent cannot read this channel
		},
	}
	gw := &Gateway{
		Conversation: reader,
		Clock:        clk,
	}

	// The human owns the agent: only its owner (or an admin) sees its
	// steps (round 38).
	humanID := newID()
	agentActor := identity.Actor{
		ID:          agentID,
		WorkspaceID: wsID,
		Kind:        identity.KindAgent,
		OwnerID:     humanID,
	}
	humanActor := identity.Actor{
		ID:          humanID,
		WorkspaceID: wsID,
		Kind:        identity.KindHuman,
	}

	// Agent without channel access -> 403 Forbidden
	err := gw.ReportProgress(ctx, agentActor, apispec.AgentProgressRequest{
		ChannelId: openapi_types.UUID(channelID),
		RunKey:    "run-denied",
		Steps:     nil,
	})
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for agent without channel access, got: %v", err)
	}

	// Human without channel access -> 403 Forbidden on ChannelProgress
	reader.canReadMap[fmt.Sprintf("%s:%s", humanActor.ID, channelID)] = false
	_, err = gw.ChannelProgress(ctx, humanActor, channelID)
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for human without channel access, got: %v", err)
	}

	// Agent actor calling ChannelProgress -> 403 Forbidden (only humans allowed)
	_, err = gw.ChannelProgress(ctx, agentActor, channelID)
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for agent calling ChannelProgress, got: %v", err)
	}
}

func TestGatewayPushFsRequest(t *testing.T) {
	gw := &Gateway{}
	ag1 := newID()
	ag2 := newID()

	// No connections: returns false
	if gw.PushFsRequest(ag1, "req-none") {
		t.Fatal("expected false when no connections open")
	}

	c1 := &connection{
		actor: identity.Actor{ID: ag1, Kind: identity.KindAgent},
	}
	c2 := &connection{
		actor: identity.Actor{ID: ag2, Kind: identity.KindAgent},
	}
	chuman := &connection{
		actor: identity.Actor{ID: newID(), Kind: identity.KindHuman},
	}

	gw.register(c1)
	gw.register(c2)
	gw.register(chuman)

	gw.mu.Lock()
	var targets []*connection
	for c := range gw.conns {
		if c.actor.Kind == identity.KindAgent && c.actor.ID == ag1 {
			targets = append(targets, c)
		}
	}
	gw.mu.Unlock()

	if len(targets) != 1 || targets[0] != c1 {
		t.Fatalf("expected only c1 targeted for ag1, got: %v", targets)
	}
}
