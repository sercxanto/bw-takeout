//go:build windows

package main

import (
	"os"
	"syscall"
)

// interruptSignals count as interrupts of a backup (design D6). Go reports
// Ctrl-C and Ctrl-Break as os.Interrupt, and the console close, logoff and
// shutdown events as SIGTERM. The system ends the process a few seconds
// after those events, so teardown may not finish.
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}

// keepRunningOnBrokenPipe does nothing on Windows, which has no SIGPIPE.
func keepRunningOnBrokenPipe() {}
