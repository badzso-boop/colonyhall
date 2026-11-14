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

func level(c apispec.Channel) string {
	if c.NotifyLevel == nil {
		return ""
	}
	return string(*c.NotifyLevel)
}

// Round 55, part I: the notification level per person and conversation,
// unified with the mute.
func TestNotifyLevels(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, otherTok := e.createUser("mari", "Mari")
	e.addMember(general, e.actorIDByHandle("mari"), true)
	var dm apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{e.actorIDByHandle("mari")}}, &dm); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open dm: %d", s)
	}

	if l := level(e.channelView(server.DevAdminToken, general)); l != "mentions" {
		t.Fatalf("channel default: %q", l)
	}
	if l := level(e.channelView(server.DevAdminToken, dm.Id.String())); l != "all" {
		t.Fatalf("DM default: %q", l)
	}

	set := func(token, ch, lv string) int {
		return e.do("PUT", "/api/v1/channels/"+ch+"/notify", token, map[string]any{"level": lv}, nil)
	}
	if s := set(server.DevAdminToken, general, "all"); s != http.StatusNoContent {
		t.Fatalf("set all: %d", s)
	}
	if c := e.channelView(server.DevAdminToken, general); level(c) != "all" || c.Muted != nil {
		t.Fatalf("after all: %+v", c)
	}
	// It is the caller's own setting.
	if l := level(e.channelView(otherTok, general)); l != "mentions" {
		t.Fatalf("another person's level changed: %q", l)
	}
	if s := set(server.DevAdminToken, general, "none"); s != http.StatusNoContent {
		t.Fatalf("set none: %d", s)
	}
	if c := e.channelView(server.DevAdminToken, general); level(c) != "none" || c.Muted == nil || !*c.Muted || c.MutedUntil != nil {
		t.Fatalf("none is a mute until unmuted: %+v", c)
	}
	// Unmuting returns to the default.
	if s := e.do("DELETE", "/api/v1/channels/"+general+"/mute", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("unmute: %d", s)
	}
	if l := level(e.channelView(server.DevAdminToken, general)); l != "mentions" {
		t.Fatalf("after unmute: %q", l)
	}
	// A timed mute ends in the default level.
	until := e.clock.Now().Add(time.Hour)
	if s := e.do("PUT", "/api/v1/channels/"+general+"/mute", server.DevAdminToken, map[string]any{"until": until}, nil); s != http.StatusNoContent {
		t.Fatalf("timed mute: %d", s)
	}
	if l := level(e.channelView(server.DevAdminToken, general)); l != "none" {
		t.Fatalf("during the mute: %q", l)
	}
	e.clock.Advance(2 * time.Hour)
	if l := level(e.channelView(server.DevAdminToken, general)); l != "mentions" {
		t.Fatalf("after the mute ended: %q", l)
	}

	if s := set(server.DevAdminToken, general, "loud"); s != http.StatusBadRequest {
		t.Fatalf("unknown level: %d", s)
	}
	if s := set(server.DevAgentToken, general, "all"); s != http.StatusForbidden {
		t.Fatalf("agent: %d", s)
	}
	secret := e.createChannel("secret")
	if s := set(otherTok, secret, "all"); s != http.StatusNotFound {
		t.Fatalf("unreadable channel: %d", s)
	}
}

func TestMarkAllRead(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	otherID, otherTok := e.createUser("mari", "Mari")
	e.addMember(general, otherID, true)
	ops := e.createChannel("ops", otherID)
	e.send(server.DevAdminToken, general, map[string]any{"body": "one"})
	e.send(server.DevAdminToken, ops, map[string]any{"body": "two"})
	if c := e.channelView(otherTok, ops); c.Unread == 0 {
		t.Fatal("expected unread before")
	}
	var out apispec.MarkedCount
	if s := e.do("POST", "/api/v1/me/channels/read", otherTok, nil, &out); s != http.StatusOK || out.Marked < 2 {
		t.Fatalf("mark all read: %d %+v", s, out)
	}
	for _, ch := range []string{general, ops} {
		if c := e.channelView(otherTok, ch); c.Unread != 0 || c.Mentions != 0 {
			t.Fatalf("still unread in %s: %+v", ch, c)
		}
	}
}
