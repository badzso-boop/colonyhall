// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The round 57 dev templates (docs/16-telepites.md, the preview_start tool)
// pass the validator as written.
const viteTemplate = `services:
  web:
    image: node:20-alpine
    working_dir: /app
    command: sh -c 'npm install && npm run dev -- --host 0.0.0.0 --port 5173 --base "$$COLONY_PREVIEW_BASE"'
    volumes:
      - ./:/app
      - /app/node_modules
    environment:
      - CHOKIDAR_USEPOLLING=1
      - WATCHPACK_POLLING=true
    ports:
      - "5173:5173"
`

const staticTemplate = `services:
  web:
    image: caddy:alpine
    command: sh -c 'mkdir -p "/srv$${COLONY_PREVIEW_BASE%/*/}" && ln -sfn /app "/srv$${COLONY_PREVIEW_BASE%/}" && caddy file-server --listen :8080 --root /srv'
    volumes:
      - ./:/app:ro
    ports:
      - "8080:8080"
`

func TestDevTemplatesPassValidation(t *testing.T) {
	root := t.TempDir()
	for name, raw := range map[string]string{"vite": viteTemplate, "static": staticTemplate} {
		info, err := ValidateComposeYAML([]byte(raw), filepath.Join(root, "compose.yaml"), root)
		if err != nil {
			t.Fatalf("%s template refused: %v", name, err)
		}
		if len(info.Ports) != 1 {
			t.Fatalf("%s template ports: %+v", name, info.Ports)
		}
	}
}

// An anonymous volume is a container path, not a host bind mount. Before
// round 57 "/app/node_modules" was refused as outside the root.
func TestAnonymousVolumeAllowedBindStillChecked(t *testing.T) {
	root := t.TempDir()
	ok := "services:\n  web:\n    image: x\n    volumes:\n      - /app/node_modules\n"
	if _, err := ValidateComposeYAML([]byte(ok), filepath.Join(root, "compose.yaml"), root); err != nil {
		t.Fatalf("anonymous volume refused: %v", err)
	}
	bad := "services:\n  web:\n    image: x\n    volumes:\n      - /etc:/app/etc\n"
	if _, err := ValidateComposeYAML([]byte(bad), filepath.Join(root, "compose.yaml"), root); err == nil || !strings.Contains(err.Error(), "outside root") {
		t.Fatalf("host bind outside the root accepted: %v", err)
	}
}

func TestOverrideSetsPreviewBaseOnEveryService(t *testing.T) {
	ports := []ComposeServicePort{{Service: "web", ContainerPort: 5173, HostPort: 41000}}
	out, err := GenerateOverrideContent("127.0.0.1", ports, nil, map[string]bool{"web": true},
		[]string{"web", "worker"}, map[string]string{"COLONY_PREVIEW_BASE": PreviewBasePath("p1")})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	for _, s := range []string{"web", "worker"} {
		if got := doc.Services[s].Environment["COLONY_PREVIEW_BASE"]; got != "/api/v1/previews/p1/proxy/" {
			t.Fatalf("%s base = %q\n%s", s, got, out)
		}
	}
}
