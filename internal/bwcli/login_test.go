package bwcli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
)

const (
	testPassword = "synthetic-master-password"
	testPrompt   = "? Master password: [input is hidden] "
)

var versionOK = bwclitest.Behavior{Stdout: "2026.10.1\n"}

// loginFixture is a fixture with a terminal, a confirmed data directory and
// the login behavior. Standard input is a pipe that holds input, and
// standard error is a file.
type loginFixture struct {
	*fixture
	stdinW *os.File
	stderr string
}

func newLoginFixture(t *testing.T, login bwclitest.Behavior, input string) *loginFixture {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	stderrPath := filepath.Join(t.TempDir(), "stderr")
	stderr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stderr.Close() })
	f := newFixture(t, scenario("--version", versionOK, "login --raw", login), func(c *Config) {
		c.Stdin, c.Stderr = r, stderr
	})
	f.term.terminal = true
	if _, err := f.client.Version(bg); err != nil {
		t.Fatal(err)
	}
	if input != "" {
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
	}
	return &loginFixture{fixture: f, stdinW: w, stderr: stderrPath}
}

func (f *loginFixture) stderrText() string {
	f.t.Helper()
	b, err := os.ReadFile(f.stderr)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

// loginRecords returns the records of login processes.
func (f *fixture) loginRecords() []bwclitest.Record {
	var out []bwclitest.Record
	for _, r := range f.records() {
		if len(r.Args) > 0 && r.Args[0] == "login" {
			out = append(out, r)
		}
	}
	return out
}

// sameDir compares two directories by identity, because the fake reports
// the resolved path, and on macOS the temporary directory is reached through
// the symbolic link /var.
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}

func wantRestored(t *testing.T, term *fakeTerminal) {
	t.Helper()
	if term.saves != 1 || term.restores != 1 || !term.restoredAfterSave {
		t.Errorf("terminal saved %d and restored %d times", term.saves, term.restores)
	}
}

func wantLoginFailed(t *testing.T, f *fixture, err error, class Class) {
	t.Helper()
	wantClass(t, err, class)
	if f.client.LoggedIn() {
		t.Error("the client holds a session key after a failed login")
	}
	assertNoText(t, err, testKey)
}

// Requirement: Interactive login needs a terminal.

func TestLoginWithoutATerminal(t *testing.T) {
	f := newFixture(t, scenario("--version", versionOK, "login --raw", bwclitest.Behavior{Stdout: testKey}))
	if _, err := f.client.Version(bg); err != nil {
		t.Fatal(err)
	}
	err := f.client.Login(bg)
	wantLoginFailed(t, f, err, ClassNoTerminal)
	if n := len(f.loginRecords()); n != 0 {
		t.Errorf("%d login processes started", n)
	}
	if f.term.saves != 0 {
		t.Error("terminal state saved without a terminal")
	}
}

func TestLoginWithATerminal(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Prompt: testPrompt, ReadLine: true, Stdout: testKey}, testPassword+"\n")
	if err := f.client.Login(bg); err != nil {
		t.Fatal(err)
	}
	if !f.client.LoggedIn() || string(f.client.session) != testKey {
		t.Fatal("session key not kept")
	}
	recs := f.loginRecords()
	if len(recs) != 1 {
		t.Fatalf("%d login processes", len(recs))
	}
	rec := recs[0]
	if !slices.Equal(rec.Args, []string{"login", "--raw"}) {
		t.Errorf("arguments %q", rec.Args)
	}
	if rec.Line != testPassword {
		t.Errorf("bw read %q from the terminal", rec.Line)
	}
	if !sameDir(t, rec.Dir, f.work) {
		t.Errorf("working directory %q, want %q", rec.Dir, f.work)
	}
	for _, name := range []string{"BW_NOINTERACTION", "BW_SESSION"} {
		if _, ok := rec.Lookup(name); ok {
			t.Errorf("the login received %s", name)
		}
	}
	if v, _ := rec.Lookup("BITWARDENCLI_APPDATA_DIR"); v != f.data {
		t.Errorf("BITWARDENCLI_APPDATA_DIR is %q", v)
	}
	text := f.stderrText()
	if !strings.Contains(text, testPrompt) {
		t.Errorf("prompt not on the terminal: %q", text)
	}
	if strings.Contains(text, testKey) {
		t.Error("standard output of the login was shown on the terminal")
	}
	wantRestored(t, f.term)
}

// Requirement: Every process uses the run's data directory (no login when
// bw does not use the data directory).

func TestNoLoginWhenBwUsesAnotherDataDirectory(t *testing.T) {
	f := newFixture(t, scenario("--version", bwclitest.Behavior{Stdout: "2026.10.1\n", SkipData: true}, "login --raw", bwclitest.Behavior{Stdout: testKey}))
	f.term.terminal = true
	_, err := f.client.Version(bg)
	wantClass(t, err, ClassProfileNotUsed)
	err = f.client.Login(bg)
	wantLoginFailed(t, f, err, ClassProfileNotUsed)
	if n := len(f.loginRecords()); n != 0 {
		t.Errorf("%d login processes started", n)
	}
	if f.term.saves != 0 {
		t.Error("terminal state saved")
	}
}

func TestNoLoginWithoutVersionQuery(t *testing.T) {
	f := newFixture(t, scenario("login --raw", bwclitest.Behavior{Stdout: testKey}))
	f.term.terminal = true
	err := f.client.Login(bg)
	wantLoginFailed(t, f, err, ClassProfileNotUsed)
	if n := len(f.records()); n != 0 {
		t.Errorf("%d processes started", n)
	}
}

// Requirement: Session key form decides the login result.

func TestExitCode0WithEmptyOutput(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{}, "")
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassOutput)
	wantRestored(t, f.term)
}

