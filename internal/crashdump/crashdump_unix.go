//go:build unix

package crashdump

import "golang.org/x/sys/unix"

// The system calls, replaced by the tests of the package.
var (
	setRlimit = unix.Setrlimit
	getRlimit = unix.Getrlimit
)

// Limit sets the soft and the hard core file size limit to zero and reads
// them back, then marks the process as not dumpable where the platform
// allows it (design D1, D4).
func Limit() (Protection, error) {
	if err := setRlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return Protection{}, ErrCoreLimit
	}
	var got unix.Rlimit
	if err := getRlimit(unix.RLIMIT_CORE, &got); err != nil || got.Cur != 0 || got.Max != 0 {
		return Protection{}, ErrCoreLimit
	}
	p := Protection{CoreLimit: true}
	notDumpable, err := markNotDumpable()
	if err != nil {
		return Protection{}, err
	}
	p.NotDumpable = notDumpable
	return p, nil
}
