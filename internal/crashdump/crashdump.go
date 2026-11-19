// Package crashdump limits what a crash of the run, or of a bw process it
// starts, can write to disk. On Linux and macOS, Limit sets the soft and the
// hard core file size limit of the process to zero, which every child
// inherits. On Linux it also marks the process itself as not dumpable. On
// Windows it changes nothing. See the design of the change memory-hardening,
// decisions D1 and D4, and the specification backup-run.
package crashdump

import "errors"

// Protection tells what holds after Limit.
type Protection struct {
	// CoreLimit is true when the soft and the hard core file size limit
	// are zero.
	CoreLimit bool
	// NotDumpable is true when the process is marked as not dumpable.
	NotDumpable bool
}

// The errors carry no detail beyond what failed (design D1). A read-back
// that differs from the set value gives the same error as a failed set.
var (
	ErrCoreLimit = errors.New("core file size limit cannot be set to zero")
	ErrDumpable  = errors.New("process cannot be marked as not dumpable")
)
