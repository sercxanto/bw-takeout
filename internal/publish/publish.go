// Package publish is the only code that touches the target directory. It
// creates each backup generation as a new directory, marks it as partial
// while it is written, and lets the verdict decide under which name it is
// published. It never modifies content that existed before the run. See the
// design of the change archive-generation, decisions D8 and D9.
//
// Publishing is built only from operations that behave the same on Linux,
// macOS and Windows and on FAT or exFAT media: creating a directory fails if
// the name exists, a file is created exclusively, and a directory is
// renamed after a check that the final name is free.
package publish

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

// The verdicts as the serialized verification result of package verify
// states them. They are repeated here so that publishing does not depend on
// the verification code. A test checks that they match.
const (
	VerdictVerified   = "verified"
	VerdictUnverified = "unverified"
	VerdictFailed     = "failed"
)

// Names inside the target.
const (
	// ArchiveName is the only file of a generation.
	ArchiveName = "vault.tar.age"
	// NamePrefix starts every generation name.
	NamePrefix = "bw-takeout-"
	// PartialMarker ends the name of a generation that is not complete.
	PartialMarker = ".partial"
	// UnverifiedMarker ends the name of a generation whose capture is
	// unverified.
	UnverifiedMarker = ".unverified"
	timeLayout       = "20060102T150405Z"
)

// maxAttempts bounds how often a new random part is drawn on a collision.
const maxAttempts = 16

var namePattern = regexp.MustCompile(`^` + NamePrefix + `(\d{8}T\d{6}Z)-([0-9a-f]{8})(\.partial|\.unverified)?$`)

// ParseName splits a generation name into its creation time and marker. The
// marker is empty, PartialMarker or UnverifiedMarker. ok is false for a
// name that is not a generation name.
func ParseName(name string) (created time.Time, marker string, ok bool) {
	m := namePattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, "", false
	}
	t, err := time.Parse(timeLayout, m[1])
	if err != nil {
		return time.Time{}, "", false
	}
	return t, m[3], true
}

// Errors of publishing.
var (
	ErrTargetMissing      = errors.New("target directory does not exist")
	ErrTargetNotDirectory = errors.New("target is not a directory")
	ErrNoFreeName         = errors.New("no free generation name found")
	ErrFinalNameExists    = errors.New("final generation name already exists")
	// ErrFailedCapture: the verdict was failed, so the generation was not
	// published and its partial directory was removed.
	ErrFailedCapture  = errors.New("capture failed verification, generation not published")
	ErrUnknownVerdict = errors.New("verification result has no known verdict")
	ErrFinished       = errors.New("generation already committed or aborted")
	ErrCommitFailed   = errors.New("an earlier commit step failed, only abort is possible")
	ErrWriteFailed    = errors.New("writing the archive failed")
)

// Publisher creates generations. The zero value uses the system clock, the
// system random source and the real file system.
type Publisher struct {
	// Now returns the creation time. Tests force collisions with it.
	Now func() time.Time
	// Rand supplies the random part of names.
	Rand io.Reader

	fs            fileSystem
	renameRetries int
	retryPause    time.Duration
}

func (p *Publisher) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Publisher) rand() io.Reader {
	if p.Rand != nil {
		return p.Rand
	}
	return rand.Reader
}

func (p *Publisher) fsys() fileSystem {
	if p.fs != nil {
		return p.fs
	}
	return osFS{}
}

// Generation is one generation while it is written. Its directory carries
// the partial marker until Publish commits it.
type Generation struct {
	p       *Publisher
	fs      fileSystem
	target  string
	base    string
	created time.Time
	file    file
	// written records the first write error, so that commit refuses an
	// archive whose writing failed.
	written *recordingWriter
	// done is set by a successful commit or Abort. broken is set when a
	// commit step failed or Abort was called. Then only Abort is possible.
	done   bool
	broken bool
}

type recordingWriter struct {
	w   io.Writer
	err error
}

func (r *recordingWriter) Write(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	n, err := r.w.Write(p)
	if err != nil {
		r.err = err
	}
	return n, err
}

// Published describes a published generation.
type Published struct {
	Name    string
	Path    string
	Verdict string
}

