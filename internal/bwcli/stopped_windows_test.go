//go:build windows

package bwcli

import "testing"

// assertStopped does not check on Windows: killing a process tree is a
// non-goal there (design D4), and the client kills only the process it
// started. The failure class is still checked by the callers.
func assertStopped(t *testing.T, pids ...int) {}
