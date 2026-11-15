//go:build windows

package main

import "os"

// statusControlCExit is the exit code of a console process ended by Ctrl+C.
const statusControlCExit = 0xC000013A

// interrupt ends the process as a console interrupt would.
func interrupt() {
	os.Exit(statusControlCExit)
}
