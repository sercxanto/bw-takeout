package bwcli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
)

var notLoggedIn = bwclitest.Behavior{Exit: 1, Stderr: "You are not logged in.\n"}

// Requirement: Child environment from an allowlist.

func TestExitCodeOverrideInTheCallingEnvironment(t *testing.T) {
	t.Setenv("BW_CLEANEXIT", "1")
	f := newFixture(t, scenario("sync --force --raw", notLoggedIn))
	_, err := f.client.run(bg, f.client.cmdSync())
	wantClass(t, err, ClassNotLoggedIn)
	if _, ok := f.lastRecord().Lookup("BW_CLEANEXIT"); ok {
		t.Error("BW_CLEANEXIT reached the process")
	}
}

func TestRuntimeOptionsInTheCallingEnvironment(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "--require=/nonexistent/synthetic.js")
	t.Setenv("NODE_TLS_REJECT_UNAUTHORIZED", "0")
	f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: "{}"}))
	if _, err := f.client.run(bg, f.client.cmdStatus()); err != nil {
		t.Fatal(err)
	}
	rec := f.lastRecord()
	for _, name := range []string{"NODE_OPTIONS", "NODE_TLS_REJECT_UNAUTHORIZED"} {
		if _, ok := rec.Lookup(name); ok {
			t.Errorf("%s reached the process", name)
		}
	}
}

func TestOnlyAllowlistedVariablesArrive(t *testing.T) {
	for _, name := range []string{"BW_SESSION", "BW_RAW", "BW_RESPONSE", "BW_QUIET", "BW_CLIENTSECRET", "BITWARDENCLI_DEBUG", "BITWARDENCLI_APPDATA_DIR", "BW_NOINTERACTION", "https_proxy", "NODE_EXTRA_CA_CERTS"} {
		t.Setenv(name, sentinel)
	}
	f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: "{}"}))
	if _, err := f.client.run(bg, f.client.cmdStatus()); err != nil {
		t.Fatal(err)
	}
	rec := f.lastRecord()
	for _, e := range rec.Env {
		if strings.Contains(e, sentinel) {
			t.Errorf("%s reached the process", strings.SplitN(e, "=", 2)[0])
		}
	}
	set := []string{"BITWARDENCLI_APPDATA_DIR", "BW_NOINTERACTION"}
	for _, name := range rec.EnvNames() {
		if !slices.Contains(set, name) && !slices.ContainsFunc(inherited, func(a string) bool { return strings.EqualFold(a, name) }) {
			t.Errorf("%s is not on the allowlist", name)
		}
	}
	if v, _ := rec.Lookup("BITWARDENCLI_APPDATA_DIR"); v != f.data {
		t.Errorf("BITWARDENCLI_APPDATA_DIR is %q", v)
	}
	if v, _ := rec.Lookup("BW_NOINTERACTION"); v != "true" {
		t.Errorf("BW_NOINTERACTION is %q", v)
	}
}

func TestCertificateAuthorityFileConfigured(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "synthetic-ca.pem")
	f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: "{}"}), func(c *Config) { c.CAFile = ca })
	if _, err := f.client.run(bg, f.client.cmdStatus()); err != nil {
		t.Fatal(err)
	}
	if v, ok := f.lastRecord().Lookup("NODE_EXTRA_CA_CERTS"); !ok || v != ca {
		t.Errorf("NODE_EXTRA_CA_CERTS is %q, %v", v, ok)
	}
}

func TestCertificateAuthorityFileOnlyInTheCallingEnvironment(t *testing.T) {
	t.Setenv("NODE_EXTRA_CA_CERTS", filepath.Join(t.TempDir(), "synthetic-ca.pem"))
	f := newFixture(t, scenario("status", bwclitest.Behavior{Stdout: "{}"}))
	if _, err := f.client.run(bg, f.client.cmdStatus()); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.lastRecord().Lookup("NODE_EXTRA_CA_CERTS"); ok {
		t.Error("NODE_EXTRA_CA_CERTS reached the process")
	}
}

