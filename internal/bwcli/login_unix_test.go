//go:build unix

package bwcli

import (
	"strings"
	"syscall"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
)

// interruptAtPrompt waits until the fake has written its prompt, sends
// SIGINT to the test process as the terminal does to the foreground process
// group, and then gives the fake its line. If the login did not hold the
// signal, the test process ends here.
func interruptAtPrompt(t *testing.T, f *loginFixture) {
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(f.stderrText(), testPrompt) {
		if time.Now().After(deadline) {
			t.Error("no prompt")
			f.stdinW.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Error(err)
	}
	// Signal delivery to the channel is asynchronous.
	time.Sleep(200 * time.Millisecond)
	f.stdinW.WriteString(testPassword + "\n")
}

func TestInterruptAtThePasswordPrompt(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Prompt: testPrompt, ReadLine: true, Interrupt: true, Stdout: testKey}, "")
	ended := false
	f.term.onRestore = func() {
		if recs := f.loginRecords(); len(recs) == 1 {
			ended = true
			assertStopped(t, recs[0].PID)
		}
	}
	done := make(chan error, 1)
	go func() { done <- f.client.Login(bg) }()
	interruptAtPrompt(t, f)
	err := <-done
	wantLoginFailed(t, f.fixture, err, ClassCancelled)
	if !ended {
		t.Error("terminal restored before bw had read the line and ended")
	}
	wantRestored(t, f.term)
}

func TestInterruptWithASuccessfulChild(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Prompt: testPrompt, ReadLine: true, Stdout: testKey}, "")
	done := make(chan error, 1)
	go func() { done <- f.client.Login(bg) }()
	interruptAtPrompt(t, f)
	err := <-done
	wantLoginFailed(t, f.fixture, err, ClassCancelled)
	wantRestored(t, f.term)
}
