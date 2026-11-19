package run_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/crashdump"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/profile"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/run"
)

// The tests run the real bwcli.Client against the fake bw of package
// bwclitest (design D11). The fake answers each command with one fixed
// behavior.

// fakePath is the fake bw, built once per package run.
var fakePath string

func TestMain(m *testing.M) {
	// A re-executed child of crashdump_test.go uses the fake of its
	// parent.
	if p := os.Getenv(helperFakeEnv); p != "" {
		fakePath = p
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "run-fake-")
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

const (
	// fakeKey is a synthetic session key of 88 base64 characters.
	fakeKey = "c3ludGhldGljLXNlc3Npb24ta2V5LXN5bnRoZXRpYy1zZXNzaW9uLWtleS1zeW50aGV0aWMtc2Vzc2lvbi0wMQ=="

	fakeServer  = "https://vault.example.test"
	fakeEmail   = "backup-user@example.test"
	fakeUser    = "a0000000-0000-4000-8000-000000000001"
	fakeVersion = "2026.10.1"

	// The fake writes this last sync time. The clock of the run reads
	// fakeSyncStart, 40 milliseconds earlier.
	fakeLastSync = "2026-11-17T10:00:00.040Z"
	// staleLastSync lies before fakeSyncStart.
	staleLastSync = "2026-11-17T09:59:59.000Z"

	premiumSentence = "Premium status is required to use this feature.\n"

	cmdVersion = "--version"
	cmdServer  = "config server " + fakeServer
	cmdLogin   = "login --raw"
	cmdSync    = "sync --force --raw"
	cmdStatus  = "status"
	cmdLogout  = "logout"
)

var fakeSyncStart = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// stderrSentinel is a sentinel value every fake command except the login
// writes to standard error. It contains the fixture marker.
var stderrSentinel = ct.Marker + "-stderr-of-bw"

func statusBehavior(lastSync, server, email string) bwclitest.Behavior {
	return bwclitest.Behavior{Stdout: `{"serverUrl":` + jsonString(server) + `,"lastSync":"` + lastSync +
		`","userEmail":` + jsonString(email) + `,"userId":"` + fakeUser + `","status":"unlocked"}`,
		Stderr: stderrSentinel}
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func attachmentCommand(itemID, attachmentID string) string {
	return "get attachment " + attachmentID + " --itemid " + itemID + " --raw"
}

// scenarioOf answers every command of a run with the documents and the
// attachments of the fixture.
func scenarioOf(t *testing.T, f *ct.Fixture) bwclitest.Scenario {
	t.Helper()
	in := f.Input(t)
	s := bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		cmdVersion:                             {Stdout: fakeVersion + "\n", Stderr: stderrSentinel},
		"config":                               {Stdout: "Saved setting `config`.\n", Stderr: stderrSentinel},
		"login":                                {Stdout: fakeKey},
		cmdSync:                                {Stdout: "Syncing complete.\n", Stderr: stderrSentinel},
		cmdStatus:                              statusBehavior(fakeLastSync, fakeServer, fakeEmail),
		"export --format json --raw":           {Stdout: string(in.PlaintextExport), Stderr: stderrSentinel},
		"export --format encrypted_json --raw": {Stdout: string(in.EncryptedExport), Stderr: stderrSentinel},
		"list items --raw":                     {Stdout: string(in.ItemList), Stderr: stderrSentinel},
		"list items --archived --raw":          {Stdout: string(in.ArchivedItemList), Stderr: stderrSentinel},
		cmdLogout:                              {Stdout: "You have logged out.\n", Stderr: stderrSentinel},
	}}
	for _, a := range f.Attachments {
		s.Commands[attachmentCommand(a.ItemID, a.AttachmentID)] = bwclitest.Behavior{Stdout: string(a.Content), Stderr: stderrSentinel}
	}
	return s
}

type fakeTerminal struct{}

func (fakeTerminal) IsTerminal() bool    { return true }
func (fakeTerminal) SaveState() error    { return nil }
func (fakeTerminal) RestoreState() error { return nil }

// call is one call of the recording progress.
type call struct {
	kind      string
	setup     run.Setup
	leftovers run.Leftovers
	source    manifest.Source
	planned   int
	position  int
	total     int
	download  capture.Download
	interrupt run.Interrupt
}

// recorder is a Progress that records its calls.
type recorder struct {
	mu    sync.Mutex
	calls []call
	// on is called after each recorded call, outside the lock.
	on func(call)
}

func (r *recorder) add(c call) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	on := r.on
	r.mu.Unlock()
	if on != nil {
		on(c)
	}
}

