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

// Status documents in the form of the 2026.10.1 bundle (notes 13, probe
// result 5).
var (
	statusUnauthenticated = bwclitest.Behavior{Stdout: `{"serverUrl":null,"lastSync":null,"status":"unauthenticated"}`}
	statusLocked          = bwclitest.Behavior{Stdout: `{"serverUrl":"https://localhost:8087","lastSync":"2026-11-14T09:06:29.835Z","userEmail":"a@example.invalid","userId":"` + testItem + `","status":"locked"}`}
	statusUnlocked        = bwclitest.Behavior{Stdout: `{"serverUrl":"https://localhost:8087","lastSync":"2026-11-14T09:06:29.835Z","userEmail":"a@example.invalid","userId":"` + testItem + `","status":"unlocked"}`}
)

const premiumLine = "Premium status is required to use this feature.\n"

// wantStatusQuery fails the test unless the records are exactly the
// command and one status query after it.
func wantStatusQuery(t *testing.T, recs []bwclitest.Record, cmd command) {
	t.Helper()
	if len(recs) != 2 || !slices.Equal(recs[0].Args, cmd.args) || !slices.Equal(recs[1].Args, []string{"status"}) {
		var args [][]string
		for _, r := range recs {
			args = append(args, r.Args)
		}
		t.Fatalf("%s: started %q, want the command and one status query", cmd.op, args)
	}
}

// Requirement: Child environment from an allowlist.

func TestExitCodeOverrideInTheCallingEnvironment(t *testing.T) {
	t.Setenv("BW_CLEANEXIT", "1")
	f := newFixture(t, scenario("sync --force --raw", notLoggedIn, "status", statusUnauthenticated))
	_, err := f.client.run(bg, f.client.cmdSync())
	wantClass(t, err, ClassNotLoggedIn)
	for _, rec := range f.records() {
		if _, ok := rec.Lookup("BW_CLEANEXIT"); ok {
			t.Errorf("BW_CLEANEXIT reached %q", rec.Args)
		}
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
	f := newFixture(t, scenario("list items --raw", bwclitest.Behavior{Exit: 1, Stderr: "Vault is locked.\n"}, "status", statusLocked),
		func(c *Config) { c.Timeouts.Long = 10 * time.Second })
	cmd := f.client.cmdListItems()
	_, err := f.client.run(bg, cmd)
	e := wantClass(t, err, ClassLocked)
	if e.ExitCode != 1 {
		t.Errorf("exit code %d", e.ExitCode)
	}
	recs := f.records()
	wantStatusQuery(t, recs, cmd)
	for _, rec := range recs {
		if rec.StdinEmpty == nil || !*rec.StdinEmpty {
			t.Errorf("%q: standard input not empty", rec.Args)
		}
		if v, _ := rec.Lookup("BW_NOINTERACTION"); v != "true" {
			t.Errorf("%q: BW_NOINTERACTION is %q", rec.Args, v)
		}
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
	f := newFixture(t, scenario("status", statusUnauthenticated))
	for _, cmd := range []command{f.client.cmdSync(), f.client.cmdExportJSON(), f.client.cmdExportEncrypted(),
		f.client.cmdListItems(), f.client.cmdListArchived(), f.client.cmdAttachment(testItem, testAttachment)} {
		t.Run(string(cmd.op), func(t *testing.T) {
			f := newFixture(t, scenario(strings.Join(cmd.args, " "), notLoggedIn, "status", statusUnauthenticated))
			_, err := f.client.run(bg, cmd)
			e := wantClass(t, err, ClassNotLoggedIn)
			if e.ExitCode != 1 {
				t.Errorf("exit code %d", e.ExitCode)
			}
			wantStatusQuery(t, f.records(), cmd)
		})
	}
}

func TestStatusQueryUsesTheSameSessionKey(t *testing.T) {
	f := newFixture(t, scenario("sync --force --raw", notLoggedIn, "status", statusUnauthenticated))
	f.client.session = []byte(testKey)
	cmd := f.client.cmdSync()
	_, err := f.client.run(bg, cmd)
	wantClass(t, err, ClassNotLoggedIn)
	recs := f.records()
	wantStatusQuery(t, recs, cmd)
	if v, ok := recs[1].Lookup("BW_SESSION"); !ok || v != testKey {
		t.Error("the status query did not receive the session key")
	}
	assertNoText(t, err, testKey)
}

func TestNotLoggedInTextWithAnUnlockedVault(t *testing.T) {
	f := newFixture(t, scenario("sync --force --raw", notLoggedIn, "status", statusUnlocked))
	cmd := f.client.cmdSync()
	_, err := f.client.run(bg, cmd)
	wantClass(t, err, ClassExit)
	wantStatusQuery(t, f.records(), cmd)
}

func TestLockedTextWithAnUnlockedVault(t *testing.T) {
	f := newFixture(t, scenario("list items --raw", bwclitest.Behavior{Exit: 1, Stderr: "Vault is locked.\n"}, "status", statusUnlocked))
	_, err := f.client.run(bg, f.client.cmdListItems())
	wantClass(t, err, ClassExit)
}

func TestNoSentenceWithALockedVault(t *testing.T) {
	f := newFixture(t, scenario("export --format json --raw", bwclitest.Behavior{Exit: 1}, "status", statusLocked))
	_, err := f.client.run(bg, f.client.cmdExportJSON())
	wantClass(t, err, ClassLocked)
}

func TestStatusQueryAfterAFailureFails(t *testing.T) {
	for name, status := range map[string]bwclitest.Behavior{
		"exit code":         {Exit: 1, Stderr: "You are not logged in.\n", Stdout: statusUnauthenticated.Stdout},
		"not a document":    {Stdout: "unauthenticated"},
		"empty output":      {},
		"trailing data":     {Stdout: statusUnauthenticated.Stdout + "{}"},
		"unknown state":     {Stdout: `{"status":"logged_out"}`},
		"no state":          {Stdout: `{"serverUrl":null,"lastSync":null}`},
		"state not a text":  {Stdout: `{"status":0}`},
		"lastSync not time": {Stdout: `{"lastSync":"yesterday","status":"unauthenticated"}`},
		"stray file":        {Stdout: statusUnauthenticated.Stdout, WriteFile: "status.json"},
		"does not end":      {Hang: true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, scenario("sync --force --raw", notLoggedIn, "status", status),
				func(c *Config) { c.Timeouts.Short = 300 * time.Millisecond })
			cmd := f.client.cmdSync()
			_, err := f.client.run(bg, cmd)
			e := wantClass(t, err, ClassExit)
			if e.Op != OpSync || e.ExitCode != 1 {
				t.Errorf("got %v", e)
			}
			wantStatusQuery(t, f.records(), cmd)
		})
	}
}

