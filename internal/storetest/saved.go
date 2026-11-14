// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/saved"
	"colonyhall/internal/platform/db"
)

// testSaved: personal items, the list filters and pages, and the reminder
// claim that sends a due reminder once (round 55, part E).
func testSaved(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Saved
	m := conversation.Message{ID: gen.New(), WorkspaceID: f.ws.ID, ChannelID: f.channel.ID, AuthorID: f.human.ID, Kind: "text", Body: "x", CreatedAt: t0}
	must(t, b.Conversation.InsertMessage(ctx, m))

	at := t0.Add(time.Hour)
	a := saved.Item{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, MessageID: m.ID, ChannelID: f.channel.ID, CreatedAt: t0}
	must(t, st.Insert(ctx, a))
	if err := st.Insert(ctx, saved.Item{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, MessageID: m.ID, ChannelID: f.channel.ID, CreatedAt: t0}); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("one item per message and person: %v", err)
	}
	r := saved.Item{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.human.ID, Note: "call", RemindAt: &at, CreatedAt: t0}
	must(t, st.Insert(ctx, r))
	got, err := st.ByMessage(ctx, f.human.ID, m.ID)
	must(t, err)
	if got.ID != a.ID {
		t.Fatalf("by message: %+v", got)
	}

	list, err := st.List(ctx, f.ws.ID, f.human.ID, saved.FilterReminders, nil, 10)
	must(t, err)
	if len(list) != 1 || list[0].ID != r.ID {
		t.Fatalf("reminders filter: %+v", list)
	}
	// Same creation time: the cursor still pages through both.
	page, err := st.List(ctx, f.ws.ID, f.human.ID, saved.FilterAll, nil, 1)
	must(t, err)
	rest, err := st.List(ctx, f.ws.ID, f.human.ID, saved.FilterAll, &saved.Cursor{At: page[0].CreatedAt, ID: page[0].ID}, 10)
	must(t, err)
	if len(page) != 1 || len(rest) != 1 || rest[0].ID == page[0].ID {
		t.Fatalf("paging: %+v then %+v", page, rest)
	}

	sys := ctx // the loop runs in system mode; one workspace's rows are enough here
	if due, err := st.Due(sys, t0, 10); err != nil || len(due) != 0 {
		t.Fatalf("due before its time: %+v %v", due, err)
	}
	due, err := st.Due(sys, at, 10)
	must(t, err)
	if len(due) != 1 || due[0].ID != r.ID {
		t.Fatalf("due: %+v", due)
	}
	// Many claimers, one winner.
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var ok bool
			err := b.Tx.InTx(ctx, func(ctx context.Context) error {
				var err error
				ok, err = st.ClaimReminder(ctx, r.ID, at)
				return err
			})
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("claimed %d times", won)
	}
	if due, _ := st.Due(sys, at.Add(time.Hour), 10); len(due) != 0 {
		t.Fatalf("still due after the claim: %+v", due)
	}
	// Moving the reminder arms it again; a done item is never due.
	later := at.Add(2 * time.Hour)
	r.RemindAt, r.RemindedAt = &later, nil
	must(t, st.Update(ctx, r))
	if due, _ := st.Due(sys, later, 10); len(due) != 1 {
		t.Fatalf("re-armed: %+v", due)
	}
	r.DoneAt = &later
	must(t, st.Update(ctx, r))
	if ok, _ := st.ClaimReminder(ctx, r.ID, later); ok {
		t.Fatal("a done reminder was claimed")
	}
	n, err := st.Count(ctx, f.ws.ID, f.human.ID)
	must(t, err)
	if n != 2 {
		t.Fatalf("count: %d", n)
	}
	must(t, st.Delete(ctx, a.ID))
	if _, err := st.Get(ctx, a.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
