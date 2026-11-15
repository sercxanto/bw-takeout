package bwcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// outcome is what a successful command leaves: its standard output and the
// number of decryption error lines.
type outcome struct {
	stdout           []byte
	decryptionErrors int
}

// run starts one non-interactive command and classifies a non-zero exit
// code of a vault command (design D7).
func (c *Client) run(ctx context.Context, cmd command) (outcome, error) {
	out, premium, err := c.start(ctx, cmd)
	var e *Error
	if errors.As(err, &e) && e.Class == ClassExit && cmd.vault {
		e.Class = c.classifyExit(ctx, cmd, premium)
	}
	return out, err
}

// classifyExit selects the class of a vault command that ended with a
// non-zero exit code. bw has no error codes, so the class is taken from a
// status query with the same session key: unauthenticated gives
// ClassNotLoggedIn and locked gives ClassLocked. An unlocked vault, a
// failed status query and a cancelled context give ClassExit. The premium
// sentence of an attachment download gives ClassPremiumRequired without a
// status query, because bw checks premium only after it has found the
// vault unlocked, and a vault without premium can hold many attachments.
func (c *Client) classifyExit(ctx context.Context, cmd command, premium bool) Class {
	if cmd.op == OpAttachment && premium {
		return ClassPremiumRequired
	}
	if ctx.Err() != nil {
		return ClassExit
	}
	st, err := c.queryStatus(ctx)
	if err != nil {
		return ClassExit
	}
	switch st.State {
	case StateUnauthenticated:
		return ClassNotLoggedIn
	case StateLocked:
		return ClassLocked
	}
	return ClassExit
}

// start starts one non-interactive command (design D4): empty standard
// input, standard output in memory up to the limit, standard error scanned
// and not stored, the time limit of the command, and the empty working
// directory, which is checked afterwards. It also reports whether standard
// error held the premium sentence.
func (c *Client) start(ctx context.Context, cmd command) (outcome, bool, error) {
	premium := false
	fail := func(class Class, exitCode int) (outcome, bool, error) {
		e := failure(cmd.op, class, exitCode)
		e.ItemID, e.AttachmentID = cmd.itemID, cmd.attachmentID
		return outcome{}, premium, e
	}
	if ctx.Err() != nil {
		return fail(ClassCancelled, NoExitCode)
	}
	limitCtx, cancelLimit := context.WithTimeout(ctx, cmd.timeout)
	defer cancelLimit()
	runCtx, stop := context.WithCancel(limitCtx)
	defer stop()

	stdout := &limitedBuffer{limit: c.cfg.OutputLimit, onExceed: stop}
	stderr := &stderrScanner{}
	p := exec.CommandContext(runCtx, c.cfg.Executable, cmd.args...)
	p.Dir = c.cfg.WorkDir
	p.Env = c.environ(false, cmd.session)
	p.Stdin = nil // the null device
	p.Stdout = stdout
	p.Stderr = stderr
	p.WaitDelay = waitDelay
	ownProcessGroup(p)

	// The error of Run is not needed: the classes below follow from the
	// process state, the contexts and the output buffer.
	_ = p.Run()
	stderr.flush()
	premium = stderr.premium
	if p.ProcessState == nil {
		stdout.wipe()
		return fail(ClassStart, NoExitCode)
	}
	exitCode := p.ProcessState.ExitCode()
	switch {
	case ctx.Err() != nil:
		stdout.wipe()
		return fail(ClassCancelled, NoExitCode)
	case stdout.exceeded:
		stdout.wipe()
		return fail(ClassOutput, NoExitCode)
	case errors.Is(limitCtx.Err(), context.DeadlineExceeded):
		stdout.wipe()
		return fail(ClassTimeout, NoExitCode)
	}
	if !c.workDirEmpty() {
		stdout.wipe()
		return fail(ClassStrayFile, exitCode)
	}
	if exitCode != 0 {
		stdout.wipe()
		return fail(ClassExit, exitCode)
	}
	return outcome{stdout: stdout.buf, decryptionErrors: stderr.decryptionErrors}, premium, nil
}

// workDirEmpty reports whether the working directory is still empty. A
// directory that cannot be read counts as not empty. Nothing in it is read
// or removed; teardown removes it with the profile.
func (c *Client) workDirEmpty() bool {
	entries, err := os.ReadDir(c.cfg.WorkDir)
	return err == nil && len(entries) == 0
}
