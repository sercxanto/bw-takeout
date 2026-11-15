//go:build unix

package bwcli

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts the process in a process group of its own and lets
// a cancel kill the whole group, so that a child of bw, such as Node under
// a start script, is stopped too (design D4).
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
