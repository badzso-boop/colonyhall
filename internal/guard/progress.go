// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// progressReporter collects steps during a run, batches them at most once per second,
// and reports them to the server via POST /api/v1/agent/progress.
type progressReporter struct {
	client       *Client
	channelID    string
	threadRootID string
	runKey       string
	log          *slog.Logger

	mu       sync.Mutex
	allSteps []Step
	pending  []Step

	stop     chan struct{}
	doneOnce sync.Once
}

func newProgressReporter(client *Client, channelID, threadRootID, runKey string, log *slog.Logger) *progressReporter {
	pr := &progressReporter{
		client:       client,
		channelID:    channelID,
		threadRootID: threadRootID,
		runKey:       runKey,
		log:          log,
		stop:         make(chan struct{}),
	}
	if client != nil && channelID != "" {
		go pr.loop()
	}
	return pr
}

// OnStep records a step and queues it for the next batch.
func (pr *progressReporter) OnStep(s Step) {
	if pr == nil {
		return
	}
	pr.mu.Lock()
	defer pr.mu.Unlock()
	pr.pending = append(pr.pending, s)
	if len(pr.allSteps) < 200 {
		pr.allSteps = append(pr.allSteps, s)
	}
}

func (pr *progressReporter) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-pr.stop:
			return
		case <-ticker.C:
			pr.flush(false)
		}
	}
}

func (pr *progressReporter) flush(done bool) {
	if pr.client == nil || pr.channelID == "" {
		return
	}
	pr.mu.Lock()
	if len(pr.pending) == 0 && !done {
		pr.mu.Unlock()
		return
	}
	toSend := pr.pending
	pr.pending = nil
	pr.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pr.client.ReportProgress(ctx, pr.channelID, pr.threadRootID, pr.runKey, toSend, done); err != nil {
		if pr.log != nil {
			pr.log.Warn("guard: progress report failed", "run_key", pr.runKey, "err", err)
		}
	}
}

// Finish stops the background batcher, flushes remaining steps with done=true,
// and returns all steps collected during the run (capped at 200).
func (pr *progressReporter) Finish() []Step {
	if pr == nil {
		return nil
	}
	pr.doneOnce.Do(func() {
		close(pr.stop)
	})
	pr.flush(true)

	pr.mu.Lock()
	defer pr.mu.Unlock()
	steps := make([]Step, len(pr.allSteps))
	copy(steps, pr.allSteps)
	return steps
}
