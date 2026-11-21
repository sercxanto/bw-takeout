package verify_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// Tests of the chosen organizations (design D5 of the change
// organization-export). The fixtures are those of design D13.

const (
	orgPlainDoc = string(capture.OrganizationPlaintextExportDocument)
	orgSealDoc  = string(capture.OrganizationEncryptedExportDocument)
)

// wantOrganization fails unless the finding names the chosen organization.
func wantOrganization(t *testing.T, f verify.Finding) {
	t.Helper()
	if f.OrganizationID != ct.ChosenOrganization {
		t.Errorf("finding %+v does not name organization %s", f, ct.ChosenOrganization)
	}
}

// Every variant of the intact organization fixture gives the verdict and
// the findings it models, and every finding names the organization except
// the personal finding the variant also causes.
func TestOrganizationVariants(t *testing.T) {
	for _, tc := range []struct {
		name    string
		verdict verify.Verdict
		reasons []verify.Reason
	}{
		{ct.OrgItemMissingFromPlaintextExport, verify.Failed, []verify.Reason{verify.ItemMissingFromPlaintextExport, verify.ListedItemMissingFromExport}},
		{ct.OrgExportHoldsOtherOrganization, verify.Failed, []verify.Reason{verify.OrganizationItemMismatch}},
		{ct.OrgItemListedAsPersonal, verify.Failed, []verify.Reason{verify.ListedItemMissingFromExport, verify.OrganizationItemMismatch}},
		{ct.OrgRevisionDiffersBetweenExports, verify.Failed, []verify.Reason{verify.OrganizationChangedDuringCapture}},
		{ct.OrgRevisionDiffersFromList, verify.Failed, []verify.Reason{verify.OrganizationChangedDuringCapture}},
		{ct.OrgListedItemNotExported, verify.Failed, []verify.Reason{verify.ListedItemMissingFromExport}},
		{ct.OrgExportedItemNotListed, verify.Unverified, []verify.Reason{verify.ExportedItemNotListed}},
		{ct.OrgBlobLoginItem, verify.Unverified, []verify.Reason{verify.PasskeyCountUnverifiable}},
		{ct.OrgEmptyPasskeyKey, verify.Failed, []verify.Reason{verify.PasskeyFieldMissing}},
		{ct.OrgPasswordProtectedExport, verify.Failed, []verify.Reason{verify.DocumentPasswordProtected}},
		{ct.OrgDocumentAbsent, verify.Failed, []verify.Reason{verify.DocumentInvalidJSON}},
		{ct.OrgDecryptionErrorLines, verify.Unverified, []verify.Reason{verify.DocumentDecryptionErrors}},
		{ct.OrgAttachmentDownloadMissing, verify.Failed, []verify.Reason{verify.AttachmentDownloadMissing}},
		{ct.OrgNotChosenItems, verify.Verified, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, variant(t, tc.name), false)
			wantVerdict(t, r, tc.verdict)
			want := append([]verify.Reason(nil), tc.reasons...)
			sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
			var got []verify.Reason
			for _, f := range r.Findings {
				got = append(got, f.Reason)
			}
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("reasons %v, want %v; result: %s", got, want, dump(t, r))
			}
			if len(r.Warnings) != 0 {
				t.Errorf("warnings: %s", dump(t, r))
			}
			for _, f := range r.Findings {
				personal := tc.name == ct.OrgItemListedAsPersonal && f.Reason == verify.ListedItemMissingFromExport
				// The attachment accounting is one set for every scope.
				if !personal && f.Reason != verify.AttachmentDownloadMissing {
					wantOrganization(t, f)
				}
			}
		})
	}
	var all []string
	for _, v := range ct.OrganizationVariants() {
		all = append(all, v.Name)
	}
	if len(all) != 14 {
		t.Errorf("%d organization variants, the table above covers 14", len(all))
	}
}

// Requirement: Organization exports are validated as documents.

func TestPasswordProtectedOrganizationExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgPasswordProtectedExport), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DocumentPasswordProtected)
	if got.Document != orgSealDoc {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
	// The personal checks still ran: the personal capture is intact.
	wantReasons(t, r, verify.DocumentPasswordProtected)
}

