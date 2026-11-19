// Command fakebw is the fake bw of the tests of package bwcli (design D12 of
// the change bw-process-adapter). It is not part of the product and no
// package imports it. bwclitest.Build builds it.
//
// It reads its scenario from the data directory named in
// BITWARDENCLI_APPDATA_DIR and appends one record per start to a file in the
// same directory: the arguments, the full environment, the working
// directory, whether standard input was empty, its process ID and, outside
// Windows, the soft and the hard core file size limit it inherited.
//
// # Behaviors and their evidence
//
// "Probe" is the live probe of 14 November 2026
// (docs/research/vaultwarden-live-probe-2026-11-14.md). "Notes" are the code
// notes and unauthenticated runs of 15 November 2026
// (docs/research/bw-cli-code-notes-2026-11-15.md), with their finding
// number. The fake reproduces only what the scenario asks for; everything
// else is configured per test.
//
//   - Login with exit code 0 and empty output: probe, result 1, standard
//     input from /dev/null.
//   - Login with exactly one session key of 88 base64 characters and no line
//     ending: probe, result 1. With one line ending: notes, design D6 (bw
//     writes one when standard output is a terminal outside Windows). With
//     extra text: not observed, a negative case of the spec.
//   - A login that writes a prompt to standard error and reads a line from
//     standard input (Prompt, ReadLine): probe, result 1, and notes 4.
//   - Exit code 0 instead of a failure when BW_CLEANEXIT is non-empty: notes
//     2, also observed with the value "false".
//   - A failure when BW_NOINTERACTION is not "true" on a command other than
//     login: notes 4. Real bw would prompt or try the desktop application;
//     the fake writes a prompt to standard error and exits with code 1, so
//     that a missing variable can never pass unnoticed.
//   - A failure with exit code 1 followed by a status document with each
//     state: notes 13, 14 and 17 (bw has no error codes; configured through
//     Exit for the command and Stdout for "status").
//   - "Premium status is required to use this feature." with exit code 1:
//     notes 5 and 9. "You are not logged in." and "Vault is locked." with
//     exit code 1: notes 5 and 14; they stay available so that a test can
//     show that they select no class (configured through Stderr and Exit).
//   - Success with three decryption error lines on standard error: probe,
//     result 6 (configured through Stderr).
//   - A status document with each state, with and without lastSync, and
//     with an unknown state: notes 13 for the profile without login, probe
//     result 5 for lastSync (configured through Stdout).
//   - An error text with a sentinel value: probe, result 4 (bw names objects
//     in error text; configured through Stderr and Stdout).
//   - A command that does not end (Hang), output beyond a limit
//     (StdoutRepeat), a file in the working directory (WriteFile; notes 8)
//     and a data directory left empty (SkipData; notes 3): failure modes the
//     client must handle, not observed as such.
//   - data.json created with mode 0600 in the data directory on every start
//     unless SkipData is set: notes 12.
//   - A child process that sleeps (SpawnChild): the npm layout starts bw
//     through a script and Node; the fake uses it to show that a timeout
//     stops more than the first process.
//   - An end by an interrupt signal (Interrupt): what the terminal does to
//     bw when the user presses the interrupt key at the password prompt. Not
//     observed from a Go parent.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
)

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == bwclitest.SleepArg {
		sleepForever()
	}
	dataDir := os.Getenv("BITWARDENCLI_APPDATA_DIR")
	if dataDir == "" {
		fmt.Fprintln(os.Stderr, "fakebw: BITWARDENCLI_APPDATA_DIR is not set")
		return 97
	}
	scenario, err := bwclitest.ReadScenario(dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakebw: no scenario:", err)
		return 98
	}
	b, ok := scenario.Lookup(args)
	if !ok {
		fmt.Fprintln(os.Stderr, "fakebw: no behavior for the arguments")
		return 96
	}
	login := len(args) > 0 && args[0] == "login"

	rec := bwclitest.Record{Args: args, Env: os.Environ(), PID: os.Getpid(), CoreLimit: coreLimit()}
	rec.Dir, _ = os.Getwd()
	if b.SpawnChild {
		self, err := os.Executable()
		if err == nil {
			child := exec.Command(self, bwclitest.SleepArg)
			if err = child.Start(); err == nil {
				rec.ChildPID = child.Process.Pid
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakebw: child not started:", err)
			return 95
		}
	}
	switch {
	case b.ReadLine:
		if b.Prompt != "" {
			fmt.Fprint(os.Stderr, b.Prompt)
		}
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(os.Stderr, "fakebw: no line on standard input")
			return 94
		}
		rec.Line = strings.TrimRight(line, "\r\n")
	case !login:
		n, _ := io.ReadFull(os.Stdin, make([]byte, 1))
		empty := n == 0
		rec.StdinEmpty = &empty
	}
	if err := bwclitest.AppendRecord(dataDir, rec); err != nil {
		fmt.Fprintln(os.Stderr, "fakebw: record not written:", err)
		return 93
	}
	if b.Interrupt {
		interrupt()
	}
	if !b.SkipData {
		if err := createData(dataDir); err != nil {
			fmt.Fprintln(os.Stderr, "fakebw: data file not written:", err)
			return 92
		}
	}
	if !login && os.Getenv("BW_NOINTERACTION") != "true" {
		fmt.Fprint(os.Stderr, "? Master password: [input is hidden] ")
		return exitCode(1)
	}
	if b.DelayMillis > 0 {
		time.Sleep(time.Duration(b.DelayMillis) * time.Millisecond)
	}
	if b.Hang {
		sleepForever()
	}
	if b.WriteFile != "" {
		if err := os.WriteFile(filepath.Join(rec.Dir, b.WriteFile), []byte("{}"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "fakebw: file not written:", err)
			return 91
		}
	}
	if err := repeat(os.Stdout, b.Stdout, b.StdoutRepeat); err != nil {
		return 90
	}
	if err := repeat(os.Stderr, b.Stderr, b.StderrRepeat); err != nil {
		return 89
	}
	return exitCode(b.Exit)
}

// exitCode applies BW_CLEANEXIT as bw does (notes 2).
func exitCode(code int) int {
	if code != 0 && os.Getenv("BW_CLEANEXIT") != "" {
		return 0
	}
	return code
}

func createData(dataDir string) error {
	f, err := os.OpenFile(filepath.Join(dataDir, bwclitest.DataFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, werr := f.WriteString("{}")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

func repeat(w io.Writer, s string, n int) error {
	if s == "" {
		return nil
	}
	if n < 1 {
		n = 1
	}
	bw := bufio.NewWriterSize(w, 64*1024)
	for i := 0; i < n; i++ {
		if _, err := bw.WriteString(s); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func sleepForever() {
	for {
		time.Sleep(time.Hour)
	}
}
