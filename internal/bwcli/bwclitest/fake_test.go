package bwclitest_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli/bwclitest"
)

var fakePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakebw-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakePath, err = bwclitest.Build(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	exit           int
	stdout, stderr string
	records        []bwclitest.Record
	data, work     string
}

// runFake starts the fake directly with a minimal environment of its own
// plus extra entries, in a fresh data and working directory.
func runFake(t *testing.T, s bwclitest.Scenario, stdin string, extra []string, args ...string) result {
	t.Helper()
	data, work := t.TempDir(), t.TempDir()
	if err := bwclitest.WriteScenario(data, s); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fakePath, args...)
	cmd.Dir = work
	cmd.Env = append([]string{"BITWARDENCLI_APPDATA_DIR=" + data}, extra...)
	if runtime.GOOS == "windows" {
		cmd.Env = append(cmd.Env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatal(err)
	}
	recs, err := bwclitest.ReadRecords(data)
	if err != nil {
		t.Fatal(err)
	}
	return result{exit: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String(), records: recs, data: data, work: work}
}

func one(cmd string, b bwclitest.Behavior) bwclitest.Scenario {
	return bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{cmd: b}}
}

var noInteraction = []string{"BW_NOINTERACTION=true"}

func TestFakeRecordsEachStart(t *testing.T) {
	r := runFake(t, one("status", bwclitest.Behavior{Stdout: "{}"}), "", append(noInteraction, "SYNTHETIC_MARKER=1"), "status")
	if r.exit != 0 || r.stdout != "{}" {
		t.Fatalf("exit %d, stdout %q", r.exit, r.stdout)
	}
	if len(r.records) != 1 {
		t.Fatalf("%d records", len(r.records))
	}
	rec := r.records[0]
	if len(rec.Args) != 1 || rec.Args[0] != "status" {
		t.Errorf("args %q", rec.Args)
	}
	if v, ok := rec.Lookup("SYNTHETIC_MARKER"); !ok || v != "1" {
		t.Error("environment not recorded")
	}
	if !sameDir(t, rec.Dir, r.work) {
		t.Errorf("working directory %q, want %q", rec.Dir, r.work)
	}
	if rec.StdinEmpty == nil || !*rec.StdinEmpty {
		t.Error("empty standard input not recorded")
	}
	if rec.PID == 0 {
		t.Error("process ID not recorded")
	}
	if _, err := os.Stat(filepath.Join(r.data, bwclitest.DataFile)); err != nil {
		t.Errorf("data file not created: %v", err)
	}
}

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

func TestFakeStdinNotEmpty(t *testing.T) {
	r := runFake(t, one("*", bwclitest.Behavior{}), "x", noInteraction, "status")
	if r.records[0].StdinEmpty == nil || *r.records[0].StdinEmpty {
		t.Error("standard input with content recorded as empty")
	}
}

func TestFakeWithoutDataDirectoryVariable(t *testing.T) {
	cmd := exec.Command(fakePath, "status")
	cmd.Env = []string{"BW_NOINTERACTION=true"}
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 97 {
		t.Fatalf("got %v, want exit code 97", err)
	}
}

func TestFakeLoginOutputs(t *testing.T) {
	key := strings.Repeat("A", 86) + "=="
	for name, stdout := range map[string]string{
		"empty":          "",
		"key":            key,
		"key and ending": key + "\n",
		"extra text":     "You are logged in!\n" + key,
	} {
		t.Run(name, func(t *testing.T) {
			r := runFake(t, one("login --raw", bwclitest.Behavior{Stdout: stdout}), "", nil, "login", "--raw")
			if r.exit != 0 || r.stdout != stdout {
				t.Errorf("exit %d, stdout %q", r.exit, r.stdout)
			}
			if r.records[0].StdinEmpty != nil {
				t.Error("login looked at standard input")
			}
		})
	}
}