func TestStatusQueryCancelled(t *testing.T) {
	f := newFixture(t, scenario("sync --force --raw", notLoggedIn, "status", bwclitest.Behavior{Hang: true}))
	ctx, cancel := context.WithCancel(bg)
	go func() {
		for len(f.records()) < 2 {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}()
	_, err := f.client.run(ctx, f.client.cmdSync())
	wantClass(t, err, ClassExit)
}

func TestNoStatusQueryForOtherCommands(t *testing.T) {
	f := newFixture(t, scenario("*", notLoggedIn, "status", notLoggedIn))
	for _, cmd := range []command{f.client.cmdVersion(), f.client.cmdStatus(), f.client.cmdServer("https://localhost:8087"), f.client.cmdLogout()} {
		before := len(f.records())
		_, err := f.client.run(bg, cmd)
		wantClass(t, err, ClassExit)
		if n := len(f.records()) - before; n != 1 {
			t.Errorf("%s: %d processes started", cmd.op, n)
		}
	}
}

func TestPremiumRequired(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: premiumLine}, "status", statusUnlocked))
	_, err := f.client.run(bg, f.client.cmdAttachment(testItem, testAttachment))
	e := wantClass(t, err, ClassPremiumRequired)
	if e.ExitCode != 1 || e.ItemID != testItem || e.AttachmentID != testAttachment {
		t.Errorf("got %v", e)
	}
	// The premium sentence needs no status query.
	if n := len(f.records()); n != 1 {
		t.Errorf("%d processes started", n)
	}
}

