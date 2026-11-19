//go:build unix

package main

import "syscall"

// coreLimit returns the soft and the hard core file size limit of the
// process, or nil when they cannot be read.
func coreLimit() *[2]uint64 {
	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &r); err != nil {
		return nil
	}
	return &[2]uint64{uint64(r.Cur), uint64(r.Max)}
}
