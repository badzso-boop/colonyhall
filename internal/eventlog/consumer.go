package eventlog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/db"
)

// Handler processes one event. It runs inside the transaction that also
// advances the consumer's offset, so its writes and the offset commit together.
type Handler func(ctx context.Context, e Event) error

// Consumer reads the event log from its stored offset and hands each event
// to a Handler. Restarting resumes exactly where it stopped.
type Consumer struct {
	Name     string
	Store    Store
	Tx       db.TxRunner
	Bus      bus.Bus
	Handle   Handler
	Logger   *slog.Logger
	Batch    int           // events per read; default 100
	Fallback time.Duration // poll interval when no signal arrives; default 5s
}

// Run processes events until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	signals, unsubscribe := c.Bus.Subscribe()
	defer unsubscribe()
	fallback := c.Fallback
	if fallback == 0 {
		fallback = 5 * time.Second
	}
	ticker := time.NewTicker(fallback)
	defer ticker.Stop()
	for {
		if err := c.Drain(ctx); err != nil && ctx.Err() == nil {
			c.logger().Error("consumer failed; retrying", "consumer", c.Name, "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-signals:
		case <-ticker.C:
		}
	}
}

// Drain processes every event available right now. Tests call it directly
// to make asynchronous consumers deterministic.
func (c *Consumer) Drain(ctx context.Context) error {
	batch := c.Batch
	if batch == 0 {
		batch = 100
	}
	for {
		offset, err := c.Store.Offset(ctx, c.Name)
		if err != nil {
			return fmt.Errorf("%s: read offset: %w", c.Name, err)
		}
		events, err := c.Store.Since(ctx, offset, batch)
		if err != nil {
			return fmt.Errorf("%s: read events: %w", c.Name, err)
		}
		if len(events) == 0 {
			return nil
		}
		for _, e := range events {
			err := c.Tx.InTx(ctx, func(ctx context.Context) error {
				if err := c.Handle(ctx, e); err != nil {
					return err
				}
				return c.Store.SetOffset(ctx, c.Name, e.Seq)
			})
			if err != nil {
				return fmt.Errorf("%s: event %d (%s): %w", c.Name, e.Seq, e.Type, err)
			}
		}
	}
}

func (c *Consumer) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}
