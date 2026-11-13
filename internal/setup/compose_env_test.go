package setup

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Settings the server reads that the production compose files deliberately
// do not pass through.
var composeEnvExceptions = map[string]string{
	"COLONY_DEV_SEED":         "development only",
	"COLONY_ROLE":             "only the default (all) works today",
	"COLONY_CALENDAR_REFRESH": "tuning; the default is fine",
}

var colonyEnvName = regexp.MustCompile(`"(COLONY_[A-Z0-9_]+)"`)

// serverEnvNames lists every COLONY_ variable the server's config package
// reads (from its source, so a new setting cannot be forgotten).
func serverEnvNames(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("..", "platform", "config")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range colonyEnvName.FindAllStringSubmatch(string(src), -1) {
			seen[m[1]] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) < 10 {
		t.Fatalf("found only %d settings in the config package: %v", len(names), names)
	}
	return names
}

// TestComposeFilesPassEveryServerSetting: a setting documented in
// .env.prod.example but missing from a compose file silently does nothing
// (COLONY_RERANK_TIMEOUT, 2026-11-13).
func TestComposeFilesPassEveryServerSetting(t *testing.T) {
	deploy, err := os.ReadFile(filepath.Join("..", "..", "deploy", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"deploy/compose.yaml":                   string(deploy),
		"installer compose (own Postgres)":      composeFile(true),
		"installer compose (existing Postgres)": composeFile(false),
	}
	for _, name := range serverEnvNames(t) {
		if _, ok := composeEnvExceptions[name]; ok {
			continue
		}
		for file, body := range files {
			if !strings.Contains(body, "      "+name+":") {
				t.Errorf("%s does not pass %s to the server", file, name)
			}
		}
	}
}
