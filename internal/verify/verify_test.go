package verify_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// Requirement: Three-state verdict.

func TestIntactCapture(t *testing.T) {
	for _, withReference := range []bool{false, true} {
		r := run(t, ct.Intact(t), withReference)
		wantVerdict(t, r, verify.Verified)
		if len(r.Findings) != 0 || len(r.Warnings) != 0 {
			t.Errorf("reference %v: findings %v, warnings %v", withReference, r.Findings, r.Warnings)
		}
	}
}

func TestFailureOutranksUnverifiableItems(t *testing.T) {
	r := run(t, variant(t, ct.BlobLoginItem, ct.MissingItem), false)
	wantVerdict(t, r, verify.Failed)
	if f := only(t, r, verify.PasskeyCountUnverifiable); f.Severity != verify.SeverityUnverified {
		t.Errorf("unverifiable item: severity %s", f.Severity)
	}
	if f := only(t, r, verify.ItemMissingFromPlaintextExport); f.Severity != verify.SeverityFailed {
		t.Errorf("missing item: severity %s", f.Severity)
	}
}

func TestVerdictPrecedence(t *testing.T) {
	failed := verify.Finding{Severity: verify.SeverityFailed, Reason: verify.PasskeyCountMismatch, ItemID: "a"}
	unverified := verify.Finding{Severity: verify.SeverityUnverified, Reason: verify.PasskeyCountUnverifiable, ItemID: "b"}
	cases := []struct {
		findings []verify.Finding
		want     verify.Verdict
	}{
		{nil, verify.Verified},
		{[]verify.Finding{unverified}, verify.Unverified},
		{[]verify.Finding{failed}, verify.Failed},
		{[]verify.Finding{unverified, failed}, verify.Failed},
		{[]verify.Finding{failed, unverified}, verify.Failed},
	}
	for _, tc := range cases {
		if got := verify.VerdictOf(tc.findings); got != tc.want {
			t.Errorf("verdict of %v = %s, want %s", tc.findings, got, tc.want)
		}
	}
}

// Warnings do not change the verdict: an empty attachment and a download of
// an organization that is not chosen produce warnings, and the capture stays
// verified.
func TestWarningsDoNotChangeTheVerdict(t *testing.T) {
	f := variant(t, ct.OrganizationItems)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentFirst)
	in.Downloads[i].SHA256, in.Downloads[i].Size = hashOf(nil), 0
	in.Downloads = append(in.Downloads, capture.Download{
		ItemID: ct.ItemOrgFirstA, AttachmentID: ct.AttachmentOrg, SHA256: hashOf([]byte("x")), Size: 1,
	})
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Verified)
	if len(r.Warnings) != 2 {
		t.Errorf("%d warnings, want 2: %s", len(r.Warnings), dump(t, r))
	}
	for _, w := range r.Warnings {
		if w.Severity != verify.SeverityWarning {
			t.Errorf("warning with severity %s", w.Severity)
		}
	}
}

// Requirement: Scope statement and standing limits.

func TestVerifiedResultWithoutReferenceInventory(t *testing.T) {
	r := run(t, ct.Intact(t), false)
	wantVerdict(t, r, verify.Verified)
	if r.Scope.Code != verify.ScopeCode || r.Scope.Statement == "" {
		t.Errorf("scope = %+v", r.Scope)
	}
	want := []verify.Limit{
		verify.LimitAttachmentContent, verify.LimitAttachmentInventory,
		verify.LimitCredentialIdentity, verify.LimitServerCompleteness,
	}
	if !reflect.DeepEqual(r.StandingLimits, want) {
		t.Errorf("standing limits = %v, want %v", r.StandingLimits, want)
	}
}

func TestVerifiedResultWithReferenceInventory(t *testing.T) {
	r := run(t, ct.Intact(t), true)
	wantVerdict(t, r, verify.Verified)
	if !reflect.DeepEqual(r.StandingLimits, []verify.Limit{verify.LimitServerCompleteness}) {
		t.Errorf("standing limits = %v", r.StandingLimits)
	}
}