func TestFakeLoginPromptAndLine(t *testing.T) {
	b := bwclitest.Behavior{Prompt: "? Master password: ", ReadLine: true, Stdout: "key"}
	r := runFake(t, one("login --raw", b), "synthetic-answer\n", nil, "login", "--raw")
	if r.exit != 0 || r.stdout != "key" {
		t.Fatalf("exit %d, stdout %q", r.exit, r.stdout)
	}
	if r.stderr != b.Prompt {
		t.Errorf("stderr %q", r.stderr)
	}
	if r.records[0].Line != "synthetic-answer" {
		t.Errorf("line %q", r.records[0].Line)
	}
}

func TestFakeCleanExit(t *testing.T) {
	s := one("sync --force --raw", bwclitest.Behavior{Exit: 1, Stderr: "You are not logged in."})
	if r := runFake(t, s, "", noInteraction, "sync", "--force", "--raw"); r.exit != 1 {
		t.Errorf("without BW_CLEANEXIT: exit %d", r.exit)
	}
	if r := runFake(t, s, "", append(noInteraction, "BW_CLEANEXIT=false"), "sync", "--force", "--raw"); r.exit != 0 {
		t.Errorf("with BW_CLEANEXIT=false: exit %d", r.exit)
	}
}

func TestFakeNeedsNoInteraction(t *testing.T) {
	s := one("*", bwclitest.Behavior{Stdout: "{}"})
	for _, extra := range [][]string{nil, {"BW_NOINTERACTION=1"}} {
		r := runFake(t, s, "", extra, "list", "items", "--raw")
		if r.exit != 1 || r.stdout != "" || !strings.Contains(r.stderr, "?") {
			t.Errorf("%v: exit %d, stdout %q", extra, r.exit, r.stdout)
		}
	}
	if r := runFake(t, s, "", nil, "login", "--raw"); r.exit != 0 {
		t.Errorf("login without BW_NOINTERACTION: exit %d", r.exit)
	}
}

func TestFakeFailureSentences(t *testing.T) {
	for _, sentence := range []string{"You are not logged in.", "Vault is locked.", "Premium status is required to use this feature."} {
		r := runFake(t, one("*", bwclitest.Behavior{Exit: 1, Stderr: sentence}), "", noInteraction, "list", "items", "--raw")
		if r.exit != 1 || r.stderr != sentence {
			t.Errorf("%q: exit %d, stderr %q", sentence, r.exit, r.stderr)
		}
	}
}

func TestFakeDecryptionErrorLines(t *testing.T) {
	lines := "ERROR bitwarden_crypto::store::context: error=The decryption operation failed\n" +
		"ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed\n" +
		"ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed\n"
	r := runFake(t, one("*", bwclitest.Behavior{Stdout: "{}", Stderr: lines}), "", noInteraction, "export", "--format", "json", "--raw")
	if r.exit != 0 || r.stdout != "{}" || strings.Count(r.stderr, "ERROR bitwarden_crypto::") != 3 {
		t.Errorf("exit %d, stdout %q, stderr %q", r.exit, r.stdout, r.stderr)
	}
}

func TestFakeStatusDocuments(t *testing.T) {
	for _, doc := range []string{
		`{"serverUrl":null,"lastSync":null,"status":"unauthenticated"}`,
		`{"serverUrl":"https://localhost:8087","lastSync":"2026-11-14T09:06:29.835Z","userEmail":"a@example.invalid","userId":"11111111-2222-4333-8444-555555555555","status":"unlocked"}`,
		`{"serverUrl":null,"lastSync":null,"status":"something-else"}`,
	} {
		r := runFake(t, one("status", bwclitest.Behavior{Stdout: doc}), "", noInteraction, "status")
		if r.exit != 0 || r.stdout != doc {
			t.Errorf("exit %d, stdout %q", r.exit, r.stdout)
		}
	}
}