// WriteFunc writes the complete archive into w and returns the serialized
// verification result that the archive embeds. created is the creation time
// that the generation name carries, for the archive's own creation time.
//
// It returns the result only when the archive is complete, which is what
// archive.Writer.Finish returns. Publish takes the verdict from these bytes.
type WriteFunc func(w io.Writer, created time.Time) (verification []byte, err error)

// Publish creates a new generation in target, lets write fill its archive
// file and publishes it by the verdict that write returns (design D8 and
// D9). It is the only way to give a generation a final name.
//
// When write fails, the run's partial directory is removed and the error is
// returned. When the verdict is failed or unknown, the partial directory is
// removed as well and ErrFailedCapture or ErrUnknownVerdict is returned.
// When a later step fails, the partial directory stays and the error is
// returned.
func (p *Publisher) Publish(target string, write WriteFunc) (*Published, error) {
	g, err := p.Begin(target)
	if err != nil {
		return nil, err
	}
	verification, err := write(g.written, g.created)
	if err != nil {
		if aerr := g.Abort(); aerr != nil {
			return nil, errors.Join(err, aerr)
		}
		return nil, err
	}
	return g.commit(verification)
}

// Begin checks the target, creates a new partial generation directory with
// mode 0700 and the archive file inside with mode 0600 (design D8, steps 1
// to 3). On failure it leaves nothing behind.
//
// The generation can only be written and aborted. A run publishes with
// Publish. Begin serves tests and tools that need a partial generation.
func (p *Publisher) Begin(target string) (*Generation, error) {
	fsys := p.fsys()
	info, err := fsys.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrTargetMissing
	}
	if err != nil {
		return nil, fmt.Errorf("check target: %w", err)
	}
	if !info.IsDir() {
		return nil, ErrTargetNotDirectory
	}
	created := p.now().UTC().Truncate(time.Second)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var r [4]byte
		if _, err := io.ReadFull(p.rand(), r[:]); err != nil {
			return nil, fmt.Errorf("draw random name part: %w", err)
		}
		base := NamePrefix + created.Format(timeLayout) + "-" + hex.EncodeToString(r[:])
		partial := filepath.Join(target, base+PartialMarker)
		if err := fsys.Mkdir(partial, 0o700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return nil, fmt.Errorf("create generation directory: %w", err)
		}
		// Another run may have published this name and thereby freed its
		// partial name just before the mkdir. The rename is atomic, so its
		// final name is visible now. Withdraw the empty directory and draw
		// again.
		if published, err := finalTaken(fsys, target, base); published || err != nil {
			if rerr := fsys.Remove(partial); rerr != nil {
				return nil, fmt.Errorf("withdraw generation directory: %w", rerr)
			}
			if err != nil {
				return nil, err
			}
			continue
		}
		f, err := fsys.CreateExclusive(filepath.Join(partial, ArchiveName), 0o600)
		if err != nil {
			_ = fsys.Remove(partial)
			return nil, fmt.Errorf("create archive file: %w", err)
		}
		return &Generation{
			p: p, fs: fsys, target: target, base: base, created: created,
			file: f, written: &recordingWriter{w: f},
		}, nil
	}
	return nil, ErrNoFreeName
}

// finalTaken reports whether a final name of base exists.
func finalTaken(fsys fileSystem, target, base string) (bool, error) {
	for _, name := range []string{base, base + UnverifiedMarker} {
		taken, err := exists(fsys, filepath.Join(target, name))
		if err != nil {
			return true, fmt.Errorf("check final name: %w", err)
		}
		if taken {
			return true, nil
		}
	}
	return false, nil
}

// exists reports whether a name is taken. A name whose state cannot be
// read counts as taken, and the error says why.
func exists(fsys fileSystem, name string) (bool, error) {
	_, err := fsys.Stat(name)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return true, err
	}
}

// Created returns the creation time that the name carries.
func (g *Generation) Created() time.Time { return g.created }

// PartialName returns the name of the directory while it is written.
func (g *Generation) PartialName() string { return g.base + PartialMarker }

// Writer returns the archive file to write the encrypted stream into.
func (g *Generation) Writer() io.Writer { return g.written }