// Every result, whatever its verdict, names the scope and never claims more.
func TestEveryResultNamesScopeAndServerLimit(t *testing.T) {
	for _, fr := range fixtureRuns() {
		t.Run(fr.name, func(t *testing.T) {
			f, in := fr.build(t)
			for _, withReference := range []bool{false, true} {
				r := runInput(t, f, in, withReference)
				wantCode, wantLimit := verify.ScopeCode, false
				if len(in.Organizations) > 0 {
					wantCode, wantLimit = verify.ScopeCodeWithOrganizations, true
				}
				if r.Scope.Code != wantCode {
					t.Errorf("scope %q, want %q", r.Scope.Code, wantCode)
				}
				hasServerLimit, hasOrganizationLimit := false, false
				for _, l := range r.StandingLimits {
					hasServerLimit = hasServerLimit || l == verify.LimitServerCompleteness
					hasOrganizationLimit = hasOrganizationLimit || l == verify.LimitOrganizationExportsSeparateReads
				}
				if !hasServerLimit {
					t.Errorf("standing limits %v lack %s", r.StandingLimits, verify.LimitServerCompleteness)
				}
				if hasOrganizationLimit != wantLimit {
					t.Errorf("standing limits %v: organization limit %v, want %v", r.StandingLimits, hasOrganizationLimit, wantLimit)
				}
			}
		})
	}
}

// Requirement: Document form validation.

func TestPasswordProtectedExportSuppliedAsInventory(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.EncryptedExport = passwordProtectedExport
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DocumentPasswordProtected)
	if got.Document != string(capture.EncryptedExportDocument) {
		t.Errorf("finding names document %q", got.Document)
	}
	wantReasons(t, r, verify.DocumentPasswordProtected)
}

func TestUnparsableDocument(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.ItemList = []byte(`[{"id": `)
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.DocumentInvalidJSON); got.Document != string(capture.ItemListDocument) {
		t.Errorf("finding names document %q", got.Document)
	}
}

func TestEveryDocumentProblemHasAReason(t *testing.T) {
	f := ct.Intact(t)
	for _, tc := range []struct {
		change func(in *capture.Input)
		want   verify.Reason
	}{
		{func(in *capture.Input) { in.PlaintextExport = in.EncryptedExport }, verify.DocumentMarkedEncrypted},
		{func(in *capture.Input) { in.EncryptedExport = in.PlaintextExport }, verify.DocumentNotMarkedEncrypted},
		{func(in *capture.Input) { in.EncryptedExport = []byte(`{"encrypted":true,"items":[]}`) }, verify.DocumentNotAccountBound},
		{func(in *capture.Input) { in.ItemList = []byte(`{"success":true,"data":{}}`) }, verify.DocumentResponseEnvelope},
		{func(in *capture.Input) { in.ItemList = []byte(`"items"`) }, verify.DocumentWrongForm},
		{func(in *capture.Input) { in.ItemList = []byte(`[{"name":"x"}]`) }, verify.DocumentMalformedItem},
	} {
		in := f.Input(t)
		tc.change(&in)
		r := runInput(t, f, in, false)
		wantVerdict(t, r, verify.Failed)
		only(t, r, tc.want)
	}
}

// Requirement: Item identity comparison.

func TestItemMissingFromPlaintextExport(t *testing.T) {
	r := run(t, variant(t, ct.MissingItem), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ItemMissingFromPlaintextExport)
	if got.ItemID != ct.ItemPlainLogin || got.Document != string(capture.PlaintextExportDocument) {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ItemMissingFromPlaintextExport)
}

func TestItemMissingFromEncryptedExport(t *testing.T) {
	f := ct.Intact(t)
	var kept []any
	for _, raw := range f.EncryptedItems() {
		if raw.(map[string]any)["id"] != ct.ItemNote {
			kept = append(kept, raw)
		}
	}
	f.EncryptedExport["items"] = kept
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ItemMissingFromEncryptedExport)
	if got.ItemID != ct.ItemNote || got.Document != string(capture.EncryptedExportDocument) {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ItemMissingFromEncryptedExport)
}

func TestDuplicateItemID(t *testing.T) {
	r := run(t, variant(t, ct.DuplicateID), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DuplicateItemID)
	if got.ItemID != ct.ItemNote || got.Document != string(capture.PlaintextExportDocument) {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.DuplicateItemID)
}

// Requirement: Passkey count comparison.

