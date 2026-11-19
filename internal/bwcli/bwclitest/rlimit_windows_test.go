//go:build windows

package bwclitest_test

import "testing"

// ownCoreLimit returns nil: Windows has no core file size limit.
func ownCoreLimit(*testing.T) *[2]uint64 { return nil }
