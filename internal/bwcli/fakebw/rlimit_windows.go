//go:build windows

package main

// coreLimit returns nil: Windows has no core file size limit.
func coreLimit() *[2]uint64 { return nil }