func TestLegacyLoginWithoutPasskeyList(t *testing.T) {
	r := run(t, ct.Intact(t), false)
	for _, f := range append(r.Findings, r.Warnings...) {
		if f.ItemID == ct.ItemPlainLogin {
			t.Errorf("finding for the login without passkeys: %+v", f)
		}
	}
}

func TestOnePasskeyMissing(t *testing.T) {
	r := run(t, variant(t, ct.OnePasskeyMissing), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.PasskeyCountMismatch)
	if got.ItemID != ct.ItemPasskeyLogin || intValue(t, got.Expected) != 2 || intValue(t, got.Found) != 1 {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.PasskeyCountMismatch)
}

func TestAllPasskeysDropped(t *testing.T) {
	r := run(t, variant(t, ct.AllPasskeysDropped), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.PasskeyCountMismatch)
	if intValue(t, got.Expected) != 2 || intValue(t, got.Found) != 0 {
		t.Errorf("finding = %+v", got)
	}
}

// Requirement: Unverifiable item formats.

func TestBlobFormatLoginItem(t *testing.T) {
	r := run(t, variant(t, ct.BlobLoginItem), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.PasskeyCountUnverifiable)
	if got.ItemID != ct.ItemPasskeyLogin || got.Severity != verify.SeverityUnverified {
		t.Errorf("finding = %+v", got)
	}
	// The item is not reported as having zero passkeys.
	if len(find(r, verify.PasskeyCountMismatch)) != 0 {
		t.Error("blob item compared as if it had zero passkeys")
	}
	wantReasons(t, r, verify.PasskeyCountUnverifiable)
}

func TestUnrecognizedFormat(t *testing.T) {
	r := run(t, variant(t, ct.UnrecognizedItemForm), false)
	wantVerdict(t, r, verify.Unverified)
	if got := only(t, r, verify.ItemFormatUnrecognized); got.ItemID != ct.ItemPasskeyLogin {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ItemFormatUnrecognized)
}

func TestLegacyContentWithUnmarkedSealedData(t *testing.T) {
	f := ct.Intact(t)
	for _, raw := range f.EncryptedItems() {
		if item := raw.(map[string]any); item["id"] == ct.ItemPasskeyLogin {
			item["data"] = ct.UnknownSealedData
		}
	}
	r := run(t, f, false)
	wantVerdict(t, r, verify.Unverified)
	if got := only(t, r, verify.ItemFormatUnrecognized); got.ItemID != ct.ItemPasskeyLogin {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ItemFormatUnrecognized)
}

func TestNonLoginBlobItem(t *testing.T) {
	f := ct.Intact(t)
	for _, raw := range f.EncryptedItems() {
		if item := raw.(map[string]any); item["id"] == ct.ItemNote {
			for _, key := range []string{"name", "notes", "fields", "secureNote"} {
				delete(item, key)
			}
			item["data"] = ct.BlobData
		}
	}
	r := run(t, f, false)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}
}

// Requirement: Passkey structure validation.

func TestTwoValidPasskeysWithDifferentKeysAndCounters(t *testing.T) {
	r := run(t, ct.Intact(t), false)
	for _, f := range r.Findings {
		if f.Passkey != nil {
			t.Errorf("passkey finding: %+v", f)
		}
	}
}

func passkeyFinding(t *testing.T, r *verify.Result, reason verify.Reason, field string) {
	t.Helper()
	got := only(t, r, reason)
	if got.ItemID != ct.ItemPasskeyLogin || got.Passkey == nil || *got.Passkey != 1 || got.Field != field {
		t.Errorf("finding = %+v, want item %s, passkey 1, field %q", got, ct.ItemPasskeyLogin, field)
	}
}

func TestEmptyKey(t *testing.T) {
	r := run(t, variant(t, ct.EmptyKey), false)
	wantVerdict(t, r, verify.Failed)
	passkeyFinding(t, r, verify.PasskeyFieldMissing, "keyValue")
	wantReasons(t, r, verify.PasskeyFieldMissing)
}

func TestKeyDoesNotParse(t *testing.T) {
	r := run(t, variant(t, ct.UnparsableKey), false)
	wantVerdict(t, r, verify.Failed)
	passkeyFinding(t, r, verify.PasskeyKeyInvalid, "keyValue")
	wantReasons(t, r, verify.PasskeyKeyInvalid)
}

