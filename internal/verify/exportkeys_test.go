package verify_test

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// The tests of this file cover the requirements "Export keys unknown to
// this project are reported", "Item types unknown to this project are
// reported" and "Reported key paths hold no content" of the change
// bw-version-2026-11 (design D6).

var exportKeyReasons = []verify.Reason{verify.ExportKeyUnknown, verify.ExportKeyUnknownOmitted, verify.ExportItemTypeUnknown}

func itemOf(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, raw := range items {
		if m, ok := raw.(map[string]any); ok && m["id"] == id {
			return m
		}
	}
	t.Fatalf("no item %s", id)
	return nil
}

// keyWarnings returns the warnings of the three new reasons.
func keyWarnings(r *verify.Result) []verify.Finding {
	var out []verify.Finding
	for _, reason := range exportKeyReasons {
		out = append(out, find(r, reason)...)
	}
	return out
}

// sameVerdict fails unless the two results have the same verdict and the
// same findings. The warnings may differ.
func sameVerdict(t *testing.T, base, changed *verify.Result) {
	t.Helper()
	if base.Verdict != changed.Verdict || !reflect.DeepEqual(base.Findings, changed.Findings) {
		t.Errorf("verdict changed from %s to %s; base: %s\nchanged: %s", base.Verdict, changed.Verdict, dump(t, base), dump(t, changed))
	}
	for _, w := range changed.Warnings {
		if w.Severity != verify.SeverityWarning {
			t.Errorf("warning %s with severity %s", w.Reason, w.Severity)
		}
	}
}

func wantKeyWarning(t *testing.T, got verify.Finding, document, org, path string, count int) {
	t.Helper()
	if got.Severity != verify.SeverityWarning || got.Reason != verify.ExportKeyUnknown || got.Document != document ||
		got.OrganizationID != org || got.Field != path || got.Count == nil || *got.Count != count ||
		got.ItemID != "" || got.ItemType != nil {
		t.Errorf("warning %+v, want %s %s %q count %d", got, document, org, path, count)
	}
}

// Scenario "Unknown key on an item".
func TestUnknownKeyOnAnItem(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	itemOf(t, f.PlaintextItems(), ct.ItemNote)["newKey"] = "value"
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "plaintext_export", "", "items[].newKey", 1)
	wantVerdict(t, r, verify.Verified)
	sameVerdict(t, base, r)
}

// Scenario "Same unknown key on several items".
func TestSameUnknownKeyOnSeveralItems(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	for _, id := range []string{ct.ItemPasskeyLogin, ct.ItemPlainLogin, ct.ItemNote} {
		itemOf(t, f.EncryptedItems(), id)["newKey"] = "2.a|b|c"
	}
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "encrypted_export", "", "items[].newKey", 3)
	sameVerdict(t, base, r)
}

// Scenario "Unknown key inside a passkey of an organization export".
func TestUnknownKeyInsideAPasskeyOfAnOrganizationExport(t *testing.T) {
	base := run(t, ct.IntactOrganization(t), false)
	f := ct.IntactOrganization(t)
	login := itemOf(t, f.Organization(t, ct.ChosenOrganization).PlaintextItems(), ct.ItemOrgLogin)["login"].(map[string]any)
	login["fido2Credentials"].([]any)[0].(map[string]any)["newPasskeyKey"] = "value"
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "organization_plaintext_export", ct.ChosenOrganization,
		"items[].login.fido2Credentials[].newPasskeyKey", 1)
	wantVerdict(t, r, verify.Verified)
	sameVerdict(t, base, r)
}

// Scenario "Unknown key at document level".
func TestUnknownKeyAtDocumentLevel(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	f.PlaintextExport["newTopKey"] = map[string]any{"inner": 1}
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "plaintext_export", "", "newTopKey", 1)
	sameVerdict(t, base, r)
}

// Scenario "Keys within an unknown value".
func TestKeysWithinAnUnknownValue(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	itemOf(t, f.PlaintextItems(), ct.ItemNote)["newObject"] = map[string]any{
		"innerKey": 1, "deeper": map[string]any{"innermostKey": 2},
	}
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "plaintext_export", "", "items[].newObject", 1)
	out := dump(t, r)
	for _, inner := range []string{"innerKey", "deeper", "innermostKey"} {
		if strings.Contains(out, inner) {
			t.Errorf("result names %s inside the unknown value", inner)
		}
	}
	sameVerdict(t, base, r)
}

