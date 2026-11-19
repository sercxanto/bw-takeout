//go:build unix

package bwclitest_test

import (
	"syscall"
	"testing"
)

// ownCoreLimit returns the core file size limit of the test process.
func ownCoreLimit(t *testing.T) *[2]uint64 {
	t.Helper()
	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_CORE, &r); err != nil {
		t.Fatal(err)
	}
	return &[2]uint64{uint64(r.Cur), uint64(r.Max)}
}
