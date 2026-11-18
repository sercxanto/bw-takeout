package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
)

// flushFailingPublisher publishes and then reports the error that
// publish.Publisher returns when the target directory cannot be flushed
// after the rename: the generation together with an error.
type flushFailingPublisher struct{}

func (flushFailingPublisher) Publish(target string, write publish.WriteFunc) (*publish.Published, error) {
	pub, err := (&publish.Publisher{}).Publish(target, write)
	if err != nil {
		return pub, err
	}
	return pub, errors.New("generation " + pub.Name + " is complete, but the target directory could not be flushed")
}

// Review fix 9.1 (C1, S1).
func TestPublishedGenerationWithFlushError(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.deps.Publisher = flushFailingPublisher{}
	r := e.runReported()
	if r.o.Published == nil || r.o.PublishWarning == nil || r.o.Failure != nil {
		t.Fatalf("published %v, warning %v, failure %v", r.o.Published, r.o.PublishWarning, r.o.Failure)
	}
	if r.status != report.ExitVerified {
		t.Errorf("status %d, want 0", r.status)
	}
	r.want(t, "Generation:  "+r.o.Published.Path+"\n", "Warning:     generation ")
	if strings.Contains(r.stdout, "none published") || strings.Contains(r.stdout, "stays in the target") {
		t.Errorf("report denies the generation\n%s", r.stdout)
	}
}

// blockAbort puts a file of its own into the partial directory of the run
// once the write function has returned, so that the abort leaves the
// directory in place.
func blockAbort(t *testing.T, e *env) {
	t.Helper()
	e.deps.Publisher = hookPublisher{afterWrite: func() {
		entries, err := os.ReadDir(e.target)
		if err != nil {
			t.Error(err)
			return
		}
		for _, x := range entries {
			if strings.HasSuffix(x.Name(), publish.PartialMarker) {
				if err := os.WriteFile(filepath.Join(e.target, x.Name(), "foreign"), nil, 0o600); err != nil {
					t.Error(err)
				}
			}
		}
	}}
}

func partials(t *testing.T, e *env) int {
	t.Helper()
	n := 0
	for _, name := range e.targetEntries() {
		if strings.HasSuffix(name, publish.PartialMarker) {
			n++
		}
	}
	return n
}

// Review fix 9.2 (C2, S3): a failed verdict whose partial directory cannot
// be removed.
func TestFailedVerdictWithFailedAbort(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.VariantByName(t, ct.MissingItem).Build(t)))
	blockAbort(t, e)
	r := e.runReported()
	if !r.o.PartialLeft || r.o.Failure == nil || r.o.Failure.Phase != run.PhaseCommit || !errors.Is(r.o.Failure.Err, publish.ErrFailedCapture) {
		t.Fatalf("partial left %v, failure %v", r.o.PartialLeft, r.o.Failure)
	}
	if partials(t, e) != 1 || r.status != 1 {
		t.Errorf("%d partial directories, status %d", partials(t, e), r.status)
	}
	r.want(t, "Verdict:     failed", "The partial generation stays in the target")
}

// Review fix 9.2 (C2): a failed write whose partial directory cannot be
// removed, and one whose partial directory is removed.
func TestWriteFailureWithFailedAbort(t *testing.T) {
	broken := func(t *testing.T) bwclitest.Scenario {
		s := scenarioOf(t, ct.Intact(t))
		s.Commands["export --format json --raw"] = bwclitest.Behavior{Stdout: "[not json"}
		return s
	}
	t.Run("abort fails", func(t *testing.T) {
		e := newEnv(t, broken(t))
		blockAbort(t, e)
		r := e.runReported()
		if !r.o.PartialLeft || r.o.Failure == nil || r.o.Failure.Phase != run.PhaseArchive {
			t.Fatalf("partial left %v, failure %v", r.o.PartialLeft, r.o.Failure)
		}
		if partials(t, e) != 1 {
			t.Errorf("%d partial directories", partials(t, e))
		}
		r.want(t, "The partial generation stays in the target")
	})
	t.Run("abort succeeds", func(t *testing.T) {
		e := newEnv(t, broken(t))
		r := e.runReported()
		if r.o.PartialLeft || len(e.targetEntries()) != 0 || strings.Contains(r.stdout, "stays in the target") {
			t.Fatalf("partial left %v, target %v\n%s", r.o.PartialLeft, e.targetEntries(), r.stdout)
		}
	})
}

