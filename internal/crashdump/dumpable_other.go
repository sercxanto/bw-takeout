//go:build unix && !linux

package crashdump

// markNotDumpable does nothing outside Linux: the core limit is the whole
// protection there.
func markNotDumpable() (bool, error) { return false, nil }
