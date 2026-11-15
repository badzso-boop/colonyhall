// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package setup

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Answers are the installer's inputs: asked interactively, or given as
// flags or in an answers file (--answers) for an unattended install.
// Secrets (passwords, a database URL) are better given in the environment
// (COLONY_ADMIN_PASSWORD, COLONY_DATABASE_URL) than in a file.
type Answers struct {
	Mode   string        `yaml:"mode"` // server, guard, both
	Server ServerAnswers `yaml:"server"`
	Guard  GuardAnswers  `yaml:"guard"`
}

// ServerAnswers configure the server.
type ServerAnswers struct {
	Dir           string `yaml:"dir"`
	Image         string `yaml:"image"`
	Bind          string `yaml:"bind"` // an address, never 0.0.0.0 without AllowAll
	AllowAll      bool   `yaml:"allow_all_interfaces"`
	Port          int    `yaml:"port"`
	DatabaseURL   string `yaml:"database_url"` // empty: its own Postgres
	AdminHandle   string `yaml:"admin_handle"`
	AdminName     string `yaml:"admin_name"`
	AdminPassword string `yaml:"admin_password"`
	Backups       *bool  `yaml:"backups"`
	SecureCookies bool   `yaml:"secure_cookies"` // behind HTTPS
	// OperatorAllowlist (round 51): the addresses admin.<domain> answers,
	// IPs or CIDRs, comma-separated; "-" or empty in the file: any address.
	OperatorAllowlist string `yaml:"operator_allowlist"`
}

// GuardAnswers configure the guard.
type GuardAnswers struct {
	ServerURL         string   `yaml:"server_url"`
	PairCode          string   `yaml:"pair_code"`
	Name              string   `yaml:"name"`
	Roots             []string `yaml:"roots"`
	AllowUnsandboxed  *bool    `yaml:"allow_unsandboxed"`
	MaxConcurrentRuns *int     `yaml:"max_concurrent_runs"`
	PreviewPorts      string   `yaml:"preview_ports"`
	ExchangeDir       string   `yaml:"exchange_dir"`
	Service           *bool    `yaml:"service"`
	Linger            *bool    `yaml:"linger"`
	WSLSystemd        *bool    `yaml:"wsl_systemd"`
	Keepalive         *bool    `yaml:"wsl_keepalive"`
	Clients           []string `yaml:"clients"` // claude, agy: install when missing
}

// LoadAnswers reads an answers file; it warns when others can read it.
func LoadAnswers(path string, warn func(string)) (Answers, error) {
	var a Answers
	raw, err := os.ReadFile(path)
	if err != nil {
		return a, err
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 && warn != nil {
		warn(path)
	}
	dec := yaml.NewDecoder(bytesReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("%s: %w", path, err)
	}
	return a, nil
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}