func TestAllowlistNameComparison(t *testing.T) {
	environ := []string{"Path=C:\\bin", "SYSTEMROOT=C:\\Windows", "=C:=C:\\", "path_extra=x", "NODE_OPTIONS=x", "noequals", ""}
	got := allowed(environ, true)
	want := []string{"Path=C:\\bin", "SYSTEMROOT=C:\\Windows"}
	if !slices.Equal(got, want) {
		t.Errorf("with case folding: got %q, want %q", got, want)
	}
	if got := allowed(environ, false); len(got) != 0 {
		t.Errorf("without case folding: got %q", got)
	}
	if got := allowed([]string{"PATH=/bin", "HOME=/h", "TERM=xterm"}, false); len(got) != 3 {
		t.Errorf("exact names: got %q", got)
	}
}

func TestNewRefusesRelativePaths(t *testing.T) {
	abs := t.TempDir()
	for name, cfg := range map[string]Config{
		"executable":        {Executable: "bw", DataDir: abs, WorkDir: filepath.Join(abs, "w")},
		"data directory":    {Executable: fakePath, DataDir: "data", WorkDir: abs},
		"working directory": {Executable: fakePath, DataDir: abs, WorkDir: "work"},
		"same directories":  {Executable: fakePath, DataDir: abs, WorkDir: abs},
		"ca file":           {Executable: fakePath, DataDir: abs, WorkDir: filepath.Join(abs, "w"), CAFile: "ca.pem"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Requirement: No interaction outside the login.

func TestEmptyStandardInputAndNoInteraction(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "{}"}))
	for _, cmd := range []command{f.client.cmdVersion(), f.client.cmdStatus(), f.client.cmdSync(), f.client.cmdExportJSON(), f.client.cmdLogout()} {
		if _, err := f.client.run(bg, cmd); err != nil {
			t.Fatalf("%s: %v", cmd.op, err)
		}
		rec := f.lastRecord()
		if rec.StdinEmpty == nil || !*rec.StdinEmpty {
			t.Errorf("%s: standard input not empty", cmd.op)
		}
		if v, _ := rec.Lookup("BW_NOINTERACTION"); v != "true" {
			t.Errorf("%s: BW_NOINTERACTION is %q", cmd.op, v)
		}
	}
}

func TestLockedVault(t *testing.T) {
	f := newFixture(t, scenario("list items --raw", bwclitest.Behavior{Exit: 1, Stderr: "Vault is locked.\n"}))
	_, err := f.client.run(bg, f.client.cmdListItems())
	wantClass(t, err, ClassLocked)
	rec := f.lastRecord()
	if rec.StdinEmpty == nil || !*rec.StdinEmpty {
		t.Error("standard input not empty")
	}
}

func TestCommandDoesNotEnd(t *testing.T) {
	f := newFixture(t, scenario("sync --force --raw", bwclitest.Behavior{Hang: true, SpawnChild: spawnChild}),
		func(c *Config) { c.Timeouts.Long = 300 * time.Millisecond })
	start := time.Now()
	_, err := f.client.run(bg, f.client.cmdSync())
	wantClass(t, err, ClassTimeout)
	if d := time.Since(start); d > waitDelay+5*time.Second {
		t.Errorf("took %v", d)
	}
	rec := f.lastRecord()
	assertStopped(t, rec.PID, rec.ChildPID)
}

func TestRunIsCancelled(t *testing.T) {
	f := newFixture(t, scenario("list items --raw", bwclitest.Behavior{Hang: true, SpawnChild: spawnChild}))
	ctx, cancel := context.WithCancel(bg)
	go func() {
		for len(f.records()) == 0 {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	_, err := f.client.run(ctx, f.client.cmdListItems())
	wantClass(t, err, ClassCancelled)
	rec := f.lastRecord()
	assertStopped(t, rec.PID, rec.ChildPID)

	// A cancelled context starts no process.
	before := len(f.records())
	_, err = f.client.run(ctx, f.client.cmdStatus())
	wantClass(t, err, ClassCancelled)
	if len(f.records()) != before {
		t.Error("a process was started after the cancel")
	}
}

func TestOutputExceedsTheLimit(t *testing.T) {
	f := newFixture(t, scenario("export --format json --raw", bwclitest.Behavior{Stdout: strings.Repeat("x", 1024), StdoutRepeat: 4096}),
		func(c *Config) { c.OutputLimit = 64 * 1024 })
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	wantClass(t, err, ClassOutput)
	if out.stdout != nil {
		t.Error("output returned with the failure")
	}
	assertStopped(t, f.lastRecord().PID)
}

func TestOutputAtTheLimit(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: strings.Repeat("x", 1024), StdoutRepeat: 64}),
		func(c *Config) { c.OutputLimit = 64 * 1024 })
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	if err != nil || len(out.stdout) != 64*1024 {
		t.Fatalf("%v, %d bytes", err, len(out.stdout))
	}
}

