package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bw-takeout/internal/archive"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/run"
)

// wantCheckFailure checks a run that ended in the checks before the first
// bw process: no generation, no bw process and no profile.
func wantCheckFailure(t *testing.T, e *env, o *run.Outcome, want error) {
	t.Helper()
	if o.Failure == nil || o.Failure.Phase != run.PhaseChecks || !errors.Is(o.Failure.Err, want) {
		t.Fatalf("failure %v, want %v in the checks", o.Failure, want)
	}
	if o.Published != nil {
		t.Errorf("published %v", o.Published)
	}
	if o.ProfileCreated || o.Logout.Result != "" {
		t.Errorf("profile created %v, logout %q", o.ProfileCreated, o.Logout.Result)
	}
	e.wantNoProcessNoProfile()
	if names := e.targetEntries(); len(names) != 0 && e.cfg.Target == e.target {
		t.Errorf("target holds %v", names)
	}
}

func TestTargetMissing(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.Target = filepath.Join(e.root, "missing")
	wantCheckFailure(t, e, e.run(), run.ErrTarget)
}

func TestBWNotFound(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.BW = ""
	t.Setenv("PATH", t.TempDir())
	wantCheckFailure(t, e, e.run(), run.ErrBWNotFound)
}

func TestTargetIsAFile(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	file := filepath.Join(e.root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	e.cfg.Target = file
	wantCheckFailure(t, e, e.run(), run.ErrTarget)
}

func TestEmptyRecipientsFile(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	if err := os.WriteFile(e.recipients, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wantCheckFailure(t, e, e.run(), archive.ErrRecipientFile)
}

func TestRecipientsFileMissing(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.RecipientsFile = filepath.Join(e.root, "missing.txt")
	wantCheckFailure(t, e, e.run(), run.ErrRecipientsOpen)
}

func TestCAPathIsADirectory(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.CAFile = t.TempDir()
	wantCheckFailure(t, e, e.run(), run.ErrCAFile)
}

func TestBWPathIsADirectory(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.BW = t.TempDir()
	wantCheckFailure(t, e, e.run(), run.ErrBWNotFound)
}

// A relative --bw path is taken relative to the working directory and
// never looked up on the program search path.
func TestRelativeBWPath(t *testing.T) {
	t.Run("not in the working directory", func(t *testing.T) {
		e := newEnv(t, scenarioOf(t, ct.Intact(t)))
		t.Chdir(t.TempDir())
		t.Setenv("PATH", filepath.Dir(fakePath))
		e.cfg.BW = filepath.Base(fakePath)
		wantCheckFailure(t, e, e.run(), run.ErrBWNotFound)
	})
	t.Run("in the working directory", func(t *testing.T) {
		e := newEnv(t, scenarioOf(t, ct.Intact(t)))
		t.Chdir(filepath.Dir(fakePath))
		e.cfg.BW = filepath.Base(fakePath)
		o := e.run()
		if o.Published == nil || o.Setup.BWPath != fakePath {
			t.Fatalf("published %v, bw path %q", o.Published, o.Setup.BWPath)
		}
	})
}

func TestBWFoundOnTheSearchPath(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	dir := t.TempDir()
	data, err := os.ReadFile(fakePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bw"+filepath.Ext(fakePath)), data, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	e.cfg.BW = ""
	o := e.run()
	if o.Published == nil || o.Setup.BWPath != filepath.Join(dir, "bw"+filepath.Ext(fakePath)) {
		t.Fatalf("published %v, bw path %q", o.Published, o.Setup.BWPath)
	}
}
