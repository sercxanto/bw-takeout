package run_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
)

// hookPublisher publishes with a zero publish.Publisher and calls
// afterWrite once the write function of the run has returned, before the
// commit, and afterPublish once Publish has returned, before teardown.
type hookPublisher struct {
	afterWrite, afterPublish func()
}

func (h hookPublisher) Publish(target string, write publish.WriteFunc) (*publish.Published, error) {
	pub, err := (&publish.Publisher{}).Publish(target, func(w io.Writer, created time.Time) ([]byte, error) {
		v, err := write(w, created)
		if h.afterWrite != nil {
			h.afterWrite()
		}
		return v, err
	})
	if h.afterPublish != nil {
		h.afterPublish()
	}
	return pub, err
}

// start runs the backup in a goroutine and returns the channel of its
// outcome.
func (e *env) start() <-chan *run.Outcome {
	done := make(chan *run.Outcome, 1)
	go func() { done <- run.Run(e.cfg, e.deps) }()
	return done
}

func (e *env) wait(done <-chan *run.Outcome) *run.Outcome {
	e.t.Helper()
	select {
	case o := <-done:
		return o
	case <-time.After(30 * time.Second):
		e.t.Fatal("the run did not end")
		return nil
	}
}

var secondAttachment = attachmentCommand(ct.ItemPasskeyLogin, ct.AttachmentSecond)

func wantNotices(t *testing.T, r *recorder, skipped ...bool) {
	t.Helper()
	got := r.all("interrupted")
	if len(got) != len(skipped) {
		t.Fatalf("%d interrupt notices, want %d: %+v", len(got), len(skipped), got)
	}
	for i, c := range got {
		if c.interrupt.LogoutSkipped != skipped[i] || c.interrupt.LogoutLimit != 5*time.Second {
			t.Errorf("notice %d is %+v, want skipped %v", i, c.interrupt, skipped[i])
		}
	}
}

func TestInterruptDuringTheAttachments(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	done := e.start()
	e.waitForCommand(secondAttachment)
	e.signals <- os.Interrupt
	o := e.wait(done)
	if !o.Interrupted || o.InterruptedIn != run.PhaseAttachments || o.Failure == nil || o.Failure.Phase != run.PhaseAttachments || o.Published != nil {
		t.Fatalf("interrupted %v, failure %v, published %v", o.Interrupted, o.Failure, o.Published)
	}
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
	wantNotices(t, e.progress, false)
	if n := countOf(e.commands(), cmdLogout); n != 1 || o.Logout.Result != run.LogoutOK {
		t.Errorf("%d logouts, logout %+v", n, o.Logout)
	}
	e.wantProfileRemoved()
}

func TestInterruptBeforeTheTargetIsTouched(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands["list items --raw"] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	preexisting(t, e)
	before := hashTree(t, e.target)
	done := e.start()
	e.waitForCommand("list items --raw")
	e.signals <- os.Interrupt
	o := e.wait(done)
	if !o.Interrupted || o.InterruptedIn != run.PhaseSnapshot || o.Failure == nil || o.Failure.Phase != run.PhaseSnapshot {
		t.Fatalf("interrupted %v, failure %v", o.Interrupted, o.Failure)
	}
	if !sameTree(before, hashTree(t, e.target)) {
		t.Error("the target changed")
	}
	e.wantProfileRemoved()
}

func TestSecondInterruptDuringTheLogout(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true}
	s.Commands[cmdLogout] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	done := e.start()
	e.waitForCommand(secondAttachment)
	e.signals <- os.Interrupt
	e.waitForCommand(cmdLogout)
	start := time.Now()
	e.signals <- os.Interrupt
	o := e.wait(done)
	if o.Logout.Result != run.LogoutSkipped || o.TeardownComplete() || !o.Logout.SessionForgotten {
		t.Fatalf("logout %+v", o.Logout)
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("the logout was not stopped at once: %v", time.Since(start))
	}
	wantNotices(t, e.progress, false, true)
	e.wantProfileRemoved()
}

func TestFirstInterruptDuringTheLogout(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdLogout] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	done := e.start()
	e.waitForCommand(cmdLogout)
	e.signals <- os.Interrupt
	o := e.wait(done)
	if !o.Interrupted || o.InterruptedIn != run.PhaseTeardown || o.Logout.Result != run.LogoutSkipped || o.Published == nil {
		t.Fatalf("interrupted %v, logout %+v, published %v", o.Interrupted, o.Logout, o.Published)
	}
	wantNotices(t, e.progress, true)
	e.wantProfileRemoved()
}

// The signal is handled only after the version query has ended with a
// non-zero exit code of its own, as when the console of Windows sends the
// interrupt to bw as well and bw ends first.
func TestBWEndsOnItsOwnAfterTheInterrupt(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdVersion] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	e.progress.on = func(c call) {
		if c.kind == "setup" {
			e.signals <- os.Interrupt
			e.progress.waitFor(t, "interrupted", 1)
		}
	}
	o := e.run()
	if o.Failure == nil || o.Failure.Phase != run.PhaseVersion || bwcli.ClassOf(o.Failure.Err) != bwcli.ClassExit {
		t.Fatalf("failure %v", o.Failure)
	}
	if !o.Interrupted {
		t.Error("the outcome does not count as interrupted")
	}
	e.wantProfileRemoved()
}

