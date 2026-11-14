// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func TestActorProfileE2E(t *testing.T) {
	e := newEnv(t)
	agentID := e.agentID()

	// Get Admin actor ID
	var meAdmin apispec.Actor
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &meAdmin)

	// Create a non-admin human member
	memberRole := apispec.Member
	var charlie apispec.CreateActorResponse
	status := e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind:   apispec.ActorKindHuman,
		Handle: "charlie",
		Role:   &memberRole,
	}, &charlie)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 for charlie, got %d", status)
	}

	// 1. Agent token returns 403 Forbidden
	desc := "Agent bio"
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", agentID), server.DevAgentToken, apispec.UpdateActorProfileRequest{
		Description: &desc,
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for agent PATCH profile, got %d", status)
	}

	// 2. Non-admin human (charlie) edits own profile
	cDesc := "Frontend specialist"
	cEmoji := "🎨"
	cTz := "America/New_York"
	cStatus := "Focusing"
	var updatedCharlie apispec.Actor
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", charlie.Actor.Id), charlie.Token, apispec.UpdateActorProfileRequest{
		Description: &cDesc,
		AvatarEmoji: &cEmoji,
		Timezone:    &cTz,
		StatusText:  &cStatus,
	}, &updatedCharlie)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for charlie updating own profile, got %d", status)
	}
	if updatedCharlie.Description == nil || *updatedCharlie.Description != cDesc ||
		updatedCharlie.AvatarEmoji == nil || *updatedCharlie.AvatarEmoji != cEmoji ||
		updatedCharlie.Timezone == nil || *updatedCharlie.Timezone != cTz ||
		updatedCharlie.StatusText == nil || *updatedCharlie.StatusText != cStatus {
		t.Fatalf("unexpected updated profile: %+v", updatedCharlie)
	}

	// 3. Non-admin human (charlie) cannot edit someone else's profile (admin's or agent's)
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", meAdmin.Id), charlie.Token, apispec.UpdateActorProfileRequest{
		Description: &cDesc,
	}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for charlie editing admin profile, got %d", status)
	}

	// 4. Admin edits another actor's profile (charlie and agent)
	agentEmoji := "🤖"
	agentBio := "AI Assistant in Colonyhall"
	var updatedAgent apispec.Actor
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", agentID), server.DevAdminToken, apispec.UpdateActorProfileRequest{
		AvatarEmoji: &agentEmoji,
		Description: &agentBio,
	}, &updatedAgent)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for admin editing agent profile, got %d", status)
	}
	if updatedAgent.AvatarEmoji == nil || *updatedAgent.AvatarEmoji != agentEmoji ||
		updatedAgent.Description == nil || *updatedAgent.Description != agentBio {
		t.Fatalf("unexpected agent profile: %+v", updatedAgent)
	}

	// 5. Validations
	// Too long description (>300 chars)
	longDesc := strings.Repeat("x", 301)
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", meAdmin.Id), server.DevAdminToken, apispec.UpdateActorProfileRequest{
		Description: &longDesc,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for too long description, got %d", status)
	}

	// Too long status text (>80 chars)
	longStatus := strings.Repeat("y", 81)
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", meAdmin.Id), server.DevAdminToken, apispec.UpdateActorProfileRequest{
		StatusText: &longStatus,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for too long status, got %d", status)
	}

	// Invalid timezone
	badTz := "Invalid/Timezone_Not_Real"
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", meAdmin.Id), server.DevAdminToken, apispec.UpdateActorProfileRequest{
		Timezone: &badTz,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad timezone, got %d", status)
	}

	// Invalid avatar emoji (text, not emoji)
	badEmoji := "not-an-emoji"
	status = e.do("PATCH", fmt.Sprintf("/api/v1/actors/%s/profile", meAdmin.Id), server.DevAdminToken, apispec.UpdateActorProfileRequest{
		AvatarEmoji: &badEmoji,
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad avatar emoji, got %d", status)
	}
}
