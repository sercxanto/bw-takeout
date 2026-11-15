//go:build windows

package bwcli

import "os"

// interruptSignals are the console events that end a process. The login
// holds them while bw runs (design D6).
var interruptSignals = []os.Signal{os.Interrupt}
