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

// TestModelServerTurnsOffTelemetry: ONNX Runtime's Linux build starts a 1DS
// telemetry client while it loads, unless ORT_DISABLE_TELEMETRY is set before
// the import (2026-11-13).
func TestModelServerTurnsOffTelemetry(t *testing.T) {
	dir := filepath.Join("..", "..", "deploy", "embed")
	src, err := os.ReadFile(filepath.Join(dir, "server.py"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	set := strings.Index(s, `os.environ["ORT_DISABLE_TELEMETRY"] = "1"`)
	imp := strings.Index(s, "import onnxruntime")
	if set < 0 || imp < 0 || set > imp {
		t.Fatalf("server.py must set ORT_DISABLE_TELEMETRY before importing onnxruntime (set at %d, import at %d)", set, imp)
	}
	docker, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(docker), "ENV ORT_DISABLE_TELEMETRY=1") {
		t.Fatal("deploy/embed/Dockerfile must set ORT_DISABLE_TELEMETRY=1")
	}
}