func TestUnknownKeyForm(t *testing.T) {
	r := run(t, variant(t, ct.UnknownKeyAlgorithm), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.PasskeyKeyFormUnknown)
	if got.Severity != verify.SeverityUnverified || got.Passkey == nil || *got.Passkey != 1 {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.PasskeyKeyFormUnknown)
}

func TestNonNumericCounter(t *testing.T) {
	r := run(t, variant(t, ct.NonNumericCounter), false)
	wantVerdict(t, r, verify.Failed)
	passkeyFinding(t, r, verify.PasskeyCounterInvalid, "counter")
	wantReasons(t, r, verify.PasskeyCounterInvalid)
}

// changeSecondPasskey changes the second passkey of the passkey login in
// the plaintext export.
func changeSecondPasskey(t *testing.T, change func(p map[string]any)) *ct.Fixture {
	t.Helper()
	f := ct.Intact(t)
	for _, raw := range f.PlaintextItems() {
		item := raw.(map[string]any)
		if item["id"] == ct.ItemPasskeyLogin {
			change(item["login"].(map[string]any)["fido2Credentials"].([]any)[1].(map[string]any))
		}
	}
	return f
}

func TestMissingRequiredField(t *testing.T) {
	for _, field := range []string{"credentialId", "keyType", "rpId", "counter", "discoverable", "creationDate"} {
		t.Run(field, func(t *testing.T) {
			r := run(t, changeSecondPasskey(t, func(p map[string]any) { delete(p, field) }), false)
			wantVerdict(t, r, verify.Failed)
			passkeyFinding(t, r, verify.PasskeyFieldMissing, field)
			wantReasons(t, r, verify.PasskeyFieldMissing)
		})
	}
}

func TestMissingOptionalFieldsPass(t *testing.T) {
	r := run(t, changeSecondPasskey(t, func(p map[string]any) {
		for _, field := range []string{"userHandle", "userName", "rpName", "userDisplayName"} {
			delete(p, field)
		}
	}), false)
	wantVerdict(t, r, verify.Verified)
}

func TestCounterForms(t *testing.T) {
	for _, tc := range []struct {
		counter any
		want    verify.Verdict
	}{
		{json.Number("6"), verify.Verified},
		{"0006", verify.Verified},
		{"-1", verify.Failed},
		{json.Number("-1"), verify.Failed},
		{json.Number("6.5"), verify.Failed},
		{"6 ", verify.Failed},
		{"99999999999999999999999", verify.Failed},
		{true, verify.Failed},
	} {
		r := run(t, changeSecondPasskey(t, func(p map[string]any) { p["counter"] = tc.counter }), false)
		if r.Verdict != tc.want {
			t.Errorf("counter %#v: verdict %s, want %s", tc.counter, r.Verdict, tc.want)
		}
	}
}

// Padding changes the encoding, not the key: a padded key that parses
// passes, also against the reference fingerprint. Wrong padding fails.
func TestKeyValueWithPadding(t *testing.T) {
	pad := func(s string) string {
		for len(s)%4 != 0 {
			s += "="
		}
		return s
	}
	padded := changeSecondPasskey(t, func(p map[string]any) {
		p["keyValue"] = pad(p["keyValue"].(string))
	})
	for _, withReference := range []bool{false, true} {
		wantVerdict(t, run(t, padded, withReference), verify.Verified)
	}

	wrong := changeSecondPasskey(t, func(p map[string]any) {
		p["keyValue"] = pad(p["keyValue"].(string)) + "===="
	})
	r := run(t, wrong, false)
	wantVerdict(t, r, verify.Failed)
	passkeyFinding(t, r, verify.PasskeyKeyInvalid, "keyValue")
}

// Requirement: Attachment accounting by ID.

func TestTwoAttachmentsWithTheSameName(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	first, second := in.Downloads[downloadIndex(t, in, ct.AttachmentFirst)], in.Downloads[downloadIndex(t, in, ct.AttachmentSecond)]
	if first.ItemID != second.ItemID || first.SHA256 == second.SHA256 {
		t.Fatalf("downloads = %+v, want two of one item with different content hashes", in.Downloads)
	}
	r := runInput(t, f, in, true)
	wantVerdict(t, r, verify.Verified)
}

