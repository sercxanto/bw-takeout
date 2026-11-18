package run_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli/bwclitest"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/collect"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/publish"
	"bw-takeout/internal/run"
	"bw-takeout/internal/verify"
)

// An intact run from the checks to a published generation that the archive
// reader opens with the test identity.
func TestIntactRunPublishes(t *testing.T) {
	f := ct.Intact(t)
	e := newEnv(t, scenarioOf(t, f))
	o := e.run()
	if o.Failure != nil || o.Published == nil || o.Published.Verdict != publish.VerdictVerified {
		t.Fatalf("failure %v, published %v", o.Failure, o.Published)
	}
	if !o.TeardownComplete() || o.Logout.Result != run.LogoutOK || !o.Logout.SessionForgotten {
		t.Errorf("teardown %+v, remove %v", o.Logout, o.RemoveErr)
	}
	e.wantProfileRemoved()

	m, got := e.open(o.Published)
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
	if m.Counts.Attachments != len(f.Attachments) || o.Planned != len(f.Attachments) || len(o.Downloads) != len(f.Attachments) {
		t.Errorf("manifest counts %d attachments, planned %d, downloads %d, want %d", m.Counts.Attachments, o.Planned, len(o.Downloads), len(f.Attachments))
	}
	wantSource := manifest.Source{ServerURL: fakeServer, AccountEmail: fakeEmail, AccountID: fakeUser}
	if m.Source != wantSource || o.Source == nil || *o.Source != wantSource {
		t.Errorf("manifest source %+v, outcome source %v, want %+v", m.Source, o.Source, wantSource)
	}
	if m.BW.Version != fakeVersion || o.Setup.BWVersion != fakeVersion {
		t.Errorf("bw version %q in the manifest, %q in the outcome", m.BW.Version, o.Setup.BWVersion)
	}
	if o.Setup.BWSHA256 != fileSHA256(t, fakePath) || o.Setup.BWPath != fakePath {
		t.Errorf("setup %+v", o.Setup)
	}
	if m.Tool.Name != "bw-takeout" || m.Tool.Version == "" {
		t.Errorf("tool %+v", m.Tool)
	}
	if o.Verification == nil || o.Verification.Verdict != verify.Verified {
		t.Errorf("verification %+v", o.Verification)
	}
	v, err := o.Verification.JSON()
	if err != nil || !bytes.Equal(m.Verification, v) {
		t.Errorf("embedded verification result differs: %v", err)
	}
	if o.Setup.Server != fakeServer || o.Setup.Target != e.target || o.Setup.Recipients != 1 {
		t.Errorf("setup %+v", o.Setup)
	}

	cmds := e.commands()
	order := []string{cmdVersion, cmdServer, cmdLogin, cmdSync, cmdStatus}
	if len(cmds) < len(order) || !slices.Equal(cmds[:len(order)], order) || cmds[len(cmds)-1] != cmdLogout {
		t.Errorf("commands %v", cmds)
	}
	if n := countOf(cmds, cmdLogout); n != 1 {
		t.Errorf("%d logouts", n)
	}
	if setups := e.progress.all("setup"); len(setups) != 1 || setups[0].setup != o.Setup {
		t.Errorf("setup progress %+v", setups)
	}
	if snaps := e.progress.all("snapshot"); len(snaps) != 1 || snaps[0].planned != len(f.Attachments) {
		t.Errorf("snapshot progress %+v", snaps)
	}
	if atts := e.progress.all("attachment"); len(atts) != len(f.Attachments) {
		t.Errorf("attachment progress %+v", atts)
	}
}

func TestUnverifiedRunPublishesWithMarker(t *testing.T) {
	f := ct.VariantByName(t, ct.BlobLoginItem).Build(t)
	e := newEnv(t, scenarioOf(t, f))
	o := e.run()
	if o.Published == nil || o.Published.Verdict != publish.VerdictUnverified {
		t.Fatalf("published %v", o.Published)
	}
	if _, marker, _ := publish.ParseName(o.Published.Name); marker != publish.UnverifiedMarker {
		t.Errorf("name %s", o.Published.Name)
	}
}

