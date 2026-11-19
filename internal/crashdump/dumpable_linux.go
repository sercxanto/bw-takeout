//go:build linux

package crashdump

import "golang.org/x/sys/unix"

// The system calls, replaced by the tests of the package.
var (
	setDumpable = func(v int) error { return unix.Prctl(unix.PR_SET_DUMPABLE, uintptr(v), 0, 0, 0) }
	getDumpable = func() (int, error) { return unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0) }
)

// markNotDumpable marks the process as not dumpable and reads the mark
// back. A plain exec resets it, so it never reaches bw.
func markNotDumpable() (bool, error) {
	if err := setDumpable(0); err != nil {
		return false, ErrDumpable
	}
	v, err := getDumpable()
	if err != nil || v != 0 {
		return false, ErrDumpable
	}
	return true, nil
}
