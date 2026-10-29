package eventlog_test

import (
	"context"
	"testing"
	"time"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/eventlog/memory"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// A handler that appends an event runs inside the consumer's transaction;
// the consumer must announce the new event after the commit, or a reader
// such as the WebSocket gateway would not see it until an unrelated signal.
func TestConsumerAnnouncesEventsItsHandlerAppended(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	tx := &db.Memory{}
	b := bus.NewInProc()
	gen := &ids.Sequential{}
	ws := gen.New()
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

	appendEvent := func(ctx context.Context, typ string) {
		t.Helper()
		e, err := eventlog.NewEvent(gen.New(), ws, typ, gen.New(), ids.Nil, "test", gen.New(), map[string]string{}, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.InTx(ctx, func(ctx context.Context) error { return store.Append(ctx, &e) }); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(ctx, "source.happened")

	c := &eventlog.Consumer{Name: "reactor", Store: store, Tx: tx, Bus: b,
		Handle: func(ctx context.Context, e eventlog.Event) error {
			if e.Type == "source.happened" {
				appendEvent(ctx, "reaction.happened") // no Notify of its own
			}
			return nil
		}}

	signals, unsubscribe := b.Subscribe()
	defer unsubscribe()
	if err := c.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	head, _ := store.Head(ctx)
	select {
	case seq := <-signals:
		if seq != head {
			t.Fatalf("announced seq %d, want head %d", seq, head)
		}
	default:
		t.Fatal("the event appended by the handler was never announced")
	}

	// Nothing new: no further signal, so consumers do not wake each other forever.
	if err := c.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case seq := <-signals:
		t.Fatalf("unexpected signal %d after an empty drain", seq)
	default:
	}
}
