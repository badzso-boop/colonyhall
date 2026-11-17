// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package extrunner runs the Workshop's code tools (round 56b,
// docs/25-onfejlesztes-terv.md 13.4–13.5): JavaScript on QuickJS compiled
// to WebAssembly, run by wazero, in its own process (COLONY_ROLE=ext-runner).
//
// Every call gets a fresh runtime: no state survives a call. The script
// sees no network, no environment, no clock beyond the wall time, and as
// its file system an empty temporary directory removed after the call
// (QuickJS's std and os modules are present but find nothing). Its memory
// is capped by the module's declared maximum (capMemory) and its time by
// the context. Everything else goes through the host function, which the
// server answers in the caller's workspace with the caller's rights.
package extrunner

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Limits bound one call.
type Limits struct {
	Timeout     time.Duration
	OutputBytes int
	HostCalls   int
}

// DefaultLimits are docs/25 13.4's numbers (the server's owner changes
// them with COLONY_EXT_*).
var DefaultLimits = Limits{Timeout: 10 * time.Second, OutputBytes: 256 * 1024, HostCalls: 50}

// Job is one call of a code tool.
type Job struct {
	Bundle string          // the compiled script (an IIFE that sets globalThis.__ext)
	Fn     string          // the exported function to call
	Input  json.RawMessage // the tool's input
	Limits Limits
}

// Host answers the script's host calls ("fetch", "store", "colony",
// "call", "log"); the payload and the answer are JSON.
type Host func(ctx context.Context, kind string, payload json.RawMessage) (json.RawMessage, error)

// Result is a call's outcome. Logs are the script's ctx.log lines.
type Result struct {
	Output json.RawMessage `json:"output"`
	Logs   []string        `json:"logs,omitempty"`
}

// Errors the server maps to outcomes.
var (
	ErrTimeout   = errors.New("the tool ran out of time")
	ErrMemory    = errors.New("the tool ran out of memory")
	ErrOutput    = errors.New("the tool's answer is too large")
	ErrHostCalls = errors.New("the tool made too many host calls")
	ErrBusy      = errors.New("the code runner is busy; try again")
	ErrNoRunner  = errors.New("the code runner is not available (COLONY_EXT_RUNNER_URL)")
)

// Engine runs jobs.
type Engine interface {
	Run(ctx context.Context, job Job, host Host) (Result, error)
}
