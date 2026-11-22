package verify_test

import (
	"bytes"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// Two runs over the same capture produce byte-identical output, although
// the verification walks maps internally. The capture combines several
// variants so that the result holds many findings and warnings.
func TestSerializationIsDeterministic(t *testing.T) {
	f := variant(t, ct.MissingItem, ct.DuplicateID, ct.BlobLoginItem, ct.NonNumericCounter,
		ct.OrganizationItems, ct.ListedItemNotExported)
	in := f.Input(t)
	in.Downloads = append(in.Downloads, in.Downloads[0], capture.Download{
		ItemID: ct.ItemNote, AttachmentID: "att-unexpected", Error: capture.ErrorProcess,
	})
	f.Reference.Items = f.Reference.Items[:2]

	first := dump(t, runInput(t, f, in, true))
	if strings.Count(first, `"reason"`) < 8 {
		t.Fatalf("too few findings to test the order: %s", first)
	}
	for i := 0; i < 20; i++ {
		if again := dump(t, runInput(t, f, in, true)); again != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first, again)
		}
	}
}

// Every fixture run, with and without the reference inventory, serializes
// byte-identically on a second run.
func TestEveryRunSerializesDeterministically(t *testing.T) {
	for _, fr := range fixtureRuns() {
		t.Run(fr.name, func(t *testing.T) {
			f, in := fr.build(t)
			for _, withReference := range []bool{false, true} {
				first := dump(t, runInput(t, f, in, withReference))
				for i := 0; i < 5; i++ {
					if again := dump(t, runInput(t, f, in, withReference)); again != first {
						t.Fatalf("reference %v, run %d differs:\n%s\n---\n%s", withReference, i, first, again)
					}
				}
			}
		})
	}
}

func TestSerializationSortsFindings(t *testing.T) {
	two := 2
	r := &verify.Result{
		Verdict:        verify.Failed,
		StandingLimits: []verify.Limit{verify.LimitServerCompleteness, verify.LimitAttachmentContent},
		Findings: []verify.Finding{
			{Severity: verify.SeverityFailed, Reason: verify.PasskeyFieldMissing, ItemID: "b", Passkey: &two},
			{Severity: verify.SeverityFailed, Reason: verify.AttachmentDownloadMissing, ItemID: "a", AttachmentID: "y"},
			{Severity: verify.SeverityFailed, Reason: verify.PasskeyCountMismatch, ItemID: "b"},
			{Severity: verify.SeverityFailed, Reason: verify.AttachmentDownloadFailed, ItemID: "a", AttachmentID: "x"},
			{Severity: verify.SeverityFailed, Reason: verify.DocumentInvalidJSON, Document: "item_list"},
		},
	}
	out, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	order := []verify.Reason{
		verify.DocumentInvalidJSON, verify.AttachmentDownloadFailed, verify.AttachmentDownloadMissing,
		verify.PasskeyCountMismatch, verify.PasskeyFieldMissing,
	}
	last := -1
	for _, reason := range order {
		i := bytes.Index(out, []byte(`"`+string(reason)+`"`))
		if i <= last {
			t.Fatalf("reason %s out of order:\n%s", reason, out)
		}
		last = i
	}
	if bytes.Index(out, []byte(verify.LimitAttachmentContent)) > bytes.Index(out, []byte(verify.LimitServerCompleteness)) {
		t.Errorf("standing limits not sorted:\n%s", out)
	}
	// Serializing does not reorder the result itself.
	if r.Findings[0].Reason != verify.PasskeyFieldMissing {
		t.Error("JSON changed the result")
	}
}

