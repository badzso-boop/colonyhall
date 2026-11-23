// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fastschema/qjs"
)

// The QuickJS module the qjs package embeds; it is read once and capped.
var (
	wasmOnce  sync.Once
	wasmBytes []byte
	wasmErr   error
)

func qjsModule(memoryMB int) ([]byte, error) {
	wasmOnce.Do(func() {
		var raw []byte
		raw, wasmErr = qjsWasm()
		if wasmErr == nil {
			wasmBytes, wasmErr = capMemory(raw, uint64(memoryMB)*1024*1024/PageSize)
		}
		if wasmErr == nil {
			wasmErr = precompile(wasmBytes, memoryMB)
		}
	})
	return wasmBytes, wasmErr
}

// precompile has the qjs package compile the module once, with a context
// that is never cancelled. The package caches the compiled module process-wide
// (keyed by the bytes), so the compilation (seconds under load or the race
// detector) is not counted against the first tool's time limit.
func precompile(module []byte, memoryMB int) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("compile the QuickJS module: %v", p)
		}
	}()
	dir, err := os.MkdirTemp("", "colonyhall-ext-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	// The same Option shape as Run (CloseOnContextDone is part of the cached runtime config).
	rt, err := qjs.New(qjs.Option{QuickJSWasmBytes: module, CWD: dir, Context: context.Background(), CloseOnContextDone: true,
		MemoryLimit: memoryMB * 1024 * 1024, Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		return fmt.Errorf("compile the QuickJS module: %w", err)
	}
	rt.Close()
	return nil
}

// prelude builds the ctx object the tool function gets. The host calls go
// through one synchronous Go function; JSON in, JSON out.
const prelude = `
const __call = (kind, payload) => {
  const r = JSON.parse(__host(kind, JSON.stringify(payload === undefined ? null : payload)));
  if (r.error) throw new Error(r.error);
  return r.value;
};
const __ctx = Object.freeze({
  fetch: async (url, opts) => __call("fetch", Object.assign({ url: String(url) }, opts || {})),
  store: Object.freeze({
    get: async (collection, id) => __call("store", { op: "get", collection, id }),
    put: async (collection, id, data) => __call("store", { op: "put", collection, id, data }),
    delete: async (collection, id) => __call("store", { op: "delete", collection, id }),
    list: async (collection, opts) => __call("store", Object.assign({ op: "list", collection }, opts || {})),
  }),
  colony: new Proxy({}, { get: (_, tool) => async (input) => __call("colony", { tool: String(tool), input: input || {} }) }),
  call: async (tool, input) => __call("call", { tool: String(tool), input: input || {} }),
  log: (...args) => { __host("log", JSON.stringify(args.map((a) => typeof a === "string" ? a : JSON.stringify(a)).join(" "))); },
});
`

// QJS is the Engine on fastschema/qjs. MemoryMB is fixed for the process
// (the module's declared maximum); Slots bounds concurrent calls.
type QJS struct {
	MemoryMB int
	slots    chan struct{}
	once     sync.Once
	Slots    int
}

func (q *QJS) init() {
	q.once.Do(func() {
		n := q.Slots
		if n <= 0 {
			n = 16
		}
		q.slots = make(chan struct{}, n)
	})
}

// Run implements Engine.
func (q *QJS) Run(ctx context.Context, job Job, host Host) (res Result, err error) {
	q.init()
	select {
	case q.slots <- struct{}{}:
		defer func() { <-q.slots }()
	default:
		return Result{}, ErrBusy
	}
	mem := q.MemoryMB
	if mem <= 0 {
		mem = 64
	}
	module, err := qjsModule(mem)
	if err != nil {
		return Result{}, err
	}
	if !validFn(job.Fn) {
		return Result{}, fmt.Errorf("invalid function name %q", job.Fn)
	}
	lim := job.Limits
	if lim.Timeout <= 0 {
		lim.Timeout = DefaultLimits.Timeout
	}
	if lim.OutputBytes <= 0 {
		lim.OutputBytes = DefaultLimits.OutputBytes
	}
	if lim.HostCalls <= 0 {
		lim.HostCalls = DefaultLimits.HostCalls
	}
	dir, err := os.MkdirTemp("", "colonyhall-ext-")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	cctx, cancel := context.WithTimeout(ctx, lim.Timeout)
	defer cancel()

	var calls atomic.Int32
	var logs []string
	var hostErr error
	defer func() {
		if p := recover(); p != nil {
			err = classify(cctx, fmt.Errorf("%v", p))
			res = Result{}
		}
	}()

	rt, err := qjs.New(qjs.Option{QuickJSWasmBytes: module, CWD: dir, Context: cctx, CloseOnContextDone: true,
		MemoryLimit: mem * 1024 * 1024, Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		return Result{}, classify(cctx, err)
	}
	defer func() {
		defer func() { _ = recover() }() // closing a runtime the context closed panics
		rt.Close()
	}()
	jc := rt.Context()
	jc.SetFunc("__host", func(this *qjs.This) (*qjs.Value, error) {
		args := this.Args()
		if len(args) < 2 {
			return nil, errors.New("__host needs two arguments")
		}
		kind, payload := args[0].String(), args[1].String()
		if kind == "log" {
			if len(logs) < 50 {
				var line string
				if json.Unmarshal([]byte(payload), &line) == nil {
					if len(line) > 500 {
						line = line[:500]
					}
					logs = append(logs, line)
				}
			}
			return this.Context().NewString(`{}`), nil
		}
		if int(calls.Add(1)) > lim.HostCalls {
			hostErr = ErrHostCalls
			return this.Context().NewString(`{"error":"too many host calls"}`), nil
		}
		value, err := host(cctx, kind, json.RawMessage(payload))
		var b []byte
		if err != nil {
			b, _ = json.Marshal(map[string]string{"error": err.Error()})
		} else {
			if len(value) == 0 {
				value = json.RawMessage("null")
			}
			b, _ = json.Marshal(map[string]json.RawMessage{"value": value})
		}
		return this.Context().NewString(string(b)), nil
	})
	in := job.Input
	if len(in) == 0 {
		in = json.RawMessage("{}")
	}
	inJSON, _ := json.Marshal(string(in))
	code := prelude + job.Bundle + "\n" +
		"const __fn = (globalThis.__ext || {})[" + quote(job.Fn) + "];\n" +
		"if (typeof __fn !== \"function\") throw new Error(\"no exported function " + strings.ReplaceAll(job.Fn, `"`, "") + "\");\n" +
		"JSON.stringify((await __fn(JSON.parse(" + string(inJSON) + "), __ctx)) ?? null);\n"
	v, err := jc.Eval("tool.js", qjs.Code(code), qjs.FlagAsync())
	if err != nil {
		if hostErr != nil {
			return Result{}, hostErr
		}
		return Result{}, classify(cctx, err)
	}
	out := v.String()
	if hostErr != nil {
		return Result{}, hostErr
	}
	if len(out) > lim.OutputBytes {
		return Result{}, ErrOutput
	}
	if !json.Valid([]byte(out)) {
		out = "null"
	}
	return Result{Output: json.RawMessage(out), Logs: logs}, nil
}

func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ErrTimeout
	}
	msg := err.Error()
	if strings.Contains(msg, "out of memory") {
		return ErrMemory
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return errors.New(msg)
}

func validFn(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		ok := r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
