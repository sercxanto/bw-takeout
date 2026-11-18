package collect_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/collect"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/verify"
)

// The status the unit tests accept: unlocked, 40 milliseconds after the
// sync started.
func freshStatus() bwcli.Status {
	last := t0.Truncate(time.Millisecond).Add(40 * time.Millisecond)
	return bwcli.Status{
		ServerURL: "https://vault.example.test",
		UserID:    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		UserEmail: "person@example.test",
		State:     bwcli.StateUnlocked,
		LastSync:  &last,
	}
}

func statusAt(last time.Time) bwcli.Status {
	st := freshStatus()
	st.LastSync = &last
	return st
}

// docsOf returns the four rendered documents of a fixture, in the order of
// design D4.
func docsOf(t *testing.T, f *ct.Fixture) [4][]byte {
	t.Helper()
	in := f.Input(t)
	return [4][]byte{in.PlaintextExport, in.EncryptedExport, in.ItemList, in.ArchivedItemList}
}

// snapshotVault queues a successful sync, the given status and the four
// documents of the fixture.
func snapshotVault(t *testing.T, f *ct.Fixture, st bwcli.Status) *memVault {
	t.Helper()
	docs := docsOf(t, f)
	return newVault(t).
		on(mSync, answer{}).
		on(mStatus, answer{status: st}).
		on(mExport, answer{data: docs[0]}).
		on(mEncrypted, answer{data: docs[1]}).
		on(mList, answer{data: docs[2]}).
		on(mArchived, answer{data: docs[3]})
}

// collectAttachment is what a receiver takes.
type collectAttachment = archive.Attachment

var snapshotCalls = []string{mSync, mStatus, mExport, mEncrypted, mList, mArchived}

func mustSnapshot(t *testing.T, v *memVault) *collect.Snapshot {
	t.Helper()
	c, _ := collector()
	s, err := c.Snapshot(context.Background(), v)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return s
}

func wantOp(t *testing.T, err error, op bwcli.Op, class bwcli.Class) {
	t.Helper()
	var e *bwcli.Error
	if !errors.As(err, &e) || e.Op != op || e.Class != class {
		t.Errorf("error = %v, want bw %s failed: %s", err, op, class)
	}
}

func ref(item, attachment string) capture.AttachmentRef {
	return capture.AttachmentRef{ItemID: item, AttachmentID: attachment}
}

func planRefs(s *collect.Snapshot) []capture.AttachmentRef {
	var out []capture.AttachmentRef
	for _, p := range s.Plan {
		out = append(out, p.Ref())
	}
	return out
}

// Requirement: Collection starts with a forced sync.

func TestSyncFails(t *testing.T) {
	v := newVault(t).on(mSync, answer{err: bwErr(bwcli.OpSync, bwcli.ClassExit)})
	c, _ := collector()
	s, err := c.Snapshot(context.Background(), v)
	if s != nil {
		t.Error("a snapshot was returned")
	}
	wantOp(t, err, bwcli.OpSync, bwcli.ClassExit)
	wantCalls(t, v, mSync)
}

