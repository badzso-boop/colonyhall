// Package httpapi exposes the sidebar sections module over REST.
package httpapi

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/sidebar"
	"colonyhall/internal/platform/ids"
)

func init() {
	httpx.RegisterForbidden(sidebar.ErrForbidden)
	httpx.RegisterStatus(http.StatusNotFound, sidebar.ErrSectionNotFound)
	httpx.RegisterValidation(sidebar.ErrInvalidName)
}

func sectionToAPI(s sidebar.Section) apispec.SidebarSection {
	items := make([]apispec.SidebarItem, len(s.Items))
	for i, it := range s.Items {
		items[i] = apispec.SidebarItem{
			ChannelId: it.ChannelID,
			Position:  it.Position,
		}
	}
	return apispec.SidebarSection{
		Id:        s.ID,
		Name:      s.Name,
		Position:  s.Position,
		Collapsed: s.Collapsed,
		Items:     items,
	}
}

// Register mounts the sidebar routes on mux.
func Register(mux *http.ServeMux, svc *sidebar.Service, auth identity.RequestAuthenticator) {
	wrap := func(h http.HandlerFunc) http.Handler { return httpx.RequireAuth(auth, h) }

	mux.Handle("GET /api/v1/sidebar/sections", wrap(func(w http.ResponseWriter, r *http.Request) {
		sections, err := svc.List(r.Context(), httpx.Actor(r))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out := apispec.SidebarSectionList{
			Sections: make([]apispec.SidebarSection, len(sections)),
		}
		for i, s := range sections {
			out.Sections[i] = sectionToAPI(s)
		}
		httpx.JSON(w, http.StatusOK, out)
	}))

	mux.Handle("POST /api/v1/sidebar/sections", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.CreateSidebarSectionRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		sec, err := svc.Create(r.Context(), httpx.Actor(r), in.Name)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, sectionToAPI(sec))
	}))

	mux.Handle("PUT /api/v1/sidebar/sections/reorder", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in apispec.ReorderSidebarSectionsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		secIDs := make([]ids.ID, len(in.SectionIds))
		for i, id := range in.SectionIds {
			secIDs[i] = ids.ID(id)
		}
		if err := svc.ReorderSections(r.Context(), httpx.Actor(r), secIDs); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PATCH /api/v1/sidebar/sections/{sectionId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		sectionID, err := ids.Parse(r.PathValue("sectionId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "sectionId must be a UUID"})
			return
		}
		var in apispec.UpdateSidebarSectionRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		sec, err := svc.Update(r.Context(), httpx.Actor(r), sectionID, in.Name, in.Position, in.Collapsed)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, sectionToAPI(sec))
	}))

	mux.Handle("DELETE /api/v1/sidebar/sections/{sectionId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		sectionID, err := ids.Parse(r.PathValue("sectionId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "sectionId must be a UUID"})
			return
		}
		if err := svc.Delete(r.Context(), httpx.Actor(r), sectionID); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PUT /api/v1/sidebar/sections/{sectionId}/reorder", wrap(func(w http.ResponseWriter, r *http.Request) {
		sectionID, err := ids.Parse(r.PathValue("sectionId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "sectionId must be a UUID"})
			return
		}
		var in apispec.ReorderSidebarItemsRequest
		if err := httpx.Decode(r, &in); err != nil {
			httpx.Error(w, r, err)
			return
		}
		chIDs := make([]ids.ID, len(in.ChannelIds))
		for i, id := range in.ChannelIds {
			chIDs[i] = ids.ID(id)
		}
		if err := svc.ReorderItems(r.Context(), httpx.Actor(r), sectionID, chIDs); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("PUT /api/v1/sidebar/sections/{sectionId}/channels/{channelId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		sectionID, err := ids.Parse(r.PathValue("sectionId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "sectionId must be a UUID"})
			return
		}
		channelID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "channelId must be a UUID"})
			return
		}
		var in apispec.SetSidebarItemRequest
		_ = httpx.Decode(r, &in) // optional body
		if err := svc.SetItem(r.Context(), httpx.Actor(r), sectionID, channelID, in.Position); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.Handle("DELETE /api/v1/sidebar/sections/{sectionId}/channels/{channelId}", wrap(func(w http.ResponseWriter, r *http.Request) {
		sectionID, err := ids.Parse(r.PathValue("sectionId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "sectionId must be a UUID"})
			return
		}
		channelID, err := ids.Parse(r.PathValue("channelId"))
		if err != nil {
			httpx.Error(w, r, &httpx.BadRequest{Detail: "channelId must be a UUID"})
			return
		}
		if err := svc.RemoveItem(r.Context(), httpx.Actor(r), sectionID, channelID); err != nil {
			httpx.Error(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}
