//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

func TestInterruptSignalsOnUnix(t *testing.T) {
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP} {
		if !slices.Contains(interruptSignals, s) {
			t.Errorf("%v does not count as an interrupt", s)
		}
	}
}

// Review fix 9.5 (S2): a closed standard output does not end the backup
// command by SIGPIPE. The run then reaches its end, here after a failed
// check, and returns its own exit status.
func TestBrokenPipeOnStandardOutput(t *testing.T) {
	s := newSandbox(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	defer w.Close()
	cmd := exec.Command(binary, "backup", "--server", "https://vault.example.org",
		"--target", filepath.Join(s.target, "missing"), "--recipients-file", s.recipients)
	cmd.Env = s.env
	cmd.Stdout = w
	err = cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != exitFailed {
		t.Fatalf("got %v, want exit status %d", err, exitFailed)
	}
}
