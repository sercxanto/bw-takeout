package run_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
	"bw-takeout/internal/verify"
)

// reported is a run wired through package report as the command wires it.
type reported struct {
	o              *run.Outcome
	stdout, stderr string
	status         int
}

func (e *env) runReported() reported {
	e.t.Helper()
	var stdout, stderr bytes.Buffer
	w := report.New(&stdout, &stderr)
	e.deps.Progress = tee{e.progress, w}
	o := run.Run(e.cfg, e.deps)
	w.Final(o)
	return reported{o: o, stdout: stdout.String(), stderr: stderr.String(), status: report.ExitStatus(o)}
}

func (r reported) want(t *testing.T, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(r.stdout, l) {
			t.Errorf("report lacks %q\n%s", l, r.stdout)
		}
	}
}

func TestLeftoversPresent(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	// A profile of another run, 3 hours old, and a partial generation,
	// 2 hours old, both at the time of the clock of the run.
	other := filepath.Join(e.location, "profile-"+fakeSyncStart.Add(-3*time.Hour).UTC().Format("20060102T150405Z")+"-0000abcd")
	for _, d := range []string{other, filepath.Join(other, profile.DataDirName)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, profile.DataDirName, "data.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := profile.ParseName(filepath.Base(other)); !ok {
		t.Fatalf("precondition: %s is no profile name", other)
	}
	partialName := publish.NamePrefix + fakeSyncStart.Add(-2*time.Hour).UTC().Format("20060102T150405Z") + "-0000abcd" + publish.PartialMarker
	partial := filepath.Join(e.target, partialName)
	if err := os.Mkdir(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(partial, publish.ArchiveName), []byte("ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeProfile, beforePartial := hashTree(t, other), hashTree(t, partial)

	r := e.runReported()
	if r.o.Published == nil {
		t.Fatalf("not published\n%s", r.stdout)
	}
	if len(r.o.Leftovers.Profiles) != 1 || len(r.o.Leftovers.Partials) != 1 {
		t.Fatalf("leftovers %+v", r.o.Leftovers)
	}
	profileAge := report.Age(r.o.Leftovers.Profiles[0].Age)
	r.want(t,
		"Leftover profile:  "+other+" (age "+profileAge+")",
		"Leftover partial:  "+partial+" (age 2h0m)",
	)
	if !sameTree(beforeProfile, hashTree(t, other)) || !sameTree(beforePartial, hashTree(t, partial)) {
		t.Error("a leftover changed")
	}
}

func TestOneDownloadFails(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[secondAttachment] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	r := e.runReported()
	if n := countOf(e.commands(), secondAttachment); n != 3 {
		t.Errorf("second attachment requested %d times, want 3", n)
	}
	lines := []string{}
	for _, l := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(l, "  ") && strings.Contains(l, "/3 ") {
			lines = append(lines, l)
		}
	}
	want := []string{
		"  1/3 ok",
		"  2/3 failed (status), item " + ct.ItemPasskeyLogin + ", attachment " + ct.AttachmentSecond,
		"  3/3 ok",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("attachment lines\n%s\nwant\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if strings.Contains(r.stdout, ct.AttachmentFileName) || strings.Contains(r.stdout, ct.Marker) {
		t.Errorf("report names a file name or a sentinel\n%s", r.stdout)
	}
}

func TestFailedVerdict(t *testing.T) {
	f := ct.VariantByName(t, ct.MissingItem).Build(t)
	ct.VariantByName(t, ct.NonNumericCounter).Apply(t, f)
	e := newEnv(t, scenarioOf(t, f))
	r := e.runReported()
	v := r.o.Verification
	if v == nil || v.Verdict != verify.Failed || len(v.Findings) != 2 {
		t.Fatalf("precondition: verification %+v", v)
	}
	if r.status != 1 || r.o.Published != nil {
		t.Errorf("status %d, published %v", r.status, r.o.Published)
	}
	r.want(t, "Verdict:     failed", "Scope:       "+verify.ScopeStatement, "Findings:    2", "Generation:  none published, the verdict is failed")
	for _, fd := range v.Findings {
		r.want(t, "  "+report.Finding(fd)+"\n")
	}
	var limits []string
	for _, l := range v.StandingLimits {
		limits = append(limits, string(l))
	}
	r.want(t, "Limits:      "+strings.Join(limits, ", "))
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
}

func TestProfileNotRemoved(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	keepProfile(t, e)
	r := e.runReported()
	r.want(t, "Profile:     not removed, remove "+e.profile().Path+" by hand")
	if r.status != 4 {
		t.Errorf("status %d, want 4", r.status)
	}
}

func TestStatusWithoutServerAddress(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdStatus] = statusBehavior(fakeLastSync, "", fakeEmail)
	e := newEnv(t, s)
	r := e.runReported()
	if r.o.Source == nil || r.o.Source.ServerURL != "" {
		t.Fatalf("precondition: source %+v", r.o.Source)
	}
	r.want(t, "Server:      "+fakeServer+"\n", "Synced from: "+fakeServer+"\n")
}

func TestVerifiedBackup(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	r := e.runReported()
	if r.status != 0 {
		t.Fatalf("status %d\n%s", r.status, r.stdout)
	}
	r.want(t, "Verdict:     verified", "Generation:  "+r.o.Published.Path, "Logout:      ok (", "Profile:     removed")
}

func TestUnverifiedBackup(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.VariantByName(t, ct.BlobLoginItem).Build(t)))
	r := e.runReported()
	if r.status != 3 || !strings.HasSuffix(r.o.Published.Name, publish.UnverifiedMarker) {
		t.Fatalf("status %d, published %v", r.status, r.o.Published)
	}
}

func TestPublishedGenerationLogoutFailed(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdLogout] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel}
	e := newEnv(t, s)
	r := e.runReported()
	if r.status != 4 || r.o.Published == nil {
		t.Fatalf("status %d, published %v", r.status, r.o.Published)
	}
	r.want(t, "Generation:  "+r.o.Published.Path+"\n", "Logout:      failed (exit)")
}

func TestNoGenerationAndProfileNotRemoved(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdStatus] = statusBehavior(staleLastSync, fakeServer, fakeEmail)
	e := newEnv(t, s)
	keepProfile(t, e)
	r := e.runReported()
	if r.status != 1 || r.o.RemoveErr == nil {
		t.Fatalf("status %d, remove error %v", r.status, r.o.RemoveErr)
	}
	r.want(t, "Failed:      snapshot: collect: snapshot is not fresh", "Profile:     not removed, remove "+e.profile().Path+" by hand")
}