func TestStartFailure(t *testing.T) {
	f := newFixture(t, scenario())
	f.client.cfg.Executable = filepath.Join(t.TempDir(), "missing-bw")
	_, err := f.client.run(bg, f.client.cmdVersion())
	wantClass(t, err, ClassStart)
}

// Requirement: Failures carry a class and no CLI text.

func TestNotLoggedIn(t *testing.T) {
	f := newFixture(t, scenario("*", notLoggedIn))
	for _, cmd := range []command{f.client.cmdSync(), f.client.cmdListItems(), f.client.cmdExportJSON(), f.client.cmdExportEncrypted()} {
		_, err := f.client.run(bg, cmd)
		wantClass(t, err, ClassNotLoggedIn)
	}
}

func TestPremiumRequired(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "Premium status is required to use this feature.\n"}))
	_, err := f.client.run(bg, f.client.cmdAttachment(testItem, testAttachment))
	wantClass(t, err, ClassPremiumRequired)
}

func TestUnrecognizedErrorText(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "Something went wrong.\n"}))
	_, err := f.client.run(bg, f.client.cmdSync())
	e := wantClass(t, err, ClassExit)
	if e.ExitCode != 1 {
		t.Errorf("exit code %d", e.ExitCode)
	}
}

func TestRecognizedSentenceInColour(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "\x1b[91mYou are not logged in.\x1b[39m\r\n"}))
	_, err := f.client.run(bg, f.client.cmdSync())
	wantClass(t, err, ClassNotLoggedIn)
}

func TestSentenceInsideOtherText(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "Note: You are not logged in. Retry\n"}))
	_, err := f.client.run(bg, f.client.cmdSync())
	wantClass(t, err, ClassExit)
}

func TestSentenceWithExitCodeZero(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stderr: "You are not logged in.\n"}))
	if _, err := f.client.run(bg, f.client.cmdSync()); err != nil {
		t.Errorf("a sentence turned exit code 0 into %v", err)
	}
}

func TestFailureText(t *testing.T) {
	for _, tc := range []struct {
		err  *Error
		want string
	}{
		{&Error{Op: OpSync, Class: ClassExit, ExitCode: 1}, "bw sync failed: exit, exit code 1"},
		{&Error{Op: OpLogin, Class: ClassNoTerminal, ExitCode: NoExitCode}, "bw login failed: no_terminal"},
		{&Error{Op: OpAttachment, Class: ClassPremiumRequired, ExitCode: 1, ItemID: testItem, AttachmentID: testAttachment},
			"bw attachment failed: premium_required, exit code 1, item " + testItem + ", attachment " + testAttachment},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
		if ClassOf(tc.err) != tc.err.Class {
			t.Error("ClassOf does not return the class")
		}
	}
}

func TestFailedDownloadWithANameInTheErrorText(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stdout: sentinel + "\n", Stderr: "Not found: " + sentinel + "\n"}))
	out, err := f.client.run(bg, f.client.cmdAttachment(testItem, testAttachment))
	e := wantClass(t, err, ClassExit)
	if e.ItemID != testItem || e.AttachmentID != testAttachment {
		t.Errorf("IDs %q, %q", e.ItemID, e.AttachmentID)
	}
	if !strings.Contains(err.Error(), testItem) || !strings.Contains(err.Error(), testAttachment) {
		t.Errorf("failure text %q lacks the IDs", err.Error())
	}
	assertNoText(t, err, sentinel)
	if bytes.Contains(out.stdout, []byte(sentinel)) {
		t.Error("output returned with the failure")
	}
}

