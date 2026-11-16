package verify_test

import (
	"reflect"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// without returns the list elements whose ID differs from id.
func without(items []any, id string) []any {
	out := []any{}
	for _, raw := range items {
		if raw.(map[string]any)["id"] != id {
			out = append(out, raw)
		}
	}
	return out
}

// listEntry returns a copy of the list element with the given ID.
func listEntry(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, raw := range items {
		if m := raw.(map[string]any); m["id"] == id {
			c := make(map[string]any, len(m))
			for k, v := range m {
				c[k] = v
			}
			return c
		}
	}
	t.Fatalf("no list element %s", id)
	return nil
}

// noFindingFor fails when a finding or warning names the item.
func noFindingFor(t *testing.T, r *verify.Result, itemID string) {
	t.Helper()
	for _, list := range [][]verify.Finding{r.Findings, r.Warnings} {
		for _, f := range list {
			if f.ItemID == itemID {
				t.Errorf("finding for item %s: %+v", itemID, f)
			}
		}
	}
}

// Requirement: Default and archived item lists form one listing.

func TestArchivedItemWithAnAttachment(t *testing.T) {
	for _, withReference := range []bool{false, true} {
		r := run(t, ct.Intact(t), withReference)
		wantVerdict(t, r, verify.Verified)
		noFindingFor(t, r, ct.ItemArchivedLogin)
	}
}

func TestArchivedItemInNeitherList(t *testing.T) {
	f := ct.Intact(t)
	f.ArchivedItemList = without(f.ArchivedItemList, ct.ItemArchivedLogin)
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentArchived)
	in.Downloads = append(in.Downloads[:i], in.Downloads[i+1:]...)
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Unverified)
	got := only(t, r, verify.ExportedItemNotListed)
	if got.ItemID != ct.ItemArchivedLogin || got.Severity != verify.SeverityUnverified || got.Document != "" {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ExportedItemNotListed)
	if len(r.Warnings) != 0 {
		t.Errorf("warnings: %s", dump(t, r))
	}
}

func TestDownloadMissingForAnArchivedItem(t *testing.T) {
	r := run(t, variant(t, ct.ArchivedAttachmentDownloadMissing), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.AttachmentDownloadMissing)
	if got.ItemID != ct.ItemArchivedLogin || got.AttachmentID != ct.AttachmentArchived {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.AttachmentDownloadMissing)
}

func TestDuplicateIDWithinTheArchivedItemList(t *testing.T) {
	r := run(t, variant(t, ct.ArchivedListDuplicateID), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DuplicateItemID)
	if got.ItemID != ct.ItemArchivedLogin || got.Document != string(capture.ArchivedItemListDocument) {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.DuplicateItemID)
	if len(find(r, verify.ArchiveFilterNotApplied)) != 0 {
		t.Error("a duplicate within the archived list counts as overlap")
	}
}

func TestDuplicateAttachmentWithinTheArchivedItemList(t *testing.T) {
	f := ct.Intact(t)
	item := f.ArchivedItemList[0].(map[string]any)
	a := item["attachments"].([]any)
	item["attachments"] = append(a, a[0])
	r := run(t, f, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.DuplicateAttachmentID)
	if got.AttachmentID != ct.AttachmentArchived || got.Document != string(capture.ArchivedItemListDocument) {
		t.Errorf("finding = %+v", got)
	}
}

func TestArchivedItemListCannotBeParsed(t *testing.T) {
	for _, name := range []string{ct.ArchivedListInvalidJSON, ct.ArchivedListAbsent} {
		t.Run(name, func(t *testing.T) {
			r := run(t, variant(t, name), false)
			wantVerdict(t, r, verify.Failed)
			got := only(t, r, verify.DocumentInvalidJSON)
			if got.Document != string(capture.ArchivedItemListDocument) {
				t.Errorf("finding names document %q", got.Document)
			}
			wantReasons(t, r, verify.DocumentInvalidJSON)
		})
	}
}