func TestOneDownloadFails(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentSecond)
	in.Downloads[i].Error, in.Downloads[i].SHA256, in.Downloads[i].Size = capture.ErrorStatus, "", 0
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.AttachmentDownloadFailed)
	if got.ItemID != ct.ItemPasskeyLogin || got.AttachmentID != ct.AttachmentSecond {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.AttachmentDownloadFailed)
}

func TestResultMissing(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.Downloads = in.Downloads[1:]
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.AttachmentDownloadMissing); got.AttachmentID != ct.AttachmentFirst {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.AttachmentDownloadMissing)
}

func TestEmptyAttachment(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentFirst)
	in.Downloads[i].SHA256, in.Downloads[i].Size = hashOf(nil), 0
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Verified)
	got := only(t, r, verify.AttachmentEmpty)
	if got.Severity != verify.SeverityWarning || got.ItemID != ct.ItemPasskeyLogin || got.AttachmentID != ct.AttachmentFirst {
		t.Errorf("warning = %+v", got)
	}
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}
}

func TestResultForUnexpectedIDPair(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	in.Downloads = append(in.Downloads, capture.Download{
		ItemID: ct.ItemNote, AttachmentID: "att-unexpected", SHA256: hashOf([]byte("x")), Size: 1,
	})
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.AttachmentDownloadNotExpected)
	if got.ItemID != ct.ItemNote || got.AttachmentID != "att-unexpected" {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.AttachmentDownloadNotExpected)
}

func TestMalformedDownloadResult(t *testing.T) {
	for name, change := range map[string]func(d *capture.Download){
		"no hash":         func(d *capture.Download) { d.SHA256 = "" },
		"hash not hex":    func(d *capture.Download) { d.SHA256 = d.SHA256[:62] + "zz" },
		"error and hash":  func(d *capture.Download) { d.Error = capture.ErrorOther },
		"negative size":   func(d *capture.Download) { d.Size = -1 },
		"short hash":      func(d *capture.Download) { d.SHA256 = d.SHA256[:10] },
		"duplicate entry": nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := ct.Intact(t)
			in := f.Input(t)
			want := verify.AttachmentDownloadInvalid
			if change == nil {
				in.Downloads = append(in.Downloads, in.Downloads[0])
				want = verify.DuplicateDownloadResult
			} else {
				change(&in.Downloads[0])
			}
			r := runInput(t, f, in, false)
			wantVerdict(t, r, verify.Failed)
			only(t, r, want)
		})
	}
}

func TestDuplicateAttachmentInItemList(t *testing.T) {
	f := ct.Intact(t)
	for _, raw := range f.ItemList {
		if item := raw.(map[string]any); item["id"] == ct.ItemPasskeyLogin {
			a := item["attachments"].([]any)
			item["attachments"] = append(a, a[0])
		}
	}
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.DuplicateAttachmentID); got.AttachmentID != ct.AttachmentFirst {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.DuplicateAttachmentID)
}

// Requirement: Attachment inventory coverage.

func TestListedItemNotExported(t *testing.T) {
	r := run(t, variant(t, ct.ListedItemNotExported), false)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.ListedItemMissingFromExport); got.ItemID != ct.ItemListedOnly {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ListedItemMissingFromExport)
}

func TestExportedItemNotListed(t *testing.T) {
	r := run(t, variant(t, ct.ExportedItemNotListed), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.ExportedItemNotListed)
	if got.ItemID != ct.ItemPlainLogin || got.Severity != verify.SeverityUnverified {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ExportedItemNotListed)
}

// Requirement: Organization items outside the scope.

func TestItemsOfOrganizationsThatAreNotChosen(t *testing.T) {
	r := run(t, variant(t, ct.OrganizationItems), false)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 || len(r.Warnings) != 0 {
		t.Errorf("items of organizations that are not chosen produced findings or warnings: %s", dump(t, r))
	}
	out := dump(t, r)
	for _, id := range []string{ct.OrganizationFirst, ct.OrganizationSecond, ct.ItemOrgFirstA, ct.ItemOrgFirstB, ct.ItemOrgSecondA} {
		if strings.Contains(out, id) {
			t.Errorf("result refers to %s", id)
		}
	}
}

