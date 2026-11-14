// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/httpx"
)

// License is the SPDX identifier of Colonyhall's licence.
const License = "AGPL-3.0-only"

// registerVersion serves GET /api/v1/version without sign-in: the version,
// the licence and the source link that AGPL section 13 asks a network
// service to offer its users (round 48).
func registerVersion(mux *http.ServeMux, sourceURL string) {
	body := apispec.VersionInfo{Version: Version, License: License, SourceUrl: sourceURL}
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		httpx.JSON(w, http.StatusOK, body)
	})
}