// Review fix 9.3 (C4): a panic after the login still logs out and removes
// the profile, and its value is not reported.
func TestPanicKeepsTeardown(t *testing.T) {
	for _, c := range []struct {
		name  string
		kind  string
		phase run.Phase
	}{
		{"after the snapshot", "snapshot", run.PhaseSnapshot},
		{"inside the write function", "attachment", run.PhaseAttachments},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t, scenarioOf(t, ct.Intact(t)))
			e.progress.on = func(x call) {
				if x.kind == c.kind {
					panic(ct.Marker + "-panic-value")
				}
			}
			r := e.runReported()
			if r.o.Failure == nil || r.o.Failure.Err != run.ErrInternal || r.o.Failure.Phase != c.phase {
				t.Fatalf("failure %v", r.o.Failure)
			}
			if n := countOf(e.commands(), cmdLogout); n != 1 || r.o.Logout.Result != run.LogoutOK {
				t.Errorf("%d logouts, logout %+v", n, r.o.Logout)
			}
			e.wantProfileRemoved()
			if names := e.targetEntries(); len(names) != 0 || r.o.Published != nil {
				t.Errorf("target holds %v, published %v", names, r.o.Published)
			}
			if strings.Contains(r.stdout+r.stderr, ct.Marker) {
				t.Errorf("panic value in the output\n%s", r.stdout)
			}
		})
	}
}

// Review fix 9.4 (C5): the cause of the teardown context decides, also
// when bw ended on its own with a non-zero exit code.
func TestLogoutResultByCause(t *testing.T) {
	exit := &bwcli.Error{Op: bwcli.OpLogout, Class: bwcli.ClassExit, ExitCode: 1}
	cancelled := &bwcli.Error{Op: bwcli.OpLogout, Class: bwcli.ClassCancelled, ExitCode: bwcli.NoExitCode}
	for _, c := range []struct {
		err   error
		ended string
		want  run.LogoutResult
		class bwcli.Class
	}{
		{nil, "", run.LogoutOK, ""},
		{exit, "", run.LogoutFailed, bwcli.ClassExit},
		{exit, "skipped", run.LogoutSkipped, ""},
		{exit, "limit", run.LogoutLimit, ""},
		{cancelled, "skipped", run.LogoutSkipped, ""},
		{cancelled, "limit", run.LogoutLimit, ""},
	} {
		got, class := run.LogoutResultOf(c.err, c.ended)
		if got != c.want || class != c.class {
			t.Errorf("%v with %q: %s %q, want %s %q", c.err, c.ended, got, class, c.want, c.class)
		}
	}
}

// Review fix 9.9 (R2-C1): a failure before the first signal stays in the
// report when the signal arrives during teardown.
func TestFailureBeforeAnInterruptDuringTheLogout(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands["login"] = bwclitest.Behavior{Stdout: "not a session key\n"}
	s.Commands[cmdLogout] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	go func() {
		e.waitForCommand(cmdLogout)
		e.signals <- os.Interrupt
	}()
	r := e.runReported()
	if r.o.Failure == nil || r.o.Failure.Phase != run.PhaseLogin || r.o.Failure.AfterInterrupt || r.o.InterruptedIn != run.PhaseTeardown {
		t.Fatalf("failure %+v, interrupted in %s", r.o.Failure, r.o.InterruptedIn)
	}
	r.want(t, "Failed:      login: bw login failed: output, exit code 0\n", "Interrupted during teardown.\n", "Logout:      skipped by the interrupt")
}

// Review fix 9.10 (R2-C2): a signal after the logout has ended changes
// neither its result nor announces that it is skipped.
func TestSignalAfterTheLogoutReturned(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdLogout] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	t.Cleanup(run.SetLogoutReturnedHook(func(interrupted func() bool) {
		e.signals <- os.Interrupt
		deadline := time.Now().Add(10 * time.Second)
		for !interrupted() && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	}))
	r := e.runReported()
	if !r.o.Interrupted || r.o.InterruptedIn != run.PhaseTeardown {
		t.Fatalf("interrupted %v in %s", r.o.Interrupted, r.o.InterruptedIn)
	}
	if r.o.Logout.Result != run.LogoutFailed || r.o.Logout.Class != bwcli.ClassExit {
		t.Errorf("logout %+v", r.o.Logout)
	}
	for _, c := range e.progress.all("interrupted") {
		if c.interrupt.LogoutSkipped {
			t.Errorf("a skip of the ended logout was announced")
		}
	}
	if strings.Contains(r.stderr, "Skipping the logout") {
		t.Errorf("standard error %q", r.stderr)
	}
	e.wantProfileRemoved()
}
