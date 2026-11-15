// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package db

import (
	"context"
	"errors"
	"testing"

	"colonyhall/internal/platform/db/opscope"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
)

func TestScopeNarrowsAndNeverWidens(t *testing.T) {
	ws, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	ctx := context.Background()
	if s := ScopeOf(ctx); s != (Scope{}) {
		t.Fatalf("a bare context has no scope: %+v", s)
	}
	if w, sys, op := (Scope{}).settings(); w != "" || sys != "" || op != "" {
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
	if w, sys, _ := ScopeOf(narrow).settings(); w != ws.String() || sys != "" {
		t.Fatalf("settings: %q %q", w, sys)
	}
	// System mode over a narrowed context wins and hides the workspace.
	if s := ScopeOf(system.Context(narrow)); !s.System || s.Workspace != ids.Nil {
		t.Fatalf("system over workspace: %+v", s)
	}
	if w, sys, _ := ScopeOf(sysCtx).settings(); w != "" || sys != "on" {
		t.Fatalf("system settings: %q %q", w, sys)
	}
}

func TestOperatorScopeSeesNoWorkspace(t *testing.T) {
	ws, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	op := opscope.Context(WithWorkspace(context.Background(), ws))
	if s := ScopeOf(op); !s.Operator || s.System || s.Workspace != ids.Nil {
		t.Fatalf("operator scope must hide the workspace: %+v", s)
	}
	if w, sys, o := ScopeOf(op).settings(); w != "" || sys != "" || o != "on" {
		t.Fatalf("operator settings: %q %q %q", w, sys, o)
	}
	if s := ScopeOf(WithWorkspace(op, ws)); s.Operator || s.Workspace != ws {
		t.Fatalf("WithWorkspace must switch the operator scope off: %+v", s)
	}
}

// Round 51: a nested InTx with another scope must fail instead of running
// with the outer transaction's scope.
func TestNestedTransactionKeepsItsScope(t *testing.T) {
	ws, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	other, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35b")
	var m Memory
	ran := false
	err := m.InTx(system.Context(context.Background()), func(ctx context.Context) error {
		return m.InTx(WithWorkspace(ctx, ws), func(context.Context) error { ran = true; return nil })
	})
	if !errors.Is(err, ErrScopeChanged) || ran {
		t.Fatalf("system → workspace: err=%v ran=%v", err, ran)
	}
	err = m.InTx(WithWorkspace(context.Background(), ws), func(ctx context.Context) error {
		return m.InTx(WithWorkspace(ctx, other), func(context.Context) error { ran = true; return nil })
	})
	if !errors.Is(err, ErrScopeChanged) || ran {
		t.Fatalf("workspace → other workspace: err=%v ran=%v", err, ran)
	}
	err = m.InTx(WithWorkspace(context.Background(), ws), func(ctx context.Context) error {
		return m.InTx(WithWorkspace(ctx, ws), func(context.Context) error { ran = true; return nil })
	})
	if err != nil || !ran {
		t.Fatalf("the same scope joins: err=%v ran=%v", err, ran)
	}
}
