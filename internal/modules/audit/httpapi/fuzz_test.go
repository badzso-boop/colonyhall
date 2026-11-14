// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package httpapi

import (
	"strings"
	"testing"
)

// FuzzNoFormula: no exported audit cell starts like a spreadsheet formula,
// and the text itself is kept (round 40).
func FuzzNoFormula(f *testing.F) {
	for _, s := range []string{"", "=1+1", "+cmd", "-2", "@SUM(A1)", "\t=x", "\r=x", "plain", " =x", "'=x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := noFormula(s)
		if !strings.HasSuffix(got, s) || len(got) > len(s)+1 {
			t.Fatalf("noFormula(%q) = %q changes the text", s, got)
		}
		if got != "" && strings.ContainsRune("=+-@\t\r", rune(got[0])) {
			t.Fatalf("noFormula(%q) = %q starts like a formula", s, got)
		}
	})
}
