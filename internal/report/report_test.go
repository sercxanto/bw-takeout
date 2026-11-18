package report_test

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
	"bw-takeout/internal/verify"
)

const (
	exampleItem       = "8c1e0000-0000-4000-8000-0000000000aa"
	exampleAttachment = "att00000000000000000000000000002"
	exampleHash       = "3f9a000000000000000000000000000000000000000000000000000000e1c0"
)

// exampleOutcome is the example of design D8 as an outcome.
func exampleOutcome() *run.Outcome {
	source := manifest.Source{ServerURL: "https://vault.example.org", AccountEmail: "user@example.org", AccountID: "8c1e0000-0000-4000-8000-000000000001"}
	return &run.Outcome{
		Setup: run.Setup{
			BWPath: "/usr/local/bin/bw", BWVersion: "2026.10.1", BWSHA256: exampleHash,
			Server: "https://vault.example.org", Target: "/mnt/backup", Recipients: 2,
		},
		Leftovers: run.Leftovers{
			Profiles: []profile.Leftover{{Name: "profile-x", Path: "/home/u/.cache/bw-takeout/profiles/profile-x", Age: 76 * time.Hour}},
			Partials: []publish.Leftover{{Name: "bw-takeout-20261118T090300Z-00000001.partial", Age: 72 * time.Minute}},
			Target:   "/mnt/backup",
		},
		Source:  &source,
		Planned: 3,
		Downloads: []capture.Download{
			{ItemID: exampleItem, AttachmentID: "att00000000000000000000000000001"},
			{ItemID: exampleItem, AttachmentID: exampleAttachment, Error: capture.ErrorStatus},
			{ItemID: exampleItem, AttachmentID: "att00000000000000000000000000003"},
		},
		Verification: &verify.Result{
			Verdict:        verify.Unverified,
			Scope:          verify.Scope{Code: verify.ScopeCode, Statement: verify.ScopeStatement},
			StandingLimits: []verify.Limit{verify.LimitAttachmentInventory, verify.LimitServerCompleteness},
			Findings: []verify.Finding{{
				Severity: verify.SeverityUnverified, Reason: verify.AttachmentDownloadFailed,
				ItemID: exampleItem, AttachmentID: exampleAttachment,
			}},
		},
		Published:      &publish.Published{Name: "bw-takeout-20261118T101500Z-4f2a91c0.unverified", Path: "/mnt/backup/bw-takeout-20261118T101500Z-4f2a91c0.unverified", Verdict: publish.VerdictUnverified},
		ProfileCreated: true,
		Logout:         run.Logout{Result: run.LogoutOK, Duration: 1400 * time.Millisecond, SessionForgotten: true},
	}
}

// render writes the progress lines of an outcome and the final part, as a
// run does.
func render(o *run.Outcome) (stdout, stderr string) {
	var out, errOut bytes.Buffer
	w := report.New(&out, &errOut)
	if o.ProfileCreated {
		w.Setup(o.Setup)
		w.Leftovers(o.Leftovers)
	}
	if o.Source != nil {
		w.Snapshot(*o.Source, o.Planned)
	}
	for i, d := range o.Downloads {
		w.Attachment(i+1, len(o.Downloads), d)
	}
	w.Final(o)
	return out.String(), errOut.String()
}