func TestOrganizationItemInBothLists(t *testing.T) {
	f := variant(t, ct.OrganizationItems)
	f.ArchivedItemList = append(f.ArchivedItemList, listEntry(t, f.ItemList, ct.ItemOrgFirstB))
	r := run(t, f, false)
	wantVerdict(t, r, verify.Verified)
	got := only(t, r, verify.OrganizationItemsExcluded)
	want := []verify.OrganizationCount{
		{OrganizationID: ct.OrganizationFirst, Count: 2},
		{OrganizationID: ct.OrganizationSecond, Count: 1},
	}
	if intValue(t, got.Count) != 3 || !reflect.DeepEqual(got.Organizations, want) || got.Document != "" {
		t.Errorf("warning = %+v", got)
	}
	if intValue(t, only(t, r, verify.ArchiveFilterNotApplied).Count) != 1 {
		t.Errorf("result: %s", dump(t, r))
	}
}

// An organization item that only the archived list shows is counted, and
// an exported item that the archived list shows as an organization item is
// reported against the archived list.
func TestOrganizationItemOnlyInTheArchivedList(t *testing.T) {
	f := ct.Intact(t)
	f.ArchivedItemList[0].(map[string]any)["organizationId"] = ct.OrganizationFirst
	in := f.Input(t)
	r := runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ItemOrganizationMismatch)
	if got.ItemID != ct.ItemArchivedLogin || got.Document != string(capture.ArchivedItemListDocument) {
		t.Errorf("finding = %+v", got)
	}
	if intValue(t, only(t, r, verify.OrganizationItemsExcluded).Count) != 1 {
		t.Errorf("result: %s", dump(t, r))
	}
}

// Requirement: Item shown by both lists must agree.

func TestSameItemWithTheSameAttachments(t *testing.T) {
	f := ct.Intact(t)
	f.ArchivedItemList = append(f.ArchivedItemList, listEntry(t, f.ItemList, ct.ItemPasskeyLogin))
	r := run(t, f, false)
	wantVerdict(t, r, verify.Verified)
	noFindingFor(t, r, ct.ItemPasskeyLogin)
	if len(r.Findings) != 0 {
		t.Errorf("findings: %s", dump(t, r))
	}

	// One download result per attachment is expected, not one per entry.
	in := f.Input(t)
	i := downloadIndex(t, in, ct.AttachmentSecond)
	in.Downloads = append(in.Downloads[:i], in.Downloads[i+1:]...)
	r = runInput(t, f, in, false)
	wantVerdict(t, r, verify.Failed)
	if got := only(t, r, verify.AttachmentDownloadMissing); got.AttachmentID != ct.AttachmentSecond {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.AttachmentDownloadMissing)

	// Duplicate download results for the shared attachments are still
	// duplicates.
	in = f.Input(t)
	in.Downloads = append(in.Downloads, in.Downloads[downloadIndex(t, in, ct.AttachmentFirst)])
	wantReasons(t, runInput(t, f, in, false), verify.DuplicateDownloadResult)
}

func TestSameAttachmentsInADifferentOrder(t *testing.T) {
	f := ct.Intact(t)
	entry := listEntry(t, f.ItemList, ct.ItemPasskeyLogin)
	a := entry["attachments"].([]any)
	entry["attachments"] = []any{a[1], a[0]}
	f.ArchivedItemList = append(f.ArchivedItemList, entry)
	r := run(t, f, true)
	wantVerdict(t, r, verify.Verified)
	if len(find(r, verify.ListedItemDiffersBetweenLists)) != 0 || len(r.Findings) != 0 {
		t.Errorf("result: %s", dump(t, r))
	}
}

