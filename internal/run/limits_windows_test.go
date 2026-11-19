//go:build windows

package run_test

// ownLimits is never called on Windows: the real-path tests skip there.
func ownLimits() string { return "" }
