// Command bw-takeout backs up a personal Bitwarden vault with the
// official CLI and checks a backup offline:
//
//	bw-takeout backup --server <https-url> --target <dir> --recipients-file <file> [--bw <path>] [--ca <file>]
//	bw-takeout verify --identity <file> <generation-directory>
//
// backup runs in package run and reports through package report (design
// of the change interactive-backup-run). verify needs no network, no bw and
// no other program; it lives in package offline (design D10 of the change
// archive-generation, D9 of interactive-backup-run). This package only
// parses flags, wires signals and returns the exit status.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/offline"
	"bw-takeout/internal/report"
	"bw-takeout/internal/run"
)

// Exit statuses (design D10 of archive-generation, D7 of
// interactive-backup-run).
const (
	exitVerified           = offline.ExitVerified
	exitFailed             = offline.ExitFailed
	exitUsage              = offline.ExitUsage
	exitUnverified         = offline.ExitUnverified
	exitIncompleteTeardown = report.ExitIncompleteTeardown
)

const backupUsage = `usage: bw-takeout backup --server <https-url> --target <dir> --recipients-file <file>
                         [--bw <path>] [--ca <file>]

Backs up one account interactively. The login and its two-step code are
typed at the terminal.

Flags:
  --server <https-url>      HTTPS address of the server (required)
  --target <dir>            existing directory for the generation (required)
  --recipients-file <file>  age recipients, one per line (required)
  --bw <path>               bw executable (default: bw on the search path)
  --ca <file>               additional certificate authority file

Exit status:
  0  generation published, capture verified, teardown complete
  3  generation published, capture unverified, teardown complete
  4  generation published, but the logout or the profile removal failed
  1  no generation
  2  usage error
`

const usage = backupUsage + "\n" + offline.Usage

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "verify":
			return offline.Run(args[1:], stdout, stderr)
		case "backup":
			return runBackup(args[1:], stdout, stderr)
		}
	}
	fmt.Fprint(stderr, usage)
	return exitUsage
}

// backupFlags parses the flags of design D2. It returns nil after a
// usage error.
func backupFlags(args []string) *run.Config {
	fl := flag.NewFlagSet("backup", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	cfg := &run.Config{}
	fl.StringVar(&cfg.Server, "server", "", "")
	fl.StringVar(&cfg.Target, "target", "", "")
	fl.StringVar(&cfg.RecipientsFile, "recipients-file", "", "")
	fl.StringVar(&cfg.BW, "bw", "", "")
	fl.StringVar(&cfg.CAFile, "ca", "", "")
	if err := fl.Parse(args); err != nil || fl.NArg() != 0 {
		return nil
	}
	if cfg.Server == "" || cfg.Target == "" || cfg.RecipientsFile == "" || !bwcli.ValidServerAddress(cfg.Server) {
		return nil
	}
	return cfg
}

func runBackup(args []string, stdout, stderr io.Writer) int {
	cfg := backupFlags(args)
	if cfg == nil {
		fmt.Fprint(stderr, backupUsage)
		return exitUsage
	}
	keepRunningOnBrokenPipe()
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, interruptSignals...)
	defer signal.Stop(signals)

	w := report.New(stdout, stderr)
	o := run.Run(*cfg, run.Deps{
		Stdin:    os.Stdin,
		Stderr:   os.Stderr,
		Signals:  signals,
		Progress: w,
	})
	w.Final(o)
	return report.ExitStatus(o)
}
