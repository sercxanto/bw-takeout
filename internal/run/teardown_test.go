package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/run"
	"bw-takeout/internal/verify"
)

func TestLoginRefused(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands["login"] = bwclitest.Behavior{Stdout: "not a session key\n"}
	e := newEnv(t, s)
	o := e.run()
	if o.Failure == nil || o.Failure.Phase != run.PhaseLogin || bwcli.ClassOf(o.Failure.Err) != bwcli.ClassOutput {
		t.Fatalf("failure %v", o.Failure)
	}
	if n := countOf(e.commands(), cmdLogout); n != 1 || o.Logout.Result != run.LogoutOK {
		t.Errorf("%d logouts, logout %+v", n, o.Logout)
	}
	e.wantProfileRemoved()
	if o.Published != nil || len(e.targetEntries()) != 0 {
		t.Errorf("published %v, target %v", o.Published, e.targetEntries())
	}
}

func TestFailureBeforeTheLogin(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdVersion] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	o := e.run()
	if o.Failure == nil || o.Failure.Phase != run.PhaseVersion {
		t.Fatalf("failure %v", o.Failure)
	}
	cmds := e.commands()
	if countOf(cmds, cmdLogin) != 0 || countOf(cmds, cmdLogout) != 0 {
		t.Errorf("commands %v", cmds)
	}
	if o.Logout.Result != run.LogoutNotNeeded || !o.TeardownComplete() {
		t.Errorf("logout %+v", o.Logout)
	}
	e.wantProfileRemoved()
}

func TestLogoutHangs(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdLogout] = bwclitest.Behavior{Hang: true}
	e := newEnv(t, s)
	e.deps.LogoutLimit = 200 * time.Millisecond
	start := time.Now()
	o := e.run()
	if o.Logout.Result != run.LogoutLimit || o.TeardownComplete() {
		t.Fatalf("logout %+v", o.Logout)
	}
	if !o.Logout.SessionForgotten {
		t.Error("session key kept")
	}
	if o.Logout.Duration < 200*time.Millisecond || time.Since(start) > 10*time.Second {
		t.Errorf("logout took %v, run %v", o.Logout.Duration, time.Since(start))
	}
	if o.Published == nil {
		t.Errorf("not published: %v", o.Failure)
	}
	e.wantProfileRemoved()
}

func TestDefaultLogoutLimit(t *testing.T) {
	if run.DefaultLogoutLimit != 10*time.Second {
		t.Fatalf("default %v", run.DefaultLogoutLimit)
	}
	if got := run.LogoutLimitOf(run.Deps{}); got != 10*time.Second {
		t.Fatalf("zero limit takes %v", got)
	}
	if got := run.LogoutLimitOf(run.Deps{LogoutLimit: time.Second}); got != time.Second {
		t.Fatalf("given limit becomes %v", got)
	}
}

func TestLogoutFails(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdLogout] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	o := e.run()
	if o.Logout.Result != run.LogoutFailed || o.Logout.Class != bwcli.ClassExit || o.TeardownComplete() {
		t.Fatalf("logout %+v", o.Logout)
	}
	e.wantProfileRemoved()
}

func TestProfileRemovedAfterFailedVerdict(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.VariantByName(t, ct.MissingItem).Build(t)))
	o := e.run()
	if o.Published != nil || o.Failure != nil {
		t.Fatalf("published %v, failure %v", o.Published, o.Failure)
	}
	if o.Verification == nil || o.Verification.Verdict != verify.Failed {
		t.Fatalf("verification %+v", o.Verification)
	}
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
	if o.Logout.Result != run.LogoutOK {
		t.Errorf("logout %+v", o.Logout)
	}
	e.wantProfileRemoved()
}

// keepProfile makes the removal of each new profile fail: on Unix the
// profile directory loses its write permission, on Windows a file inside
// it stays open. The cleanup undoes both.
func keepProfile(t *testing.T, e *env) {
	t.Helper()
	e.onProfile = func(p *profile.Profile) {
		if runtime.GOOS == "windows" {
			f, err := os.Create(filepath.Join(p.Path, "held-open"))
			if err != nil {
				t.Error(err)
				return
			}
			t.Cleanup(func() { f.Close(); os.RemoveAll(p.Path) })
			return
		}
		if err := os.Chmod(p.Path, 0o500); err != nil {
			t.Error(err)
		}
		t.Cleanup(func() { os.Chmod(p.Path, 0o700); os.RemoveAll(p.Path) })
	}
}

func TestRemovalFailureInOutcome(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	keepProfile(t, e)
	o := e.run()
	var re *profile.RemoveError
	if !errors.As(o.RemoveErr, &re) || re.Path != e.profile().Path {
		t.Fatalf("remove error %v", o.RemoveErr)
	}
	if o.TeardownComplete() || o.Published == nil || o.Published.Verdict != publish.VerdictVerified {
		t.Errorf("teardown complete %v, published %v", o.TeardownComplete(), o.Published)
	}
}