func TestValidSessionKey(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Stdout: testKey}, "")
	if err := f.client.Login(bg); err != nil {
		t.Fatal(err)
	}
	if string(f.client.session) != testKey {
		t.Error("session key not kept")
	}
}

func TestSessionKeyFollowedByALineEnding(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		f := newLoginFixture(t, bwclitest.Behavior{Stdout: testKey + ending}, "")
		if err := f.client.Login(bg); err != nil {
			t.Fatalf("%q: %v", ending, err)
		}
		if string(f.client.session) != testKey {
			t.Errorf("%q: session key %q", ending, f.client.session)
		}
	}
}

func TestOtherTextNextToTheKey(t *testing.T) {
	for _, out := range []string{
		"You are logged in!\n" + testKey,
		testKey + "\nTo unlock your vault, set your session key\n",
		testKey + " ",
		" " + testKey,
		testKey + testKey,
		"\x1b[0m" + testKey,
	} {
		f := newLoginFixture(t, bwclitest.Behavior{Stdout: out}, "")
		err := f.client.Login(bg)
		wantLoginFailed(t, f.fixture, err, ClassOutput)
	}
}

func TestNonZeroExitCodeWithAKey(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Exit: 1, Stdout: testKey}, "")
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassExit)
	// A login is never followed by a status query.
	if n := len(f.records()); n != 2 {
		t.Errorf("%d processes started", n)
	}
}

func TestSessionKeyForms(t *testing.T) {
	urlSafe := strings.NewReplacer("+", "-", "/", "_").Replace(
		"//++" + testKey[4:])
	for out, ok := range map[string]bool{
		testKey:                            true,
		testKey + "\n":                     true,
		testKey + "\r\n":                   true,
		testKey[:87]:                       false,
		testKey + "A":                      false,
		testKey + "\n\n":                   false,
		testKey + "\r\n\r\n":               false,
		testKey + "\r":                     false,
		testKey[:40] + "\n" + testKey[41:]: false,
		urlSafe:                            false,
		strings.Repeat("A", 88):            false, // no padding: 66 bytes
		testKey[:85] + "B==":               false, // padding bits not zero
		"":                                 false,
	} {
		if _, got := sessionKey([]byte(out)); got != ok {
			t.Errorf("%q: accepted %v, want %v", out, got, ok)
		}
	}
	if !strings.HasSuffix(testKey, "==") || len(testKey) != sessionKeyLength {
		t.Fatal("the synthetic key does not have the form of a session key")
	}
}

func TestLoginOutputBeyondTheLimit(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Stdout: strings.Repeat("A", 1024), StdoutRepeat: 64}, "")
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassOutput)
	wantRestored(t, f.term)
}

// Requirement: Interrupted login leaves a usable terminal.

func TestLoginEndsBySignal(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Prompt: testPrompt, ReadLine: true, Interrupt: true, Stdout: testKey}, testPassword+"\n")
	f.term.onRestore = func() {
		recs := f.loginRecords()
		if len(recs) != 1 {
			t.Error("terminal restored before bw started")
			return
		}
		assertStopped(t, recs[0].PID)
	}
	err := f.client.Login(bg)
	if err == nil || f.client.LoggedIn() {
		t.Fatal("a login that ended by a signal succeeded")
	}
	wantRestored(t, f.term)
}

func TestTerminalRestoredOnEveryFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		b     bwclitest.Behavior
		class Class
	}{
		"exit code":     {bwclitest.Behavior{Exit: 1, Stderr: "Username or password is incorrect. Try again.\n"}, ClassExit},
		"invalid key":   {bwclitest.Behavior{Stdout: "not a key"}, ClassOutput},
		"stray file":    {bwclitest.Behavior{Stdout: testKey, WriteFile: "bw-login.txt"}, ClassStrayFile},
		"no line":       {bwclitest.Behavior{Prompt: testPrompt, ReadLine: true}, ClassExit},
		"output beyond": {bwclitest.Behavior{Stdout: strings.Repeat("A", 5000)}, ClassOutput},
	} {
		t.Run(name, func(t *testing.T) {
			f := newLoginFixture(t, tc.b, "")
			f.stdinW.Close() // standard input at its end
			err := f.client.Login(bg)
			wantLoginFailed(t, f.fixture, err, tc.class)
			wantRestored(t, f.term)
		})
	}
}

func TestLoginCancelled(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Prompt: testPrompt, ReadLine: true, Stdout: testKey}, "")
	ctx, cancel := context.WithCancel(bg)
	go func() {
		for !strings.Contains(f.stderrText(), testPrompt) {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	err := f.client.Login(ctx)
	wantLoginFailed(t, f.fixture, err, ClassCancelled)
	wantRestored(t, f.term)

	// A cancelled context starts no login.
	err = f.client.Login(ctx)
	wantLoginFailed(t, f.fixture, err, ClassCancelled)
	if n := len(f.loginRecords()); n != 0 {
		t.Errorf("%d login records; the cancelled login read no line", n)
	}
}

func TestTerminalStateCannotBeSaved(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Stdout: testKey}, "")
	f.term.saveErr = os.ErrInvalid
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassNoTerminal)
	if n := len(f.loginRecords()); n != 0 {
		t.Errorf("%d login processes started", n)
	}
}

func TestTerminalStateCannotBeRestored(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Stdout: testKey}, "")
	f.term.restErr = os.ErrInvalid
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassNoTerminal)
}

func TestLoginStartFailure(t *testing.T) {
	f := newLoginFixture(t, bwclitest.Behavior{Stdout: testKey}, "")
	f.client.cfg.Executable = filepath.Join(t.TempDir(), "missing-bw")
	err := f.client.Login(bg)
	wantLoginFailed(t, f.fixture, err, ClassStart)
	wantRestored(t, f.term)
}