func (g *Generation) partialPath() string { return filepath.Join(g.target, g.PartialName()) }

// commit takes the verdict from the serialized verification result that the
// archive embeds and publishes accordingly (design D9). Verified leads to
// the plain name, unverified to the unverified marker. Failed and an unknown
// verdict remove the partial directory.
//
// Before renaming, the archive file and its directory are flushed (design
// D8, steps 4 and 5). If any step fails, the generation stays partial and
// the error is returned. Abort may then remove it.
func (g *Generation) commit(verification []byte) (*Published, error) {
	if g.done {
		return nil, ErrFinished
	}
	if g.broken {
		return nil, ErrCommitFailed
	}
	if g.written.err != nil {
		g.broken = true
		return nil, fmt.Errorf("%w: %w", ErrWriteFailed, g.written.err)
	}
	var r struct {
		Verdict string `json:"verdict"`
	}
	var final string
	if err := json.Unmarshal(verification, &r); err == nil {
		switch r.Verdict {
		case VerdictVerified:
			final = g.base
		case VerdictUnverified:
			final = g.base + UnverifiedMarker
		}
	}
	if final == "" {
		refused := ErrUnknownVerdict
		if r.Verdict == VerdictFailed {
			refused = ErrFailedCapture
		}
		if err := g.Abort(); err != nil {
			return nil, errors.Join(refused, err)
		}
		return nil, refused
	}
	if err := g.file.Sync(); err != nil {
		return nil, g.fail(fmt.Errorf("flush archive file: %w", err))
	}
	f := g.file
	g.file = nil
	if err := f.Close(); err != nil {
		return nil, g.fail(fmt.Errorf("close archive file: %w", err))
	}
	if err := g.fs.SyncDir(g.partialPath()); err != nil {
		return nil, g.fail(fmt.Errorf("flush generation directory: %w", err))
	}
	finalPath := filepath.Join(g.target, final)
	if err := g.rename(finalPath); err != nil {
		return nil, g.fail(err)
	}
	g.done = true
	pub := &Published{Name: final, Path: finalPath, Verdict: r.Verdict}
	if err := g.fs.SyncDir(g.target); err != nil {
		return pub, fmt.Errorf("generation %s is complete, but the target directory could not be flushed: %w", final, err)
	}
	return pub, nil
}

// rename moves the partial directory to its final name after checking that
// the name is free. On Windows a scanner holding a file inside can make the
// rename fail for a moment, so it is retried there.
func (g *Generation) rename(finalPath string) error {
	retries := g.p.renameRetries
	if retries == 0 && runtime.GOOS == "windows" {
		retries = 5
	}
	pause := g.p.retryPause
	if pause == 0 {
		pause = 200 * time.Millisecond
	}
	var renameErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			time.Sleep(pause)
		}
		taken, err := exists(g.fs, finalPath)
		if err != nil {
			return fmt.Errorf("check final name: %w", err)
		}
		if taken {
			return ErrFinalNameExists
		}
		if renameErr = g.fs.Rename(g.partialPath(), finalPath); renameErr == nil {
			return nil
		}
	}
	return fmt.Errorf("rename generation directory: %w", renameErr)
}

// fail marks the generation as broken and closes the archive file. The
// partial directory stays.
func (g *Generation) fail(err error) error {
	g.broken = true
	g.closeFile()
	return err
}

func (g *Generation) closeFile() {
	if g.file != nil {
		_ = g.file.Close()
		g.file = nil
	}
}

// Abort removes the run's own partial directory: the archive file, then the
// directory. If the directory holds anything else, it is left in place and
// an error is returned. A failed Abort may be repeated. Abort after a
// successful commit or Abort returns ErrFinished.
func (g *Generation) Abort() error {
	if g.done {
		return ErrFinished
	}
	g.closeFile()
	// From here on the generation is never published, even if a removal
	// fails and Abort is repeated.
	g.broken = true
	archive := filepath.Join(g.partialPath(), ArchiveName)
	if err := g.fs.Remove(archive); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove partial archive file: %w", err)
	}
	if err := g.fs.Remove(g.partialPath()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove partial generation directory: %w", err)
	}
	g.done = true
	return nil
}