// Scenario "Known keys only": the intact captures, also with a blob item
// that carries data next to legacy items that carry key.
func TestKnownKeysOnly(t *testing.T) {
	for _, f := range []*ct.Fixture{ct.Intact(t), ct.IntactOrganization(t), variant(t, ct.BlobLoginItem), variant(t, ct.OrgBlobLoginItem)} {
		r := run(t, f, false)
		if got := keyWarnings(r); len(got) != 0 {
			t.Errorf("warnings %+v", got)
		}
	}
}

// Scenario "Known key absent".
func TestKnownKeyAbsent(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	item := itemOf(t, f.PlaintextItems(), ct.ItemNote)
	for _, k := range []string{"notes", "favorite", "fields", "reprompt"} {
		delete(item, k)
	}
	r := run(t, f, false)
	if got := keyWarnings(r); len(got) != 0 {
		t.Errorf("warnings %+v", got)
	}
	sameVerdict(t, base, r)
}

// Scenario "New item type": the warning names the plaintext export, and
// the encrypted export reports the two items as unrecognized.
func TestNewItemType(t *testing.T) {
	encryptedOnly := ct.Intact(t)
	both := ct.Intact(t)
	for _, id := range []string{ct.ItemPlainLogin, ct.ItemNote} {
		itemOf(t, encryptedOnly.EncryptedItems(), id)["type"] = 9
		itemOf(t, both.EncryptedItems(), id)["type"] = 9
		itemOf(t, both.PlaintextItems(), id)["type"] = 9
	}
	base := run(t, encryptedOnly, false)
	r := run(t, both, false)
	got := only(t, r, verify.ExportItemTypeUnknown)
	if got.Severity != verify.SeverityWarning || got.Document != "plaintext_export" || got.OrganizationID != "" ||
		got.ItemType == nil || *got.ItemType != 9 || got.Count == nil || *got.Count != 2 || got.Field != "" {
		t.Errorf("warning %+v", got)
	}
	unrecognized := find(r, verify.ItemFormatUnrecognized)
	var ids []string
	for _, f := range unrecognized {
		ids = append(ids, f.ItemID)
	}
	sort.Strings(ids)
	if want := []string{ct.ItemPlainLogin, ct.ItemNote}; !reflect.DeepEqual(ids, want) {
		t.Errorf("unrecognized items %v, want %v", ids, want)
	}
	wantVerdict(t, r, verify.Unverified)
	sameVerdict(t, base, r)
}

// Scenario "New item type in an organization export".
func TestNewItemTypeInAnOrganizationExport(t *testing.T) {
	base := run(t, ct.IntactOrganization(t), false)
	f := ct.IntactOrganization(t)
	itemOf(t, f.Organization(t, ct.ChosenOrganization).PlaintextItems(), ct.ItemOrgNote)["type"] = 9
	r := run(t, f, false)
	got := only(t, r, verify.ExportItemTypeUnknown)
	if got.Document != "organization_plaintext_export" || got.OrganizationID != ct.ChosenOrganization ||
		got.ItemType == nil || *got.ItemType != 9 || got.Count == nil || *got.Count != 1 {
		t.Errorf("warning %+v", got)
	}
	sameVerdict(t, base, r)
}

// Scenario "Known item types".
func TestKnownItemTypes(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	for typ := 1; typ <= 8; typ++ {
		f := ct.Intact(t)
		itemOf(t, f.PlaintextItems(), ct.ItemNote)["type"] = typ
		r := run(t, f, false)
		if got := keyWarnings(r); len(got) != 0 {
			t.Errorf("type %d: warnings %+v", typ, got)
		}
		sameVerdict(t, base, r)
	}
}

// Scenario "Unknown key that is not an identifier".
func TestUnknownKeyThatIsNotAnIdentifier(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	sentinelKey := ct.Marker + " key of a user @ example.invalid"
	f := ct.Intact(t)
	itemOf(t, f.PlaintextItems(), ct.ItemNote)[sentinelKey] = ct.Marker + " its value"
	r := run(t, f, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "plaintext_export", "", "items[]."+capture.OtherKey, 1)
	out := dump(t, r)
	if strings.Contains(out, sentinelKey) || strings.Contains(out, ct.Marker) || strings.Contains(out, "@") {
		t.Errorf("serialized result holds the key text:\n%s", out)
	}
	sameVerdict(t, base, r)
}

