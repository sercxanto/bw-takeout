package run

import (
	"context"
	"errors"
	"os"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/crashdump"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
)

// DefaultLogoutLimit bounds the logout in teardown (design D5, maintainer
// decision of 18 November 2026). A local logout does not need the minute
// of the short time limit of bwcli, and an interrupted user should not
// wait that long.
const DefaultLogoutLimit = 10 * time.Second

// Config holds the inputs of one run, as the flags give them (design D2).
type Config struct {
	// Server is the HTTPS address of the server.
	Server string
	// Target is the directory that receives the generation.
	Target string
	// RecipientsFile is the file of age recipients (design D10).
	RecipientsFile string
	// BW is the path of the bw executable. Empty looks bw up on the
	// program search path.
	BW string
	// CAFile is an optional certificate authority file.
	CAFile string
}

// Publisher publishes a generation. *publish.Publisher implements it.
// Tests wrap it to hold a run between the write and the commit.
type Publisher interface {
	Publish(target string, write publish.WriteFunc) (*publish.Published, error)
}

// Progress receives the lines that appear while the run goes on (design
// D1). Interrupted is called from the goroutine that reads the signals, so
// an implementation must be safe for concurrent use.
type Progress interface {
	// Setup is called once the version of bw was queried, also when the
	// query failed.
	Setup(Setup)
	// Leftovers is called after Setup.
	Leftovers(Leftovers)
	// Snapshot is called once the snapshot is complete.
	Snapshot(source manifest.Source, planned int)
	// Attachment is called for each download result, in the order of the
	// plan, once all downloads have ended. collect returns the results
	// only then. position counts from 1.
	Attachment(position, total int, d capture.Download)
	// Interrupted is called for the first signal, and once more when a
	// signal cancels the logout.
	Interrupted(Interrupt)
}

// Interrupt is the notice of design D6.
type Interrupt struct {
	// LogoutSkipped is true when the signal cancelled the logout.
	LogoutSkipped bool
	// LogoutLimit is the limit of the logout that follows.
	LogoutLimit time.Duration
}

// Deps are the parts of the environment a run uses. Tests replace them.
type Deps struct {
	// NewProfile creates the profile of the run.
	NewProfile func() (*profile.Profile, error)
	// Leftovers lists the profiles of other runs.
	Leftovers func(own *profile.Profile) ([]profile.Leftover, error)
	// Terminal, Stdin and Stderr are given to the login. Nil takes the
	// defaults of bwcli.
	Terminal      bwcli.Terminal
	Stdin, Stderr *os.File
	// Signals delivers the interrupts. Nil delivers none.
	Signals <-chan os.Signal
	// Now is the clock of the freshness check and of the ages of partial
	// generations. Nil takes time.Now.
	Now func() time.Time
	// Wait waits between download attempts. Nil waits with a timer.
	Wait func(ctx context.Context, d time.Duration) error
	// Publisher publishes the generation. Nil takes a zero
	// publish.Publisher.
	Publisher Publisher
	// LogoutLimit bounds the logout. Zero takes DefaultLogoutLimit.
	LogoutLimit time.Duration
	// Progress receives the lines while the run goes on. Nil drops them.
	Progress Progress
	// LimitCrashDumps limits the crash dumps of the process, the first
	// step of the checks (design D2 of the change memory-hardening). Nil
	// takes crashdump.Limit.
	LimitCrashDumps func() (crashdump.Protection, error)
}

// Phase names the step of a run in which it ended.
type Phase string

// The phases of design D4.
const (
	PhaseChecks       Phase = "checks"
	PhaseProfile      Phase = "profile"
	PhaseVersion      Phase = "version"
	PhaseServer       Phase = "server"
	PhaseLogin        Phase = "login"
	PhaseSnapshot     Phase = "snapshot"
	PhaseBegin        Phase = "begin"
	PhaseArchive      Phase = "archive"
	PhaseAttachments  Phase = "attachments"
	PhaseVerification Phase = "verification"
	PhaseCommit       Phase = "commit"
	PhaseTeardown     Phase = "teardown"
)

