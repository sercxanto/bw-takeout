//go:build unix

package run_test

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

// ownLimits reads the core limit and, on Linux, the dumpable value of the
// process, as the lines "core <soft> <hard>" and "dumpable <v>".
func ownLimits() string {
	var r unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &r); err != nil {
		return "error: " + err.Error()
	}
	s := fmt.Sprintf("core %d %d", r.Cur, r.Max)
	if runtime.GOOS == "linux" {
		s += "\n" + dumpable()
	}
	return s
}