// preexisting fills the target with entries a run must not touch: an
// older generation, a partial generation and a file of the user.
func preexisting(t *testing.T, e *env) {
	t.Helper()
	for _, d := range []string{publish.NamePrefix + "20261101T080000Z-0000abcd", publish.NamePrefix + "20261102T080000Z-0000abce" + publish.PartialMarker} {
		dir := filepath.Join(e.target, d)
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, publish.ArchiveName), []byte("older archive"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.target, "notes.txt"), []byte("user file"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestStaleSnapshot(t *testing.T) {
	s := scenarioOf(t, ct.Intact(t))
	s.Commands[cmdStatus] = statusBehavior(staleLastSync, fakeServer, fakeEmail)
	e := newEnv(t, s)
	preexisting(t, e)
	before := hashTree(t, e.target)
	o := e.run()
	if o.Failure == nil || o.Failure.Phase != run.PhaseSnapshot || !errors.Is(o.Failure.Err, collect.ErrNotFresh) {
		t.Fatalf("failure %v", o.Failure)
	}
	if o.Published != nil {
		t.Errorf("published %v", o.Published)
	}
	if !sameTree(before, hashTree(t, e.target)) {
		t.Error("the target changed")
	}
	if o.Logout.Result != run.LogoutOK {
		t.Errorf("logout %+v", o.Logout)
	}
	e.wantProfileRemoved()
}

func TestNoTemporaryFiles(t *testing.T) {
	tmp := emptyTempDir(t)
	f := ct.Intact(t)
	e := newEnv(t, scenarioOf(t, f))
	o := e.run()
	if o.Published == nil || len(o.Downloads) == 0 {
		t.Fatalf("published %v, downloads %d", o.Published, len(o.Downloads))
	}
	if left, err := os.ReadDir(tmp); err != nil || len(left) != 0 {
		t.Errorf("temporary directory holds %d entries (%v)", len(left), err)
	}
}

// captureSnapshot records the four document slices of the snapshot of a
// run.
func captureSnapshot(t *testing.T) *[][]byte {
	t.Helper()
	var docs [][]byte
	t.Cleanup(run.SetSnapshotHook(func(s *collect.Snapshot) {
		docs = [][]byte{s.PlaintextExport, s.EncryptedExport, s.ItemList, s.ArchivedItemList}
	}))
	return &docs
}

func wantWiped(t *testing.T, docs [][]byte) {
	t.Helper()
	if len(docs) != 4 {
		t.Fatalf("%d documents seen, want 4", len(docs))
	}
	for i, d := range docs {
		if len(d) == 0 {
			t.Errorf("document %d is empty", i)
		}
		for _, b := range d {
			if b != 0 {
				t.Errorf("document %d holds a byte that is not zero", i)
				break
			}
		}
	}
}

func TestDocumentsWipedAfterPublishedRun(t *testing.T) {
	docs := captureSnapshot(t)
	e := newEnv(t, scenarioOf(t, ct.Intact(t)))
	if o := e.run(); o.Published == nil {
		t.Fatalf("not published: %v", o.Failure)
	}
	wantWiped(t, *docs)
}

func TestDocumentsWipedAfterWriterFailure(t *testing.T) {
	docs := captureSnapshot(t)
	s := scenarioOf(t, ct.Intact(t))
	// The snapshot accepts any document that is not empty. The archive
	// writer parses the plaintext export to count its items.
	s.Commands["export --format json --raw"] = bwclitest.Behavior{Stdout: "[not json"}
	e := newEnv(t, s)
	o := e.run()
	if o.Failure == nil || o.Failure.Phase != run.PhaseArchive || o.Published != nil {
		t.Fatalf("failure %v, published %v", o.Failure, o.Published)
	}
	if names := e.targetEntries(); len(names) != 0 {
		t.Errorf("target holds %v", names)
	}
	wantWiped(t, *docs)
}