func TestFakeSentinel(t *testing.T) {
	const sentinel = "SENTINEL-name-of-a-file"
	r := runFake(t, one("*", bwclitest.Behavior{Exit: 1, Stdout: sentinel, Stderr: sentinel}), "", noInteraction, "get", "attachment")
	if r.exit != 1 || r.stdout != sentinel || r.stderr != sentinel {
		t.Errorf("exit %d, stdout %q, stderr %q", r.exit, r.stdout, r.stderr)
	}
}

func TestFakeHang(t *testing.T) {
	data, work := t.TempDir(), t.TempDir()
	if err := bwclitest.WriteScenario(data, one("*", bwclitest.Behavior{Hang: true})); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fakePath, "sync", "--force", "--raw")
	cmd.Dir = work
	cmd.Env = []string{"BITWARDENCLI_APPDATA_DIR=" + data, "BW_NOINTERACTION=true", "SystemRoot=" + os.Getenv("SystemRoot")}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("fake ended: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestFakeLargeOutput(t *testing.T) {
	r := runFake(t, one("*", bwclitest.Behavior{Stdout: "0123456789abcdef", StdoutRepeat: 65536, Stderr: "x\n", StderrRepeat: 100000}), "", noInteraction, "export", "--format", "json", "--raw")
	if r.exit != 0 || len(r.stdout) != 16*65536 || len(r.stderr) != 200000 {
		t.Errorf("exit %d, %d bytes stdout, %d bytes stderr", r.exit, len(r.stdout), len(r.stderr))
	}
}

func TestFakeWritesFileIntoWorkingDirectory(t *testing.T) {
	r := runFake(t, one("*", bwclitest.Behavior{WriteFile: "bitwarden_export.json"}), "", noInteraction, "export", "--format", "json")
	if _, err := os.Stat(filepath.Join(r.work, "bitwarden_export.json")); err != nil {
		t.Errorf("file not written: %v", err)
	}
}

func TestFakeLeavesDataDirectoryWithoutDataFile(t *testing.T) {
	r := runFake(t, one("*", bwclitest.Behavior{SkipData: true, Stdout: "2026.10.1\n"}), "", noInteraction, "--version")
	if _, err := os.Stat(filepath.Join(r.data, bwclitest.DataFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("data file: %v", err)
	}
}

func TestFakeDataFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		return // no POSIX permissions; the file is checked for existence above
	}
	r := runFake(t, one("*", bwclitest.Behavior{}), "", noInteraction, "--version")
	fi, err := os.Stat(filepath.Join(r.data, bwclitest.DataFile))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}

func TestFakeInterrupt(t *testing.T) {
	r := runFake(t, one("login --raw", bwclitest.Behavior{Prompt: "? Master password: ", ReadLine: true, Interrupt: true, Stdout: "key"}), "x\n", nil, "login", "--raw")
	if r.stdout != "" {
		t.Errorf("stdout %q", r.stdout)
	}
	if !endedByInterrupt(t, r.exit) {
		t.Errorf("exit code %d", r.exit)
	}
}

func endedByInterrupt(t *testing.T, code int) bool {
	t.Helper()
	if runtime.GOOS == "windows" {
		return uint32(code) == 0xC000013A
	}
	// ExitCode reports -1 for a process ended by a signal.
	return code == -1
}

func TestFakeSpawnsChild(t *testing.T) {
	r := runFake(t, one("*", bwclitest.Behavior{SpawnChild: true}), "", noInteraction, "status")
	pid := r.records[0].ChildPID
	if pid == 0 {
		t.Fatal("child process ID not recorded")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Kill(); err != nil {
		t.Errorf("child not running: %v", err)
	}
}

func TestFakeUnknownCommand(t *testing.T) {
	r := runFake(t, one("status", bwclitest.Behavior{}), "", noInteraction, "list", "items")
	if r.exit != 96 {
		t.Errorf("exit %d", r.exit)
	}
}
