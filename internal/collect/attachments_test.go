package collect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/collect"
)

// Synthetic IDs of the attachment tests.
const (
	itemA = "aaaaaaaa-0000-4000-8000-00000000000a"
	itemB = "bbbbbbbb-0000-4000-8000-00000000000b"
	att1  = "att1"
	att2  = "att2"
	att3  = "att3"
)

// planned returns a snapshot that plans the given attachments of itemA.
func planned(attachmentIDs ...string) *collect.Snapshot {
	s := &collect.Snapshot{}
	for _, id := range attachmentIDs {
		s.Plan = append(s.Plan, collect.PlannedAttachment{ItemID: itemA, AttachmentID: id, FileName: ct.Marker + "-" + id + ".txt"})
	}
	return s
}

func attachCall(attachmentID string) string { return mAttach + " " + itemA + "/" + attachmentID }

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// received records what a receiver was given and keeps the slices, so that
// a test can check they were cleared.
type received struct {
	got      []collectAttachment
	contents []string
	err      error
	// failOn makes the receiver refuse the attachment with this ID.
	failOn string
}

func (r *received) receive(a collectAttachment) error {
	r.got = append(r.got, a)
	r.contents = append(r.contents, string(a.Content))
	if a.AttachmentID == r.failOn {
		r.err = errors.New("receiver refused the attachment")
		return r.err
	}
	return nil
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func runAttachments(t *testing.T, v *memVault, s *collect.Snapshot) ([]capture.Download, *received, *waits, error) {
	t.Helper()
	c, w := collector()
	r := &received{}
	d, err := c.Attachments(context.Background(), v, s, r.receive)
	return d, r, w, err
}

func ok(data string) answer { return answer{data: []byte(data)} }

func fails(class bwcli.Class) answer {
	return answer{err: &bwcli.Error{Op: bwcli.OpAttachment, Class: class, ExitCode: 1, ItemID: itemA, AttachmentID: att1}}
}

// Wipe clears the four documents (design D10).
func TestWipeClearsTheDocuments(t *testing.T) {
	s := mustSnapshot(t, snapshotVault(t, ct.Intact(t), freshStatus()))
	docs := [][]byte{s.PlaintextExport, s.EncryptedExport, s.ItemList, s.ArchivedItemList}
	shared := s.Documents()
	s.Wipe()
	for i, d := range docs {
		if len(d) == 0 || !allZero(d) {
			t.Errorf("document %d: %d bytes, not all zero", i, len(d))
		}
	}
	if !allZero(shared.PlaintextExport) {
		t.Error("the slice from Documents was not cleared")
	}
}

// A document failure wipes the documents collected before it.
func TestDocumentFailureWipesEarlierDocuments(t *testing.T) {
	var kept []byte
	v := snapshotVault(t, ct.Intact(t), freshStatus())
	v.answers[mList][0] = answer{err: bwErr(bwcli.OpListItems, bwcli.ClassExit)}
	// Capture the slice the collection holds by wrapping the export call.
	wrapped := &capturingVault{memVault: v, keep: &kept}
	c, _ := collector()
	if _, err := c.Snapshot(context.Background(), wrapped); err == nil {
		t.Fatal("snapshot succeeded")
	}
	if len(kept) == 0 || !allZero(kept) {
		t.Errorf("plaintext export after the failure: %d bytes, not all zero", len(kept))
	}
}

// capturingVault keeps the slice of the plaintext export it returns.
type capturingVault struct {
	*memVault
	keep *[]byte
}

func (v *capturingVault) ExportJSON(ctx context.Context) (bwcli.Output, error) {
	out, err := v.memVault.ExportJSON(ctx)
	*v.keep = out.Data
	return out, err
}

// Requirement: Attachments are handed on one at a time.

func TestTwoAttachments(t *testing.T) {
	var order []string
	v := newVault(t).on(mAttach,
		answer{data: []byte("first"), before: func() { order = append(order, "request "+att1) }},
		answer{data: []byte("second"), before: func() { order = append(order, "request "+att2) }},
	)
	s := planned(att1, att2)
	c, _ := collector()
	downloads, err := c.Attachments(context.Background(), v, s, func(a collectAttachment) error {
		order = append(order, "receive "+a.AttachmentID)
		return nil
	})
	if err != nil || len(downloads) != 2 {
		t.Fatalf("downloads %v, error %v", downloads, err)
	}
	want := []string{"request " + att1, "receive " + att1, "request " + att2, "receive " + att2}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

// The receiver gets IDs, file name and content, and the content is cleared
// after the receiver returns (design D10).
func TestReceiverGetsTheAttachmentAndContentIsCleared(t *testing.T) {
	v := newVault(t).on(mAttach, ok("first content"), ok("second content"))
	s := planned(att1, att2)
	_, r, _, err := runAttachments(t, v, s)
	if err != nil {
		t.Fatal(err)
	}
	for i, a := range r.got {
		p := s.Plan[i]
		if a.ItemID != p.ItemID || a.AttachmentID != p.AttachmentID || a.FileName != p.FileName {
			t.Errorf("receiver got %s/%s %q, want %+v", a.ItemID, a.AttachmentID, a.FileName, p)
		}
		if len(a.Content) == 0 || !allZero(a.Content) {
			t.Errorf("content of %s not cleared after the receiver returned", a.AttachmentID)
		}
	}
	if r.contents[0] != "first content" || r.contents[1] != "second content" {
		t.Errorf("contents while receiving = %q", r.contents)
	}
}

func TestReceiverFails(t *testing.T) {
	v := newVault(t).on(mAttach, ok("first"), ok("second"))
	s := planned(att1, att2, att3)
	c, _ := collector()
	r := &received{failOn: att2}
	downloads, err := c.Attachments(context.Background(), v, s, r.receive)
	if !errors.Is(err, r.err) || downloads != nil {
		t.Errorf("downloads %v, error %v, want the receiver failure", downloads, err)
	}
	wantCalls(t, v, attachCall(att1), attachCall(att2))
	if !allZero(r.got[1].Content) {
		t.Error("content refused by the receiver was not cleared")
	}
}

// Requirement: Download results record hash and size.

func TestSuccessfulDownload(t *testing.T) {
	content := "twelve bytes"
	v := newVault(t).on(mAttach, ok(content))
	downloads, _, _, err := runAttachments(t, v, planned(att1))
	if err != nil {
		t.Fatal(err)
	}
	want := capture.Download{ItemID: itemA, AttachmentID: att1, SHA256: hashOf([]byte(content)), Size: 12}
	if len(downloads) != 1 || downloads[0] != want {
		t.Errorf("downloads = %+v, want %+v", downloads, want)
	}
}

// An empty attachment is a successful download of size 0.
func TestEmptyDownload(t *testing.T) {
	v := newVault(t).on(mAttach, ok(""))
	downloads, r, _, err := runAttachments(t, v, planned(att1))
	if err != nil || len(downloads) != 1 || downloads[0].Size != 0 || downloads[0].SHA256 != hashOf(nil) {
		t.Errorf("downloads %+v, error %v", downloads, err)
	}
	if len(r.got) != 1 {
		t.Errorf("receiver called %d times, want 1", len(r.got))
	}
}

// At unit level the sentinel stands in the file name, which is vault
// content, and in the text of a failure that is not a *bwcli.Error. The
// fake bw writes it to standard error in the integration test.
func TestErrorTextWithASentinel(t *testing.T) {
	t.Run("failed download", func(t *testing.T) {
		v := newVault(t).on(mAttach, fails(bwcli.ClassPremiumRequired), ok("second"))
		downloads, _, _, err := runAttachments(t, v, planned(att1, att2))
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range downloads {
			if strings.Contains(string(d.Error)+d.SHA256+d.ItemID+d.AttachmentID, ct.Marker) {
				t.Errorf("download result carries the sentinel: %+v", d)
			}
		}
	})
	t.Run("failure of another type", func(t *testing.T) {
		v := newVault(t).on(mAttach, answer{err: errors.New("bw said " + ct.Marker + "-secret")})
		downloads, _, _, err := runAttachments(t, v, planned(att1))
		if downloads != nil || !errors.Is(err, collect.ErrUnexpected) {
			t.Fatalf("downloads %v, error %v, want ErrUnexpected", downloads, err)
		}
		if strings.Contains(err.Error(), ct.Marker) {
			t.Errorf("error %q carries the sentinel", err)
		}
	})
}

// Requirement: Decryption error counts are passed on.

func TestDownloadWithDecryptionErrors(t *testing.T) {
	v := newVault(t).on(mAttach, answer{data: []byte("content"), count: 3})
	downloads, _, _, err := runAttachments(t, v, planned(att1))
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 1 || downloads[0].Error != "" || downloads[0].DecryptionErrors != 3 {
		t.Errorf("downloads = %+v, want a success with the count three", downloads)
	}
}

// Requirement: Transient download failures are retried.

func TestSecondAttemptSucceeds(t *testing.T) {
	v := newVault(t).on(mAttach, fails(bwcli.ClassTimeout), ok("content"))
	downloads, r, w, err := runAttachments(t, v, planned(att1))
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 1 || downloads[0].Error != "" || downloads[0].SHA256 != hashOf([]byte("content")) {
		t.Errorf("downloads = %+v, want a success", downloads)
	}
	wantCalls(t, v, attachCall(att1), attachCall(att1))
	if len(w.got) != 1 || w.got[0] != 2*time.Second {
		t.Errorf("waits = %v, want [2s]", w.got)
	}
	if len(r.got) != 1 {
		t.Errorf("receiver called %d times, want 1", len(r.got))
	}
}

func TestEveryAttemptFails(t *testing.T) {
	v := newVault(t).on(mAttach, fails(bwcli.ClassExit), fails(bwcli.ClassExit), fails(bwcli.ClassExit), ok("next"))
	downloads, r, w, err := runAttachments(t, v, planned(att1, att2))
	if err != nil {
		t.Fatal(err)
	}
	want := []capture.Download{
		{ItemID: itemA, AttachmentID: att1, Error: capture.ErrorStatus},
		{ItemID: itemA, AttachmentID: att2, SHA256: hashOf([]byte("next")), Size: 4},
	}
	if len(downloads) != 2 || downloads[0] != want[0] || downloads[1] != want[1] {
		t.Errorf("downloads = %+v, want %+v", downloads, want)
	}
	wantCalls(t, v, attachCall(att1), attachCall(att1), attachCall(att1), attachCall(att2))
	if len(w.got) != 2 || w.got[0] != 2*time.Second || w.got[1] != 5*time.Second {
		t.Errorf("waits = %v, want [2s 5s]", w.got)
	}
	if len(r.got) != 1 || r.got[0].AttachmentID != att2 {
		t.Errorf("receiver got %d attachments, want only the second", len(r.got))
	}
}

// Requirement: Premium failure is a failed download.

func TestPremiumRequired(t *testing.T) {
	v := newVault(t).on(mAttach, fails(bwcli.ClassPremiumRequired), ok("next"))
	downloads, _, w, err := runAttachments(t, v, planned(att1, att2))
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 || downloads[0].Error != capture.ErrorStatus || downloads[0].SHA256 != "" || downloads[1].Error != "" {
		t.Errorf("downloads = %+v", downloads)
	}
	wantCalls(t, v, attachCall(att1), attachCall(att2))
	if len(w.got) != 0 {
		t.Errorf("waits = %v, want none", w.got)
	}
}

// Requirement: Run-level failures end the collection.

func TestSessionLostDuringTheDownloads(t *testing.T) {
	v := newVault(t).on(mAttach, ok("first"), fails(bwcli.ClassLocked))
	downloads, _, _, err := runAttachments(t, v, planned(att1, att2, att3))
	if downloads != nil {
		t.Errorf("downloads = %+v, want none", downloads)
	}
	wantOp(t, err, bwcli.OpAttachment, bwcli.ClassLocked)
	wantCalls(t, v, attachCall(att1), attachCall(att2))
}

func TestCancelledRun(t *testing.T) {
	t.Run("during a download", func(t *testing.T) {
		v := newVault(t).on(mAttach, ok("first"), fails(bwcli.ClassCancelled))
		downloads, _, _, err := runAttachments(t, v, planned(att1, att2, att3))
		if downloads != nil {
			t.Errorf("downloads = %+v, want none", downloads)
		}
		wantOp(t, err, bwcli.OpAttachment, bwcli.ClassCancelled)
		wantCalls(t, v, attachCall(att1), attachCall(att2))
	})
	t.Run("before the next download", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		v := newVault(t).on(mAttach, answer{data: []byte("first"), before: cancel})
		c, _ := collector()
		downloads, err := c.Attachments(ctx, v, planned(att1, att2), func(collectAttachment) error { return nil })
		if downloads != nil {
			t.Errorf("downloads = %+v, want none", downloads)
		}
		wantOp(t, err, bwcli.OpAttachment, bwcli.ClassCancelled)
		wantCalls(t, v, attachCall(att1))
	})
}

// A cancellation during the wait before a retry ends the collection as a
// cancelled run, and no further attempt is made (design D8).
func TestCancellationDuringAWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := newVault(t).on(mAttach, fails(bwcli.ClassTimeout))
	c, w := collector()
	w.onWait = func(ctx context.Context) error {
		cancel()
		return ctx.Err()
	}
	downloads, err := c.Attachments(ctx, v, planned(att1, att2), func(collectAttachment) error { return nil })
	if downloads != nil {
		t.Errorf("downloads = %+v, want none", downloads)
	}
	wantOp(t, err, bwcli.OpAttachment, bwcli.ClassCancelled)
	var e *bwcli.Error
	if errors.As(err, &e) && (e.ItemID != itemA || e.AttachmentID != att1) {
		t.Errorf("error names %s/%s, want the attachment", e.ItemID, e.AttachmentID)
	}
	wantCalls(t, v, attachCall(att1))
	if len(w.got) != 1 {
		t.Errorf("waits = %v, want one", w.got)
	}
}

