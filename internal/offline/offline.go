// Package offline is the verify command of bw-takeout, the offline check of
// a published generation:
//
//	bw-takeout verify --identity <file> <generation-directory>
//
// It needs no network, no bw and no other program. It writes no file and
// prints no vault content. See the design of the change archive-generation,
// decision D10, and of the change interactive-backup-run, decision D9.
//
// The package must never depend on the network packages, on os/exec, or on
// the packages that start bw or judge a capture. A test checks its
// dependency list.
package offline

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/printable"
	"bw-takeout/internal/publish"
)

// Exit statuses (design D10 of archive-generation).
const (
	ExitVerified   = 0
	ExitFailed     = 1
	ExitUsage      = 2
	ExitUnverified = 3
)

// Usage is the usage text of the verify command.
const Usage = `usage: bw-takeout verify --identity <file> <generation-directory>

Checks a backup generation offline with an age identity file.

Exit status:
  0  intact generation with a verified capture
  3  intact generation with an unverified capture
  1  verification failed
  2  usage error
`

// Run runs the verify command with the arguments that follow the word
// verify and returns the exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("verify", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	identity := fl.String("identity", "", "age identity file")
	if err := fl.Parse(args); err != nil || *identity == "" || fl.NArg() != 1 {
		fmt.Fprint(stderr, Usage)
		return ExitUsage
	}
	return verifyGeneration(*identity, fl.Arg(0), stdout, stderr)
}

func failed(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "verification failed: "+format+"\n", args...)
	return ExitFailed
}

// generationName returns the name of the directory that dir resolves to,
// so that ".", ".." and symbolic links do not hide the partial marker.
func generationName(dir string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		resolved = dir
	}
	if abs, err := filepath.Abs(resolved); err == nil {
		resolved = abs
	}
	return filepath.Base(resolved)
}

func verifyGeneration(identityPath, dir string, stdout, stderr io.Writer) int {
	name := generationName(dir)
	if strings.HasSuffix(name, publish.PartialMarker) {
		return failed(stderr, "generation %s is incomplete: it carries the %s marker", printable.Clean(name), publish.PartialMarker)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return failed(stderr, "%s is not a generation directory", printable.Clean(name))
	}
	ids, err := readIdentities(identityPath)
	if err != nil {
		return failed(stderr, "%v", err)
	}
	f, err := os.Open(filepath.Join(dir, publish.ArchiveName))
	if errors.Is(err, fs.ErrNotExist) {
		return failed(stderr, "generation %s is incomplete: it holds no %s", printable.Clean(name), publish.ArchiveName)
	}
	if err != nil {
		return failed(stderr, "cannot open %s", publish.ArchiveName)
	}
	defer f.Close()

	m, err := archive.Read(f, ids, nil)
	if err != nil {
		return reportReadError(stderr, err)
	}
	var result storedResult
	if err := json.Unmarshal(m.Verification, &result); err != nil {
		return failed(stderr, "the stored verification result cannot be read")
	}

	fmt.Fprintf(stdout, "Generation:  %s\n", printable.Clean(name))
	fmt.Fprintf(stdout, "Created:     %s\n", m.Created.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(stdout, "Server:      %s\n", printable.Clean(m.Source.ServerURL))
	fmt.Fprintf(stdout, "Account:     %s (%s)\n", printable.Clean(m.Source.AccountEmail), printable.Clean(m.Source.AccountID))
	fmt.Fprintf(stdout, "Items:       %d\n", m.Counts.Items)
	fmt.Fprintf(stdout, "Attachments: %d\n", m.Counts.Attachments)
	fmt.Fprintf(stdout, "Archive:     intact, %d entries match the manifest\n", len(m.Entries))
	fmt.Fprintf(stdout, "Verdict:     %s\n", printable.Clean(result.Verdict))
	fmt.Fprintf(stdout, "Findings:    %d\n", len(result.Findings))
	fmt.Fprintf(stdout, "Warnings:    %d\n", len(result.Warnings))

	switch result.Verdict {
	case publish.VerdictVerified:
		return ExitVerified
	case publish.VerdictUnverified:
		return ExitUnverified
	default:
		return failed(stderr, "the stored verdict is not verified or unverified")
	}
}

// storedResult is the part of the serialized verification result of
// package verify that the command reports. The command does not import that
// package, because its key parsing pulls the network package into the
// dependencies (design D10). A test checks the keys.
type storedResult struct {
	Verdict  string            `json:"verdict"`
	Findings []json.RawMessage `json:"findings"`
	Warnings []json.RawMessage `json:"warnings"`
}

// readIdentities reads the identity file. Errors name the problem, never
// the content of the file.
func readIdentities(path string) ([]age.Identity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("identity file cannot be opened")
	}
	defer f.Close()
	return archive.ParseIdentityFile(f)
}

func reportReadError(stderr io.Writer, err error) int {
	var uv *manifest.UnsupportedVersionError
	switch {
	case errors.Is(err, archive.ErrNoIdentityMatch):
		return failed(stderr, "no identity in the identity file matches the archive")
	case errors.As(err, &uv):
		return failed(stderr, "format version %d is not supported by this version of bw-takeout", uv.Version)
	}
	if entries := archive.EntryErrors(err); len(entries) > 0 {
		for _, e := range entries {
			fmt.Fprintf(stderr, "entry %s: %s\n", printable.Clean(e.Path), e.Problem)
		}
		return failed(stderr, "%d entries do not match the format or the manifest", len(entries))
	}
	switch {
	case errors.Is(err, archive.ErrDamaged):
		return failed(stderr, "the archive is damaged or truncated")
	case errors.Is(err, archive.ErrStructure):
		return failed(stderr, "%v", err)
	}
	return failed(stderr, "the archive cannot be read")
}
