package search_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/compaction"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/search"
	"colonyhall/internal/platform/ids"
)

// recordingStore is the part of the search store the summary handler uses;
// it counts the chunks written (the other methods are not called).
type recordingStore struct {
	search.Store
	chunks int
}

func (r *recordingStore) DeleteSummaries(context.Context, ids.ID, ids.ID) error { return nil }
func (r *recordingStore) ReplaceSource(_ context.Context, _ search.SourceKind, _ ids.ID, cs []search.Chunk) error {
	r.chunks += len(cs)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// oneChannel is a Conversation with a single public channel.
type oneChannel struct{ ch conversation.Channel }

func (o oneChannel) MessageByID(context.Context, ids.ID) (conversation.Message, error) {
	return conversation.Message{}, fmt.Errorf("not used")
}
func (o oneChannel) ChannelByID(context.Context, ids.ID) (conversation.Channel, error) {
	return o.ch, nil
}
func (o oneChannel) MemberIDs(context.Context, ids.ID) ([]ids.ID, error) { return nil, nil }
func (o oneChannel) ChannelIDsOf(context.Context, identity.Actor, ids.ID) ([]ids.ID, error) {
	return []ids.ID{o.ch.ID}, nil
}
func (o oneChannel) ListChannels(context.Context, identity.Actor, bool) ([]conversation.ChannelView, error) {
	return nil, nil
}

// A summary whose file is gone from the file store (a lost or unrestored
// file) is skipped: an error would make the event consumer retry that
// event forever, and nothing after it would be indexed (found on a copy of
// the production data, 2026-11-10).
func TestIndexerSkipsSummaryWithMissingFile(t *testing.T) {
	gen := &ids.Sequential{}
	ws := gen.New()
	ch := conversation.Channel{ID: gen.New(), WorkspaceID: ws, Kind: conversation.KindPublic, Name: "general"}
	lost, present := gen.New(), gen.New()
	store := &recordingStore{}
	x := &search.Indexer{
		Store: store,
		Conv:  oneChannel{ch: ch},
		Summaries: func(_ context.Context, fileID ids.ID) (string, error) {
			if fileID == lost {
				return "", &fs.PathError{Op: "open", Path: "/data/files/" + fileID.String(), Err: fs.ErrNotExist}
			}
			return "# Összefoglaló\nA projektek wikioldalai elkészültek.", nil
		},
		IDs:   gen,
		Clock: fixedClock{time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)},
	}
	event := func(fileID ids.ID) eventlog.Event {
		raw, _ := json.Marshal(compaction.CompactionDone{
			CompactionID: gen.New().String(), WorkspaceID: ws.String(), ChannelID: ch.ID.String(), SummaryFileID: fileID.String(),
		})
		return eventlog.Event{Type: compaction.EventDone, WorkspaceID: ws, Payload: raw, At: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	}
	if err := x.HandleEvent(context.Background(), event(lost)); err != nil {
		t.Fatalf("a missing summary file stalled the indexer: %v", err)
	}
	if err := x.HandleEvent(context.Background(), event(present)); err != nil {
		t.Fatal(err)
	}
	if store.chunks != 1 {
		t.Fatalf("want only the summary whose file exists indexed, got %d chunks", store.chunks)
	}
}
