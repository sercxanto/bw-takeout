// Package profile holds the CLI data directory of a run. Each run gets a
// fresh profile directory under the profile location of this project, with
// one directory for the CLI data and one empty working directory for bw
// processes. Teardown removes the profile also when the logout fails.
// Profiles of other runs are listed as leftovers and never touched. See the
// design of the change bw-process-adapter, decision D9, and the
// specification bw-profile.
//
// The package knows nothing about bw. It receives the logout as a function
// and never reads the content of a profile.
package profile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Names inside the profile location.
const (
	// NamePrefix starts every profile name.
	NamePrefix = "profile-"
	// DataDirName is the directory for the CLI data inside a profile.
	DataDirName = "data"
	// WorkDirName is the working directory of bw processes inside a
	// profile.
	WorkDirName = "work"
	timeLayout  = "20060102T150405Z"
)

// dirMode is the mode of the location, the profile and its subdirectories.
const dirMode fs.FileMode = 0o700

// maxAttempts bounds how often a new random part is drawn on a collision.
const maxAttempts = 16

var namePattern = regexp.MustCompile(`^` + NamePrefix + `(\d{8}T\d{6}Z)-[0-9a-f]{8}$`)

// Errors of the package.
var (
	ErrNoFreeName = errors.New("no free profile name found")
)

// ParseName returns the creation time in a profile name. ok is false for a
// name that is not a profile name.
func ParseName(name string) (created time.Time, ok bool) {
	m := namePattern.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(timeLayout, m[1])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// DefaultLocation returns bw-takeout/profiles under the user cache
// directory of the system.
func DefaultLocation() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("profile location: %w", err)
	}
	return filepath.Join(dir, "bw-takeout", "profiles"), nil
}

// Creator creates profiles and lists leftovers. The zero value uses the
// default location, the system clock and the system random source.
type Creator struct {
	// Location is the directory that holds the profiles. Empty takes
	// DefaultLocation.
	Location string
	// Now returns the creation time. Tests force collisions with it.
	Now func() time.Time
	// Rand supplies the random part of names.
	Rand io.Reader

	// removeAll replaces os.RemoveAll in tests.
	removeAll func(path string) error
}

func (c *Creator) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Creator) rand() io.Reader {
	if c.Rand != nil {
		return c.Rand
	}
	return rand.Reader
}

func (c *Creator) location() (string, error) {
	if c.Location != "" {
		return c.Location, nil
	}
	return DefaultLocation()
}

// Profile is the profile directory of one run.
type Profile struct {
	// Name is the name of the profile directory.
	Name string
	// Path is the absolute path of the profile directory.
	Path string
	// DataDir is the CLI data directory, for BITWARDENCLI_APPDATA_DIR.
	DataDir string
	// WorkDir is the empty working directory of bw processes.
	WorkDir string
	// Created is the time in the name.
	Created time.Time

	removeAll func(path string) error
}

// Create creates the profile location if it is missing and a new profile
// inside it with an empty data directory and an empty working directory,
// each with mode 0700. A name that exists is never used: the profile
// directory is created with a call that fails on an existing name, and
// another random part is drawn on a collision.
func (c *Creator) Create() (*Profile, error) {
	loc, err := c.location()
	if err != nil {
		return nil, err
	}
	loc, err = filepath.Abs(loc)
	if err != nil {
		return nil, fmt.Errorf("profile location: %w", err)
	}
	if err := os.MkdirAll(loc, dirMode); err != nil {
		return nil, fmt.Errorf("create profile location: %w", err)
	}
	created := c.now().UTC().Truncate(time.Second)
	for range maxAttempts {
		var b [4]byte
		if _, err := io.ReadFull(c.rand(), b[:]); err != nil {
			return nil, fmt.Errorf("random part of profile name: %w", err)
		}
		name := NamePrefix + created.Format(timeLayout) + "-" + hex.EncodeToString(b[:])
		path := filepath.Join(loc, name)
		if err := os.Mkdir(path, dirMode); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return nil, fmt.Errorf("create profile: %w", err)
		}
		p := &Profile{
			Name:      name,
			Path:      path,
			DataDir:   filepath.Join(path, DataDirName),
			WorkDir:   filepath.Join(path, WorkDirName),
			Created:   created,
			removeAll: c.removeAll,
		}
		for _, dir := range []string{p.DataDir, p.WorkDir} {
			if err := os.Mkdir(dir, dirMode); err != nil {
				// The directory was created by this call and holds
				// nothing but what it created, so it can be removed.
				_ = os.RemoveAll(path)
				return nil, fmt.Errorf("create profile: %w", err)
			}
		}
		return p, nil
	}
	return nil, ErrNoFreeName
}

// RemoveError reports a profile directory that teardown could not remove.
type RemoveError struct {
	Path string
	Err  error
}

func (e *RemoveError) Error() string {
	return fmt.Sprintf("remove profile %s: %v", e.Path, e.Err)
}

func (e *RemoveError) Unwrap() error { return e.Err }

// Teardown calls logout if it is not nil, then removes the profile
// directory with everything in it, also when the logout failed. The caller
// passes nil when no login succeeded. Both errors are returned separately.
// A removal error is a *RemoveError that names the directory.
func (p *Profile) Teardown(logout func() error) (logoutErr, removeErr error) {
	if logout != nil {
		logoutErr = logout()
	}
	remove := p.removeAll
	if remove == nil {
		remove = os.RemoveAll
	}
	if err := remove(p.Path); err != nil {
		removeErr = &RemoveError{Path: p.Path, Err: err}
	}
	return logoutErr, removeErr
}

// Leftover is the profile directory of another run.
type Leftover struct {
	Name string
	// Path is the absolute path of the directory.
	Path string
	// Age is the time since the creation time in the name.
	Age time.Duration
}

// Leftovers returns the profile directories in the location other than
// own, sorted by name, with their age. own may be nil. Only directories
// whose name has the form of a profile name are listed. A missing location
// gives no leftovers. The listing reads only the directory entries and
// changes nothing: a leftover of an ended run cannot be told from the
// profile of a running one, so the caller must not remove it.
func (c *Creator) Leftovers(own *Profile) ([]Leftover, error) {
	loc, err := c.location()
	if err != nil {
		return nil, err
	}
	loc, err = filepath.Abs(loc)
	if err != nil {
		return nil, fmt.Errorf("profile location: %w", err)
	}
	entries, err := os.ReadDir(loc)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list profile location: %w", err)
	}
	now := c.now()
	var out []Leftover
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if own != nil && filepath.Join(loc, e.Name()) == own.Path {
			continue
		}
		created, ok := ParseName(e.Name())
		if !ok {
			continue
		}
		out = append(out, Leftover{Name: e.Name(), Path: filepath.Join(loc, e.Name()), Age: now.Sub(created)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
