// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// Tokens show their prefix and last use, people their last activity, and runs
// the instructions version they got (the round 35 leftovers).
func TestTokenSessionAndRunDetails(t *testing.T) {
	e := newEnv(t)
	owner := server.DevAdminToken
	var me apispec.Actor
	e.do("GET", "/api/v1/me", owner, nil, &me)

	var issued apispec.IssueTokenResponse
	if st := e.do("POST", "/api/v1/actors/"+me.Id.String()+"/tokens", owner, map[string]any{"label": "laptop"}, &issued); st != http.StatusCreated {
		t.Fatalf("issue: %d", st)
	}
	if issued.Token.Prefix == nil || *issued.Token.Prefix != issued.Secret[:8] || len(*issued.Token.Prefix) != 8 {
		t.Fatalf("prefix: %v", issued.Token.Prefix)
	}
	if issued.Token.LastUsedAt != nil {
		t.Fatal("a new token was never used")
	}
	lastUsed := func() *time.Time {
		var list apispec.TokenList
		e.do("GET", "/api/v1/actors/"+me.Id.String()+"/tokens", owner, nil, &list)
		for _, tok := range list.Tokens {
			if tok.Id == issued.Token.Id {
				return tok.LastUsedAt
			}
		}
		t.Fatal("token not listed")
		return nil
	}
	e.do("GET", "/api/v1/me", issued.Secret, nil, nil)
	first := lastUsed()
	if first == nil {
		t.Fatal("use not recorded")
	}
	e.clock.Advance(30 * time.Second)
	e.do("GET", "/api/v1/me", issued.Secret, nil, nil)
	if got := lastUsed(); !got.Equal(*first) {
		t.Fatal("recorded again within the minute")
	}
	e.clock.Advance(2 * time.Minute)
	e.do("GET", "/api/v1/me", issued.Secret, nil, nil)
	if got := lastUsed(); !got.After(*first) {
		t.Fatal("not recorded after the minute")
	}

	// A person's last activity follows their browser.
	var anna struct {
		Actor apispec.Actor `json:"actor"`
	}
	if st := e.do("POST", "/api/v1/actors", owner, map[string]any{"kind": "human", "handle": "anna", "display_name": "Anna", "role": "member", "password": "anna-long-password-1"}, &anna); st != http.StatusCreated {
		t.Fatalf("create anna: %d", st)
	}
	b := e.browser()
	if st := b.login("anna", "anna-long-password-1"); st != http.StatusOK {
		t.Fatalf("login: %d", st)
	}
	e.clock.Advance(5 * time.Minute)
	b.do("GET", "/api/v1/me", nil, nil)
	var users apispec.UserList
	e.do("GET", "/api/v1/admin/users", owner, nil, &users)
	var row *apispec.UserRow
	for i := range users.Users {
		if users.Users[i].Actor.Id == anna.Actor.Id {
			row = &users.Users[i]
		}
	}
	if row == nil || row.LastActiveAt == nil || row.LastLoginAt == nil || !row.LastActiveAt.After(*row.LastLoginAt) {
		t.Fatalf("last activity: %+v", row)
	}

	// The run carries the instructions version the guard reported.
	general := e.generalID()
	if st := e.do("POST", "/api/v1/agent/runs", server.DevAgentToken, map[string]any{
		"phase": "work", "client": "claude-code", "model": "m", "session_id": "s", "channel_id": general, "items": 1,
		"input_tokens": 1, "output_tokens": 1, "cache_read_tokens": 0, "cache_write_tokens": 0, "duration_ms": 1, "status": "ok",
		"instructions_version": 7,
	}, nil); st != http.StatusCreated {
		t.Fatalf("report run: %d", st)
	}
	var runs apispec.RunList
	e.do("GET", "/api/v1/runs", owner, nil, &runs)
	if len(runs.Runs) == 0 || runs.Runs[0].InstructionsVersion == nil || *runs.Runs[0].InstructionsVersion != 7 {
		t.Fatalf("instructions version: %+v", runs.Runs)
	}
}
