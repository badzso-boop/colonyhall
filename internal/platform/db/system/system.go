// Package system marks a context as working for every workspace at once:
// the event consumers, the scheduler and the other background loops read
// across tenants (round 45, docs/23-elkulonites.md). Row-level security
// lets such a context see every row.
//
// Only background code may import this package; a depguard rule
// (system-mode-is-background-only) keeps request handlers, MCP tools and
// the WebSocket gateway out. They narrow to one workspace with
// db.WithWorkspace instead.
package system

import "context"

type key struct{}

// Context returns ctx in system mode.
func Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, true)
}

// Off returns ctx with system mode switched off (db.WithWorkspace uses it).
func Off(ctx context.Context) context.Context {
	if !Is(ctx) {
		return ctx
	}
	return context.WithValue(ctx, key{}, false)
}

// Is reports whether ctx is in system mode.
func Is(ctx context.Context) bool {
	v, _ := ctx.Value(key{}).(bool)
	return v
}
