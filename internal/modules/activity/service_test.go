// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package activity_test

import (
	"context"
	"testing"
	"time"

	"colonyhall/internal/modules/activity"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// pageStore returns its items as the page, newest first as stored; the
// other Store methods are not used by List.
type pageStore struct {
	activity.Store
	items []activity.Item
}

func (s *pageStore) List(context.Context, ids.ID, ids.ID, activity.Query) ([]activity.Item, error) {
	return s.items, nil
}

func (s *pageStore) CountUnread(context.Context, ids.ID, ids.ID) (int, error) {
	return len(s.items), nil
}

// countingConversation answers from maps and counts the calls the feed
// makes; the other methods are not used by List.
type countingConversation struct {
	activity.Conversation
	readable                           map[ids.ID]bool
	canRead, channel, message, batches int
}

func (c *countingConversation) CanRead(_ context.Context, _ identity.Actor, id ids.ID) (bool, error) {
	c.canRead++
	return c.readable[id], nil
}

func (c *countingConversation) ChannelByID(_ context.Context, id ids.ID) (conversation.Channel, error) {
	c.channel++
	return conversation.Channel{ID: id, Name: "c"}, nil
}

func (c *countingConversation) MessageByID(_ context.Context, id ids.ID) (conversation.Message, error) {
	c.message++
	return conversation.Message{ID: id}, nil
}

func (c *countingConversation) MessagesByIDs(_ context.Context, list []ids.ID) ([]conversation.Message, error) {
	c.batches++
	out := make([]conversation.Message, len(list))
	for i, id := range list {
		out[i] = conversation.Message{ID: id, Body: "m"}
	}
	return out, nil
}

// The feed page looks each conversation up once and loads its messages in
// one batch, however many items it has (it was two queries per item).
func TestListLooksUpEachConversationOnceAndBatchesMessages(t *testing.T) {
	gen := &ids.Sequential{}
	ws, me := gen.New(), gen.New()
	open1, open2, closed := gen.New(), gen.New(), gen.New()
	conv := &countingConversation{readable: map[ids.ID]bool{open1: true, open2: true}}
	store := &pageStore{}
	svc := &activity.Service{Store: store, Conversation: conv}
	ctx := context.Background()
	t0 := time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)

	channels := []ids.ID{open1, open1, open1, open2, open2, closed, closed}
	msgOf := map[ids.ID]ids.ID{}
	for i, ch := range channels {
		it := activity.Item{ID: gen.New(), WorkspaceID: ws, RecipientID: me, Kind: activity.KindMention,
			ChannelID: ch, MessageID: gen.New(), SourceSeq: int64(i + 1), CreatedAt: t0.Add(time.Duration(i) * time.Minute)}
		msgOf[it.ID] = it.MessageID
		store.items = append(store.items, it)
	}
	// One item with no conversation at all (e.g. a reminder of a lost place).
	store.items = append(store.items, activity.Item{ID: gen.New(), WorkspaceID: ws, RecipientID: me, Kind: activity.KindMention,
		SourceSeq: 99, CreatedAt: t0.Add(time.Hour)})

	views, _, _, err := svc.List(ctx, identity.Actor{ID: me, WorkspaceID: ws, Kind: identity.KindHuman}, activity.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 8 {
		t.Fatalf("got %d views, want 8", len(views))
	}
	if conv.canRead != 3 || conv.channel != 2 || conv.message != 0 || conv.batches != 1 {
		t.Fatalf("calls: CanRead %d, ChannelByID %d, MessageByID %d, MessagesByIDs %d; want 3, 2, 0, 1",
			conv.canRead, conv.channel, conv.message, conv.batches)
	}
	for _, v := range views {
		switch v.ChannelID {
		case ids.Nil, closed:
			// A lost (or no) conversation: only the kind and the time.
			if v.Channel != nil || v.Message != nil {
				t.Fatalf("item %v in an unreadable place shows %+v %+v", v.ID, v.Channel, v.Message)
			}
		default:
			if v.Channel == nil || v.Channel.ID != v.ChannelID || v.Message == nil || v.Message.ID != msgOf[v.ID] {
				t.Fatalf("item %v: channel %+v message %+v", v.ID, v.Channel, v.Message)
			}
		}
	}
}
