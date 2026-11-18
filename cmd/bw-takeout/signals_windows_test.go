//go:build windows

package main

import (
	"os"
	"slices"
	"syscall"
	"testing"
)

func TestInterruptSignalsOnWindows(t *testing.T) {
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		if !slices.Contains(interruptSignals, s) {
			t.Errorf("%v does not count as an interrupt", s)
		}
	}
}
