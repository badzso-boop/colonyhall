//go:build unix

package guard

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the client in its own process group, so stopping it
// also stops whatever it started (shells, MCP servers, sub-agents).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
