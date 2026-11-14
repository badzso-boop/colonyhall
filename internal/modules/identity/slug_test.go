// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"testing"

	"colonyhall/internal/platform/ids"
)

func TestValidSlug(t *testing.T) {
	for s, want := range map[string]bool{
		"acme": true, "beta-2": true, "a1b": true, "abc-def-ghi": true,
		"ab": false, "-acme": false, "acme-": false, "ac--me": false, "Acme": false, "ac_me": false, "ac.me": false,
		"admin": false, "www": false, "api": false,
		"a234567890123456789012345678901234567890": true, "a2345678901234567890123456789012345678901": false,
	} {
		if got := ValidSlug(s); got != want {
			t.Errorf("ValidSlug(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestSlugFromName(t *testing.T) {
	id, _ := ids.Parse("01a1cb18-55ea-7099-8d5e-651963b2b35a")
	for name, want := range map[string]string{
		"Colonyhall":      "colonyhall",
		"ACME Kft.":       "acme-kft",
		"  Béta  Csapat ": "b-ta-csapat",
		"x":               "ws-01a1cb1855ea",
		"admin":           "ws-01a1cb1855ea",
		"!!!":             "ws-01a1cb1855ea",
		"a very long workspace name that goes on and on and on": "a-very-long-workspace-name-that-goes-on",
	} {
		if got := SlugFromName(name, id); got != want {
			t.Errorf("SlugFromName(%q) = %q, want %q", name, got, want)
		}
	}
}