func TestEmptyListsSerializeAsArrays(t *testing.T) {
	out := dump(t, run(t, ct.Intact(t), true))
	for _, want := range []string{`"findings": []`, `"warnings": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("serialized result lacks %s:\n%s", want, out)
		}
	}
}

// leaks returns every sensitive value that occurs in the serialized result.
func leaks(serialized []byte, sensitive []string) []string {
	var out []string
	for _, s := range sensitive {
		if bytes.Contains(serialized, []byte(s)) {
			out = append(out, s)
		}
	}
	return out
}

// Scenario "Failing capture with sensitive content": no run, failing or
// not, serializes a sentinel value, a key value, an attachment file name or
// an attachment content.
func TestFailingCaptureWithSensitiveContent(t *testing.T) {
	failing := 0
	for _, fr := range fixtureRuns() {
		t.Run(fr.name, func(t *testing.T) {
			f, in := fr.build(t)
			sensitive := f.Sensitive()
			if len(sensitive) < 50 {
				t.Fatalf("only %d sensitive values; the fixture lost its sentinels", len(sensitive))
			}
			for _, withReference := range []bool{false, true} {
				r := runInput(t, f, in, withReference)
				if r.Verdict == verify.Failed {
					failing++
				}
				out := dump(t, r)
				if got := leaks([]byte(out), sensitive); len(got) != 0 {
					t.Errorf("reference %v: result contains sensitive values %q", withReference, got)
				}
				if strings.Contains(out, ct.Marker) {
					t.Errorf("reference %v: result contains the sentinel marker", withReference)
				}
			}
		})
	}
	if failing < 10 {
		t.Errorf("only %d failing runs; the redaction test needs failing captures", failing)
	}
}

// The redaction check notices a name that a finding carries on purpose.
func TestRedactionCheckDetectsALeak(t *testing.T) {
	f := variant(t, ct.OnePasskeyMissing)
	r := run(t, f, false)
	r.Findings[0].Field = ct.AttachmentFileName
	out, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if got := leaks(out, f.Sensitive()); len(got) != 1 || got[0] != ct.AttachmentFileName {
		t.Errorf("leaks = %q, want the planted file name", got)
	}

	r = run(t, f, false)
	r.Findings[0].ItemID = f.Sensitive()[0]
	out, _ = r.JSON()
	if len(leaks(out, f.Sensitive())) == 0 {
		t.Error("planted sensitive value not detected")
	}
}

// What each reason code must refer to.
type needs struct{ document, item, attachment, passkey, counts, organization bool }

var reasonNeeds = map[verify.Reason]needs{
	verify.DocumentInvalidJSON:              {document: true},
	verify.DocumentWrongForm:                {document: true},
	verify.DocumentMarkedEncrypted:          {document: true},
	verify.DocumentNotMarkedEncrypted:       {document: true},
	verify.DocumentPasswordProtected:        {document: true},
	verify.DocumentNotAccountBound:          {document: true},
	verify.DocumentResponseEnvelope:         {document: true},
	verify.DocumentMalformedItem:            {document: true},
	verify.DuplicateItemID:                  {document: true, item: true},
	verify.ItemMissingFromPlaintextExport:   {document: true, item: true},
	verify.ItemMissingFromEncryptedExport:   {document: true, item: true},
	verify.ItemFormatUnrecognized:           {document: true, item: true},
	verify.ListedItemMissingFromExport:      {document: true, item: true},
	verify.ExportedItemNotListed:            {item: true},
	verify.ItemOrganizationMismatch:         {document: true, item: true},
	verify.OrganizationItemMismatch:         {document: true, item: true, organization: true},
	verify.OrganizationChangedDuringCapture: {item: true, organization: true},
	verify.ListedItemDiffersBetweenLists:    {item: true},
	verify.ArchiveFilterNotApplied:          {counts: true},
	verify.DocumentDecryptionErrors:         {document: true, counts: true},
	verify.AttachmentDecryptionErrors:       {item: true, attachment: true, counts: true},
	verify.AttachmentDownloadOutOfScope:     {document: true, item: true, attachment: true},
	verify.PasskeyCountMismatch:             {item: true, counts: true},
	verify.PasskeyCountUnverifiable:         {item: true},
	verify.PasskeyFieldMissing:              {item: true, passkey: true},
	verify.PasskeyFieldInvalid:              {item: true, passkey: true},
	verify.PasskeyKeyInvalid:                {item: true, passkey: true},
	verify.PasskeyKeyFormUnknown:            {item: true, passkey: true},
	verify.PasskeyCounterInvalid:            {item: true, passkey: true},
	verify.DuplicateAttachmentID:            {document: true, item: true, attachment: true},
	verify.DuplicateDownloadResult:          {document: true, item: true, attachment: true},
	verify.AttachmentDownloadFailed:         {item: true, attachment: true},
	verify.AttachmentDownloadMissing:        {item: true, attachment: true},
	verify.AttachmentDownloadInvalid:        {item: true, attachment: true},
	verify.AttachmentDownloadNotExpected:    {document: true, item: true, attachment: true},
	verify.AttachmentEmpty:                  {item: true, attachment: true},
	verify.ReferenceItemMissing:             {item: true},
	verify.ReferenceItemNotExpected:         {item: true},
	verify.ReferencePasskeyMissing:          {item: true},
	verify.ReferencePasskeyNotExpected:      {item: true, passkey: true},
	verify.ReferencePasskeyKeyMismatch:      {item: true, passkey: true},
	verify.ReferencePasskeyCounterMismatch:  {item: true, passkey: true},
	verify.ReferenceAttachmentMissing:       {item: true, attachment: true},
	verify.ReferenceAttachmentNotExpected:   {item: true, attachment: true},
	verify.ReferenceAttachmentMismatch:      {item: true, attachment: true},
}

func TestEveryReasonHasNeeds(t *testing.T) {
	seen := map[verify.Reason]bool{}
	for _, reason := range verify.Reasons() {
		if seen[reason] {
			t.Errorf("reason %s listed twice", reason)
		}
		seen[reason] = true
		if _, ok := reasonNeeds[reason]; !ok {
			t.Errorf("reason %s has no entry in reasonNeeds", reason)
		}
		if strings.ToLower(string(reason)) != string(reason) || strings.ContainsAny(string(reason), " -") {
			t.Errorf("reason %s is not lowercase snake case", reason)
		}
	}
	if len(reasonNeeds) != len(seen) {
		t.Errorf("reasonNeeds has %d entries, Reasons() %d", len(reasonNeeds), len(seen))
	}
}

// Scenario "Finding for a missing passkey", extended to every finding of
// every fixture run: each carries a severity, a known reason code and the
// IDs it refers to.
func TestFindingForAMissingPasskey(t *testing.T) {
	r := run(t, variant(t, ct.OnePasskeyMissing), false)
	got := only(t, r, verify.PasskeyCountMismatch)
	if got.Severity != verify.SeverityFailed || got.ItemID != ct.ItemPasskeyLogin {
		t.Errorf("finding = %+v", got)
	}

	known := map[verify.Reason]bool{}
	for _, reason := range verify.Reasons() {
		known[reason] = true
	}
	covered := map[verify.Reason]bool{}
	for _, fr := range fixtureRuns() {
		t.Run(fr.name, func(t *testing.T) {
			f, in := fr.build(t)
			for _, withReference := range []bool{false, true} {
				r := runInput(t, f, in, withReference)
				check := func(f verify.Finding, warning bool) {
					covered[f.Reason] = true
					if !known[f.Reason] {
						t.Errorf("unknown reason %q", f.Reason)
					}
					switch {
					case warning && f.Severity != verify.SeverityWarning,
						!warning && f.Severity != verify.SeverityFailed && f.Severity != verify.SeverityUnverified:
						t.Errorf("%s: severity %q in the wrong list", f.Reason, f.Severity)
					}
					n := reasonNeeds[f.Reason]
					if n.document && f.Document == "" || n.item && f.ItemID == "" ||
						n.attachment && f.AttachmentID == "" || n.passkey && f.Passkey == nil ||
						n.counts && f.Count == nil && (f.Expected == nil || f.Found == nil) ||
						n.organization && f.OrganizationID == "" {
						t.Errorf("%s: finding lacks a reference it needs: %+v", f.Reason, f)
					}
					if f.AttachmentID != "" && f.ItemID == "" {
						t.Errorf("%s: attachment ID without item ID", f.Reason)
					}
					// A finding about an organization document names its
					// organization (requirement "Attributable findings").
					if strings.HasPrefix(f.Document, "organization_") && f.OrganizationID == "" {
						t.Errorf("%s: organization document without organization ID: %+v", f.Reason, f)
					}
				}
				for _, f := range r.Findings {
					check(f, false)
				}
				for _, w := range r.Warnings {
					check(w, true)
				}
			}
		})
	}
	if len(covered) < 15 {
		t.Errorf("fixture runs cover only %d reason codes", len(covered))
	}
}
