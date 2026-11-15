// Command loginprobe runs the manual login exercise of the change
// bw-process-adapter (design D14) against a server with an account. It is a
// development tool and not part of the product. No other package imports
// it.
//
// Usage:
//
//	go run ./internal/tools/loginprobe -bw "$(command -v bw)" -server https://localhost:8443 [-ca cert.pem]
//
// It creates a fresh profile, queries the version, sets the server, logs
// in, reads the status, syncs, reads the status again, logs out and tears
// down. The logout runs after every login attempt, also a failed one,
// because bw can be logged in although the client refused its session key.
// It prints each step with its result: ok or the failure class, the
// form of the session key, the status state, whether lastSync is set and
// later than the start, and the number of entries in the data file whose
// name starts with user_ after the logout. It never prints the session key
// or any text bw wrote. Reading the data file is for research only, to
// learn what a logout leaves behind; the product never reads it (ADR 0001).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/profile"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, deps{}))
}

// deps are replaced in tests.
type deps struct {
	newProfile func() (*profile.Profile, error)
	terminal   bwcli.Terminal
	stdin      *os.File
	stderr     *os.File
	now        func() time.Time
}

// run returns 0 when every step succeeded, 1 when a step failed and 2 on a
// usage error.
func run(args []string, stdout, stderr io.Writer, d deps) int {
	fs := flag.NewFlagSet("loginprobe", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bwPath := fs.String("bw", "", "absolute path of the bw executable")
	server := fs.String("server", "", "HTTPS address of the server")
	caFile := fs.String("ca", "", "optional absolute path of a certificate authority file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *bwPath == "" || *server == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: loginprobe -bw <path> -server <https address> [-ca <file>]")
		return 2
	}
	if d.newProfile == nil {
		d.newProfile = (&profile.Creator{}).Create
	}
	if d.now == nil {
		d.now = time.Now
	}
	start := d.now().UTC()
	fmt.Fprintf(stdout, "start: %s\n", start.Format(time.RFC3339))

	p, err := d.newProfile()
	if err != nil {
		fmt.Fprintln(stdout, "profile: failed")
		return 1
	}
	fmt.Fprintln(stdout, "profile: created")
	failed := false
	step := func(name string, err error) bool {
		if err != nil {
			failed = true
			fmt.Fprintf(stdout, "%s: failed (class %s)\n", name, bwcli.ClassOf(err))
			return false
		}
		fmt.Fprintf(stdout, "%s: ok\n", name)
		return true
	}

	c, err := bwcli.New(bwcli.Config{
		Executable: *bwPath, DataDir: p.DataDir, WorkDir: p.WorkDir, CAFile: *caFile,
		Stdin: d.stdin, Stderr: d.stderr, Terminal: d.terminal,
	})
	ctx := context.Background()
	loginTried := false
	if err != nil {
		fmt.Fprintln(stdout, "client: failed (invalid configuration)")
		failed = true
	} else if probe(ctx, c, *server, start, stdout, step, &loginTried) {
		failed = true
	}

	var logout func() error
	logoutStep := "logout"
	if loginTried {
		if !c.LoggedIn() {
			logoutStep = "logout after failed login"
		}
		logout = func() error {
			err := c.Logout(ctx)
			fmt.Fprintf(stdout, "data file after logout: %s\n", userEntries(p.DataDir))
			return err
		}
	} else {
		fmt.Fprintln(stdout, "data file after logout: not read, no login attempted")
	}
	logoutErr, removeErr := p.Teardown(logout)
	if logout != nil {
		step(logoutStep, logoutErr)
	}
	if removeErr != nil {
		failed = true
		var re *profile.RemoveError
		if errors.As(removeErr, &re) {
			fmt.Fprintf(stdout, "teardown: failed, remove %s by hand\n", re.Path)
		} else {
			fmt.Fprintln(stdout, "teardown: failed")
		}
	} else {
		fmt.Fprintln(stdout, "teardown: profile removed")
	}
	if failed {
		return 1
	}
	return 0
}

// probe runs the steps up to the logout and reports whether one failed. It
// sets loginTried before it starts the login.
func probe(ctx context.Context, c *bwcli.Client, server string, start time.Time, stdout io.Writer, step func(string, error) bool, loginTried *bool) bool {
	failed := false
	if _, err := c.Version(ctx); !step("version", err) {
		return true
	}
	if !step("server", c.SetServer(ctx, server)) {
		return true
	}
	*loginTried = true
	err := c.Login(ctx)
	switch {
	case err == nil:
		fmt.Fprintln(stdout, "login: ok, session key form valid")
	case bwcli.ClassOf(err) == bwcli.ClassOutput:
		fmt.Fprintln(stdout, "login: failed (class output), session key form invalid")
		return true
	default:
		step("login", err)
		return true
	}
	if !status(ctx, c, start, stdout, step, "status after login") {
		failed = true
	}
	if !step("sync", c.Sync(ctx)) {
		failed = true
	}
	if !status(ctx, c, start, stdout, step, "status after sync") {
		failed = true
	}
	return failed
}

func status(ctx context.Context, c *bwcli.Client, start time.Time, stdout io.Writer, step func(string, error) bool, name string) bool {
	s, err := c.Status(ctx)
	if err != nil {
		return step(name, err)
	}
	last := "not set"
	if s.LastSync != nil {
		if s.LastSync.After(start) {
			last = "set, later than the start"
		} else {
			last = "set, not later than the start"
		}
	}
	fmt.Fprintf(stdout, "%s: ok, state %s, lastSync %s\n", name, s.State, last)
	return true
}

// userEntries counts the top-level entries of data.json whose name starts
// with user_. It returns a fixed text when the file is missing or does not
// parse, never content of the file.
func userEntries(dataDir string) string {
	b, err := os.ReadFile(filepath.Join(dataDir, "data.json"))
	if err != nil {
		return "missing"
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return "does not parse as a JSON object"
	}
	n := 0
	for k := range top {
		if strings.HasPrefix(k, "user_") {
			n++
		}
	}
	return fmt.Sprintf("%d entries whose name starts with user_", n)
}
