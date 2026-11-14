// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package secretscan flags text that looks like it carries a secret, so
// the search index can leave it out (round 36, docs/11-rag-terv.md, 6.).
// The rules are a small subset written after the categories of gitleaks'
// default rules; no code is copied. A false positive only means a chunk
// is not searchable, so the rules lean towards flagging.
package secretscan

import (
	"regexp"
	"strings"
)

// rule is one named pattern.
type rule struct {
	name string
	re   *regexp.Regexp
	// check, when set, must also accept the match (to cut false positives).
	check func(match string) bool
}

var rules = []rule{
	{name: "colonyhall-token", re: regexp.MustCompile(`\b(?:col|colh)_[A-Za-z0-9_\-]{16,}`)},
	{name: "anthropic-key", re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}`)},
	{name: "openai-key", re: regexp.MustCompile(`\bsk-(?:proj-)?[A-Za-z0-9_\-]{20,}`)},
	{name: "google-api-key", re: regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}`)},
	{name: "google-oauth", re: regexp.MustCompile(`\bya29\.[0-9A-Za-z_\-]{20,}`)},
	{name: "github-token", re: regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{30,}|\bgithub_pat_[A-Za-z0-9_]{40,}`)},
	{name: "gitlab-token", re: regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20,}`)},
	{name: "slack-token", re: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9\-]{10,}`)},
	{name: "aws-access-key", re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{name: "private-key", re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{name: "jwt", re: regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`)},
	{name: "url-credentials", re: regexp.MustCompile(`\b[a-z][a-z0-9+.\-]*://[^\s/:@]+:[^\s/@]{6,}@`),
		check: func(m string) bool { return !strings.Contains(m, "://user:pass") && !strings.Contains(m, "${") }},
	{name: "assigned-secret", re: regexp.MustCompile(`(?i)[A-Za-z0-9_.\-]*(?:password|passwd|secret|api[_-]?key|access[_-]?token|auth[_-]?token)[A-Za-z0-9_\-]*["']?\s*[:=]\s*["']?([^\s"'<>]{12,})`),
		check: looksRandom},
}

// looksRandom accepts a value with both letters and digits and no
// placeholder markers, so "password: changeme-please" or "token=${TOKEN}"
// do not count.
func looksRandom(m string) bool {
	i := strings.IndexAny(m, ":=")
	v := strings.Trim(m[i+1:], " \t\"'")
	if strings.ContainsAny(v, "${}<>*") || strings.Contains(strings.ToLower(v), "redacted") || strings.Contains(strings.ToLower(v), "example") {
		return false
	}
	var letters, digits bool
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digits = true
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letters = true
		}
	}
	return letters && digits
}

// Find returns the name of the first rule that matches s, or "".
func Find(s string) string {
	for _, r := range rules {
		for _, m := range r.re.FindAllString(s, -1) {
			if r.check == nil || r.check(m) {
				return r.name
			}
		}
	}
	return ""
}

// Contains reports whether s looks like it carries a secret.
func Contains(s string) bool { return Find(s) != "" }