func TestExportedItemListedAsOrganizationItem(t *testing.T) {
	r := run(t, listNoteAsOrganizationItem(t), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ItemOrganizationMismatch)
	if got.ItemID != ct.ItemNote || got.Document != string(capture.ItemListDocument) {
		t.Errorf("finding = %+v", got)
	}
	// The item is reported as a contradiction, not as unlisted or missing.
	wantReasons(t, r, verify.ItemOrganizationMismatch)
	if len(r.Warnings) != 0 {
		t.Errorf("warnings: %s", dump(t, r))
	}
}

// listNoteAsOrganizationItem lets the item list show the secure note as an
// item of the first organization. Both exports keep it as personal item.
func listNoteAsOrganizationItem(t *testing.T) *ct.Fixture {
	t.Helper()
	f := ct.Intact(t)
	for _, raw := range f.ItemList {
		if item := raw.(map[string]any); item["id"] == ct.ItemNote {
			item["organizationId"] = ct.OrganizationFirst
		}
	}
	return f
}

func TestDownloadResultForAnOrganizationAttachment(t *testing.T) {
	f := variant(t, ct.OrganizationItems)
	in := f.Input(t)
	in.Downloads = append(in.Downloads, capture.Download{
		ItemID: ct.ItemOrgFirstA, AttachmentID: ct.AttachmentOrg, SHA256: hashOf([]byte("x")), Size: 1,
	})
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Verified)
	got := only(t, r, verify.AttachmentDownloadOutOfScope)
	if got.Severity != verify.SeverityWarning || got.ItemID != ct.ItemOrgFirstA || got.AttachmentID != ct.AttachmentOrg {
		t.Errorf("warning = %+v", got)
	}
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}

	// An attachment ID that the organization item does not show still fails.
	in.Downloads[len(in.Downloads)-1].AttachmentID = "att-not-shown"
	r = runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	only(t, r, verify.AttachmentDownloadNotExpected)
}

// Organization items never appear as missing, also when a reference
// inventory is supplied and the exports are otherwise broken.
func TestOrganizationItemsNeverMissing(t *testing.T) {
	orgItems := map[string]bool{ct.ItemOrgFirstA: true, ct.ItemOrgFirstB: true, ct.ItemOrgSecondA: true}
	for _, names := range [][]string{
		{ct.OrganizationItems},
		{ct.OrganizationItems, ct.MissingItem},
		{ct.OrganizationItems, ct.ExportedItemNotListed},
	} {
		for _, withReference := range []bool{false, true} {
			r := run(t, variant(t, names...), withReference)
			for _, f := range r.Findings {
				if orgItems[f.ItemID] {
					t.Errorf("%v: finding for organization item: %+v", names, f)
				}
			}
		}
	}
}

// Requirement: Reference inventory comparison.

func TestSwappedCredentialWithoutReference(t *testing.T) {
	r := run(t, variant(t, ct.SwappedCredential), false)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}
	hasLimit := false
	for _, l := range r.StandingLimits {
		hasLimit = hasLimit || l == verify.LimitCredentialIdentity
	}
	if !hasLimit {
		t.Errorf("standing limits %v lack %s", r.StandingLimits, verify.LimitCredentialIdentity)
	}
}

func TestSwappedCredentialWithReference(t *testing.T) {
	r := run(t, variant(t, ct.SwappedCredential), true)
	wantVerdict(t, r, verify.Failed)
	wantReasons(t, r, verify.ReferencePasskeyMissing, verify.ReferencePasskeyNotExpected)
	for _, f := range r.Findings {
		if f.ItemID != ct.ItemPasskeyLogin {
			t.Errorf("finding names item %s", f.ItemID)
		}
	}
}

// A well-formed but wrong key with the original credential ID passes
// without a reference and fails with it.
func TestWrongKeyWithReference(t *testing.T) {
	other := ct.VariantByName(t, ct.SwappedCredential).Build(t)
	var otherKey any
	for _, raw := range other.PlaintextItems() {
		if item := raw.(map[string]any); item["id"] == ct.ItemPasskeyLogin {
			otherKey = item["login"].(map[string]any)["fido2Credentials"].([]any)[1].(map[string]any)["keyValue"]
		}
	}
	f := changeSecondPasskey(t, func(p map[string]any) { p["keyValue"] = otherKey })
	wantVerdict(t, run(t, f, false), verify.Verified)
	r := run(t, f, true)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ReferencePasskeyKeyMismatch)
	if got.Passkey == nil || *got.Passkey != 1 {
		t.Errorf("finding = %+v", got)
	}
}

