package report

import (
	"errors"
	"strconv"
	"strings"

	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/run"
	"bw-takeout/internal/verify"
)

// Exit statuses of a backup (design D7). 0, 1, 2 and 3 mean the same as
// for verify.
const (
	ExitVerified   = 0
	ExitFailed     = 1
	ExitUsage      = 2
	ExitUnverified = 3
	// ExitIncompleteTeardown: a generation is published, but the logout
	// did not succeed or the profile was not removed.
	ExitIncompleteTeardown = 4
)

// ExitStatus is the exit status of a run (design D7).
func ExitStatus(o *run.Outcome) int {
	switch {
	case o.Published == nil:
		return ExitFailed
	case !o.TeardownComplete():
		return ExitIncompleteTeardown
	case o.Published.Verdict == publish.VerdictVerified:
		return ExitVerified
	case o.Published.Verdict == publish.VerdictUnverified:
		return ExitUnverified
	}
	// Publish gives a final name only to these two verdicts.
	return ExitFailed
}

// Final writes the part of the report that follows the progress lines:
// the verdict in full, the generation, a failure that ended the run, the
// logout and the profile (design D8).
func (w *Writer) Final(o *run.Outcome) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if o.Verification != nil {
		w.verdict(o.Verification)
	}
	w.ending(o)
	w.teardown(o)
}

func (w *Writer) verdict(r *verify.Result) {
	w.line("Verdict:     %s", clean(string(r.Verdict)))
	w.line("Scope:       %s", clean(r.Scope.Statement))
	limits := make([]string, len(r.StandingLimits))
	for i, l := range r.StandingLimits {
		limits[i] = clean(string(l))
	}
	if len(limits) == 0 {
		w.line("Limits:      none")
	} else {
		w.line("Limits:      %s", strings.Join(limits, ", "))
	}
	w.line("Findings:    %d", len(r.Findings))
	for _, f := range r.Findings {
		w.line("  %s", Finding(f))
	}
	w.line("Warnings:    %d", len(r.Warnings))
	for _, f := range r.Warnings {
		w.line("  %s", Finding(f))
	}
}

// Finding renders one finding or warning: severity, reason and each field
// that is set. These are IDs, positions, counts and codes only (design D10
// of capture-verification).
func Finding(f verify.Finding) string {
	parts := []string{clean(string(f.Severity)), clean(string(f.Reason))}
	add := func(name, value string) {
		if value != "" {
			parts = append(parts, name+"="+clean(value))
		}
	}
	addInt := func(name string, v *int) {
		if v != nil {
			parts = append(parts, name+"="+strconv.Itoa(*v))
		}
	}
	add("document", f.Document)
	add("item", f.ItemID)
	add("attachment", f.AttachmentID)
	addInt("passkey", f.Passkey)
	add("field", f.Field)
	addInt("expected", f.Expected)
	addInt("found", f.Found)
	addInt("count", f.Count)
	for _, oc := range f.Organizations {
		parts = append(parts, "organization="+clean(oc.OrganizationID)+":"+strconv.Itoa(oc.Count))
	}
	return strings.Join(parts, " ")
}

// ending writes the generation line and the line of what ended the run.
func (w *Writer) ending(o *run.Outcome) {
	switch {
	case o.Published != nil:
		w.line("Generation:  %s", clean(o.Published.Path))
		if o.PublishWarning != nil {
			w.line("Warning:     %s", errText(o.PublishWarning))
		}
	case o.Failure == nil && o.Verification != nil && o.Verification.Verdict == verify.Failed:
		w.line("Generation:  none published, the verdict is failed")
	default:
		w.line("Generation:  none published")
	}
	// A failure after the first signal is replaced by the interrupt line,
	// because the class of the last bw command says nothing about an
	// interrupt the run recorded itself (design D6). A failure that ended
	// the run before the signal is still named (review of 18 November
	// 2026, second round).
	if f := o.Failure; f != nil && !(o.Interrupted && f.AfterInterrupt) {
		w.line("Failed:      %s: %s", clean(string(f.Phase)), errText(f.Err))
	}
	if o.Interrupted {
		w.line("Interrupted during %s.", clean(string(o.InterruptedIn)))
	}
	if o.PartialLeft {
		w.line("             The partial generation stays in the target. The next run lists it as a leftover.")
	}
}

// errText is the cleaned text of err on one line. errors.Join separates
// its parts by line breaks, which become "; " here.
func errText(err error) string {
	return clean(strings.ReplaceAll(err.Error(), "\n", "; "))
}

func (w *Writer) teardown(o *run.Outcome) {
	if !o.ProfileCreated {
		return
	}
	l := o.Logout
	switch l.Result {
	case run.LogoutNotNeeded:
		w.line("Logout:      not needed, no login was started")
	case run.LogoutOK:
		w.line("Logout:      ok (%s)", Seconds(l.Duration))
	case run.LogoutFailed:
		w.line("Logout:      failed (%s) (%s)", clean(string(l.Class)), Seconds(l.Duration))
	case run.LogoutLimit:
		w.line("Logout:      stopped at the limit (%s)", Seconds(l.Duration))
	case run.LogoutSkipped:
		w.line("Logout:      skipped by the interrupt (%s)", Seconds(l.Duration))
	}
	if o.RemoveErr == nil {
		w.line("Profile:     removed")
		return
	}
	var re *profile.RemoveError
	if errors.As(o.RemoveErr, &re) {
		w.line("Profile:     not removed, remove %s by hand", clean(re.Path))
		return
	}
	w.line("Profile:     not removed")
}
