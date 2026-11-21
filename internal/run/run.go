package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/collect"
	"bw-takeout/internal/crashdump"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
)

// The causes that tell the two ends of a cancelled logout apart (design
// D5). The class of the bwcli failure is cancelled in both cases.
var (
	errLogoutLimit   = errors.New("logout limit reached")
	errLogoutSkipped = errors.New("logout skipped by an interrupt")
)

// snapshotTaken lets the tests of this package see the snapshot, to check
// that its documents were wiped.
var snapshotTaken = func(*collect.Snapshot) {}

// logoutReturned lets the tests of this package act between the end of the
// logout and the removal of the profile.
var logoutReturned = func(*runner) {}

// Run makes one backup and returns its outcome. It never panics on a
// failure of a step and always tears down what it created.
func Run(cfg Config, d Deps) *Outcome {
	d = d.withDefaults()
	r := &runner{cfg: cfg, d: d, o: &Outcome{}}
	r.ctx, r.cancelRun = context.WithCancel(context.Background())
	r.teardownBase, r.cancelTeardown = context.WithCancelCause(context.Background())
	stop := r.watchSignals()
	r.run()
	stop()
	r.cancelRun()
	r.cancelTeardown(nil)
	r.mu.Lock()
	r.o.Interrupted, r.o.InterruptedIn = r.interrupted, r.interruptedIn
	r.mu.Unlock()
	return r.o
}

