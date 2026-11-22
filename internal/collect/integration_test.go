package collect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/archive/archivetest"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/bwcli/bwclitest"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/collect"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/verify"
)

// The integration tests start the real bwcli.Client against the fake bw of
// package bwclitest (design D11). The fake answers each command with one
// fixed behavior, so a retry with a later success is tested at unit level
// only.

// fakePath is the fake bw, built once per package run.
var fakePath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "collect-fake-")
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

	fakeServer = "https://vault.example.test"
	fakeEmail  = "backup-user@example.test"
	fakeUser   = "a0000000-0000-4000-8000-000000000001"

	// The fake writes this last sync time. The collector's clock reads
	// fakeSyncStart, 40 milliseconds earlier.
	fakeLastSync = "2026-11-17T10:00:00.040Z"

	decryptionLine  = "ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed\n"
	premiumSentence = "Premium status is required to use this feature.\n"
)

var fakeSyncStart = time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

// stderrSentinel is a sentinel value the fake writes to standard error. It
// contains the fixture marker, like every sentinel of capturetest.
var stderrSentinel = ct.Marker + "-stderr-of-bw"

type fakeTerminal struct{}

func (fakeTerminal) IsTerminal() bool    { return true }
func (fakeTerminal) SaveState() error    { return nil }
func (fakeTerminal) RestoreState() error { return nil }

func statusBehavior(lastSync string) bwclitest.Behavior {
	return bwclitest.Behavior{Stdout: `{"serverUrl":"` + fakeServer + `","lastSync":"` + lastSync +
		`","userEmail":"` + fakeEmail + `","userId":"` + fakeUser + `","status":"unlocked"}`}
}

func attachmentCommand(itemID, attachmentID string) string {
	return "get attachment " + attachmentID + " --itemid " + itemID + " --raw"
}

// fakeScenario answers every command of a run with the documents and the
// attachments of the fixture.
func fakeScenario(t *testing.T, f *ct.Fixture) bwclitest.Scenario {
	t.Helper()
	in := f.Input(t)
	s := bwclitest.Scenario{Commands: map[string]bwclitest.Behavior{
		"--version":                            {Stdout: "2026.10.1\n"},
		"login":                                {Stdout: fakeKey},
		"sync --force --raw":                   {Stdout: "Syncing complete.\n"},
		"status":                               statusBehavior(fakeLastSync),
		"export --format json --raw":           {Stdout: string(in.PlaintextExport)},
		"export --format encrypted_json --raw": {Stdout: string(in.EncryptedExport)},
		"list items --raw":                     {Stdout: string(in.ItemList)},
		"list items --archived --raw":          {Stdout: string(in.ArchivedItemList)},
	}}
	for _, a := range f.Attachments {
		s.Commands[attachmentCommand(a.ItemID, a.AttachmentID)] = bwclitest.Behavior{Stdout: string(a.Content)}
	}
	for _, o := range in.Organizations {
		s.Commands[bwclitest.OrganizationExport(o.ID)] = bwclitest.Behavior{Stdout: string(o.PlaintextExport)}
		s.Commands[bwclitest.OrganizationEncryptedExport(o.ID)] = bwclitest.Behavior{Stdout: string(o.EncryptedExport)}
	}
	return s
}

// harness is one run against the fake: a logged-in client, a target
// directory and a key pair.
type harness struct {
	t         *testing.T
	client    *bwcli.Client
	data      string
	target    string
	id        *age.X25519Identity
	collector *collect.Collector
	waits     *waits
}

