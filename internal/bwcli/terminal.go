package bwcli

import (
	"errors"
	"os"

	"golang.org/x/term"
)

// Terminal is what the login needs to know about the terminal of the caller
// (design D6). Tests supply a fake, because go test has no terminal.
type Terminal interface {
	// IsTerminal reports whether standard input and standard error are both
	// terminals.
	IsTerminal() bool
	// SaveState saves the state of the terminal on standard input.
	SaveState() error
	// RestoreState restores the state saved last.
	RestoreState() error
}

// NewTerminal returns the Terminal for the given standard input and
// standard error, built on golang.org/x/term. A character device such as
// /dev/null is not a terminal.
func NewTerminal(stdin, stderr *os.File) Terminal {
	return &sysTerminal{stdin: stdin, stderr: stderr}
}

type sysTerminal struct {
	stdin, stderr *os.File
	saved         *term.State
}

func (t *sysTerminal) IsTerminal() bool {
	return t.stdin != nil && t.stderr != nil &&
		term.IsTerminal(int(t.stdin.Fd())) && term.IsTerminal(int(t.stderr.Fd()))
}

func (t *sysTerminal) SaveState() error {
	s, err := term.GetState(int(t.stdin.Fd()))
	if err != nil {
		return err
	}
	t.saved = s
	return nil
}

func (t *sysTerminal) RestoreState() error {
	if t.saved == nil {
		return errors.New("no terminal state saved")
	}
	return term.Restore(int(t.stdin.Fd()), t.saved)
}