// The default wait returns when the context is done.
func TestDefaultWaitStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	v := newVault(t).on(mAttach, answer{err: &bwcli.Error{Op: bwcli.OpAttachment, Class: bwcli.ClassTimeout, ExitCode: bwcli.NoExitCode},
		before: func() { time.AfterFunc(10*time.Millisecond, cancel) }})
	c := &collect.Collector{}
	start := time.Now()
	_, err := c.Attachments(ctx, v, planned(att1), func(collectAttachment) error { return nil })
	wantOp(t, err, bwcli.OpAttachment, bwcli.ClassCancelled)
	if d := time.Since(start); d > time.Second {
		t.Errorf("wait took %v after the cancellation", d)
	}
}

// One test per row of the table in design D7.
func TestDownloadClassification(t *testing.T) {
	for _, tc := range []struct {
		class    bwcli.Class
		attempts int
		// result is the download error class, or "" when the collection
		// ends.
		result capture.ErrorClass
	}{
		{bwcli.ClassTimeout, 3, capture.ErrorProcess},
		{bwcli.ClassExit, 3, capture.ErrorStatus},
		{bwcli.ClassOutput, 3, capture.ErrorOther},
		{bwcli.ClassPremiumRequired, 1, capture.ErrorStatus},
		{bwcli.ClassInvalidArgument, 1, capture.ErrorOther},
		{bwcli.ClassNotLoggedIn, 1, ""},
		{bwcli.ClassLocked, 1, ""},
		{bwcli.ClassCancelled, 1, ""},
		{bwcli.ClassStart, 1, ""},
		{bwcli.ClassProfileNotUsed, 1, ""},
		{bwcli.ClassStrayFile, 1, ""},
		{bwcli.ClassNoTerminal, 1, ""},
		{"a class this table does not know", 1, ""},
	} {
		t.Run(string(tc.class), func(t *testing.T) {
			v := newVault(t)
			for i := 0; i < tc.attempts; i++ {
				v.on(mAttach, fails(tc.class))
			}
			if tc.result != "" {
				v.on(mAttach, ok("next"))
			}
			downloads, _, w, err := runAttachments(t, v, planned(att1, att2))

			wantRequests := make([]string, 0, tc.attempts+1)
			for i := 0; i < tc.attempts; i++ {
				wantRequests = append(wantRequests, attachCall(att1))
			}
			if tc.result == "" {
				if downloads != nil {
					t.Errorf("downloads = %+v, want none", downloads)
				}
				wantOp(t, err, bwcli.OpAttachment, tc.class)
			} else {
				wantRequests = append(wantRequests, attachCall(att2))
				if err != nil || len(downloads) != 2 {
					t.Fatalf("downloads %+v, error %v", downloads, err)
				}
				want := capture.Download{ItemID: itemA, AttachmentID: att1, Error: tc.result}
				if downloads[0] != want {
					t.Errorf("result = %+v, want %+v", downloads[0], want)
				}
			}
			wantCalls(t, v, wantRequests...)
			if len(w.got) != tc.attempts-1 {
				t.Errorf("waits = %v, want %d", w.got, tc.attempts-1)
			}
			v.leftover(t)
		})
	}

	t.Run("an error that is not a bwcli.Error", func(t *testing.T) {
		v := newVault(t).on(mAttach, answer{err: errors.New("other")})
		downloads, _, _, err := runAttachments(t, v, planned(att1, att2))
		if downloads != nil || !errors.Is(err, collect.ErrUnexpected) {
			t.Errorf("downloads %+v, error %v, want ErrUnexpected", downloads, err)
		}
		wantCalls(t, v, attachCall(att1))
	})
}

// Every planned pair gets exactly one result, in plan order.
func TestOneResultPerPlannedAttachment(t *testing.T) {
	v := newVault(t).on(mAttach, ok("a"), fails(bwcli.ClassPremiumRequired), fails(bwcli.ClassInvalidArgument))
	s := planned(att1, att2)
	s.Plan = append(s.Plan, collect.PlannedAttachment{ItemID: itemB, AttachmentID: att3})
	downloads, _, _, err := runAttachments(t, v, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 3 {
		t.Fatalf("downloads = %+v", downloads)
	}
	for i, d := range downloads {
		if d.Ref() != s.Plan[i].Ref() {
			t.Errorf("result %d is for %v, want %v", i, d.Ref(), s.Plan[i].Ref())
		}
	}
}