// The clock is read before the sync starts.
func TestSyncStartsAfterTheClockIsRead(t *testing.T) {
	var order []string
	v := snapshotVault(t, ct.Intact(t), freshStatus())
	v.answers[mSync][0].before = func() { order = append(order, "sync") }
	c := &collect.Collector{Now: func() time.Time { order = append(order, "clock"); return t0 }}
	if _, err := c.Snapshot(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "clock" || order[1] != "sync" {
		t.Errorf("order = %v, want the clock before the sync", order)
	}
}

// Requirement: Snapshot must be fresh.

func TestFreshSnapshot(t *testing.T) {
	v := snapshotVault(t, ct.Intact(t), freshStatus())
	mustSnapshot(t, v)
	wantCalls(t, v, snapshotCalls...)
	v.leftover(t)
}

func TestStaleSnapshot(t *testing.T) {
	v := newVault(t).
		on(mSync, answer{}).
		on(mStatus, answer{status: statusAt(t0.Add(-time.Hour))})
	c, _ := collector()
	s, err := c.Snapshot(context.Background(), v)
	if s != nil || !errors.Is(err, collect.ErrNotFresh) {
		t.Errorf("snapshot %v, error %v, want ErrNotFresh", s, err)
	}
	wantCalls(t, v, mSync, mStatus)
}

func TestNoLastSyncTime(t *testing.T) {
	st := freshStatus()
	st.LastSync = nil
	v := newVault(t).on(mSync, answer{}).on(mStatus, answer{status: st})
	c, _ := collector()
	s, err := c.Snapshot(context.Background(), v)
	if s != nil || !errors.Is(err, collect.ErrNotFresh) {
		t.Errorf("snapshot %v, error %v, want ErrNotFresh", s, err)
	}
	wantCalls(t, v, mSync, mStatus)
}

func TestVaultNotUnlocked(t *testing.T) {
	for _, state := range []bwcli.State{bwcli.StateLocked, bwcli.StateUnauthenticated} {
		t.Run(string(state), func(t *testing.T) {
			st := freshStatus()
			st.State = state
			v := newVault(t).on(mSync, answer{}).on(mStatus, answer{status: st})
			c, _ := collector()
			s, err := c.Snapshot(context.Background(), v)
			if s != nil || !errors.Is(err, collect.ErrNotUnlocked) {
				t.Errorf("snapshot %v, error %v, want ErrNotUnlocked", s, err)
			}
			wantCalls(t, v, mSync, mStatus)
		})
	}
}

// The comparison is "at or after" at millisecond precision (design D3). t0
// has a sub-millisecond part, so a last sync time in the same millisecond
// is accepted although it lies before t0 in nanoseconds.
func TestFreshnessAtMillisecondPrecision(t *testing.T) {
	ms := t0.Truncate(time.Millisecond)
	for _, tc := range []struct {
		name  string
		last  time.Time
		fresh bool
	}{
		{"equal to t0", t0, true},
		{"same millisecond as t0", ms, true},
		{"one millisecond before t0", ms.Add(-time.Millisecond), false},
		{"one nanosecond before the millisecond of t0", ms.Add(-time.Nanosecond), false},
		{"40 milliseconds after t0", ms.Add(40 * time.Millisecond), true},
		{"other time zone", t0.In(time.FixedZone("UTC+2", 2*3600)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := snapshotVault(t, ct.Intact(t), statusAt(tc.last))
			c, _ := collector()
			_, err := c.Snapshot(context.Background(), v)
			if tc.fresh && err != nil {
				t.Errorf("error %v, want a fresh snapshot", err)
			}
			if !tc.fresh && !errors.Is(err, collect.ErrNotFresh) {
				t.Errorf("error %v, want ErrNotFresh", err)
			}
		})
	}
}

// A failed status query ends the collection with the status failure.
func TestStatusFails(t *testing.T) {
	v := newVault(t).on(mSync, answer{}).on(mStatus, answer{err: bwErr(bwcli.OpStatus, bwcli.ClassOutput)})
	c, _ := collector()
	_, err := c.Snapshot(context.Background(), v)
	wantOp(t, err, bwcli.OpStatus, bwcli.ClassOutput)
	wantCalls(t, v, mSync, mStatus)
}

// Requirement: Source comes from the status.

func TestSourceOfACapture(t *testing.T) {
	st := freshStatus()
	s := mustSnapshot(t, snapshotVault(t, ct.Intact(t), st))
	want := manifest.Source{ServerURL: st.ServerURL, AccountEmail: st.UserEmail, AccountID: st.UserID}
	if s.Source != want {
		t.Errorf("source = %+v, want %+v", s.Source, want)
	}

	t.Run("values the status does not hold", func(t *testing.T) {
		st := freshStatus()
		st.ServerURL, st.UserEmail, st.UserID = "", "", ""
		s := mustSnapshot(t, snapshotVault(t, ct.Intact(t), st))
		if s.Source != (manifest.Source{}) {
			t.Errorf("source = %+v, want empty values", s.Source)
		}
	})
}

// Requirement: Four documents are collected.

func TestAllDocumentsCollected(t *testing.T) {
	f := ct.Intact(t)
	docs := docsOf(t, f)
	v := snapshotVault(t, f, freshStatus())
	s := mustSnapshot(t, v)
	for i, got := range [][]byte{s.PlaintextExport, s.EncryptedExport, s.ItemList, s.ArchivedItemList} {
		if !bytes.Equal(got, docs[i]) {
			t.Errorf("document %d differs from what bw wrote", i)
		}
	}
	wantCalls(t, v, snapshotCalls...)
	if s.DecryptionErrors != (capture.DecryptionErrors{}) {
		t.Errorf("counts = %+v, want zero", s.DecryptionErrors)
	}

	d := s.Documents()
	if !bytes.Equal(d.PlaintextExport, docs[0]) || !bytes.Equal(d.EncryptedExport, docs[1]) ||
		!bytes.Equal(d.ItemList, docs[2]) || !bytes.Equal(d.ArchivedItemList, docs[3]) {
		t.Error("Documents does not return the four documents")
	}
	in := s.Input(nil)
	if !bytes.Equal(in.PlaintextExport, docs[0]) || !bytes.Equal(in.ArchivedItemList, docs[3]) {
		t.Error("Input does not return the documents")
	}
}

func TestExportFails(t *testing.T) {
	v := snapshotVault(t, ct.Intact(t), freshStatus())
	v.answers[mEncrypted][0] = answer{err: bwErr(bwcli.OpExportEncrypted, bwcli.ClassExit)}
	c, _ := collector()
	s, err := c.Snapshot(context.Background(), v)
	if s != nil {
		t.Error("a snapshot was returned")
	}
	wantOp(t, err, bwcli.OpExportEncrypted, bwcli.ClassExit)
	wantCalls(t, v, mSync, mStatus, mExport, mEncrypted)
}

// A failure of each document command ends the collection, and nothing after
// it is requested.
func TestEachDocumentCommandFails(t *testing.T) {
	for i, tc := range []struct {
		method string
		op     bwcli.Op
	}{
		{mExport, bwcli.OpExportJSON},
		{mEncrypted, bwcli.OpExportEncrypted},
		{mList, bwcli.OpListItems},
		{mArchived, bwcli.OpListArchived},
	} {
		t.Run(tc.method, func(t *testing.T) {
			v := snapshotVault(t, ct.Intact(t), freshStatus())
			v.answers[tc.method][0] = answer{err: bwErr(tc.op, bwcli.ClassExit)}
			c, _ := collector()
			s, err := c.Snapshot(context.Background(), v)
			if s != nil {
				t.Error("a snapshot was returned")
			}
			wantOp(t, err, tc.op, bwcli.ClassExit)
			wantCalls(t, v, snapshotCalls[:3+i]...)
		})
	}
}

// Requirement: Attachments of personal items are downloaded once.

func TestAttachmentShownByBothLists(t *testing.T) {
	f := ct.VariantByName(t, ct.ArchiveFilterIgnored).Build(t)
	s := mustSnapshot(t, snapshotVault(t, f, freshStatus()))
	want := []capture.AttachmentRef{
		ref(ct.ItemPasskeyLogin, ct.AttachmentFirst),
		ref(ct.ItemPasskeyLogin, ct.AttachmentSecond),
	}
	if got := planRefs(s); !equalRefs(got, want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}

	v := newVault(t).on(mAttach, answer{data: []byte("one")}, answer{data: []byte("two")})
	c, _ := collector()
	if _, err := c.Attachments(context.Background(), v, s, func(collectAttachment) error { return nil }); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, v,
		mAttach+" "+ct.ItemPasskeyLogin+"/"+ct.AttachmentFirst,
		mAttach+" "+ct.ItemPasskeyLogin+"/"+ct.AttachmentSecond)
}

