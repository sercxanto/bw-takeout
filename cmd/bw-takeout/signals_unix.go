//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// interruptSignals count as interrupts of a backup (design D6): the
// keyboard interrupt and quit, terminate and hangup.
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP}

// keepRunningOnBrokenPipe keeps a closed standard output from ending the
// run before teardown. Without a subscription Go ends the process on a
// write to a broken pipe on file descriptor 1 or 2. With one, the write
// fails with EPIPE, and the run goes on to log out and remove its profile.
// A subscription, unlike ignoring the signal, is not inherited by bw.
func keepRunningOnBrokenPipe() {
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)
}