func (d Deps) withDefaults() Deps {
	if d.NewProfile == nil || d.Leftovers == nil {
		c := &profile.Creator{}
		if d.NewProfile == nil {
			d.NewProfile = c.Create
		}
		if d.Leftovers == nil {
			d.Leftovers = c.Leftovers
		}
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Publisher == nil {
		d.Publisher = &publish.Publisher{}
	}
	if d.LogoutLimit <= 0 {
		d.LogoutLimit = DefaultLogoutLimit
	}
	if d.Progress == nil {
		d.Progress = discard{}
	}
	if d.LimitCrashDumps == nil {
		d.LimitCrashDumps = crashdump.Limit
	}
	return d
}

type runner struct {
	cfg Config
	d   Deps
	o   *Outcome

	ctx            context.Context
	cancelRun      context.CancelFunc
	teardownBase   context.Context
	cancelTeardown context.CancelCauseFunc

	// mu guards the fields the signal goroutine shares.
	mu            sync.Mutex
	phase         Phase
	interrupted   bool
	interruptedIn Phase
	inTeardown    bool
	logoutDone    bool
	logoutSkipped bool

	recipients []string
	bwPath     string
	caFile     string
}

// watchSignals starts the goroutine of design D6 and returns the function
// that stops it.
func (r *runner) watchSignals() (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		signals := r.d.Signals
		for {
			select {
			case <-done:
				return
			case _, ok := <-signals:
				if !ok {
					signals = nil
					continue
				}
				r.signal()
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

// signal handles one signal (design D6).
func (r *runner) signal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	first := !r.interrupted
	if first {
		r.interruptedIn = r.phase
	}
	r.interrupted = true
	r.cancelRun()
	if first && !r.inTeardown {
		r.d.Progress.Interrupted(Interrupt{LogoutLimit: r.d.LogoutLimit})
		return
	}
	// A logout that has ended, or none at all, cannot be skipped.
	if !r.logoutSkipped && !r.logoutDone {
		r.logoutSkipped = true
		r.cancelTeardown(errLogoutSkipped)
		r.d.Progress.Interrupted(Interrupt{LogoutSkipped: true, LogoutLimit: r.d.LogoutLimit})
	}
}

// at records the phase the run has reached.
func (r *runner) at(p Phase) {
	r.mu.Lock()
	r.phase = p
	r.mu.Unlock()
}

func (r *runner) current() Phase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phase
}

func (r *runner) fail(phase Phase, err error) {
	r.mu.Lock()
	after := r.interrupted
	r.mu.Unlock()
	r.o.Failure = &Failure{Phase: phase, Err: err, AfterInterrupt: after}
}

func (r *runner) run() {
	// A panic is recorded without its value, which could hold vault
	// content. After the profile exists, the deferred teardown below
	// recovers it first and still tears down.
	defer func() {
		if recover() != nil {
			r.fail(r.current(), ErrInternal)
		}
	}()
	r.at(PhaseChecks)
	if err := r.check(); err != nil {
		r.fail(PhaseChecks, err)
		return
	}
	if r.ctx.Err() != nil {
		r.fail(PhaseChecks, ErrInterrupted)
		return
	}
	r.at(PhaseProfile)
	p, err := r.d.NewProfile()
	if err != nil {
		r.fail(PhaseProfile, err)
		return
	}
	r.o.ProfileCreated = true
	r.o.Leftovers.Profiles, r.o.Leftovers.ProfilesErr = r.d.Leftovers(p)

	var client *bwcli.Client
	loginStarted := false
	defer func() {
		// Recorded before teardown changes the phase.
		if recover() != nil {
			r.fail(r.current(), ErrInternal)
		}
		r.teardown(p, client, loginStarted)
	}()
	client, err = bwcli.New(bwcli.Config{
		Executable: r.bwPath, DataDir: p.DataDir, WorkDir: p.WorkDir, CAFile: r.caFile,
		Stdin: r.d.Stdin, Stderr: r.d.Stderr, Terminal: r.d.Terminal,
	})
	if err != nil {
		r.d.Progress.Setup(r.o.Setup)
		r.d.Progress.Leftovers(r.o.Leftovers)
		r.fail(PhaseVersion, err)
	} else {
		r.session(client, &loginStarted)
	}
}

// check runs the checks of design D3. It starts no process and creates
// nothing. Crash dumps are limited first, so that no later check runs with
// them enabled (design D2 of the change memory-hardening).
func (r *runner) check() error {
	protection, err := r.d.LimitCrashDumps()
	if err != nil {
		return err
	}
	r.o.Setup.CrashDumps = protection
	r.o.Setup.Server = r.cfg.Server
	if info, err := os.Stat(r.cfg.Target); err != nil || !info.IsDir() {
		return ErrTarget
	}
	r.o.Setup.Target = r.cfg.Target
	f, err := os.Open(r.cfg.RecipientsFile)
	if err != nil {
		return ErrRecipientsOpen
	}
	r.recipients, err = archive.ParseRecipientFile(f)
	f.Close()
	if err != nil {
		return err
	}
	r.o.Setup.Recipients = len(r.recipients)
	r.o.Setup.Organizations = append([]string(nil), r.cfg.Organizations...)

	r.bwPath, err = resolveBW(r.cfg.BW)
	if err != nil {
		return err
	}
	r.o.Setup.BWPath = r.bwPath

	if r.cfg.CAFile != "" {
		r.caFile, err = regularFile(r.cfg.CAFile)
		if err != nil {
			return ErrCAFile
		}
	}
	partials, err := publish.ListPartial(r.cfg.Target, r.d.Now())
	if err != nil {
		return ErrTargetList
	}
	r.o.Leftovers.Partials, r.o.Leftovers.Target = partials, r.cfg.Target
	return nil
}

// resolveBW returns the absolute path of bw: the given path, or the one
// found on the program search path. It must be a regular file after
// following links. A given relative path is taken relative to the working
// directory, never looked up on the search path.
func resolveBW(given string) (string, error) {
	path := given
	if path == "" {
		found, err := exec.LookPath("bw")
		if err != nil {
			return "", ErrBWNotFound
		}
		path = found
	}
	abs, err := regularFile(path)
	if err != nil {
		return "", ErrBWNotFound
	}
	return abs, nil
}

// regularFile returns the absolute form of path when it opens and is a
// regular file after following links.
func regularFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return abs, nil
}

// session runs the steps from the version query to the published
// generation (design D4).
func (r *runner) session(client *bwcli.Client, loginStarted *bool) {
	r.at(PhaseVersion)
	id, err := client.Version(r.ctx)
	r.o.Setup.BWVersion, r.o.Setup.BWSHA256 = id.Version, id.SHA256
	r.d.Progress.Setup(r.o.Setup)
	r.d.Progress.Leftovers(r.o.Leftovers)
	if err != nil {
		r.fail(PhaseVersion, err)
		return
	}
	r.at(PhaseServer)
	if err := client.SetServer(r.ctx, r.cfg.Server); err != nil {
		r.fail(PhaseServer, err)
		return
	}
	r.at(PhaseLogin)
	*loginStarted = true
	if err := client.Login(r.ctx); err != nil {
		r.fail(PhaseLogin, err)
		return
	}

	r.at(PhaseSnapshot)
	collector := &collect.Collector{Now: r.d.Now, Wait: r.d.Wait, Organizations: r.cfg.Organizations}
	snap, err := collector.Snapshot(r.ctx, client)
	if err != nil {
		r.fail(PhaseSnapshot, err)
		return
	}
	defer snap.Wipe()
	snapshotTaken(snap)
	source := snap.Source
	r.o.Source = &source
	r.o.Planned = len(snap.Plan)
	r.d.Progress.Snapshot(source, len(snap.Plan))

	tool := manifest.Tool{Name: "bw-takeout", Version: toolVersion()}
	bw := manifest.BW{Version: id.Version}
	r.at(PhaseBegin)
	var writeErr error
	written := false
	write := func(w io.Writer, created time.Time) (v []byte, err error) {
		defer func() {
			// A panic must not skip the abort of the partial directory.
			if recover() != nil {
				v, err = nil, ErrInternal
			}
			writeErr, written = err, err == nil
			if written {
				r.at(PhaseCommit)
			}
		}()
		return r.write(w, created, collector, client, snap, tool, bw)
	}
	pub, err := r.d.Publisher.Publish(r.cfg.Target, write)
	if pub != nil {
		// The generation has its final name. An error then concerns only
		// the flush of the target directory.
		r.o.Published, r.o.PublishWarning = pub, err
		return
	}
	switch {
	case err == nil:
		r.fail(PhaseCommit, errors.New("no generation returned"))
	case writeErr != nil:
		r.fail(r.current(), err)
		// Publish returns the write error itself after a successful abort.
		r.o.PartialLeft = err != writeErr
	case !written:
		r.fail(PhaseBegin, err)
	case err == publish.ErrFailedCapture:
		// The verdict decided, and the partial directory was removed.
		// Verification holds the verdict.
	case err == publish.ErrUnknownVerdict:
		r.fail(PhaseCommit, err)
	default:
		// A commit step failed, or the abort after a refused verdict.
		r.fail(PhaseCommit, err)
		r.o.PartialLeft = true
	}
}

// write is the write function of the publishing step.
func (r *runner) write(w io.Writer, created time.Time, collector *collect.Collector, client *bwcli.Client,
	snap *collect.Snapshot, tool manifest.Tool, bw manifest.BW) ([]byte, error) {
	r.at(PhaseArchive)
	aw, err := archive.NewWriter(w, r.recipients, created, snap.Documents())
	if err != nil {
		return nil, err
	}
	r.at(PhaseAttachments)
	downloads, err := collector.Attachments(r.ctx, client, snap, aw.AddAttachment)
	if err != nil {
		return nil, err
	}
	r.o.Downloads = downloads
	for i, dl := range downloads {
		r.d.Progress.Attachment(i+1, len(downloads), dl)
	}
	r.at(PhaseVerification)
	c := capture.Parse(snap.Input(downloads))
	snap.Wipe()
	result := verify.Verify(c, nil)
	r.o.Verification = result
	verification, err := result.JSON()
	if err != nil {
		return nil, err
	}
	r.at(PhaseArchive)
	if r.ctx.Err() != nil {
		return nil, ErrInterrupted
	}
	out, err := aw.Finish(archive.Meta{Source: snap.Source, Tool: tool, BW: bw, Verification: verification,
		OrganizationNames: snap.OrganizationNames()})
	if err != nil {
		return nil, err
	}
	// The generation is not committed yet, so an interrupt still aborts
	// it.
	if r.ctx.Err() != nil {
		return nil, ErrInterrupted
	}
	return out, nil
}

// teardown logs out after every login attempt, within the logout limit,
// and removes the profile (design D5).
func (r *runner) teardown(p *profile.Profile, client *bwcli.Client, loginStarted bool) {
	r.mu.Lock()
	r.inTeardown = true
	r.logoutDone = !loginStarted
	r.phase = PhaseTeardown
	r.mu.Unlock()

	var logout func() error
	// cause is the cause of the end of ctx as it stood when the logout
	// returned. A signal or the deadline during the removal of the
	// profile does not change the result of a logout that has ended.
	var cause error
	ctx, cancel := context.WithTimeoutCause(r.teardownBase, r.d.LogoutLimit, errLogoutLimit)
	defer cancel()
	if loginStarted {
		logout = func() error {
			start := time.Now()
			err := client.Logout(ctx)
			r.o.Logout.Duration = time.Since(start)
			cause = context.Cause(ctx)
			r.mu.Lock()
			r.logoutDone = true
			r.mu.Unlock()
			logoutReturned(r)
			return err
		}
	}
	logoutErr, removeErr := p.Teardown(logout)
	r.o.RemoveErr = removeErr
	if loginStarted {
		r.o.Logout.Result, r.o.Logout.Class = logoutResult(logoutErr, cause)
	} else {
		r.o.Logout.Result = LogoutNotNeeded
	}
	if client != nil {
		r.o.Logout.SessionForgotten = !client.LoggedIn()
	}
}

// logoutResult classifies the logout by the cause of the end of its
// context, not by the class of the failure (design D6): on Windows the
// console also sends the interrupt to bw, which can end on its own before
// the context ends.
func logoutResult(err, cause error) (LogoutResult, bwcli.Class) {
	switch {
	case err == nil:
		return LogoutOK, ""
	case errors.Is(cause, errLogoutSkipped):
		return LogoutSkipped, ""
	case errors.Is(cause, errLogoutLimit):
		return LogoutLimit, ""
	}
	return LogoutFailed, bwcli.ClassOf(err)
}

// toolVersion is the version of the main module, "(devel)" for a local
// build.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

type discard struct{}

func (discard) Setup(Setup)                           {}
func (discard) Leftovers(Leftovers)                   {}
func (discard) Snapshot(manifest.Source, int)         {}
func (discard) Attachment(int, int, capture.Download) {}
func (discard) Interrupted(Interrupt)                 {}

// String names the phase and the error, for logs of tests.
func (f *Failure) String() string { return fmt.Sprintf("%s: %v", f.Phase, f.Err) }
