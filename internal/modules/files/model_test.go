// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files_test

import (
	"strings"
	"testing"

	"colonyhall/internal/modules/files"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple file",
			input:    "report.pdf",
			expected: "report.pdf",
		},
		{
			name:     "path traversal directory",
			input:    "../../etc/passwd",
			expected: "passwd",
		},
		{
			name:     "windows path separators",
			input:    `C:\Users\test\image.png`,
			expected: "image.png",
		},
		{
			name:     "empty filename",
			input:    "",
			expected: "attachment",
		},
		{
			name:     "dot dot only",
			input:    "..",
			expected: "attachment",
		},
		{
			name:     "slash only",
			input:    "///",
			expected: "attachment",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "attachment",
		},
		{
			name:     "long name truncated to 200",
			input:    strings.Repeat("a", 250) + ".txt",
			expected: strings.Repeat("a", 196) + ".txt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := files.SanitizeFileName(tc.input)
			if got != tc.expected {
				t.Fatalf("SanitizeFileName(%q) = %q, want %q", tc.input, got, tc.expected)
			}
			if len(got) > 200 {
				t.Fatalf("SanitizeFileName result length %d > 200", len(got))
			}
		})
	}
}
