// Package report renders the outcome of a backup run as line-oriented
// English text and computes its exit status (designs D7 and D8 of the change
// interactive-backup-run). It is the only package that writes run output.
//
// The progress lines and the final report go to standard output. The
// interrupt messages go to standard error, where the login prompts of bw
// appear as well. Every value passes through printable.Clean. The report
// names IDs, counts, codes and paths, never vault content and never text
// that bw wrote.
package report

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"bw-takeout/internal/capture"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/printable"
	"bw-takeout/internal/run"
)

// Writer implements run.Progress and writes the final report. It is safe
// for concurrent use, because the run calls Interrupted from the goroutine
// that reads the signals.
type Writer struct {
	mu             sync.Mutex
	stdout, stderr io.Writer
	// server is the address the run was given.
	server string
}

// New returns a Writer for the two output streams.
func New(stdout, stderr io.Writer) *Writer {
	return &Writer{stdout: stdout, stderr: stderr}
}

var _ run.Progress = (*Writer)(nil)

// clean is printable.Clean, short for the many values below.
func clean(s string) string { return printable.Clean(s) }

func (w *Writer) line(format string, args ...any) {
	fmt.Fprintf(w.stdout, format+"\n", args...)
}

// Setup writes the lines that name bw, the server, the target and the
// number of recipients.
func (w *Writer) Setup(s run.Setup) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.setup(s)
}

func (w *Writer) setup(s run.Setup) {
	w.server = s.Server
	w.line("bw:          %s", clean(s.BWPath))
	if s.BWVersion != "" || s.BWSHA256 != "" {
		w.line("bw version:  %s, sha256 %s", clean(s.BWVersion), clean(s.BWSHA256))
	} else {
		w.line("bw version:  unknown")
	}
	w.line("Server:      %s", clean(s.Server))
	w.line("Target:      %s", clean(s.Target))
	w.line("Recipients:  %d", s.Recipients)
}

// Leftovers writes one line per leftover profile and partial generation.
func (w *Writer) Leftovers(l run.Leftovers) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.leftovers(l)
}

func (w *Writer) leftovers(l run.Leftovers) {
	if l.ProfilesErr != nil {
		w.line("Leftover profiles: not listed, the profile location cannot be read")
	}
	for _, p := range l.Profiles {
		w.line("Leftover profile:  %s (age %s)", clean(p.Path), Age(p.Age))
	}
	for _, p := range l.Partials {
		path := filepath.Join(l.Target, p.Name)
		w.line("Leftover partial:  %s (age %s)", clean(path), Age(p.Age))
	}
	if l.ProfilesErr == nil && len(l.Profiles) == 0 && len(l.Partials) == 0 {
		w.line("Leftovers:   none")
	}
}

// Snapshot writes the lines of the accepted snapshot.
func (w *Writer) Snapshot(source manifest.Source, planned int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	server := source.ServerURL
	if server == "" {
		// The status named no server, so the report names the one the
		// run was given (design D8).
		server = w.server
	}
	w.line("Sync:        fresh")
	w.line("Synced from: %s", clean(server))
	w.line("Account:     %s (%s)", clean(source.AccountEmail), clean(source.AccountID))
	w.line("Attachments: %d planned", planned)
}

// Attachment writes the line of one download result. Only a failed
// download names its IDs.
func (w *Writer) Attachment(position, total int, d capture.Download) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if d.Error == "" {
		w.line("  %d/%d ok", position, total)
		return
	}
	w.line("  %d/%d failed (%s), item %s, attachment %s", position, total, clean(string(d.Error)), clean(d.ItemID), clean(d.AttachmentID))
}

// Interrupted writes a message of design D6 to standard error.
func (w *Writer) Interrupted(i run.Interrupt) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Fprintln(w.stderr, InterruptMessage(i))
}

// InterruptMessage is the message of design D6.
func InterruptMessage(i run.Interrupt) string {
	if i.LogoutSkipped {
		return "Interrupted. Skipping the logout."
	}
	return "Interrupted. Logging out, at most " + Seconds(i.LogoutLimit) + ". Press Ctrl-C again to skip the logout."
}

// Seconds formats a duration in seconds with at most one decimal, such as
// "10 s" or "1.4 s".
func Seconds(d time.Duration) string {
	return strconv.FormatFloat(d.Round(100*time.Millisecond).Seconds(), 'f', -1, 64) + " s"
}

// Age formats the age of a leftover with its two largest units, such as
// "3d4h", "1h12m", "5m" or "40s". A negative age, from a clock that was
// set back, is "0s".
func Age(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd%dh", d/(24*time.Hour), (d%(24*time.Hour))/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", d/time.Hour, (d%time.Hour)/time.Minute)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}