func TestTerminateSignal(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	done := e.start()
	e.waitForCommand(secondAttachment)
	e.signals <- syscall.SIGTERM
	o := e.wait(done)
	if !o.Interrupted || o.Published != nil || o.Logout.Result != run.LogoutOK {
		t.Fatalf("interrupted %v, published %v, logout %+v", o.Interrupted, o.Published, o.Logout)
	}
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
	e.wantProfileRemoved()
}

// A second signal that arrives before the logout starts lets the logout
// end without starting bw, and the session key is still forgotten.
func TestSecondInterruptBeforeTheLogout(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	e.deps.Publisher = hookPublisher{afterPublish: func() {
		e.signals <- os.Interrupt
		e.progress.waitFor(t, "interrupted", 2)
	}}
	done := e.start()
	e.waitForCommand(secondAttachment)
	e.signals <- os.Interrupt
	e.progress.waitFor(t, "interrupted", 1)
	o := e.wait(done)
	if n := countOf(e.commands(), cmdLogout); n != 0 {
		t.Errorf("%d logout processes, want none", n)
	}
	if o.Logout.Result != run.LogoutSkipped || !o.Logout.SessionForgotten {
		t.Errorf("logout %+v", o.Logout)
	}
	wantNotices(t, e.progress, false, true)
	e.wantProfileRemoved()
}

// Publishing takes no context: a signal after the write function has
// returned lets the commit finish.
func TestInterruptDuringTheCommit(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.deps.Publisher = hookPublisher{afterWrite: func() {
		e.signals <- os.Interrupt
		e.progress.waitFor(t, "interrupted", 1)
	}}
	o := e.run()
	if !o.Interrupted || o.InterruptedIn != run.PhaseCommit || o.Published == nil || o.Published.Verdict != publish.VerdictVerified {
		t.Fatalf("interrupted %v, published %v, failure %v", o.Interrupted, o.Published, o.Failure)
	}
	if _, marker, ok := publish.ParseName(o.Published.Name); !ok || marker != "" {
		t.Errorf("published as %s", o.Published.Name)
	}
	if o.Logout.Result != run.LogoutOK {
		t.Errorf("logout %+v", o.Logout)
	}
	e.wantProfileRemoved()
}

// A signal after the downloads, while the archive is not committed yet,
// aborts the generation.
func TestInterruptBeforeTheCommitAborts(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.progress.on = func(c call) {
		if c.kind == "attachment" && c.position == c.total {
			e.signals <- os.Interrupt
			e.progress.waitFor(t, "interrupted", 1)
		}
	}
	o := e.run()
	if !o.Interrupted || o.Published != nil || o.Failure == nil || o.Failure.Err != run.ErrInterrupted {
		t.Fatalf("interrupted %v, published %v, failure %v", o.Interrupted, o.Published, o.Failure)
	}
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
	e.wantProfileRemoved()
}

// tee sends every progress call to the recorder and to a report writer.
type tee struct {
	*recorder
	w *report.Writer
}

func (t tee) Setup(s run.Setup)         { t.w.Setup(s); t.recorder.Setup(s) }
func (t tee) Leftovers(l run.Leftovers) { t.w.Leftovers(l); t.recorder.Leftovers(l) }
func (t tee) Snapshot(s manifest.Source, planned int) {
	t.w.Snapshot(s, planned)
	t.recorder.Snapshot(s, planned)
}
func (t tee) Attachment(position, total int, d capture.Download) {
	t.w.Attachment(position, total, d)
	t.recorder.Attachment(position, total, d)
}
func (t tee) Interrupted(i run.Interrupt) { t.w.Interrupted(i); t.recorder.Interrupted(i) }

// wireReport sends the progress of the run to a report writer as well and
// returns its two streams.
func (e *env) wireReport() (stdout, stderr *bytes.Buffer) {
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	e.deps.Progress = tee{e.progress, report.New(stdout, stderr)}
	return stdout, stderr
}

func TestInterruptMessagesOnStandardError(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Hang: true}
	s.Commands[cmdLogout] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	e.deps.LogoutLimit = 10 * time.Second
	stdout, stderr := e.wireReport()
	done := e.start()
	e.waitForCommand(secondAttachment)
	e.signals <- os.Interrupt
	e.waitForCommand(cmdLogout)
	e.signals <- os.Interrupt
	o := e.wait(done)
	if o.Logout.Result != run.LogoutSkipped {
		t.Fatalf("logout %+v", o.Logout)
	}
	want := "Interrupted. Logging out, at most 10 s. Press Ctrl-C again to skip the logout.\n" +
		"Interrupted. Skipping the logout.\n"
	if stderr.String() != want {
		t.Errorf("standard error is %q, want %q", stderr.String(), want)
	}
	if strings.Contains(stdout.String(), "Interrupted.") {
		t.Errorf("interrupt message on standard output:\n%s", stdout)
	}
}