func TestReportOfTheDesignExample(t *testing.T) {
	stdout, stderr := render(exampleOutcome())
	want := `bw:          /usr/local/bin/bw
bw version:  2026.10.1, sha256 ` + exampleHash + `
Server:      https://vault.example.org
Target:      /mnt/backup
Recipients:  2
Leftover profile:  /home/u/.cache/bw-takeout/profiles/profile-x (age 3d4h)
Leftover partial:  ` + filepath.Join("/mnt/backup", "bw-takeout-20261118T090300Z-00000001.partial") + ` (age 1h12m)
Sync:        fresh
Synced from: https://vault.example.org
Account:     user@example.org (8c1e0000-0000-4000-8000-000000000001)
Attachments: 3 planned
  1/3 ok
  2/3 failed (status), item ` + exampleItem + `, attachment ` + exampleAttachment + `
  3/3 ok
Verdict:     unverified
Scope:       ` + verify.ScopeStatement + `
Limits:      attachment_inventory_single_source, server_completeness_not_checked
Findings:    1
  unverified attachment_download_failed item=` + exampleItem + ` attachment=` + exampleAttachment + `
Warnings:    0
Generation:  /mnt/backup/bw-takeout-20261118T101500Z-4f2a91c0.unverified
Logout:      ok (1.4 s)
Profile:     removed
`
	if stdout != want {
		t.Errorf("report differs\n--- got\n%s\n--- want\n%s", stdout, want)
	}
	if stderr != "" {
		t.Errorf("standard error %q", stderr)
	}
}

func intp(n int) *int { return &n }

func TestFindingLineNamesEverySetField(t *testing.T) {
	f := verify.Finding{
		Severity: verify.SeverityWarning, Reason: verify.OrganizationItemsExcluded,
		Document: "plaintext_export", ItemID: exampleItem, AttachmentID: exampleAttachment,
		Passkey: intp(1), Field: "keyValue", Expected: intp(2), Found: intp(1), Count: intp(3),
		Organizations: []verify.OrganizationCount{{OrganizationID: "org-a", Count: 2}, {OrganizationID: "org-b", Count: 1}},
	}
	want := "warning organization_items_excluded document=plaintext_export item=" + exampleItem +
		" attachment=" + exampleAttachment + " passkey=1 field=keyValue expected=2 found=1 count=3 organization=org-a:2 organization=org-b:1"
	if got := report.Finding(f); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := report.Finding(verify.Finding{Severity: verify.SeverityFailed, Reason: verify.DuplicateItemID, Passkey: intp(0)}); got != "failed duplicate_item_id passkey=0" {
		t.Errorf("got %s", got)
	}
}

func TestExitStatusTable(t *testing.T) {
	published := func(verdict string) *publish.Published {
		return &publish.Published{Name: "g", Path: "/t/g", Verdict: verdict}
	}
	removeErr := &profile.RemoveError{Path: "/p", Err: errors.New("busy")}
	logouts := []run.LogoutResult{run.LogoutNotNeeded, run.LogoutOK, run.LogoutFailed, run.LogoutLimit, run.LogoutSkipped}
	for _, pub := range []*publish.Published{nil, published(publish.VerdictVerified), published(publish.VerdictUnverified)} {
		for _, l := range logouts {
			for _, rerr := range []error{nil, removeErr} {
				o := &run.Outcome{Published: pub, Logout: run.Logout{Result: l}, RemoveErr: rerr, ProfileCreated: true}
				complete := (l == run.LogoutNotNeeded || l == run.LogoutOK) && rerr == nil
				var want int
				switch {
				case pub == nil:
					want = 1
				case !complete:
					want = 4
				case pub.Verdict == publish.VerdictVerified:
					want = 0
				default:
					want = 3
				}
				name := fmt.Sprintf("published=%v logout=%s removed=%v", pub != nil, l, rerr == nil)
				if pub != nil {
					name += " verdict=" + pub.Verdict
				}
				if got := report.ExitStatus(o); got != want {
					t.Errorf("%s: status %d, want %d", name, got, want)
				}
			}
		}
	}
	if report.ExitUsage != 2 || report.ExitVerified != 0 || report.ExitFailed != 1 || report.ExitUnverified != 3 || report.ExitIncompleteTeardown != 4 {
		t.Error("exit constants differ from design D7")
	}
}

