//go:build vaultwarden && linux

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal through /dev/ptmx: it unlocks the
// secondary side and opens it by its number (design D6).
func openPTY() (primary, secondary *os.File, err error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	primary = os.NewFile(uintptr(fd), "/dev/ptmx")
	fail := func(err error) (*os.File, *os.File, error) {
		primary.Close()
		return nil, nil, err
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		return fail(fmt.Errorf("unlock: %w", err))
	}
	n, err := unix.IoctlGetUint32(fd, unix.TIOCGPTN)
	if err != nil {
		return fail(fmt.Errorf("secondary number: %w", err))
	}
	name := "/dev/pts/" + strconv.FormatUint(uint64(n), 10)
	sfd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(fmt.Errorf("open %s: %w", name, err))
	}
	// A size, so that the prompt library does not wrap at column 0.
	if err := unix.IoctlSetWinsize(sfd, unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 120}); err != nil {
		unix.Close(sfd)
		return fail(fmt.Errorf("window size: %w", err))
	}
	return primary, os.NewFile(uintptr(sfd), name), nil
}

// promptPattern matches a prompt of bw in the form "? <name>:".
var (
	promptPattern = regexp.MustCompile(`\? ([A-Za-z][A-Za-z0-9 ()'-]*?):`)
	ansiPattern   = regexp.MustCompile(`\x1b(\[[0-9;?]*[ -/]*[@-~]|[@-Z\\-_])`)
)

// terminalRun is what a command under the pseudo-terminal left: its exit
// status, its standard output and the names of the prompts it showed, in
// order.
type terminalRun struct {
	ExitCode int
	Stdout   []byte
	Prompts  []string
}

// runWithPrompts starts cmd with the secondary side of a fresh
// pseudo-terminal as standard input, standard error and controlling
// terminal, and standard output to a buffer. A reader on the primary side
// answers each known prompt once with its answer and a carriage return. An
// unknown prompt or no exit within limit ends the command and fails.
//
// The terminal text is kept in memory only. An error names the prompts
// seen, never the text or an answer, because bw echoes the address there.
func runWithPrompts(cmd *exec.Cmd, answers map[string]string, limit time.Duration) (*terminalRun, error) {
	primary, secondary, err := openPTY()
	if err != nil {
		return nil, err
	}
	defer primary.Close()
	var stdout bytes.Buffer
	cmd.Stdin = secondary
	cmd.Stderr = secondary
	cmd.Stdout = &stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		secondary.Close()
		return nil, err
	}
	secondary.Close()

	var (
		mu  sync.Mutex
		raw []byte
	)
	newData := make(chan struct{}, 1)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := primary.Read(buf)
			if n > 0 {
				mu.Lock()
				raw = append(raw, buf[:n]...)
				mu.Unlock()
				select {
				case newData <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	run := &terminalRun{}
	answered := map[string]bool{}
	// handled counts the prompt matches already looked at. The stripped
	// text only grows at its end, so earlier matches keep their order.
	handled := 0
	timer := time.NewTimer(limit)
	defer timer.Stop()
	var failure error
	scan := func() {
		mu.Lock()
		text := ansiPattern.ReplaceAllString(string(raw), "")
		mu.Unlock()
		matches := promptPattern.FindAllStringSubmatch(text, -1)
		for ; handled < len(matches); handled++ {
			name := matches[handled][1]
			answer, known := answers[name]
			if !known {
				run.Prompts = append(run.Prompts, name)
				failure = fmt.Errorf("unknown prompt %q", name)
				return
			}
			// A prompt line that the library draws again after the answer
			// is not answered twice.
			if answered[name] {
				continue
			}
			run.Prompts = append(run.Prompts, name)
			answered[name] = true
			if _, err := primary.Write([]byte(answer + "\r")); err != nil {
				failure = fmt.Errorf("answer to %q: %w", name, err)
				return
			}
		}
	}

	var waitErr error
loop:
	for {
		select {
		case <-newData:
			scan()
			if failure != nil {
				cmd.Process.Kill()
				waitErr = <-waitDone
				break loop
			}
		case waitErr = <-waitDone:
			break loop
		case <-timer.C:
			failure = fmt.Errorf("no exit within %v", limit)
			cmd.Process.Kill()
			waitErr = <-waitDone
			break loop
		}
	}
	// The secondary side is closed in the child now; the reader ends with
	// an error once the buffer is drained.
	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
	}
	if failure == nil {
		scan()
	}
	run.Stdout = stdout.Bytes()
	if failure != nil {
		return run, fmt.Errorf("%w (prompts seen: %s)", failure, promptNames(run.Prompts))
	}
	var exit *exec.ExitError
	switch {
	case waitErr == nil:
		run.ExitCode = 0
	case errors.As(waitErr, &exit):
		run.ExitCode = exit.ExitCode()
	default:
		return run, waitErr
	}
	return run, nil
}

func promptNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