func TestSameItemWithDifferentAttachments(t *testing.T) {
	r := run(t, variant(t, ct.ListsDisagreeOnAttachments), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ListedItemDiffersBetweenLists)
	if got.ItemID != ct.ItemPasskeyLogin || got.Severity != verify.SeverityFailed || got.Document != "" {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ListedItemDiffersBetweenLists)

	// An attachment that only the archived entry names differs as well.
	f := ct.Intact(t)
	entry := listEntry(t, f.ItemList, ct.ItemNote)
	entry["attachments"] = []any{map[string]any{"id": "att-only-in-archived-entry"}}
	f.ArchivedItemList = append(f.ArchivedItemList, entry)
	r = run(t, f, false)
	if got := only(t, r, verify.ListedItemDiffersBetweenLists); got.ItemID != ct.ItemNote {
		t.Errorf("finding = %+v", got)
	}
}

func TestSameItemWithADifferentOrganization(t *testing.T) {
	r := run(t, variant(t, ct.ListsDisagreeOnOrganization), false)
	wantVerdict(t, r, verify.Failed)
	got := only(t, r, verify.ListedItemDiffersBetweenLists)
	if got.ItemID != ct.ItemNote || got.Severity != verify.SeverityFailed {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ListedItemDiffersBetweenLists)
}

// Requirement: Archive filter that did not apply is reported.

func TestArchivedListRepeatsTheDefaultList(t *testing.T) {
	for _, withReference := range []bool{false, true} {
		r := run(t, variant(t, ct.ArchiveFilterIgnoredNoArchivedItem), withReference)
		wantVerdict(t, r, verify.Verified)
		got := only(t, r, verify.ArchiveFilterNotApplied)
		if got.Severity != verify.SeverityWarning || intValue(t, got.Count) != 3 || got.ItemID != "" {
			t.Errorf("warning = %+v", got)
		}
		if len(r.Findings) != 0 || len(r.Warnings) != 1 {
			t.Errorf("result: %s", dump(t, r))
		}
	}
}

func TestDisjointLists(t *testing.T) {
	r := run(t, ct.Intact(t), false)
	if len(find(r, verify.ArchiveFilterNotApplied)) != 0 {
		t.Errorf("result: %s", dump(t, r))
	}
}

func TestBothListsEmpty(t *testing.T) {
	f := ct.Intact(t)
	f.PlaintextExport["items"] = []any{}
	f.EncryptedExport["items"] = []any{}
	f.ItemList = []any{}
	f.ArchivedItemList = []any{}
	f.Attachments = nil
	f.Reference.Items = nil
	in := f.Input(t)
	if string(in.ItemList) != "[]" || string(in.ArchivedItemList) != "[]" {
		t.Fatalf("lists render as %q and %q", in.ItemList, in.ArchivedItemList)
	}
	for _, withReference := range []bool{false, true} {
		r := runInput(t, f, in, withReference)
		wantVerdict(t, r, verify.Verified)
		if len(r.Findings) != 0 || len(r.Warnings) != 0 {
			t.Errorf("result: %s", dump(t, r))
		}
	}
}

// The warning leaves the verdict as the other findings set it.
func TestArchiveFilterWarningLeavesTheVerdict(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  verify.Verdict
	}{
		{[]string{ct.MissingItem, ct.ArchiveFilterIgnored}, verify.Failed},
		{[]string{ct.ArchiveFilterIgnored}, verify.Unverified},
		{[]string{ct.ArchiveFilterIgnoredNoArchivedItem}, verify.Verified},
	} {
		r := run(t, variant(t, tc.names...), false)
		wantVerdict(t, r, tc.want)
		only(t, r, verify.ArchiveFilterNotApplied)
	}

	// The ignored filter with an archived item in the vault: the item is in
	// neither list and stays unverified.
	r := run(t, variant(t, ct.ArchiveFilterIgnored), false)
	if got := only(t, r, verify.ExportedItemNotListed); got.ItemID != ct.ItemArchivedLogin {
		t.Errorf("finding = %+v", got)
	}
	wantReasons(t, r, verify.ExportedItemNotListed)
}
