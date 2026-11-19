package run_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/crashdump"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
)

// The crash dump tests of the change memory-hardening (design D6). The
// in-process tests inject the limit. The real crashdump.Limit runs only in
// a re-executed child of the test binary, so that the test process keeps
// its own limits.

// wantCrashDumpFailure checks a run whose crash dump limit failed: exit
// status 1, the checks phase, the Failed: line naming what failed, no
// record of the fake and no profile.
func wantCrashDumpFailure(t *testing.T, e *env, r reported, want error, names string) {
	t.Helper()
	wantCheckFailure(t, e, r.o, want)
	if r.status != report.ExitFailed {
		t.Errorf("exit status %d", r.status)
	}
	r.want(t, "Failed:      checks: "+want.Error())
	if !strings.Contains(want.Error(), names) {
		t.Errorf("error %q does not name the %s", want, names)
	}
	for _, d := range e.dataDirs {
		if _, err := os.Stat(filepath.Join(d, bwclitest.RecordFile)); !os.IsNotExist(err) {
			t.Errorf("record file of the fake exists (%v)", err)
		}
	}
	if e.limitCalls != 1 {
		t.Errorf("limit called %d times", e.limitCalls)
	}
}

func TestCoreLimitCannotBeSet(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.limitErr = crashdump.ErrCoreLimit
	wantCrashDumpFailure(t, e, e.runReported(), crashdump.ErrCoreLimit, "core file size limit")
}

// crashdump.Limit gives the same error for a read-back that is not zero
// as for a failed set (design D1). Its own tests cover that mapping.
func TestCoreLimitDoesNotReadBackAsZero(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.limitErr = crashdump.ErrCoreLimit
	wantCrashDumpFailure(t, e, e.runReported(), crashdump.ErrCoreLimit, "core file size limit")
}

func TestNotDumpableMarkCannotBeSetOnLinux(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.limitErr = crashdump.ErrDumpable
	wantCrashDumpFailure(t, e, e.runReported(), crashdump.ErrDumpable, "not dumpable")
}

// The limit is the first check: a run that fails on a missing target has
// already called it.
func TestCrashDumpsLimitedBeforeTargetCheck(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.cfg.Target = filepath.Join(e.root, "missing")
	wantCheckFailure(t, e, e.run(), run.ErrTarget)
	if e.limitCalls != 1 {
		t.Errorf("limit called %d times", e.limitCalls)
	}
}

func TestBWLeavesACoreFile(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	b := s.Commands["export --format json --raw"]
	b.WriteFile = "core"
	s.Commands["export --format json --raw"] = b
	e := newEnv(t, s)
	before := hashTree(t, e.target)
	r := e.runReported()
	if r.status != report.ExitFailed || r.o.Published != nil {
		t.Fatalf("exit status %d, published %v", r.status, r.o.Published)
	}
	var be *bwcli.Error
	if r.o.Failure == nil || !errors.As(r.o.Failure.Err, &be) || be.Class != bwcli.ClassStrayFile {
		t.Errorf("failure %v, want a stray file", r.o.Failure)
	}
	if !sameTree(before, hashTree(t, e.target)) {
		t.Error("target changed")
	}
	e.wantProfileRemoved()
}

const (
	helperFakeEnv = "BW_TAKEOUT_RUN_HELPER_FAKE"
	helperModeEnv = "BW_TAKEOUT_RUN_HELPER_MODE"
)