func TestOrganizationAttachment(t *testing.T) {
	f := ct.VariantByName(t, ct.OrganizationItems).Build(t)
	s := mustSnapshot(t, snapshotVault(t, f, freshStatus()))
	for _, p := range s.Plan {
		if p.ItemID == ct.ItemOrgFirstA || p.AttachmentID == ct.AttachmentOrg {
			t.Errorf("plan holds the organization attachment: %+v", p)
		}
	}
	if len(s.Plan) != 3 {
		t.Errorf("plan = %v, want the three personal attachments", planRefs(s))
	}
}

func TestUnparsableItemList(t *testing.T) {
	for _, name := range []string{ct.ArchivedListInvalidJSON, ct.ArchivedListAbsent} {
		t.Run(name, func(t *testing.T) {
			f := ct.VariantByName(t, name).Build(t)
			s := mustSnapshot(t, snapshotVault(t, f, freshStatus()))
			if len(s.Plan) != 0 {
				t.Fatalf("plan = %v, want none", planRefs(s))
			}
			v := newVault(t)
			c, _ := collector()
			downloads, err := c.Attachments(context.Background(), v, s, func(collectAttachment) error {
				t.Error("receiver called")
				return nil
			})
			if err != nil || len(downloads) != 0 {
				t.Errorf("downloads %v, error %v", downloads, err)
			}
			wantCalls(t, v)

			// The capture still goes to the verification, which fails it.
			r := verify.Verify(capture.Parse(s.Input(downloads)), nil)
			if r.Verdict != verify.Failed {
				t.Errorf("verdict %s, want failed", r.Verdict)
			}
		})
	}
	t.Run("default list", func(t *testing.T) {
		f := ct.Intact(t)
		v := snapshotVault(t, f, freshStatus())
		v.answers[mList][0].data = []byte(`[{"id":`)
		if s := mustSnapshot(t, v); len(s.Plan) != 0 {
			t.Errorf("plan = %v, want none", planRefs(s))
		}
	})
}

