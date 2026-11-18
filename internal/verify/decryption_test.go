package verify_test

import (
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// Requirement: Decryption error lines are reported.

func TestDecryptionErrorsOnThePlaintextExport(t *testing.T) {
	r := run(t, variant(t, ct.DecryptionErrorLines), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.DocumentDecryptionErrors)
	if got.Severity != verify.SeverityUnverified || got.Document != string(capture.PlaintextExportDocument) ||
		got.Count == nil || *got.Count != 3 || got.ItemID != "" {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.DocumentDecryptionErrors)
}

func TestDecryptionErrorsTogetherWithAProvenDeviation(t *testing.T) {
	f := variant(t, ct.OnePasskeyMissing)
	in := f.Input(t)
	in.DecryptionErrors.ItemList = 2
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DocumentDecryptionErrors)
	if got.Document != string(capture.ItemListDocument) || got.Count == nil || *got.Count != 2 {
		t.Errorf("finding = %+v", got)
	}
	only(t, r, verify.PasskeyCountMismatch)
	wantReasons(t, r, verify.DocumentDecryptionErrors, verify.PasskeyCountMismatch)
}

func TestDecryptionErrorsOnAnAttachmentDownload(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.Downloads[downloadIndex(t, in, ct.AttachmentSecond)].DecryptionErrors = 3
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Verified)
	got := only(t, r, verify.AttachmentDecryptionErrors)
	if got.Severity != verify.SeverityWarning || got.ItemID != ct.ItemPasskeyLogin ||
		got.AttachmentID != ct.AttachmentSecond || got.Count == nil || *got.Count != 3 {
		t.Errorf("warning = %+v", got)
	}
	wantReasons(t, r)
}

func TestNoDecryptionErrors(t *testing.T) {
	for _, withReference := range []bool{false, true} {
		r := run(t, ct.Intact(t), withReference)
		wantVerdict(t, r, verify.Verified)
		for _, reason := range []verify.Reason{verify.DocumentDecryptionErrors, verify.AttachmentDecryptionErrors} {
			if got := find(r, reason); len(got) != 0 {
				t.Errorf("findings with reason %s: %+v", reason, got)
			}
		}
	}
}

// Every document with lines gives its own unverified finding.
func TestDecryptionErrorsOnEachDocument(t *testing.T) {
	for _, tc := range []struct {
		document capture.Document
		set      func(*capture.DecryptionErrors)
	}{
		{capture.PlaintextExportDocument, func(d *capture.DecryptionErrors) { d.PlaintextExport = 1 }},
		{capture.EncryptedExportDocument, func(d *capture.DecryptionErrors) { d.EncryptedExport = 1 }},
		{capture.ItemListDocument, func(d *capture.DecryptionErrors) { d.ItemList = 1 }},
		{capture.ArchivedItemListDocument, func(d *capture.DecryptionErrors) { d.ArchivedItemList = 1 }},
	} {
		t.Run(string(tc.document), func(t *testing.T) {
			f := ct.Intact(t)
			in := f.Input(t)
			tc.set(&in.DecryptionErrors)
			r := runInput(t, f, in, true)
			wantVerdict(t, r, verify.Unverified)
			got := only(t, r, verify.DocumentDecryptionErrors)
			if got.Document != string(tc.document) || got.Count == nil || *got.Count != 1 {
				t.Errorf("finding = %+v", got)
			}
			wantReasons(t, r, verify.DocumentDecryptionErrors)
		})
	}

	t.Run("all four documents", func(t *testing.T) {
		f := ct.Intact(t)
		in := f.Input(t)
		in.DecryptionErrors = capture.DecryptionErrors{PlaintextExport: 1, EncryptedExport: 2, ItemList: 3, ArchivedItemList: 4}
		r := runInput(t, f, in, false)
		if got := find(r, verify.DocumentDecryptionErrors); len(got) != 4 {
			t.Errorf("%d findings, want one per document: %+v", len(got), got)
		}
	})

	// A rejected document does not hide the lines of the others.
	t.Run("rejected document", func(t *testing.T) {
		f := ct.Intact(t)
		in := f.Input(t)
		in.ItemList = in.ItemList[:len(in.ItemList)/2]
		in.DecryptionErrors.PlaintextExport = 2
		r := runInput(t, f, in, false)
		wantVerdict(t, r, verify.Failed)
		if got := only(t, r, verify.DocumentDecryptionErrors); got.Document != string(capture.PlaintextExportDocument) {
			t.Errorf("finding = %+v", got)
		}
	})
}

// A count of zero or less reports nothing, and a failed download carries no
// warning.
func TestDecryptionErrorsNegativeOrOnAFailedDownload(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.DecryptionErrors = capture.DecryptionErrors{PlaintextExport: -1, EncryptedExport: -3, ItemList: 0, ArchivedItemList: -2}
	in.Downloads[downloadIndex(t, in, ct.AttachmentFirst)].DecryptionErrors = -4
	failed := downloadIndex(t, in, ct.AttachmentSecond)
	in.Downloads[failed].Error, in.Downloads[failed].SHA256, in.Downloads[failed].Size = capture.ErrorStatus, "", 0
	in.Downloads[failed].DecryptionErrors = 2
	r := runInput(t, f, in, false)
	for _, reason := range []verify.Reason{verify.DocumentDecryptionErrors, verify.AttachmentDecryptionErrors} {
		if got := find(r, reason); len(got) != 0 {
			t.Errorf("findings with reason %s: %+v", reason, got)
		}
	}
	wantReasons(t, r, verify.AttachmentDownloadFailed)
}