// Scenario "Many distinct unknown keys": 32 paths, the first in byte order,
// and one warning for the 8 left out.
func TestManyDistinctUnknownKeys(t *testing.T) {
	base := run(t, ct.Intact(t), false)
	f := ct.Intact(t)
	var paths []string
	for i := 0; i < 40; i++ {
		k := fmt.Sprintf("newKey%02d", 39-i)
		f.PlaintextExport[k] = i
		paths = append(paths, k)
	}
	sort.Strings(paths)
	r := run(t, f, false)
	got := find(r, verify.ExportKeyUnknown)
	if len(got) != 32 {
		t.Fatalf("%d key warnings, want 32", len(got))
	}
	for i, w := range got {
		wantKeyWarning(t, w, "plaintext_export", "", paths[i], 1)
	}
	omitted := only(t, r, verify.ExportKeyUnknownOmitted)
	if omitted.Severity != verify.SeverityWarning || omitted.Document != "plaintext_export" ||
		omitted.Count == nil || *omitted.Count != 8 || omitted.Field != "" {
		t.Errorf("omitted warning %+v", omitted)
	}
	sameVerdict(t, base, r)
}

// The cap applies per document: 33 paths in each export give one omitted
// warning each.
func TestUnknownKeyCapPerDocument(t *testing.T) {
	f := ct.Intact(t)
	for i := 0; i < 33; i++ {
		f.PlaintextExport[fmt.Sprintf("plainKey%02d", i)] = 1
		f.EncryptedExport[fmt.Sprintf("sealedKey%02d", i)] = 1
	}
	r := run(t, f, false)
	if n := len(find(r, verify.ExportKeyUnknown)); n != 64 {
		t.Errorf("%d key warnings, want 64", n)
	}
	omitted := find(r, verify.ExportKeyUnknownOmitted)
	if len(omitted) != 2 {
		t.Fatalf("omitted warnings %+v", omitted)
	}
	for _, w := range omitted {
		if *w.Count != 1 {
			t.Errorf("omitted warning %+v", w)
		}
	}
}

// The warnings of an accepted document are reported also when another
// document was rejected, and the rejected document adds none.
func TestUnknownKeysNextToARejectedDocument(t *testing.T) {
	f := ct.Intact(t)
	f.PlaintextExport["newTopKey"] = 1
	in := f.Input(t)
	in.ItemList = []byte("not json")
	r := runInput(t, f, in, false)
	wantKeyWarning(t, only(t, r, verify.ExportKeyUnknown), "plaintext_export", "", "newTopKey", 1)
	wantVerdict(t, r, verify.Failed)
}

// The warnings sort deterministically: by document, then path, then type.
func TestExportKeyWarningsAreDeterministic(t *testing.T) {
	build := func() *verify.Result {
		f := ct.IntactOrganization(t)
		f.PlaintextExport["zKey"] = 1
		f.PlaintextExport["aKey"] = 1
		f.EncryptedExport["mKey"] = 1
		for _, id := range []string{ct.ItemPlainLogin, ct.ItemNote} {
			itemOf(t, f.PlaintextItems(), id)["type"] = 10 + len(id)%3
		}
		itemOf(t, f.PlaintextItems(), ct.ItemArchivedLogin)["type"] = 9
		f.Organization(t, ct.ChosenOrganization).EncryptedExport["oKey"] = 1
		return run(t, f, false)
	}
	first := dump(t, build())
	for i := 0; i < 10; i++ {
		if again := dump(t, build()); again != first {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, first, again)
		}
	}
	var types []int
	for _, w := range find(build(), verify.ExportItemTypeUnknown) {
		types = append(types, *w.ItemType)
	}
	if !sort.IntsAreSorted(types) || len(types) < 2 {
		t.Errorf("item types %v not sorted", types)
	}
}

// The fixture run "unknown export keys and item types" brings every reason
// of the change into the cross-cutting tests, the omitted paths included
// (second review of 24 November 2026, finding C1).
func TestUnknownExportKeysFixtureCoversEveryReason(t *testing.T) {
	r := run(t, unknownExportKeys(t), false)
	for _, reason := range exportKeyReasons {
		if len(find(r, reason)) == 0 {
			t.Errorf("the fixture run has no warning %s", reason)
		}
	}
}
