// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"colonyhall/internal/platform/ids"
)

func TestPathID(t *testing.T) {
	id := ids.ID{0: 1, 15: 2}
	for _, tc := range []struct {
		path   string
		ok     bool
		detail string
	}{
		{"/x/" + id.String(), true, ""},
		{"/x/not-a-uuid", false, "channel_id must be a UUID"},
	} {
		var got ids.ID
		var ok bool
		mux := http.NewServeMux()
		mux.HandleFunc("GET /x/{channel_id}", func(w http.ResponseWriter, r *http.Request) {
			got, ok = PathID(w, r, "channel_id")
		})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if ok != tc.ok {
			t.Fatalf("%s: ok = %v, want %v", tc.path, ok, tc.ok)
		}
		if tc.ok {
			if got != id || rec.Code != http.StatusOK {
				t.Fatalf("%s: got %s (status %d), want %s", tc.path, got, rec.Code, id)
			}
			continue
		}
		var p problem
		if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusBadRequest || p.Detail != tc.detail || got != ids.Nil {
			t.Fatalf("%s: status %d, detail %q, id %s", tc.path, rec.Code, p.Detail, got)
		}
	}
}
