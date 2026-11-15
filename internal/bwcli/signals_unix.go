//go:build unix

package bwcli

import (
	"os"
	"syscall"
)

// interruptSignals are the signals the terminal sends to the foreground
// process group from the keyboard. The login holds them while bw runs
// (design D6).
var interruptSignals = []os.Signal{os.Interrupt, syscall.SIGQUIT}
