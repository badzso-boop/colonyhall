// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package opscope marks a context as working for the platform operator
// (round 51, docs/24-hosztolt-ajanlat.md). Row-level security lets such a
// context see the platform tables (operators, their sessions and audit)
// and no workspace's rows at all: the operator reaches customer data only
// through the content-free overview function or a break-glass session.
//
// Only the operator module and the CLI may import this package (depguard
// rule operator-scope-is-the-operator-module).
package opscope

import "context"

type key struct{}

// Context returns ctx in operator scope.
func Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, key{}, true)
}

// Off returns ctx with the operator scope switched off (db.WithWorkspace
// uses it).
func Off(ctx context.Context) context.Context {
	if !Is(ctx) {
		return ctx
	}
	return context.WithValue(ctx, key{}, false)
}

// Is reports whether ctx is in operator scope.
func Is(ctx context.Context) bool {
	v, _ := ctx.Value(key{}).(bool)
	return v
}
