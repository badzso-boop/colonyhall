// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package extrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedModuleIsTheReleasedOne(t *testing.T) {
	sum := sha256.Sum256(embeddedQJS)
	if hex.EncodeToString(sum[:]) != QJSWasmSHA256 {
		t.Fatal("qjs.wasm differs from fastschema/qjs v0.0.6's; update QJSWasmSHA256 with the module")
	}
}

var engine = &QJS{MemoryMB: 64, Slots: 4}

func noHost(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("no host in this test")
}

func run(t *testing.T, src, fn, input string, lim Limits, host Host) (Result, error) {
	t.Helper()
	if host == nil {
		host = noHost
	}
	return engine.Run(context.Background(), Job{Bundle: src, Fn: fn, Input: json.RawMessage(input), Limits: lim}, host)
}

const echo = `globalThis.__ext = { hello: async (input, ctx) => { ctx.log("hi", input.name); return { greeting: "Hello " + input.name }; } };`

func TestRunsAToolWithInputAndLogs(t *testing.T) {
	res, err := run(t, echo, "hello", `{"name":"Norbi"}`, Limits{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Output) != `{"greeting":"Hello Norbi"}` || len(res.Logs) != 1 || res.Logs[0] != "hi Norbi" {
		t.Fatalf("result: %s %v", res.Output, res.Logs)
	}
}

func TestHostCalls(t *testing.T) {
	src := `globalThis.__ext = { f: async (input, ctx) => {
		const page = await ctx.fetch("https://api.example.com/x", { method: "GET", auth: "jira" });
		await ctx.store.put("seen", "a", { n: 1 });
		const sent = await ctx.colony.send({ channel: "general", text: page.body });
		return { page, sent };
	} };`
	var kinds []string
	host := func(_ context.Context, kind string, payload json.RawMessage) (json.RawMessage, error) {
		kinds = append(kinds, kind+":"+string(payload))
		switch kind {
		case "fetch":
			return json.RawMessage(`{"status":200,"body":"PROJ-1"}`), nil
		case "store":
			return json.RawMessage(`null`), nil
		case "colony":
			return json.RawMessage(`"message sent"`), nil
		}
		return nil, errors.New("unknown")
	}
	res, err := run(t, src, "f", `{}`, Limits{}, host)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Output), `"sent":"message sent"`) || len(kinds) != 3 ||
		!strings.Contains(kinds[0], `"auth":"jira"`) || !strings.Contains(kinds[2], `"tool":"send"`) {
		t.Fatalf("output %s, host calls %v", res.Output, kinds)
	}
}

func TestHostErrorBecomesException(t *testing.T) {
	src := `globalThis.__ext = { f: async (i, ctx) => { try { await ctx.fetch("https://x"); } catch (e) { return "caught: " + e.message; } } };`
	res, err := run(t, src, "f", `{}`, Limits{}, nil)
	if err != nil || string(res.Output) != `"caught: no host in this test"` {
		t.Fatalf("%s %v", res.Output, err)
	}
}

// The hostile tools of docs/25 13.13: each is stopped, and the engine
// keeps working afterwards.
func TestHostileTools(t *testing.T) {
	cases := []struct {
		name, src string
		lim       Limits
		want      error
		contains  string
	}{
		{"endless loop", `globalThis.__ext = { f: async () => { for (;;) {} } };`, Limits{Timeout: time.Second}, ErrTimeout, ""},
		{"memory bomb (arrays)", `globalThis.__ext = { f: async () => { const a = []; for (let i = 0;; i++) a.push(new Array(1e5).fill(i)); } };`, Limits{Timeout: 20 * time.Second}, ErrMemory, ""},
		{"memory bomb (string)", `globalThis.__ext = { f: async () => { let s = "x"; for (;;) s = s + s; } };`, Limits{Timeout: 20 * time.Second}, ErrMemory, ""},
		{"huge answer", `globalThis.__ext = { f: async () => "x".repeat(300000) };`, Limits{}, ErrOutput, ""},
		{"too many host calls", `globalThis.__ext = { f: async (i, ctx) => { for (let n = 0; n < 100; n++) { try { await ctx.fetch("https://x"); } catch (e) {} } return 1; } };`, Limits{HostCalls: 5}, ErrHostCalls, ""},
		{"no such function", `globalThis.__ext = {};`, Limits{}, nil, "no exported function"},
		{"syntax error", `globalThis.__ext = { f: async () => { return ( } };`, Limits{}, nil, "SyntaxError"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			_, err := run(t, c.src, "f", `{}`, c.lim, nil)
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if c.contains != "" && (err == nil || !strings.Contains(err.Error(), c.contains)) {
				t.Fatalf("err = %v, want it to contain %q", err, c.contains)
			}
			if time.Since(start) > 25*time.Second {
				t.Fatalf("took %s", time.Since(start))
			}
		})
	}
	if res, err := run(t, echo, "hello", `{"name":"after"}`, Limits{}, nil); err != nil || !strings.Contains(string(res.Output), "after") {
		t.Fatalf("the engine after the hostile tools: %s %v", res.Output, err)
	}
}

// The file system is an empty directory, the environment is empty, and
// nothing survives a call.
func TestNoFilesNoEnvNoState(t *testing.T) {
	src := `globalThis.__ext = { f: async () => {
		const std = await import("qjs:std");
		const os = await import("qjs:os");
		const files = os.readdir("/")[0].filter((n) => n !== "." && n !== "..");
		return { passwd: std.loadFile("/etc/passwd"), files, home: std.getenv("HOME") ?? null, kept: globalThis.kept ?? null };
	}, set: async () => { globalThis.kept = "x"; return 1; } };`
	if _, err := run(t, src, "set", `{}`, Limits{}, nil); err != nil {
		t.Fatal(err)
	}
	res, err := run(t, src, "f", `{}`, Limits{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(res.Output) != `{"passwd":null,"files":[],"home":null,"kept":null}` {
		t.Fatalf("the script saw: %s", res.Output)
	}
}

func TestCapMemoryRejectsGarbage(t *testing.T) {
	if _, err := capMemory([]byte("nope"), 10); err == nil {
		t.Fatal("garbage accepted")
	}
}
