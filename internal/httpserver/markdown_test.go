package httpserver_test

import (
	"os/exec"
	"testing"
)

func TestMarkdownJS(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found on PATH, skipping frontend markdown tests")
	}

	cmd := exec.Command(nodePath, "--test", "web/markdown.test.mjs")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node --test web/markdown.test.mjs failed:\n%s", string(out))
	}
	t.Logf("node test output:\n%s", string(out))
}