// Errors of the checks before the first bw process (design D3). They
// carry no content of the files they name.
var (
	ErrTarget         = errors.New("target is not an existing directory")
	ErrRecipientsOpen = errors.New("recipients file cannot be opened")
	ErrBWNotFound     = errors.New("bw executable not found or not a regular file")
	ErrCAFile         = errors.New("certificate authority file cannot be opened or is not a regular file")
	ErrTargetList     = errors.New("target directory cannot be listed")
	// ErrInterrupted ends a run that was interrupted between two steps.
	ErrInterrupted = errors.New("interrupted")
	// ErrInternal replaces a panic of a step. The value of the panic is
	// dropped, because it could hold vault content.
	ErrInternal = errors.New("internal error")
)

// Setup is what the report names before the login (design D8).
type Setup struct {
	// BWPath is the absolute path of bw.
	BWPath string
	// BWVersion and BWSHA256 are empty when the version query failed.
	BWVersion, BWSHA256 string
	Server              string
	Target              string
	Recipients          int
	// CrashDumps is the crash dump protection of the run.
	CrashDumps crashdump.Protection
}

// Leftovers are the profiles of other runs and the partial generations in
// the target. Neither is modified.
type Leftovers struct {
	Profiles []profile.Leftover
	// ProfilesErr is set when the profile location could not be listed.
	ProfilesErr error
	// Partials are named relative to Target.
	Partials []publish.Leftover
	Target   string
}

// Failure is what ended a run.
type Failure struct {
	Phase Phase
	// Err is a *bwcli.Error, an error of the step, or ErrInterrupted. Its
	// text holds no text bw wrote.
	Err error
	// AfterInterrupt is set when the first signal had arrived before the
	// failure was recorded. The interrupt then counts as its cause (design
	// D6), whatever the class.
	AfterInterrupt bool
}

// LogoutResult is the outcome of the logout (design D5).
type LogoutResult string

// The five logout outcomes.
const (
	LogoutNotNeeded LogoutResult = "not_needed"
	LogoutOK        LogoutResult = "ok"
	LogoutFailed    LogoutResult = "failed"
	LogoutLimit     LogoutResult = "stopped_at_limit"
	LogoutSkipped   LogoutResult = "skipped"
)

// Logout is the logout of teardown.
type Logout struct {
	Result LogoutResult
	// Class is the class of a failed logout.
	Class    bwcli.Class
	Duration time.Duration
	// SessionForgotten reports that the client holds no session key after
	// teardown.
	SessionForgotten bool
}

// Outcome is everything the report and the exit status are computed from
// (design D7).
type Outcome struct {
	Setup     Setup
	Leftovers Leftovers
	// Source is set once the snapshot is complete.
	Source *manifest.Source
	// Planned is the number of planned attachments.
	Planned int
	// Downloads are the download results, in the order of the plan.
	Downloads []capture.Download
	// Verification is set when a result was computed, also for a failed
	// verdict.
	Verification *verify.Result
	// Published is the published generation, if any.
	Published *publish.Published
	// PublishWarning is set when the generation was published but the
	// target directory could not be flushed afterwards.
	PublishWarning error
	// PartialLeft is set when the partial directory of this run stays in
	// the target, because a commit step or its removal failed.
	PartialLeft bool
	// Failure is what ended the run, if anything did. A failed verdict
	// is not a failure: Verification shows it.
	Failure *Failure
	// Interrupted is set when a signal arrived, whatever ended the run.
	// InterruptedIn is the phase the first signal arrived in.
	Interrupted   bool
	InterruptedIn Phase
	// ProfileCreated is set once the profile of the run exists.
	ProfileCreated bool
	Logout         Logout
	// RemoveErr is a *profile.RemoveError when the profile could not be
	// removed.
	RemoveErr error
}

// TeardownComplete reports whether the logout was not needed or
// succeeded, and the profile was removed (design D7).
func (o *Outcome) TeardownComplete() bool {
	return (o.Logout.Result == LogoutNotNeeded || o.Logout.Result == LogoutOK) && o.RemoveErr == nil
}
