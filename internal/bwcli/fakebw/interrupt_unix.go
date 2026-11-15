//go:build unix

package main

import (
	"os"
	"syscall"
	"time"
)

// interrupt ends the process by SIGINT with the default disposition, as the
// terminal does when the user presses the interrupt key.
func interrupt() {
	_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
	time.Sleep(10 * time.Second)
	os.Exit(1)
}
