// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package db

import (
	"context"
	"testing"

	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
)

func TestScopeNarrowsAndNeverWidens(t *testing.T) {
	ws, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	ctx := context.Background()
	if s := ScopeOf(ctx); s != (Scope{}) {
		t.Fatalf("a bare context has no scope: %+v", s)
	}
	if w, sys := (Scope{}).settings(); w != "" || sys != "" {
		t.Fatalf("no scope means empty settings: %q %q", w, sys)
	}
	sysCtx := system.Context(ctx)
	if s := ScopeOf(sysCtx); !s.System || s.Workspace != ids.Nil {
		t.Fatalf("system scope: %+v", s)
	}
	narrow := WithWorkspace(sysCtx, ws)
	if s := ScopeOf(narrow); s.System || s.Workspace != ws {
		t.Fatalf("WithWorkspace must switch system mode off: %+v", s)
	}
	if w, sys := ScopeOf(narrow).settings(); w != ws.String() || sys != "" {
		t.Fatalf("settings: %q %q", w, sys)
	}
	// System mode over a narrowed context wins and hides the workspace.
	if s := ScopeOf(system.Context(narrow)); !s.System || s.Workspace != ids.Nil {
		t.Fatalf("system over workspace: %+v", s)
	}
	if w, sys := ScopeOf(sysCtx).settings(); w != "" || sys != "on" {
		t.Fatalf("system settings: %q %q", w, sys)
	}
}
