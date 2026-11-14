// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// The file and terminal panes run through a conversation the viewer shares
// with the agent; a DM with it counts and comes first.
func TestAgentOverviewCommonChannelsIncludeTheDM(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	var dm apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{agent}}, &dm); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	var ov struct {
		CommonChannels []struct{ ID, Kind string } `json:"common_channels"`
	}
	if s := e.do("GET", "/api/v1/agents/"+agent+"/overview", server.DevAdminToken, nil, &ov); s != http.StatusOK {
		t.Fatalf("overview: %d", s)
	}
	if len(ov.CommonChannels) < 2 || ov.CommonChannels[0].ID != dm.Id.String() || ov.CommonChannels[0].Kind != "dm" {
		t.Fatalf("common channels: %+v (dm %s)", ov.CommonChannels, dm.Id)
	}
	// #general is in there too; a member who is not in the DM does not get it.
	_, memberToken := e.createActor("human", "anna", "member")
	general := e.generalID()
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": e.actorIDByHandle("anna")}, nil)
	// Only the agent's owner (or an admin) sees its overview (round 38).
	e.do("PUT", "/api/v1/agents/"+agent+"/owner", server.DevAdminToken, map[string]any{"owner_id": e.actorIDByHandle("anna")}, nil)
	if s := e.do("GET", "/api/v1/agents/"+agent+"/overview", memberToken, nil, &ov); s != http.StatusOK {
		t.Fatalf("member overview: %d", s)
	}
	for _, c := range ov.CommonChannels {
		if c.ID == dm.Id.String() {
			t.Fatal("a member sees the admin's DM as common")
		}
	}
}

func (e *env) actorIDByHandle(handle string) string {
	e.t.Helper()
	var list struct{ Actors []struct{ ID, Handle string } }
	e.do("GET", "/api/v1/actors", server.DevAdminToken, nil, &list)
	for _, a := range list.Actors {
		if a.Handle == handle {
			return a.ID
		}
	}
	e.t.Fatalf("@%s not found", handle)
	return ""
}
