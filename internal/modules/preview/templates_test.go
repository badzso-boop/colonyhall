// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package preview

import (
	"strings"
	"testing"
)

// The templates use the proxy base and the polling variables, and the
// guide carries both (the agent sees only the guide).
func TestTemplates(t *testing.T) {
	for name, tpl := range map[string]string{"vite": ViteTemplate, "static": StaticTemplate} {
		if !strings.Contains(tpl, "COLONY_PREVIEW_BASE") || !strings.Contains(tpl, "./:/app") {
			t.Errorf("%s template: no base or bind mount", name)
		}
		if !strings.Contains(TemplateGuide, tpl) {
			t.Errorf("%s template missing from the guide", name)
		}
	}
	for _, v := range []string{"CHOKIDAR_USEPOLLING=1", "WATCHPACK_POLLING=true", "/app/node_modules"} {
		if !strings.Contains(ViteTemplate, v) {
			t.Errorf("vite template lacks %s", v)
		}
	}
}