func TestEndingLines(t *testing.T) {
	failed := exampleOutcome()
	failed.Published = nil
	failed.Verification.Verdict = verify.Failed
	interrupted := exampleOutcome()
	interrupted.Published, interrupted.Verification = nil, nil
	interrupted.Interrupted, interrupted.InterruptedIn = true, run.PhaseAttachments
	interrupted.Failure = &run.Failure{Phase: run.PhaseAttachments, Err: &bwcli.Error{Op: bwcli.OpAttachment, Class: bwcli.ClassExit, ExitCode: 1}, AfterInterrupt: true}
	// Review fix 9.9: a refused login, then a signal during the logout.
	loginThenInterrupt := exampleOutcome()
	loginThenInterrupt.Source, loginThenInterrupt.Downloads, loginThenInterrupt.Verification, loginThenInterrupt.Published = nil, nil, nil, nil
	loginThenInterrupt.Failure = &run.Failure{Phase: run.PhaseLogin, Err: &bwcli.Error{Op: bwcli.OpLogin, Class: bwcli.ClassOutput, ExitCode: 0}}
	loginThenInterrupt.Interrupted, loginThenInterrupt.InterruptedIn = true, run.PhaseTeardown
	loginThenInterrupt.Logout = run.Logout{Result: run.LogoutSkipped}
	interrupted.Logout = run.Logout{Result: run.LogoutSkipped, Duration: 200 * time.Millisecond}
	login := exampleOutcome()
	login.Source, login.Downloads, login.Verification, login.Published = nil, nil, nil, nil
	login.Failure = &run.Failure{Phase: run.PhaseLogin, Err: &bwcli.Error{Op: bwcli.OpLogin, Class: bwcli.ClassOutput, ExitCode: 0}}
	commit := exampleOutcome()
	commit.Published = nil
	commit.Failure = &run.Failure{Phase: run.PhaseCommit, Err: errors.New("rename failed")}
	commit.PartialLeft = true
	unknown := exampleOutcome()
	unknown.Published = nil
	unknown.Failure = &run.Failure{Phase: run.PhaseCommit, Err: publish.ErrUnknownVerdict}
	abortFailed := exampleOutcome()
	abortFailed.Published = nil
	abortFailed.Verification.Verdict = verify.Failed
	abortFailed.Failure = &run.Failure{Phase: run.PhaseCommit, Err: errors.Join(publish.ErrFailedCapture, errors.New("remove partial directory"))}
	abortFailed.PartialLeft = true
	notFlushed := exampleOutcome()
	notFlushed.PublishWarning = errors.New("generation g is complete, but the target directory could not be flushed")
	kept := exampleOutcome()
	kept.RemoveErr = &profile.RemoveError{Path: "/home/u/.cache/bw-takeout/profiles/profile-y", Err: errors.New("busy")}
	kept.Logout = run.Logout{Result: run.LogoutFailed, Class: bwcli.ClassExit, Duration: time.Second}
	checks := &run.Outcome{Failure: &run.Failure{Phase: run.PhaseChecks, Err: run.ErrTarget}}
	hang := exampleOutcome()
	hang.Logout = run.Logout{Result: run.LogoutLimit, Duration: 10 * time.Second}
	noLogin := exampleOutcome()
	noLogin.Logout = run.Logout{Result: run.LogoutNotNeeded}

	for name, c := range map[string]struct {
		o         *run.Outcome
		want, not []string
	}{
		"failed verdict": {failed, []string{"Verdict:     failed", "Findings:    1", "Generation:  none published, the verdict is failed"}, []string{"Failed:"}},
		"interrupted": {interrupted, []string{"Generation:  none published\n", "Interrupted during attachments.\n", "Logout:      skipped by the interrupt (0.2 s)"},
			[]string{"exit, exit code", "Failed:", "Verdict:"}},
		"login refused":   {login, []string{"Failed:      login: bw login failed: output, exit code 0", "Generation:  none published"}, []string{"Interrupted", "Sync:"}},
		"commit failed":   {commit, []string{"Failed:      commit: rename failed", "The partial generation stays in the target"}, nil},
		"unknown verdict": {unknown, []string{"Failed:      commit: verification result has no known verdict"}, []string{"stays in the target"}},
		"abort failed": {abortFailed, []string{"Verdict:     failed", "Generation:  none published",
			"Failed:      commit: capture failed verification, generation not published; remove partial directory\n", "The partial generation stays in the target"}, []string{"?remove"}},
		"failure before the interrupt": {loginThenInterrupt, []string{"Failed:      login: bw login failed: output, exit code 0\nInterrupted during teardown.\n", "Logout:      skipped by the interrupt"}, nil},
		"not flushed":                  {notFlushed, []string{"Generation:  /mnt/backup/bw-takeout-20261118T101500Z-4f2a91c0.unverified\nWarning:     generation g is complete, but the target directory could not be flushed\n"}, []string{"none published", "stays in the target"}},
		"profile not removed":          {kept, []string{"Logout:      failed (exit) (1 s)", "Profile:     not removed, remove /home/u/.cache/bw-takeout/profiles/profile-y by hand"}, nil},
		"checks":                       {checks, []string{"Generation:  none published\nFailed:      checks: target is not an existing directory\n"}, []string{"Logout:", "Profile:", "bw:"}},
		"logout at the limit":          {hang, []string{"Logout:      stopped at the limit (10 s)"}, nil},
		"no login":                     {noLogin, []string{"Logout:      not needed, no login was started"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, _ := render(c.o)
			for _, w := range c.want {
				if !strings.Contains(stdout, w) {
					t.Errorf("lacks %q\n%s", w, stdout)
				}
			}
			for _, n := range c.not {
				if strings.Contains(stdout, n) {
					t.Errorf("contains %q\n%s", n, stdout)
				}
			}
		})
	}
}