func newHarness(t *testing.T, s bwclitest.Scenario) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{t: t, data: filepath.Join(root, "data"), target: filepath.Join(root, "target"), id: archivetest.NewKey(t)}
	work := filepath.Join(root, "work")
	for _, d := range []string{h.data, work, h.target} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := bwclitest.WriteScenario(h.data, s); err != nil {
		t.Fatal(err)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { null.Close() })
	errFile, err := os.Create(filepath.Join(root, "login-stderr"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { errFile.Close() })
	h.client, err = bwcli.New(bwcli.Config{
		Executable: fakePath, DataDir: h.data, WorkDir: work,
		Stdin: null, Stderr: errFile, Terminal: fakeTerminal{},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := h.client.Version(ctx); err != nil {
		t.Fatalf("version: %v", err)
	}
	if err := h.client.Login(ctx); err != nil {
		t.Fatalf("login: %v", err)
	}
	h.waits = &waits{}
	h.collector = &collect.Collector{Now: func() time.Time { return fakeSyncStart }, Wait: h.waits.wait}
	return h
}

// outcome is what one run produced.
type outcome struct {
	published    *publish.Published
	err          error
	downloads    []capture.Download
	verification []byte
}

// run orders the phases as change C will: the snapshot before the target is
// touched, the attachments inside the write function, then the wipe.
func (h *harness) run() outcome {
	h.t.Helper()
	ctx := context.Background()
	var o outcome
	snap, err := h.collector.Snapshot(ctx, h.client)
	if err != nil {
		o.err = err
		return o
	}
	defer snap.Wipe()
	o.published, o.err = (&publish.Publisher{}).Publish(h.target, func(w io.Writer, created time.Time) ([]byte, error) {
		aw, err := archive.NewWriter(w, []string{h.id.Recipient().String()}, created, snap.Documents())
		if err != nil {
			return nil, err
		}
		o.downloads, err = h.collector.Attachments(ctx, h.client, snap, aw.AddAttachment)
		if err != nil {
			return nil, err
		}
		r := verify.Verify(capture.Parse(snap.Input(o.downloads)), nil)
		o.verification, err = r.JSON()
		if err != nil {
			return nil, err
		}
		return aw.Finish(archive.Meta{
			Source:            snap.Source,
			Tool:              manifest.Tool{Name: "bw-takeout", Version: "test"},
			Verification:      o.verification,
			OrganizationNames: snap.OrganizationNames(),
		})
	})
	return o
}

// targetEntries lists the names in the target directory.
func (h *harness) targetEntries() []string {
	h.t.Helper()
	entries, err := os.ReadDir(h.target)
	if err != nil {
		h.t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func (h *harness) wantNoGeneration() {
	h.t.Helper()
	if names := h.targetEntries(); len(names) != 0 {
		h.t.Errorf("target holds %v, want no generation and no partial directory", names)
	}
}

// open reads the published archive with the test identity.
func (h *harness) open(p *publish.Published) (*manifest.Manifest, map[string][]byte) {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.Path, publish.ArchiveName))
	if err != nil {
		h.t.Fatal(err)
	}
	got := map[string][]byte{}
	m, err := archive.Read(bytes.NewReader(raw), []age.Identity{h.id}, func(path string, r io.Reader) error {
		c, err := io.ReadAll(r)
		got[path] = c
		return err
	})
	if err != nil {
		h.t.Fatalf("read archive: %v", err)
	}
	return m, got
}

func (h *harness) commands() []string {
	h.t.Helper()
	recs, err := bwclitest.ReadRecords(h.data)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, r := range recs {
		out = append(out, strings.Join(r.Args, " "))
	}
	return out
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

func resultOf(t *testing.T, raw []byte) verify.Result {
	t.Helper()
	var r verify.Result
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatalf("verification result: %v", err)
	}
	return r
}

// An intact run from the snapshot to a published generation that the
// archive reader opens with the test identity.
func TestIntegrationIntactRun(t *testing.T) {
	f := ct.Intact(t)
	h := newHarness(t, fakeScenario(t, f))
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if o.published.Verdict != publish.VerdictVerified {
		t.Errorf("verdict %s, want verified", o.published.Verdict)
	}
	if _, marker, ok := publish.ParseName(o.published.Name); !ok || marker != "" {
		t.Errorf("generation name %q, want a final name without marker", o.published.Name)
	}

	m, got := h.open(o.published)
	in := f.Input(t)
	for path, want := range map[string][]byte{
		archive.PlaintextExportPath:  in.PlaintextExport,
		archive.EncryptedExportPath:  in.EncryptedExport,
		archive.ItemListPath:         in.ItemList,
		archive.ArchivedItemListPath: in.ArchivedItemList,
	} {
		if !bytes.Equal(got[path], want) {
			t.Errorf("%s differs from the fixture", path)
		}
	}
	for _, a := range f.Attachments {
		if !bytes.Equal(got[archive.AttachmentPath(a.ItemID, a.AttachmentID)], a.Content) {
			t.Errorf("attachment %s/%s differs from the fixture", a.ItemID, a.AttachmentID)
		}
	}
	want := manifest.Source{ServerURL: fakeServer, AccountEmail: fakeEmail, AccountID: fakeUser}
	if m.Source != want {
		t.Errorf("manifest source = %+v, want %+v", m.Source, want)
	}
	if m.Counts.Attachments != len(f.Attachments) {
		t.Errorf("manifest counts %d attachments, want %d", m.Counts.Attachments, len(f.Attachments))
	}
	for _, e := range m.Entries {
		if e.Kind == manifest.KindAttachment && e.FileName != ct.AttachmentFileName {
			t.Errorf("manifest entry %s has file name %q", e.Path, e.FileName)
		}
	}
	if !bytes.Equal(m.Verification, o.verification) {
		t.Error("embedded verification result differs")
	}

	cmds := h.commands()
	for _, a := range f.Attachments {
		if n := countOf(cmds, attachmentCommand(a.ItemID, a.AttachmentID)); n != 1 {
			t.Errorf("attachment %s requested %d times, want 1", a.AttachmentID, n)
		}
	}
	if n := countOf(cmds, "sync --force --raw"); n != 1 {
		t.Errorf("%d syncs, want 1", n)
	}
}

// Scenario "Stale snapshot" against the fake.
func TestIntegrationStaleLastSync(t *testing.T) {
	s := fakeScenario(t, ct.Intact(t))
	s.Commands["status"] = statusBehavior("2026-11-17T09:59:59.999Z")
	h := newHarness(t, s)
	o := h.run()
	if !errors.Is(o.err, collect.ErrNotFresh) {
		t.Errorf("error %v, want ErrNotFresh", o.err)
	}
	h.wantNoGeneration()
	for _, c := range h.commands() {
		if strings.HasPrefix(c, "export") || strings.HasPrefix(c, "list") || strings.HasPrefix(c, "get") {
			t.Errorf("document or attachment requested: %s", c)
		}
	}
}

// Scenario "Sync fails" against the fake.
func TestIntegrationFailingSync(t *testing.T) {
	s := fakeScenario(t, ct.Intact(t))
	s.Commands["sync --force --raw"] = bwclitest.Behavior{Exit: 1, Stderr: "Sync failed " + stderrSentinel + "\n"}
	h := newHarness(t, s)
	o := h.run()
	wantOp(t, o.err, bwcli.OpSync, bwcli.ClassExit)
	if o.err != nil && strings.Contains(o.err.Error(), ct.Marker) {
		t.Errorf("error %q carries the sentinel", o.err)
	}
	h.wantNoGeneration()
	for _, c := range h.commands() {
		if strings.HasPrefix(c, "export") || strings.HasPrefix(c, "list") {
			t.Errorf("document requested after the failed sync: %s", c)
		}
	}
}

// Scenario "Plaintext export with decryption errors", judged unverified and
// published under the unverified marker.
func TestIntegrationDecryptionErrorsOnThePlaintextExport(t *testing.T) {
	f := ct.Intact(t)
	s := fakeScenario(t, f)
	b := s.Commands["export --format json --raw"]
	b.Stderr, b.StderrRepeat = decryptionLine, 3
	s.Commands["export --format json --raw"] = b
	h := newHarness(t, s)
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if o.published.Verdict != publish.VerdictUnverified {
		t.Errorf("verdict %s, want unverified", o.published.Verdict)
	}
	if _, marker, ok := publish.ParseName(o.published.Name); !ok || marker != publish.UnverifiedMarker {
		t.Errorf("generation name %q, want the unverified marker", o.published.Name)
	}
	r := resultOf(t, o.verification)
	var found bool
	for _, fd := range r.Findings {
		if fd.Reason == verify.DocumentDecryptionErrors && fd.Document == string(capture.PlaintextExportDocument) &&
			fd.Count != nil && *fd.Count == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("no finding for three lines on the plaintext export: %s", o.verification)
	}
	h.open(o.published)
}

// Scenario "Download with decryption errors": a warning, the verdict stays
// verified.
func TestIntegrationDecryptionErrorsOnADownload(t *testing.T) {
	f := ct.Intact(t)
	s := fakeScenario(t, f)
	cmd := attachmentCommand(ct.ItemPasskeyLogin, ct.AttachmentSecond)
	b := s.Commands[cmd]
	b.Stderr, b.StderrRepeat = decryptionLine, 3
	s.Commands[cmd] = b
	h := newHarness(t, s)
	o := h.run()
	if o.err != nil {
		t.Fatalf("run: %v", o.err)
	}
	if o.published.Verdict != publish.VerdictVerified {
		t.Errorf("verdict %s, want verified", o.published.Verdict)
	}
	for _, d := range o.downloads {
		want := 0
		if d.AttachmentID == ct.AttachmentSecond {
			want = 3
		}
		if d.DecryptionErrors != want || d.Error != "" {
			t.Errorf("download %s: %+v, want a success with count %d", d.AttachmentID, d, want)
		}
	}
	r := resultOf(t, o.verification)
	if len(r.Warnings) != 1 || r.Warnings[0].Reason != verify.AttachmentDecryptionErrors ||
		r.Warnings[0].AttachmentID != ct.AttachmentSecond || r.Warnings[0].Count == nil || *r.Warnings[0].Count != 3 {
		t.Errorf("warnings = %+v", r.Warnings)
	}
}

// Scenario "Premium required": the download is requested once, recorded as
// failed, and the failed capture is not published.
func TestIntegrationPremiumRequired(t *testing.T) {
	f := ct.Intact(t)
	s := fakeScenario(t, f)
	cmd := attachmentCommand(ct.ItemPasskeyLogin, ct.AttachmentFirst)
	s.Commands[cmd] = bwclitest.Behavior{Exit: 1, Stderr: premiumSentence}
	h := newHarness(t, s)
	o := h.run()
	if !errors.Is(o.err, publish.ErrFailedCapture) {
		t.Fatalf("error %v, want ErrFailedCapture", o.err)
	}
	h.wantNoGeneration()
	cmds := h.commands()
	if n := countOf(cmds, cmd); n != 1 {
		t.Errorf("premium download requested %d times, want 1", n)
	}
	if len(o.downloads) != len(f.Attachments) {
		t.Fatalf("downloads = %+v", o.downloads)
	}
	for _, d := range o.downloads {
		failed := d.AttachmentID == ct.AttachmentFirst
		if failed && (d.Error != capture.ErrorStatus || d.SHA256 != "") || !failed && d.Error != "" {
			t.Errorf("download %s: %+v", d.AttachmentID, d)
		}
	}
	if len(h.waits.got) != 0 {
		t.Errorf("waits = %v, want none", h.waits.got)
	}
}

// Scenario "Error text with a sentinel": a download fails on every attempt
// and bw writes a sentinel to standard error. Neither the error values nor
// the results carry it.
func TestIntegrationSentinelInErrorText(t *testing.T) {
	f := ct.Intact(t)
	s := fakeScenario(t, f)
	cmd := attachmentCommand(ct.ItemArchivedLogin, ct.AttachmentArchived)
	s.Commands[cmd] = bwclitest.Behavior{Exit: 1, Stdout: stderrSentinel, Stderr: "Error: " + stderrSentinel + "\n"}
	h := newHarness(t, s)
	o := h.run()
	if !errors.Is(o.err, publish.ErrFailedCapture) {
		t.Fatalf("error %v, want ErrFailedCapture", o.err)
	}
	if strings.Contains(o.err.Error(), ct.Marker) {
		t.Errorf("error %q carries the sentinel", o.err)
	}
	if n := countOf(h.commands(), cmd); n != collect.MaxAttempts {
		t.Errorf("download requested %d times, want %d", n, collect.MaxAttempts)
	}
	for _, d := range o.downloads {
		if strings.Contains(fmt.Sprintf("%+v", d), ct.Marker) {
			t.Errorf("download result carries the sentinel: %+v", d)
		}
	}
	if bytes.Contains(o.verification, []byte(ct.Marker)) {
		t.Errorf("verification result carries the sentinel: %s", o.verification)
	}
	h.wantNoGeneration()

	t.Run("failure that ends the collection", func(t *testing.T) {
		s := fakeScenario(t, f)
		s.Commands["export --format encrypted_json --raw"] = bwclitest.Behavior{Exit: 1, Stderr: stderrSentinel + "\n"}
		h := newHarness(t, s)
		o := h.run()
		wantOp(t, o.err, bwcli.OpExportEncrypted, bwcli.ClassExit)
		if o.err != nil && strings.Contains(fmt.Sprintf("%v %+v", o.err, o.err), ct.Marker) {
			t.Errorf("error %v carries the sentinel", o.err)
		}
		h.wantNoGeneration()
	})
}