// waitFor waits until n calls of kind were recorded.
func (r *recorder) waitFor(t *testing.T, kind string, n int) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(r.all(kind)) >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("fewer than %d %s calls", n, kind)
}

func (r *recorder) Setup(s run.Setup)         { r.add(call{kind: "setup", setup: s}) }
func (r *recorder) Leftovers(l run.Leftovers) { r.add(call{kind: "leftovers", leftovers: l}) }
func (r *recorder) Snapshot(s manifest.Source, planned int) {
	r.add(call{kind: "snapshot", source: s, planned: planned})
}
func (r *recorder) Attachment(position, total int, d capture.Download) {
	r.add(call{kind: "attachment", position: position, total: total, download: d})
}
func (r *recorder) Interrupted(i run.Interrupt) { r.add(call{kind: "interrupted", interrupt: i}) }

func (r *recorder) all(kind string) []call {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []call
	for _, c := range r.calls {
		if c.kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// env is the environment of one run against the fake: a profile location,
// a target, a recipients file with the test identity, and the
// dependencies of the run.
type env struct {
	t          *testing.T
	root       string
	location   string
	target     string
	recipients string
	id         *age.X25519Identity
	scenario   bwclitest.Scenario
	progress   *recorder
	signals    chan os.Signal
	cfg        run.Config
	deps       run.Deps

	mu       sync.Mutex
	profiles []*profile.Profile
	dataDirs []string
	// onProfile is called with each new profile once the scenario is
	// written.
	onProfile func(p *profile.Profile)
	// limitCalls counts the calls of the recording LimitCrashDumps, and
	// limitErr is what it returns.
	limitCalls int
	limitErr   error
}

func newEnv(t *testing.T, s bwclitest.Scenario) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{
		t: t, root: root,
		location:   filepath.Join(root, "profiles"),
		target:     filepath.Join(root, "target"),
		recipients: filepath.Join(root, "recipients.txt"),
		id:         archivetest.NewKey(t),
		scenario:   s,
		progress:   &recorder{},
		signals:    make(chan os.Signal, 4),
	}
	if err := os.Mkdir(e.target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.recipients, []byte("# test recipient\n"+e.id.Recipient().String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { null.Close() })
	loginErr, err := os.Create(filepath.Join(root, "login-stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loginErr.Close() })
	creator := &profile.Creator{Location: e.location}
	e.cfg = run.Config{Server: fakeServer, Target: e.target, RecipientsFile: e.recipients, BW: fakePath}
	e.deps = run.Deps{
		NewProfile:  e.newProfile(creator),
		Leftovers:   creator.Leftovers,
		Terminal:    fakeTerminal{},
		Stdin:       null,
		Stderr:      loginErr,
		Signals:     e.signals,
		Now:         func() time.Time { return fakeSyncStart },
		Wait:        func(ctx context.Context, d time.Duration) error { return ctx.Err() },
		LogoutLimit: 5 * time.Second,
		Progress:    e.progress,
		// No in-process test lowers the limits of the test binary. The
		// real path runs in a child process (crashdump_test.go).
		LimitCrashDumps: e.recordLimit,
	}
	return e
}

// recordLimit is the recording LimitCrashDumps of the in-process tests. It
// returns the protection the real function gives on this platform, or
// limitErr.
func (e *env) recordLimit() (crashdump.Protection, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.limitCalls++
	if e.limitErr != nil {
		return crashdump.Protection{}, e.limitErr
	}
	return platformProtection(), nil
}

// platformProtection is what crashdump.Limit gives on this platform when it
// succeeds.
func platformProtection() crashdump.Protection {
	return crashdump.Protection{CoreLimit: runtime.GOOS != "windows", NotDumpable: runtime.GOOS == "linux"}
}

// newProfile wraps the creator. The data directory of each new profile
// is moved out of the profile directory, so that the records of the fake
// survive the removal of the profile; teardown still removes the profile
// directory with its working directory.
func (e *env) newProfile(c *profile.Creator) func() (*profile.Profile, error) {
	return func() (*profile.Profile, error) {
		p, err := c.Create()
		if err != nil {
			return nil, err
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		data := filepath.Join(e.root, fmt.Sprintf("data-%d", len(e.dataDirs)))
		if err := os.Mkdir(data, 0o700); err != nil {
			return nil, err
		}
		p.DataDir = data
		if err := bwclitest.WriteScenario(data, e.scenario); err != nil {
			return nil, err
		}
		e.profiles = append(e.profiles, p)
		e.dataDirs = append(e.dataDirs, data)
		if e.onProfile != nil {
			e.onProfile(p)
		}
		return p, nil
	}
}

func (e *env) run() *run.Outcome {
	e.t.Helper()
	o := run.Run(e.cfg, e.deps)
	if o.Failure != nil {
		e.t.Logf("run ended in %s", o.Failure)
	}
	return o
}

// profile returns the only profile of the run.
func (e *env) profile() *profile.Profile {
	e.t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.profiles) != 1 {
		e.t.Fatalf("%d profiles created, want 1", len(e.profiles))
	}
	return e.profiles[0]
}

// commands returns the argument lists of the fake starts, joined by
// spaces. No data directory means no start.
func (e *env) commands() []string {
	e.t.Helper()
	e.mu.Lock()
	dirs := append([]string(nil), e.dataDirs...)
	e.mu.Unlock()
	var out []string
	for _, d := range dirs {
		recs, err := bwclitest.ReadRecords(d)
		if err != nil {
			e.t.Fatal(err)
		}
		for _, r := range recs {
			out = append(out, strings.Join(r.Args, " "))
		}
	}
	return out
}

// waitForCommand waits until the fake has recorded the start of cmd.
func (e *env) waitForCommand(cmd string) {
	e.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		dirs := append([]string(nil), e.dataDirs...)
		e.mu.Unlock()
		for _, d := range dirs {
			recs, _ := bwclitest.ReadRecords(d)
			for _, r := range recs {
				if strings.Join(r.Args, " ") == cmd {
					return
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Errorf("the fake never started %q", cmd)
}

func countOf(cmds []string, cmd string) int {
	n := 0
	for _, c := range cmds {
		if c == cmd {
			n++
		}
	}
	return n
}

// wantNoProcessNoProfile checks that no bw was started and no profile was
// created.
func (e *env) wantNoProcessNoProfile() {
	e.t.Helper()
	if cmds := e.commands(); len(cmds) != 0 {
		e.t.Errorf("bw started: %v", cmds)
	}
	e.mu.Lock()
	n := len(e.profiles)
	e.mu.Unlock()
	if n != 0 {
		e.t.Errorf("%d profiles created", n)
	}
	entries, err := os.ReadDir(e.location)
	if err == nil && len(entries) != 0 {
		e.t.Errorf("profile location holds %d entries", len(entries))
	}
}

func (e *env) wantProfileRemoved() {
	e.t.Helper()
	p := e.profile()
	if _, err := os.Lstat(p.Path); !os.IsNotExist(err) {
		e.t.Errorf("profile directory %s still exists (%v)", p.Path, err)
	}
}

// targetEntries lists the names in the target directory.
func (e *env) targetEntries() []string {
	e.t.Helper()
	entries, err := os.ReadDir(e.target)
	if err != nil {
		e.t.Fatal(err)
	}
	var names []string
	for _, x := range entries {
		names = append(names, x.Name())
	}
	return names
}

// open reads the published archive with the test identity.
func (e *env) open(p *publish.Published) (*manifest.Manifest, map[string][]byte) {
	e.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.Path, publish.ArchiveName))
	if err != nil {
		e.t.Fatal(err)
	}
	got := map[string][]byte{}
	m, err := archive.Read(bytes.NewReader(raw), []age.Identity{e.id}, func(path string, r io.Reader) error {
		c, err := io.ReadAll(r)
		got[path] = c
		return err
	})
	if err != nil {
		e.t.Fatalf("read archive: %v", err)
	}
	return m, got
}

// hashTree hashes every file below root, as the tests of cmd/bw-takeout
// do.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// emptyTempDir points the temporary directory of the process at a new
// empty directory and returns it.
func emptyTempDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tmp")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, dir)
	}
	return dir
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestSetupSmoke(t *testing.T) {
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	p, err := e.deps.NewProfile()
	if err != nil {
		t.Fatal(err)
	}
	c, err := bwcli.New(bwcli.Config{Executable: fakePath, DataDir: p.DataDir, WorkDir: p.WorkDir})
	if err != nil {
		t.Fatal(err)
	}
	id, err := c.Version(context.Background())
	if err != nil || id.Version != fakeVersion || id.SHA256 != fileSHA256(t, fakePath) {
		t.Fatalf("version %+v, %v", id, err)
	}
	if cmds := e.commands(); len(cmds) != 1 || cmds[0] != cmdVersion {
		t.Fatalf("commands %v", cmds)
	}
	if _, err := p.Teardown(nil); err != nil {
		t.Fatal(err)
	}
}