// Requirement: Decryption error lines are counted.

const decryptionLines = "\x1b[31mERROR\x1b[0m bitwarden_crypto::store::context: error=The decryption operation failed\n" +
	"ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed\n" +
	"ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed\n"

func TestExportWithDecryptionErrors(t *testing.T) {
	f := newFixture(t, scenario("export --format json --raw", bwclitest.Behavior{Stdout: "{}", Stderr: decryptionLines}))
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	if err != nil {
		t.Fatal(err)
	}
	if out.decryptionErrors != 3 {
		t.Errorf("count %d, want 3", out.decryptionErrors)
	}
}

func TestExportWithoutErrorLines(t *testing.T) {
	f := newFixture(t, scenario("export --format json --raw", bwclitest.Behavior{Stdout: "{}"}))
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	if err != nil {
		t.Fatal(err)
	}
	if out.decryptionErrors != 0 {
		t.Errorf("count %d, want 0", out.decryptionErrors)
	}
}

func TestLargeStandardError(t *testing.T) {
	line := "ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed " + strings.Repeat("y", 900) + "\n"
	const lines = 8000 // about 8 MB
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "{}", Stderr: line, StderrRepeat: lines}),
		func(c *Config) { c.Timeouts.Long = 30 * time.Second })
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	if err != nil {
		t.Fatal(err)
	}
	if out.decryptionErrors != lines {
		t.Errorf("count %d, want %d", out.decryptionErrors, lines)
	}
}

func TestStderrScannerHoldsOneBoundedLine(t *testing.T) {
	s := &stderrScanner{}
	chunk := bytes.Repeat([]byte("z"), 64*1024)
	for i := 0; i < 128; i++ { // 8 MiB without a line ending
		if n, err := s.Write(chunk); n != len(chunk) || err != nil {
			t.Fatal(n, err)
		}
		if cap(s.line) > 2*maxStderrLine {
			t.Fatalf("holds %d bytes", cap(s.line))
		}
	}
	s.Write([]byte("\nERROR bitwarden_crypto::x\nVault is locked.\nYou are not logged in.\n"))
	s.flush()
	if s.decryptionErrors != 1 || s.recognized != ClassLocked {
		t.Errorf("count %d, class %s", s.decryptionErrors, s.recognized)
	}
	if len(s.line) != 0 {
		t.Error("text kept after the last line")
	}
}

func TestStderrScannerLastLineWithoutEnding(t *testing.T) {
	s := &stderrScanner{}
	s.Write([]byte("Vault is "))
	s.Write([]byte("locked."))
	s.flush()
	if s.recognized != ClassLocked {
		t.Errorf("class %q", s.recognized)
	}
}

// Requirement: Output stays in memory.

func TestExport(t *testing.T) {
	doc := `{"encrypted":false,"items":[]}`
	f := newFixture(t, scenario("export --format json --raw", bwclitest.Behavior{Stdout: doc}))
	out, err := f.client.run(bg, f.client.cmdExportJSON())
	if err != nil {
		t.Fatal(err)
	}
	if string(out.stdout) != doc {
		t.Errorf("got %q", out.stdout)
	}
	entries, err := os.ReadDir(f.work)
	if err != nil || len(entries) != 0 {
		t.Errorf("working directory: %v, %d entries", err, len(entries))
	}
}

func TestBwWritesAFile(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{WriteFile: "bitwarden_export.json"}))
	_, err := f.client.run(bg, f.client.cmdExportJSON())
	wantClass(t, err, ClassStrayFile)
	b, err := os.ReadFile(filepath.Join(f.work, "bitwarden_export.json"))
	if err != nil || string(b) != "{}" {
		t.Errorf("stray file changed: %v", err)
	}
	// The file stays, so the next command fails as well.
	_, err = f.client.run(bg, f.client.cmdStatus())
	wantClass(t, err, ClassStrayFile)
}
