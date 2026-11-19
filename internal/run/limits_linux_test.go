//go:build linux

package run_test

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func dumpable() string {
	v, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		return "error: " + err.Error()
	}
	return fmt.Sprintf("dumpable %d", v)
}
