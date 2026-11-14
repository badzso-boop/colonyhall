// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func TestSidebarSectionsE2E(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()

	// 1. Agent token returns 403 Forbidden on sidebar operations
	agentStatus := e.do("GET", "/api/v1/sidebar/sections", server.DevAgentToken, nil, nil)
	if agentStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for agent GET /sidebar/sections, got %d", agentStatus)
	}

	createSecReq := apispec.CreateSidebarSectionRequest{
		Name: "Projects",
	}
	agentCreateStatus := e.do("POST", "/api/v1/sidebar/sections", server.DevAgentToken, createSecReq, nil)
	if agentCreateStatus != http.StatusForbidden {
		t.Fatalf("expected 403 for agent POST /sidebar/sections, got %d", agentCreateStatus)
	}

	// 2. Human user (Admin) creates a section
	var createdSec apispec.SidebarSection
	status := e.do("POST", "/api/v1/sidebar/sections", server.DevAdminToken, createSecReq, &createdSec)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", status)
	}
	if createdSec.Name != "Projects" {
		t.Fatalf("expected name 'Projects', got %q", createdSec.Name)
	}
	if createdSec.Position != 0 {
		t.Fatalf("expected position 0, got %d", createdSec.Position)
	}
	if len(createdSec.Items) != 0 {
		t.Fatalf("expected empty items, got %v", createdSec.Items)
	}

	// Create a second section
	var createdSec2 apispec.SidebarSection
	status = e.do("POST", "/api/v1/sidebar/sections", server.DevAdminToken, apispec.CreateSidebarSectionRequest{
		Name: "Random",
	}, &createdSec2)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", status)
	}
	if createdSec2.Position != 1 {
		t.Fatalf("expected position 1, got %d", createdSec2.Position)
	}

	// 3. List sections for Admin
	var secList apispec.SidebarSectionList
	status = e.do("GET", "/api/v1/sidebar/sections", server.DevAdminToken, nil, &secList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", status)
	}
	if len(secList.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(secList.Sections))
	}
	if secList.Sections[0].Id != createdSec.Id || secList.Sections[1].Id != createdSec2.Id {
		t.Fatalf("unexpected sections order or IDs: %v", secList.Sections)
	}

	// 4. Put channel item into section
	itemPos := 0
	status = e.do("PUT", fmt.Sprintf("/api/v1/sidebar/sections/%s/channels/%s", createdSec.Id, general), server.DevAdminToken, apispec.SetSidebarItemRequest{
		Position: &itemPos,
	}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for put item, got %d", status)
	}

	// Verify item is in section
	status = e.do("GET", "/api/v1/sidebar/sections", server.DevAdminToken, nil, &secList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", status)
	}
	if len(secList.Sections[0].Items) != 1 || secList.Sections[0].Items[0].ChannelId.String() != general {
		t.Fatalf("expected channel %s in section %s, got %v", general, createdSec.Id, secList.Sections[0].Items)
	}

	// 5. Update section name and collapsed state
	newName := "Work Projects"
	collapsed := true
	var updatedSec apispec.SidebarSection
	status = e.do("PATCH", fmt.Sprintf("/api/v1/sidebar/sections/%s", createdSec.Id), server.DevAdminToken, apispec.UpdateSidebarSectionRequest{
		Name:      &newName,
		Collapsed: &collapsed,
	}, &updatedSec)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK for patch section, got %d", status)
	}
	if updatedSec.Name != "Work Projects" || !updatedSec.Collapsed {
		t.Fatalf("expected updated fields, got %+v", updatedSec)
	}

	// 6. Reorder sections
	reorderReq := apispec.ReorderSidebarSectionsRequest{
		SectionIds: []openapi_types.UUID{createdSec2.Id, createdSec.Id},
	}
	status = e.do("PUT", "/api/v1/sidebar/sections/reorder", server.DevAdminToken, reorderReq, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for reorder sections, got %d", status)
	}

	status = e.do("GET", "/api/v1/sidebar/sections", server.DevAdminToken, nil, &secList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", status)
	}
	if len(secList.Sections) != 2 || secList.Sections[0].Id != createdSec2.Id || secList.Sections[1].Id != createdSec.Id {
		t.Fatalf("expected reordered sections: %v", secList.Sections)
	}

	// 7. Delete item from section
	status = e.do("DELETE", fmt.Sprintf("/api/v1/sidebar/sections/%s/channels/%s", createdSec.Id, general), server.DevAdminToken, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete item, got %d", status)
	}

	status = e.do("GET", "/api/v1/sidebar/sections", server.DevAdminToken, nil, &secList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", status)
	}
	for _, sec := range secList.Sections {
		if sec.Id == createdSec.Id && len(sec.Items) != 0 {
			t.Fatalf("expected 0 items in section %s, got %v", createdSec.Id, sec.Items)
		}
	}

	// 8. Delete section
	status = e.do("DELETE", fmt.Sprintf("/api/v1/sidebar/sections/%s", createdSec.Id), server.DevAdminToken, nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for delete section, got %d", status)
	}

	status = e.do("GET", "/api/v1/sidebar/sections", server.DevAdminToken, nil, &secList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", status)
	}
	if len(secList.Sections) != 1 || secList.Sections[0].Id != createdSec2.Id {
		t.Fatalf("expected only section 2 remaining, got %v", secList.Sections)
	}

	// 9. Multi-user isolation: create another human and verify sections are isolated
	var bobActor apispec.CreateActorResponse
	status = e.do("POST", "/api/v1/actors", server.DevAdminToken, apispec.CreateActorRequest{
		Kind:   apispec.ActorKindHuman,
		Handle: "bob",
	}, &bobActor)
	if status != http.StatusCreated {
		t.Fatalf("expected 201 Created for actor bob, got %d", status)
	}

	var bobSecList apispec.SidebarSectionList
	status = e.do("GET", "/api/v1/sidebar/sections", bobActor.Token, nil, &bobSecList)
	if status != http.StatusOK {
		t.Fatalf("expected 200 OK for bob sections, got %d", status)
	}
	if len(bobSecList.Sections) != 0 {
		t.Fatalf("expected bob to have 0 sections, got %d", len(bobSecList.Sections))
	}

	// Bob cannot modify or delete Admin's section
	status = e.do("DELETE", fmt.Sprintf("/api/v1/sidebar/sections/%s", createdSec2.Id), bobActor.Token, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when bob deletes admin section, got %d", status)
	}
}
