// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package mcptools

import (
	"testing"
)

func TestIsTextMime(t *testing.T) {
	tests := []struct {
		mime string
		want bool
	}{
		{"text/plain", true},
		{"text/markdown", true},
		{"text/csv", true},
		{"application/json", true},
		{"application/xml", true},
		{"application/yaml", true},
		{"application/octet-stream", false},
		{"image/png", false},
		{"image/jpeg", false},
		{"application/pdf", false},
	}

	for _, tt := range tests {
		if got := isTextMime(tt.mime); got != tt.want {
			t.Errorf("isTextMime(%q) = %v, want %v", tt.mime, got, tt.want)
		}
	}
}