func TestStatusWithoutServerAddressUsesTheFlag(t *testing.T) {
	o := exampleOutcome()
	o.Source.ServerURL = ""
	stdout, _ := render(o)
	if !strings.Contains(stdout, "Synced from: https://vault.example.org\n") {
		t.Errorf("server of the flags missing\n%s", stdout)
	}
}

func TestValuesAreCleaned(t *testing.T) {
	o := exampleOutcome()
	o.Source.AccountEmail = "user\x1b]0;title\x07@example.org"
	o.Setup.Target = "/mnt/back\nup"
	stdout, _ := render(o)
	if strings.ContainsAny(stdout, "\x1b\x07") || !strings.Contains(stdout, "Account:     user?]0;title?@example.org (") || !strings.Contains(stdout, "Target:      /mnt/back?up\n") {
		t.Errorf("control characters not replaced\n%q", stdout)
	}
}

func TestInterruptMessages(t *testing.T) {
	if got := report.InterruptMessage(run.Interrupt{LogoutLimit: 10 * time.Second}); got != "Interrupted. Logging out, at most 10 s. Press Ctrl-C again to skip the logout." {
		t.Errorf("got %q", got)
	}
	if got := report.InterruptMessage(run.Interrupt{LogoutSkipped: true, LogoutLimit: 10 * time.Second}); got != "Interrupted. Skipping the logout." {
		t.Errorf("got %q", got)
	}
	var out, errOut bytes.Buffer
	report.New(&out, &errOut).Interrupted(run.Interrupt{LogoutLimit: 200 * time.Millisecond})
	if out.Len() != 0 || errOut.String() != "Interrupted. Logging out, at most 0.2 s. Press Ctrl-C again to skip the logout.\n" {
		t.Errorf("stdout %q, stderr %q", out.String(), errOut.String())
	}
}

func TestAge(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                "0s",
		40 * time.Second:            "40s",
		5*time.Minute + time.Second: "5m",
		72 * time.Minute:            "1h12m",
		76 * time.Hour:              "3d4h",
	} {
		if got := report.Age(d); got != want {
			t.Errorf("Age(%v) = %q, want %q", d, got, want)
		}
	}
}
