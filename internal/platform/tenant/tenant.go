// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package tenant carries the workspace a request's subdomain names, in
// multi-tenant mode (round 45, docs/23-elkulonites.md). The server's tenant
// middleware puts it in the request context before anything else runs;
// in single-tenant mode there is none and every credential decides its
// own workspace, as before.
package tenant

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Request is the subdomain's workspace and what the security log needs
// about the caller.
type Request struct {
	Workspace ids.ID
	Slug      string
	// HostCookie: the session cookie carries the __Host- prefix (HTTPS).
	HostCookie bool
	RemoteAddr string
	UserAgent  string
}

type key struct{}

// With returns ctx carrying the request's tenant.
func With(ctx context.Context, r Request) context.Context {
	return context.WithValue(ctx, key{}, r)
}

// From returns the request's tenant; ok is false in single-tenant mode.
func From(ctx context.Context) (Request, bool) {
	r, ok := ctx.Value(key{}).(Request)
	return r, ok
}
