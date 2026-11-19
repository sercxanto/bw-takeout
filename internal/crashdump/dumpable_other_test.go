//go:build unix && !linux

package crashdump

import "testing"

const hasDumpableMark = false

// fakeDumpable does nothing: there is no dumpable mark outside Linux.
func fakeDumpable(*testing.T, error, error, int) {}

// dumpableValue reports no value outside Linux.
func dumpableValue() (int, bool) { return 0, false }