func TestOrganizationExportAbsent(t *testing.T) {
	r := run(t, variant(t, ct.OrgDocumentAbsent), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DocumentInvalidJSON)
	if got.Document != orgPlainDoc {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// A rejected personal document still ends the comparisons, and a rejected
// organization document is reported with it.
func TestRejectedPersonalAndOrganizationDocuments(t *testing.T) {
	f := variant(t, ct.OrgDocumentAbsent)
	in := f.Input(t)
	in.ItemList = in.ItemList[:len(in.ItemList)/2]
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	wantReasons(t, r, verify.DocumentInvalidJSON, verify.DocumentInvalidJSON)
	if r.Findings[0].OrganizationID != "" || r.Findings[1].OrganizationID != ct.ChosenOrganization {
		t.Errorf("findings %s", dump(t, r))
	}
}

// Requirement: Organization exports are compared like the personal exports.

func TestItemMissingFromThePlaintextOrganizationExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgItemMissingFromPlaintextExport), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ItemMissingFromPlaintextExport)
	if got.ItemID != ct.ItemOrgNote || got.Document != orgPlainDoc {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestBlobFormatItemInAnOrganizationExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgBlobLoginItem), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.PasskeyCountUnverifiable)
	if got.ItemID != ct.ItemOrgLogin {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestPasskeyWithAnEmptyKeyInAnOrganizationExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgEmptyPasskeyKey), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.PasskeyFieldMissing)
	if got.ItemID != ct.ItemOrgLogin || got.Field != "keyValue" || intValue(t, got.Passkey) != 0 {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestIntactOrganization(t *testing.T) {
	for _, withReference := range []bool{false, true} {
		r := run(t, ct.IntactOrganization(t), withReference)
		wantVerdict(t, r, verify.Verified)
		if len(r.Findings) != 0 || len(r.Warnings) != 0 {
			t.Errorf("reference %v: %s", withReference, dump(t, r))
		}
	}
}

// A duplicate item ID in an organization export is reported against that
// export.
func TestDuplicateIDInAnOrganizationExport(t *testing.T) {
	f := ct.IntactOrganization(t)
	o := f.Organization(t, ct.ChosenOrganization)
	o.EncryptedExport["items"] = append(o.EncryptedItems(), o.EncryptedItems()[1])
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DuplicateItemID)
	if got.Document != orgSealDoc || got.ItemID != ct.ItemOrgNote {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// Requirement: Organization items belong to their organization.

func TestItemOfAnotherOrganizationInTheExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgExportHoldsOtherOrganization), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.OrganizationItemMismatch)
	if got.ItemID != ct.ItemOrgNote || got.Document != orgPlainDoc {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestExportedItemListedAsPersonalItem(t *testing.T) {
	r := run(t, variant(t, ct.OrgItemListedAsPersonal), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.OrganizationItemMismatch)
	if got.ItemID != ct.ItemOrgNote || got.Document != string(capture.ItemListDocument) {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// An item of an organization export without an organization ID is not an
// item of that organization either.
func TestOrganizationExportItemWithoutOrganization(t *testing.T) {
	f := ct.IntactOrganization(t)
	for _, raw := range f.Organization(t, ct.ChosenOrganization).EncryptedItems() {
		if item := raw.(map[string]any); item["id"] == ct.ItemOrgLogin {
			delete(item, "organizationId")
		}
	}
	r := run(t, f, false)
	got := only(t, r, verify.OrganizationItemMismatch)
	if got.ItemID != ct.ItemOrgLogin || got.Document != orgSealDoc {
		t.Errorf("finding %+v", got)
	}
}

// Requirement: Organization exports agree on revisions.

func TestItemChangedBetweenTheTwoExports(t *testing.T) {
	r := run(t, variant(t, ct.OrgRevisionDiffersBetweenExports), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.OrganizationChangedDuringCapture)
	if got.ItemID != ct.ItemOrgNote || got.Severity != verify.SeverityFailed {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestSameRevisions(t *testing.T) {
	r := run(t, ct.IntactOrganization(t), false)
	if got := find(r, verify.OrganizationChangedDuringCapture); len(got) != 0 {
		t.Errorf("revision findings %+v", got)
	}
}

// Revision dates are compared as instants at millisecond precision, so a
// different spelling of the same time is no change.
func TestRevisionDatesComparedAtMillisecondPrecision(t *testing.T) {
	for value, changed := range map[string]bool{
		"2026-02-03T04:05:06Z":           false,
		"2026-02-03T04:05:06.000400Z":    false,
		"2026-02-03T05:05:06.000+01:00":  false,
		"2026-02-03T04:05:06.001Z":       true,
		"2026-02-03T04:05:05.999999999Z": true,
		"2026-02-03T04:05:06.000000001Z": false,
	} {
		f := ct.IntactOrganization(t)
		for _, raw := range f.Organization(t, ct.ChosenOrganization).EncryptedItems() {
			if item := raw.(map[string]any); item["id"] == ct.ItemOrgNote {
				item["revisionDate"] = value
			}
		}
		r := run(t, f, false)
		if got := len(find(r, verify.OrganizationChangedDuringCapture)) == 1; got != changed {
			t.Errorf("%s: changed %v, want %v; result: %s", value, got, changed, dump(t, r))
		}
		if !changed {
			wantVerdict(t, r, verify.Verified)
		}
	}
}

// Every item of an organization export carries a revision date, so an item
// without one, or with one that is not a time, makes its document
// malformed.
func TestOrganizationItemWithoutARevisionDate(t *testing.T) {
	for _, value := range []any{nil, "yesterday", 5} {
		f := ct.IntactOrganization(t)
		for _, raw := range f.Organization(t, ct.ChosenOrganization).PlaintextItems() {
			if item := raw.(map[string]any); item["id"] == ct.ItemOrgLogin {
				if value == nil {
					delete(item, "revisionDate")
				} else {
					item["revisionDate"] = value
				}
			}
		}
		r := run(t, f, false)
		wantVerdict(t, r, verify.Failed)
		got := only(t, r, verify.DocumentMalformedItem)
		if got.Document != orgPlainDoc || got.ItemID != ct.ItemOrgLogin {
			t.Errorf("%v: finding %+v", value, got)
		}
		wantOrganization(t, got)
		wantReasons(t, r, verify.DocumentMalformedItem)
	}
}

// A personal item without a revision date is judged as before.
func TestPersonalItemWithoutARevisionDate(t *testing.T) {
	f := ct.IntactOrganization(t)
	for _, items := range [][]any{f.PlaintextItems(), f.EncryptedItems(), f.ItemList} {
		for _, raw := range items {
			delete(raw.(map[string]any), "revisionDate")
			if raw.(map[string]any)["organizationId"] == ct.ChosenOrganization {
				raw.(map[string]any)["revisionDate"] = ct.RevisionDate
			}
		}
	}
	wantVerdict(t, run(t, f, false), verify.Verified)
}

// Requirement: Items of chosen organizations are matched with the item
// lists.

func TestListedOrganizationItemNotExported(t *testing.T) {
	r := run(t, variant(t, ct.OrgListedItemNotExported), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ListedItemMissingFromExport)
	if got.ItemID != ct.ItemOrgListedOnly || got.Document != orgPlainDoc {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestExportedOrganizationItemNotListed(t *testing.T) {
	r := run(t, variant(t, ct.OrgExportedItemNotListed), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.ExportedItemNotListed)
	if got.ItemID != ct.ItemOrgNote || got.Severity != verify.SeverityUnverified {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

func TestItemChangedAfterTheSnapshot(t *testing.T) {
	r := run(t, variant(t, ct.OrgRevisionDiffersFromList), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.OrganizationChangedDuringCapture)
	if got.ItemID != ct.ItemOrgNote {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// A listed item of a chosen organization without a parsable revision date
// makes its list malformed.
func TestListedOrganizationItemWithoutARevisionDate(t *testing.T) {
	f := ct.IntactOrganization(t)
	for _, raw := range f.ItemList {
		if item := raw.(map[string]any); item["id"] == ct.ItemOrgNote {
			delete(item, "revisionDate")
		}
	}
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DocumentMalformedItem)
	if got.Document != string(capture.ItemListDocument) || got.ItemID != ct.ItemOrgNote {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// Requirement: Organization items outside the scope.

// The intact organization fixture with items of two other organizations:
// they give no finding and no warning, and their attachment is not
// expected.
func TestOrganizationNotChosenWithAChosenOne(t *testing.T) {
	r := run(t, variant(t, ct.OrgNotChosenItems), true)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 || len(r.Warnings) != 0 {
		t.Errorf("result: %s", dump(t, r))
	}
}

func TestDownloadResultForAnAttachmentOutsideTheScope(t *testing.T) {
	f := variant(t, ct.OrgNotChosenItems)
	in := f.Input(t)
	in.Downloads = append(in.Downloads, capture.Download{
		ItemID: ct.ItemUnchosenA, AttachmentID: ct.AttachmentUnchosen, SHA256: hashOf([]byte("x")), Size: 1,
	})
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Verified)
	got := only(t, r, verify.AttachmentDownloadOutOfScope)
	if got.ItemID != ct.ItemUnchosenA || got.AttachmentID != ct.AttachmentUnchosen {
		t.Errorf("warning %+v", got)
	}
}

// Requirement: Scope statement and standing limits.

func TestScopeWithAChosenOrganization(t *testing.T) {
	r := run(t, ct.IntactOrganization(t), false)
	if r.Scope.Code != verify.ScopeCodeWithOrganizations || r.Scope.Statement != verify.ScopeStatementWithOrganizations ||
		!reflect.DeepEqual(r.Scope.Organizations, []string{ct.ChosenOrganization}) {
		t.Errorf("scope %+v", r.Scope)
	}
	want := []verify.Limit{
		verify.LimitAttachmentContent, verify.LimitAttachmentInventory, verify.LimitCredentialIdentity,
		verify.LimitOrganizationExportsSeparateReads, verify.LimitServerCompleteness,
	}
	if !reflect.DeepEqual(r.StandingLimits, want) {
		t.Errorf("standing limits %v, want %v", r.StandingLimits, want)
	}
	r = run(t, ct.IntactOrganization(t), true)
	want = []verify.Limit{verify.LimitOrganizationExportsSeparateReads, verify.LimitServerCompleteness}
	if !reflect.DeepEqual(r.StandingLimits, want) {
		t.Errorf("standing limits with reference %v, want %v", r.StandingLimits, want)
	}
}

func TestNoOrganizationChosen(t *testing.T) {
	r := run(t, ct.Intact(t), false)
	if r.Scope.Code != verify.ScopeCode || r.Scope.Statement != verify.ScopeStatement || r.Scope.Organizations != nil {
		t.Errorf("scope %+v", r.Scope)
	}
	for _, l := range r.StandingLimits {
		if l == verify.LimitOrganizationExportsSeparateReads {
			t.Errorf("standing limits %v", r.StandingLimits)
		}
	}
	out := dump(t, r)
	for _, key := range []string{`"organizations"`, `"organization_id"`} {
		if strings.Contains(out, key) {
			t.Errorf("result without organization holds %s", key)
		}
	}
}

// Requirement: Attachment accounting by ID.

func TestResultMissingForAChosenOrganization(t *testing.T) {
	r := run(t, variant(t, ct.OrgAttachmentDownloadMissing), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.AttachmentDownloadMissing)
	if got.ItemID != ct.ItemOrgLogin || got.AttachmentID != ct.AttachmentOrgLogin {
		t.Errorf("finding %+v", got)
	}
}

// Requirement: Reference inventory comparison.

func TestOrganizationItemMissingFromTheCapture(t *testing.T) {
	r := run(t, variant(t, ct.OrgItemMissingFromPlaintextExport), true)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.ReferenceItemMissing); got.ItemID != ct.ItemOrgNote {
		t.Errorf("finding %+v", got)
	}
}

// The reference comparison covers the organization items: a swapped
// passkey, a changed attachment and an item the reference does not hold.
func TestReferenceCoversOrganizationItems(t *testing.T) {
	f := ct.IntactOrganization(t)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentOrgLogin)
	in.Downloads[i].SHA256 = hashOf([]byte("other content"))
	r := runInput(t, f, in, true)
	if got := only(t, r, verify.ReferenceAttachmentMismatch); got.ItemID != ct.ItemOrgLogin {
		t.Errorf("finding %+v", got)
	}

	f = ct.IntactOrganization(t)
	f.Reference.Items = f.Reference.Items[:len(f.Reference.Items)-1]
	r = run(t, f, true)
	if got := only(t, r, verify.ReferenceItemNotExpected); got.ItemID != ct.ItemOrgNote {
		t.Errorf("finding %+v", got)
	}

	f = ct.IntactOrganization(t)
	for i := range f.Reference.Items {
		if f.Reference.Items[i].ItemID == ct.ItemOrgLogin {
			f.Reference.Items[i].Passkeys[0].KeySHA256 = hashOf([]byte("other key"))
		}
	}
	r = run(t, f, true)
	if got := only(t, r, verify.ReferencePasskeyKeyMismatch); got.ItemID != ct.ItemOrgLogin {
		t.Errorf("finding %+v", got)
	}
}

// Requirement: Attributable findings.

func TestFindingAboutAnOrganizationExport(t *testing.T) {
	f := ct.IntactOrganization(t)
	for _, raw := range f.Organization(t, ct.ChosenOrganization).PlaintextItems() {
		if item := raw.(map[string]any); item["id"] == ct.ItemOrgLogin {
			item["login"].(map[string]any)["fido2Credentials"] = []any{}
		}
	}
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.PasskeyCountMismatch)
	if got.ItemID != ct.ItemOrgLogin || intValue(t, got.Expected) != 1 || intValue(t, got.Found) != 0 {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// Findings sort by organization ID first, personal findings before those
// of an organization.
func TestFindingsSortByOrganization(t *testing.T) {
	f := variant(t, ct.OrgItemListedAsPersonal)
	r := run(t, f, false)
	if len(r.Findings) != 2 || r.Findings[0].OrganizationID != "" || r.Findings[1].OrganizationID != ct.ChosenOrganization {
		t.Errorf("findings %s", dump(t, r))
	}
}

// Requirement: Default and archived item lists form one listing.

func TestOrganizationItemInBothLists(t *testing.T) {
	f := ct.IntactOrganization(t)
	f.ArchivedItemList = append(f.ArchivedItemList, listEntry(t, f.ItemList, ct.ItemOrgLogin))
	in := f.Input(t)
	r := runInput(t, f, in, true)
	wantVerdict(t, r, verify.Verified)
	if len(r.Findings) != 0 {
		t.Errorf("findings %s", dump(t, r))
	}
	// The only warning is the shared item of the two lists.
	if intValue(t, only(t, r, verify.ArchiveFilterNotApplied).Count) != 1 || len(r.Warnings) != 1 {
		t.Errorf("warnings %s", dump(t, r))
	}
	expected := capture.ScopedAttachments([]string{ct.ChosenOrganization}, capture.Parse(in).List, capture.Parse(in).ArchivedList)
	n := 0
	for _, ref := range expected {
		if ref.ItemID == ct.ItemOrgLogin {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d expected downloads for the organization login, want 1", n)
	}
}

// Requirement: Decryption error lines are reported.

func TestDecryptionErrorsOnAnOrganizationExport(t *testing.T) {
	r := run(t, variant(t, ct.OrgDecryptionErrorLines), false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.DocumentDecryptionErrors)
	if got.Document != orgPlainDoc || intValue(t, got.Count) != 2 {
		t.Errorf("finding %+v", got)
	}
	wantOrganization(t, got)
}

// A rejected organization document skips only its organization: the
// personal checks still run and report their own deviation.
func TestRejectedOrganizationDocumentKeepsThePersonalChecks(t *testing.T) {
	f := variant(t, ct.OrgDocumentAbsent)
	ct.VariantByName(t, ct.OnePasskeyMissing).Apply(t, f)
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	wantReasons(t, r, verify.PasskeyCountMismatch, verify.DocumentInvalidJSON)
	if got := only(t, r, verify.PasskeyCountMismatch); got.ItemID != ct.ItemPasskeyLogin || got.OrganizationID != "" {
		t.Errorf("finding %+v", got)
	}
}

// Without the organization as first sort key, the organization document
// would sort before the personal one by its name.
func TestFindingsSortByOrganizationBeforeDocument(t *testing.T) {
	f := variant(t, ct.OrgDecryptionErrorLines)
	f.DecryptionErrors.PlaintextExport = 3
	r := run(t, f, false)
	if len(r.Findings) != 2 || r.Findings[0].Document != string(capture.PlaintextExportDocument) ||
		r.Findings[1].Document != orgPlainDoc {
		t.Errorf("findings %s", dump(t, r))
	}
}