// The plan carries the file names, from the default list first (design
// D5).
func TestPlanFileNames(t *testing.T) {
	s := mustSnapshot(t, snapshotVault(t, ct.Intact(t), freshStatus()))
	want := []collect.PlannedAttachment{
		{ItemID: ct.ItemPasskeyLogin, AttachmentID: ct.AttachmentFirst, FileName: ct.AttachmentFileName},
		{ItemID: ct.ItemPasskeyLogin, AttachmentID: ct.AttachmentSecond, FileName: ct.AttachmentFileName},
		{ItemID: ct.ItemArchivedLogin, AttachmentID: ct.AttachmentArchived, FileName: ct.AttachmentFileName},
	}
	if len(s.Plan) != len(want) {
		t.Fatalf("plan = %+v", s.Plan)
	}
	for i := range want {
		if s.Plan[i] != want[i] {
			t.Errorf("plan[%d] = %+v, want %+v", i, s.Plan[i], want[i])
		}
	}

	t.Run("default list first", func(t *testing.T) {
		f := ct.VariantByName(t, ct.ArchiveFilterIgnored).Build(t)
		for _, raw := range f.ArchivedItemList {
			item := raw.(map[string]any)
			attachments, _ := item["attachments"].([]any)
			for _, a := range attachments {
				a.(map[string]any)["fileName"] = "archived-name.txt"
			}
		}
		s := mustSnapshot(t, snapshotVault(t, f, freshStatus()))
		for _, p := range s.Plan {
			if p.FileName != ct.AttachmentFileName {
				t.Errorf("%+v: file name not taken from the default list", p)
			}
		}
	})
}

// Requirement: Decryption error counts are passed on.

func TestPlaintextExportWithDecryptionErrors(t *testing.T) {
	v := snapshotVault(t, ct.Intact(t), freshStatus())
	v.answers[mExport][0].count = 3
	v.answers[mArchived][0].count = 1
	s := mustSnapshot(t, v)
	want := capture.DecryptionErrors{PlaintextExport: 3, ArchivedItemList: 1}
	if s.DecryptionErrors != want {
		t.Errorf("counts = %+v, want %+v", s.DecryptionErrors, want)
	}
	if got := s.Input(nil).DecryptionErrors; got != want {
		t.Errorf("input counts = %+v, want %+v", got, want)
	}
}

func equalRefs(a, b []capture.AttachmentRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