func TestCounterGrewSinceTheReference(t *testing.T) {
	f := changeSecondPasskey(t, func(p map[string]any) { p["counter"] = "7" })
	r := run(t, f, true)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}
}

func TestCounterLowerThanTheReference(t *testing.T) {
	f := changeSecondPasskey(t, func(p map[string]any) { p["counter"] = "5" })
	r := run(t, f, true)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ReferencePasskeyCounterMismatch)
	if got.ItemID != ct.ItemPasskeyLogin || got.Passkey == nil || *got.Passkey != 1 {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ReferencePasskeyCounterMismatch)
}

func TestAttachmentContentDiffersFromReference(t *testing.T) {
	f := ct.Intact(t)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentSecond)
	content := []byte(ct.Marker + " other content of the same length..")
	in.Downloads[i].SHA256, in.Downloads[i].Size = hashOf(content), int64(len(content))

	wantVerdict(t, runInput(t, f, in, false), verify.Verified)
	r := runInput(t, f, in, true)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ReferenceAttachmentMismatch)
	if got.ItemID != ct.ItemPasskeyLogin || got.AttachmentID != ct.AttachmentSecond {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ReferenceAttachmentMismatch)
}

func TestReferenceItemsAndAttachments(t *testing.T) {
	t.Run("item missing from all documents", func(t *testing.T) {
		f := ct.Intact(t)
		f.Reference.Items = append(f.Reference.Items, capture.ReferenceItem{ItemID: "item-known-only-to-reference"})
		r := run(t, f, true)
		wantVerdict(t, r, verify.Failed)
		wantReasons(t, r, verify.ReferenceItemMissing)
	})
	t.Run("item unknown to reference", func(t *testing.T) {
		f := ct.Intact(t)
		var kept []capture.ReferenceItem
		for _, item := range f.Reference.Items {
			if item.ItemID != ct.ItemNote {
				kept = append(kept, item)
			}
		}
		f.Reference.Items = kept
		r := run(t, f, true)
		wantVerdict(t, r, verify.Failed)
		if got := only(t, r, verify.ReferenceItemNotExpected); got.ItemID != ct.ItemNote {
			t.Errorf("finding = %+v", got)
		}
	})
	t.Run("attachment missing from the item list", func(t *testing.T) {
		f := ct.Intact(t)
		for _, raw := range f.ItemList {
			if item := raw.(map[string]any); item["id"] == ct.ItemPasskeyLogin {
				item["attachments"] = item["attachments"].([]any)[:1]
			}
		}
		in := f.Input(t)
		i := downloadIndex(t, in, ct.AttachmentSecond)
		in.Downloads = append(in.Downloads[:i], in.Downloads[i+1:]...)
		// Without a reference the single attachment source hides the loss.
		wantVerdict(t, runInput(t, f, in, false), verify.Verified)
		r := runInput(t, f, in, true)
		wantVerdict(t, r, verify.Failed)
		if got := only(t, r, verify.ReferenceAttachmentMissing); got.AttachmentID != ct.AttachmentSecond {
			t.Errorf("finding = %+v", got)
		}
	})
	t.Run("attachment unknown to reference", func(t *testing.T) {
		f := ct.Intact(t)
		f.Reference.Items[0].Attachments = f.Reference.Items[0].Attachments[:1]
		r := run(t, f, true)
		wantVerdict(t, r, verify.Failed)
		if got := only(t, r, verify.ReferenceAttachmentNotExpected); got.AttachmentID != ct.AttachmentSecond {
			t.Errorf("finding = %+v", got)
		}
	})
}

// Verify changes neither the capture nor the reference.
func TestVerifyDoesNotChangeItsInput(t *testing.T) {
	f := variant(t, ct.SwappedCredential, ct.OrganizationItems)
	c := f.Capture(t)
	before := f.Capture(t)
	ref := f.Clone().Reference
	verify.Verify(c, &f.Reference)
	if !reflect.DeepEqual(c, before) || !reflect.DeepEqual(f.Reference, ref) {
		t.Error("Verify changed its input")
	}
}
