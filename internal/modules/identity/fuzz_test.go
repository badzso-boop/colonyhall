// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"strings"
	"testing"
)

// FuzzValidateHandle: an accepted handle is 2-32 lower-case letters,
// digits and hyphens, not starting with a hyphen (round 40).
func FuzzValidateHandle(f *testing.F) {
	for _, s := range []string{"bob", "a", "-ab", "ab-", "Bob", "böb", "ab\n", strings.Repeat("a", 33), "ab cd"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, h string) {
		if ValidateHandle(h) != nil {
			return
		}
		if len(h) < 2 || len(h) > 32 || h[0] == '-' || strings.Trim(h, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			t.Fatalf("accepted invalid handle %q", h)
		}
	})
}

// FuzzTokenPrefix: what may be shown of a secret is a short prefix of it.
func FuzzTokenPrefix(f *testing.F) {
	for _, s := range []string{"", "col_", "col_abcdefgh", "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, secret string) {
		p := TokenPrefix(secret)
		if !strings.HasPrefix(secret, p) || len(p) > len(tokenPrefix)+4 {
			t.Fatalf("TokenPrefix(%q) = %q", secret, p)
		}
	})
}
