package guard

import "testing"

// A Supervisor built without Validate (tests, embedders) must not crash on
// a missing require_sandbox: it means the default, true.
func TestRequireSandboxDefaultsToTrue(t *testing.T) {
	off := false
	if !(&Config{}).requireSandbox() {
		t.Error("unset require_sandbox should mean true")
	}
	if (&Config{RequireSandbox: &off}).requireSandbox() {
		t.Error("require_sandbox: false ignored")
	}
}
