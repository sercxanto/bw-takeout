package bwcli

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"os/signal"
)

// Session key form (design D6): base64 text of 88 characters that decodes
// to 64 bytes.
const (
	sessionKeyBytes  = 64
	sessionKeyLength = 88
)

// Login runs the interactive bw login --raw (design D6). It refuses before
// any start unless standard input and standard error are a terminal and
// Version has confirmed that bw uses the data directory. The child reads
// from and prompts on the terminal; its standard output goes through a pipe
// and is never shown. The child gets neither BW_NOINTERACTION nor a session
// key, has no time limit, and stays in the process group of the caller so
// that it can read the terminal. A cancelled context stops it.
//
// The terminal state is saved before the start and restored after the
// child has ended, whatever the outcome. While the child runs, interrupt
// signals are received here instead of ending the caller; an interrupt
// makes the login fail with the class cancelled.
//
// The login counts as successful only when bw exits with code 0 and its
// standard output is exactly one session key, optionally followed by one
// line ending. The key is kept in the client and never returned.
func (c *Client) Login(ctx context.Context) error {
	if !c.cfg.Terminal.IsTerminal() {
		return failure(OpLogin, ClassNoTerminal, NoExitCode)
	}
	if !c.profileUsed {
		return failure(OpLogin, ClassProfileNotUsed, NoExitCode)
	}
	if ctx.Err() != nil {
		return failure(OpLogin, ClassCancelled, NoExitCode)
	}
	if err := c.cfg.Terminal.SaveState(); err != nil {
		// Without a saved state the terminal could not be restored.
		return failure(OpLogin, ClassNoTerminal, NoExitCode)
	}

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	stdout := &limitedBuffer{limit: loginOutputLimit, onExceed: stop}
	defer stdout.wipe()
	p := exec.CommandContext(runCtx, c.cfg.Executable, "login", "--raw")
	p.Dir = c.cfg.WorkDir
	p.Env = c.environ(true, false)
	p.Stdin = c.cfg.Stdin
	p.Stderr = c.cfg.Stderr
	p.Stdout = stdout
	p.WaitDelay = waitDelay

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, interruptSignals...)
	_ = p.Run()
	signal.Stop(interrupts)
	interrupted := len(interrupts) > 0
	restoreErr := c.cfg.Terminal.RestoreState()

	if p.ProcessState == nil {
		return failure(OpLogin, ClassStart, NoExitCode)
	}
	exitCode := p.ProcessState.ExitCode()
	switch {
	case ctx.Err() != nil, interrupted:
		return failure(OpLogin, ClassCancelled, exitCode)
	case stdout.exceeded:
		return failure(OpLogin, ClassOutput, NoExitCode)
	case restoreErr != nil:
		// A terminal left in another state is not one this login may
		// count on, and the caller must be able to tear down.
		return failure(OpLogin, ClassNoTerminal, exitCode)
	case !c.workDirEmpty():
		return failure(OpLogin, ClassStrayFile, exitCode)
	case exitCode != 0:
		return failure(OpLogin, ClassExit, exitCode)
	}
	key, ok := sessionKey(stdout.buf)
	if !ok {
		return failure(OpLogin, ClassOutput, 0)
	}
	c.forgetSession()
	c.session = key
	return nil
}

// sessionKey returns a copy of the session key when out is exactly one key,
// optionally followed by "\n" or "\r\n".
func sessionKey(out []byte) ([]byte, bool) {
	n := len(out)
	switch {
	case n == sessionKeyLength:
	case n == sessionKeyLength+1 && out[n-1] == '\n':
	case n == sessionKeyLength+2 && out[n-2] == '\r' && out[n-1] == '\n':
	default:
		return nil, false
	}
	text := out[:sessionKeyLength]
	for _, b := range text {
		// The decoder skips line endings, so the alphabet is checked
		// first.
		if !isBase64Char(b) {
			return nil, false
		}
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(sessionKeyLength))
	defer clear(decoded)
	m, err := base64.StdEncoding.Strict().Decode(decoded, text)
	if err != nil || m != sessionKeyBytes {
		return nil, false
	}
	return append([]byte(nil), text...), true
}

func isBase64Char(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '+' || b == '/' || b == '='
}
