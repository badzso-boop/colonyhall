// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Command fake-agent answers Colonyhall inbox items from a YAML scenario.
// Phase 0 has no guard yet, so it polls the inbox on an interval.
//
//	fake-agent -server http://localhost:8080 -token col_… -scenario scenarios/ping.yaml
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"colonyhall/internal/fakeagent"
)

func main() {
	server := flag.String("server", "http://localhost:8080", "Colonyhall base URL")
	token := flag.String("token", os.Getenv("COLONY_TOKEN"), "agent token (or COLONY_TOKEN)")
	scenarioPath := flag.String("scenario", "scenarios/ping.yaml", "scenario file")
	interval := flag.Duration("interval", 2*time.Second, "inbox poll interval")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	scenario, err := fakeagent.Load(*scenarioPath)
	if err != nil {
		logger.Error("load scenario", "err", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	session, err := fakeagent.Connect(ctx, strings.TrimRight(*server, "/")+"/mcp", *token, nil)
	if err != nil {
		logger.Error("connect", "err", err)
		os.Exit(1)
	}
	defer func() { _ = session.Close() }()
	agent := &fakeagent.Agent{Session: session, Scenario: scenario}
	logger.Info("fake agent running", "agent", scenario.Agent, "server", *server)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()
	for {
		n, err := agent.RunOnce(ctx)
		if err != nil {
			logger.Error("run", "err", err)
		} else if n > 0 {
			logger.Info("handled inbox items", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
