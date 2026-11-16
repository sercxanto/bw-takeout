//go:build windows

package bwcli

import "os/exec"

// ownProcessGroup leaves the default cancel, which kills the process.
// Killing a process tree on Windows is a non-goal (design D4); reading the
// pipes still ends after waitDelay.
func ownProcessGroup(cmd *exec.Cmd) {}