// TestCrashDumpHelper is not a test of its own. The real-path tests start
// the test binary with it. It runs one backup with the real
// crashdump.Limit and prints, one per line, "record <json>" for each core
// limit the fake recorded, "core <soft> <hard>" and on Linux "dumpable
// <v>" as read afterwards, and "result <phase or published>". It returns
// instead of exiting, so that the cleanups of newEnv remove its temporary
// directories (review of 19 November 2026, finding C1).
func TestCrashDumpHelper(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		t.Skip("helper process for the real-path crash dump tests, runs only when started by them")
	}
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.deps.LimitCrashDumps = nil
	if mode == "missing-target" {
		e.cfg.Target = filepath.Join(e.root, "missing")
	}
	o := run.Run(e.cfg, e.deps)
	for _, d := range e.dataDirs {
		recs, err := bwclitest.ReadRecords(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, rec := range recs {
			b, _ := json.Marshal(rec.CoreLimit)
			fmt.Printf("record %s\n", b)
		}
	}
	fmt.Println(ownLimits())
	switch {
	case o.Published != nil:
		fmt.Println("result published")
	case o.Failure != nil:
		fmt.Printf("result %s %v\n", o.Failure.Phase, errors.Is(o.Failure.Err, run.ErrTarget))
	default:
		fmt.Println("result none")
	}
}

// runHelper starts TestCrashDumpHelper in mode and returns its lines.
func runHelper(t *testing.T, mode string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no core file size limit, and crashdump.Limit changes nothing there (design D3)")
	}
	// The child gets a temporary directory of its own, which must be empty
	// once it has ended.
	tmp := filepath.Join(t.TempDir(), "tmp")
	if err := os.Mkdir(tmp, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashDumpHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), helperFakeEnv+"="+fakePath, helperModeEnv+"="+mode, "TMPDIR="+tmp)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	if left, err := os.ReadDir(tmp); err != nil || len(left) != 0 {
		t.Errorf("helper left %d entries in its temporary directory (%v)", len(left), err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

// wantOwnLimits checks the lines of the child's own limits after the run.
func wantOwnLimits(t *testing.T, lines []string) {
	t.Helper()
	want := []string{"core 0 0"}
	if runtime.GOOS == "linux" {
		want = append(want, "dumpable 0")
	}
	for _, w := range want {
		if !contains(lines, w) {
			t.Errorf("helper lacks %q: %q", w, lines)
		}
	}
}

func contains(lines []string, s string) bool {
	for _, l := range lines {
		if l == s {
			return true
		}
	}
	return false
}

// Scenario "bw inherits the limit".
func TestBWInheritsTheLimit(t *testing.T) {
	lines := runHelper(t, "backup")
	if !contains(lines, "result published") {
		t.Fatalf("helper did not publish: %q", lines)
	}
	n := 0
	for _, l := range lines {
		if rec, ok := strings.CutPrefix(l, "record "); ok {
			n++
			if rec != "[0,0]" {
				t.Errorf("bw saw core limit %s", rec)
			}
		}
	}
	if n == 0 {
		t.Error("no start of the fake recorded")
	}
	wantOwnLimits(t, lines)
	t.Logf("%d starts of the fake saw core limit [0,0]", n)
}

// Scenario "Run process not dumpable" (review of 19 November 2026,
// finding C3).
func TestRunProcessNotDumpable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the not-dumpable mark exists on Linux only")
	}
	lines := runHelper(t, "backup")
	if !contains(lines, "result published") {
		t.Fatalf("helper did not publish: %q", lines)
	}
	if !contains(lines, "dumpable 0") {
		t.Errorf("run process dumpable after the checks: %q", lines)
	}
}

// Scenario "Limit set before the first bw process".
func TestLimitSetBeforeTheFirstBWProcess(t *testing.T) {
	lines := runHelper(t, "missing-target")
	if !contains(lines, "result checks true") {
		t.Fatalf("helper did not fail on the target: %q", lines)
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "record ") {
			t.Errorf("bw started: %s", l)
		}
	}
	wantOwnLimits(t, lines)
}

// Scenario "Run on Windows". The real stub changes nothing, so the test
// runs in process.
func TestRunOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("checks the Windows stub of crashdump.Limit, runs in the Windows CI job")
	}
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	e.deps.LimitCrashDumps = nil
	r := e.runReported()
	if r.o.Failure != nil && r.o.Failure.Phase == run.PhaseChecks {
		t.Fatalf("checks failed: %v", r.o.Failure)
	}
	if !r.o.ProfileCreated {
		t.Error("no profile created")
	}
	r.want(t, "Crash dumps: not limited on this platform")
}
