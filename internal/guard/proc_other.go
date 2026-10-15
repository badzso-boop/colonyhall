//go:build !unix

package guard

import "os/exec"

// setProcessGroup is a no-op here; ctx cancellation kills the direct child.
func setProcessGroup(*exec.Cmd) {}