func TestPremiumSentenceInColour(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "\x1b[91m  Premium status is required to use this feature.\x1b[39m\r\n"}))
	_, err := f.client.run(bg, f.client.cmdAttachment(testItem, testAttachment))
	wantClass(t, err, ClassPremiumRequired)
}

func TestPremiumSentenceOnAnotherCommand(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: premiumLine}, "status", statusUnlocked))
	for _, cmd := range []command{f.client.cmdSync(), f.client.cmdExportJSON(), f.client.cmdListItems(), f.client.cmdVersion(), f.client.cmdLogout()} {
		_, err := f.client.run(bg, cmd)
		wantClass(t, err, ClassExit)
	}
}

func TestPremiumSentenceInsideOtherText(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "Note: Premium status is required to use this feature. Retry\n"}, "status", statusUnlocked))
	cmd := f.client.cmdAttachment(testItem, testAttachment)
	_, err := f.client.run(bg, cmd)
	wantClass(t, err, ClassExit)
	wantStatusQuery(t, f.records(), cmd)
}

func TestPremiumSentenceWithExitCodeZero(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Stdout: "content", Stderr: premiumLine}))
	out, err := f.client.run(bg, f.client.cmdAttachment(testItem, testAttachment))
	if err != nil || string(out.stdout) != "content" {
		t.Errorf("a sentence turned exit code 0 into %v", err)
	}
}

func TestUnrecognizedErrorText(t *testing.T) {
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stderr: "Something went wrong.\n"}, "status", statusUnlocked))
	cmd := f.client.cmdSync()
	_, err := f.client.run(bg, cmd)
	e := wantClass(t, err, ClassExit)
	if e.ExitCode != 1 {
		t.Errorf("exit code %d", e.ExitCode)
	}
	wantStatusQuery(t, f.records(), cmd)
}

func TestParseStatus(t *testing.T) {
	st, err := parseStatus([]byte(statusUnauthenticated.Stdout))
	if err != nil || st.State != StateUnauthenticated || st.ServerURL != "" || st.UserID != "" || st.LastSync != nil {
		t.Errorf("unauthenticated: %+v, %v", st, err)
	}
	st, err = parseStatus([]byte(statusUnlocked.Stdout + "\n"))
	want := time.Date(2026, 11, 14, 9, 6, 29, 835e6, time.UTC)
	if err != nil || st.State != StateUnlocked || st.ServerURL != "https://localhost:8087" || st.UserID != testItem ||
		st.UserEmail != "a@example.invalid" || st.LastSync == nil || !st.LastSync.Equal(want) {
		t.Errorf("unlocked: %+v, %v", st, err)
	}
	if st, err := parseStatus([]byte(`{"status":"locked","added":1}`)); err != nil || st.State != StateLocked {
		t.Errorf("unknown field: %+v, %v", st, err)
	}
	for _, doc := range []string{"", "null", "[]", `{"status":"Unlocked"}`, `{"status":null}`, `{"status":"locked","userId":5}`,
		`{"status":"locked","lastSync":1}`, `{"status":"locked","lastSync":"2026-11-14"}`, `{"status":"locked"} x`} {
		if _, err := parseStatus([]byte(doc)); err == nil {
			t.Errorf("%q: accepted", doc)
		}
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
	f := newFixture(t, scenario("*", bwclitest.Behavior{Exit: 1, Stdout: sentinel + "\n", Stderr: "Not found: " + sentinel + "\n"}, "status", statusUnlocked))
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
	if s.decryptionErrors != 1 || s.premium {
		t.Errorf("count %d, premium %v", s.decryptionErrors, s.premium)
	}
	s.Write([]byte(premiumLine))
	if !s.premium {
		t.Error("premium sentence not seen")
	}
	if len(s.line) != 0 {
		t.Error("text kept after the last line")
	}
}

func TestStderrScannerLastLineWithoutEnding(t *testing.T) {
	s := &stderrScanner{}
	s.Write([]byte("Premium status is required "))
	s.Write([]byte("to use this feature."))
	s.flush()
	if !s.premium {
		t.Error("premium sentence not seen")
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
