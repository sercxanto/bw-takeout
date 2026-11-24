package verify_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
	"bw-takeout/internal/verify"
)

// run verifies a fixture, with its reference inventory when withReference
// is set.
func run(t *testing.T, f *ct.Fixture, withReference bool) *verify.Result {
	t.Helper()
	return runInput(t, f, f.Input(t), withReference)
}

func runInput(t *testing.T, f *ct.Fixture, in capture.Input, withReference bool) *verify.Result {
	t.Helper()
	var ref *capture.Reference
	if withReference {
		ref = &f.Reference
	}
	return verify.Verify(capture.Parse(in), ref)
}

// variant builds the intact fixture of the first named variant, Intact or
// IntactOrganization, and applies every named variant to it.
func variant(t *testing.T, names ...string) *ct.Fixture {
	t.Helper()
	if len(names) == 0 {
		return ct.Intact(t)
	}
	f := ct.VariantByName(t, names[0]).Base(t)
	for _, name := range names {
		ct.VariantByName(t, name).Apply(t, f)
	}
	return f
}

// find returns the findings and warnings with the given reason.
func find(r *verify.Result, reason verify.Reason) []verify.Finding {
	var out []verify.Finding
	for _, list := range [][]verify.Finding{r.Findings, r.Warnings} {
		for _, f := range list {
			if f.Reason == reason {
				out = append(out, f)
			}
		}
	}
	return out
}

// only fails unless the result holds exactly one finding or warning with
// the given reason, and returns it.
func only(t *testing.T, r *verify.Result, reason verify.Reason) verify.Finding {
	t.Helper()
	got := find(r, reason)
	if len(got) != 1 {
		t.Fatalf("%d findings with reason %s, want 1; result: %s", len(got), reason, dump(t, r))
	}
	return got[0]
}

func wantVerdict(t *testing.T, r *verify.Result, want verify.Verdict) {
	t.Helper()
	if r.Verdict != want {
		t.Errorf("verdict %s, want %s; result: %s", r.Verdict, want, dump(t, r))
	}
}

// wantReasons fails unless the findings (not the warnings) carry exactly
// the given reasons, in sorted order.
func wantReasons(t *testing.T, r *verify.Result, want ...verify.Reason) {
	t.Helper()
	var got []verify.Reason
	for _, f := range r.Findings {
		got = append(got, f.Reason)
	}
	if len(got) != len(want) {
		t.Fatalf("finding reasons %v, want %v; result: %s", got, want, dump(t, r))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("finding reasons %v, want %v; result: %s", got, want, dump(t, r))
		}
	}
}

func dump(t *testing.T, r *verify.Result) string {
	t.Helper()
	out, err := r.JSON()
	if err != nil {
		t.Fatalf("serialize result: %v", err)
	}
	return string(out)
}

func intValue(t *testing.T, p *int) int {
	t.Helper()
	if p == nil {
		t.Fatal("value not set")
	}
	return *p
}

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func downloadIndex(t *testing.T, in capture.Input, attachmentID string) int {
	t.Helper()
	for i, d := range in.Downloads {
		if d.AttachmentID == attachmentID {
			return i
		}
	}
	t.Fatalf("no download for %s", attachmentID)
	return -1
}

// fixtureRun is one capture the cross-cutting tests run.
type fixtureRun struct {
	name  string
	build func(t *testing.T) (*ct.Fixture, capture.Input)
}

// fixtureRuns returns every fixture variant and every additional failure
// case of the tests, each built fresh.
func fixtureRuns() []fixtureRun {
	fromFixture := func(names ...string) func(t *testing.T) (*ct.Fixture, capture.Input) {
		return func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := variant(t, names...)
			return f, f.Input(t)
		}
	}
	withInput := func(change func(t *testing.T, in *capture.Input)) func(t *testing.T) (*ct.Fixture, capture.Input) {
		return func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := ct.Intact(t)
			in := f.Input(t)
			change(t, &in)
			return f, in
		}
	}

	runs := []fixtureRun{
		{name: "intact", build: fromFixture()},
		{name: "intact organization", build: func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := ct.IntactOrganization(t)
			return f, f.Input(t)
		}},
	}
	for _, v := range append(ct.Variants(), ct.OrganizationVariants()...) {
		runs = append(runs, fixtureRun{name: v.Name, build: fromFixture(v.Name)})
	}
	return append(runs,
		fixtureRun{name: "blob login and missing item", build: fromFixture(ct.BlobLoginItem, ct.MissingItem)},
		fixtureRun{name: "download failed", build: withInput(func(t *testing.T, in *capture.Input) {
			i := downloadIndex(t, *in, ct.AttachmentSecond)
			in.Downloads[i].Error, in.Downloads[i].SHA256, in.Downloads[i].Size = capture.ErrorStatus, "", 0
		})},
		fixtureRun{name: "download missing", build: withInput(func(t *testing.T, in *capture.Input) {
			in.Downloads = in.Downloads[1:]
		})},
		fixtureRun{name: "download not expected", build: withInput(func(t *testing.T, in *capture.Input) {
			in.Downloads = append(in.Downloads, capture.Download{
				ItemID: ct.ItemNote, AttachmentID: "att-unexpected", SHA256: hashOf([]byte("x")), Size: 1,
			})
		})},
		fixtureRun{name: "empty attachment", build: withInput(func(t *testing.T, in *capture.Input) {
			i := downloadIndex(t, *in, ct.AttachmentFirst)
			in.Downloads[i].SHA256, in.Downloads[i].Size = hashOf(nil), 0
		})},
		fixtureRun{name: "exported item listed as organization item", build: func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := listNoteAsOrganizationItem(t)
			return f, f.Input(t)
		}},
		fixtureRun{name: "organization attachment downloaded", build: func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := variant(t, ct.OrganizationItems)
			in := f.Input(t)
			in.Downloads = append(in.Downloads, capture.Download{
				ItemID: ct.ItemOrgFirstA, AttachmentID: ct.AttachmentOrg, SHA256: hashOf([]byte("x")), Size: 1,
			})
			return f, in
		}},
		fixtureRun{name: "password-protected export", build: withInput(func(t *testing.T, in *capture.Input) {
			in.EncryptedExport = passwordProtectedExport
		})},
		fixtureRun{name: "item list not valid JSON", build: withInput(func(t *testing.T, in *capture.Input) {
			in.ItemList = in.ItemList[:len(in.ItemList)/2]
		})},
		fixtureRun{name: "unknown export keys and item types", build: func(t *testing.T) (*ct.Fixture, capture.Input) {
			f := unknownExportKeys(t)
			return f, f.Input(t)
		}},
	)
}

// unknownExportKeys returns the intact organization fixture with every
// warning of the change bw-version-2026-11: identifier keys with sentinel
// values, a key that is not an identifier and holds the sentinel marker
// itself, an unknown item type, and more distinct paths than are reported.
// The identifier keys hold no marker, because they are reported by name.
func unknownExportKeys(t *testing.T) *ct.Fixture {
	t.Helper()
	f := ct.IntactOrganization(t)
	note := itemOf(t, f.PlaintextItems(), ct.ItemNote)
	note["newItemKey"] = ct.Marker + "-value-of-a-new-item-key"
	note[ct.Marker+" key @ example.invalid"] = ct.Marker + "-value-of-a-key-that-is-no-identifier"
	note["type"] = 9
	for i := 0; i < verify.MaxUnknownKeyPaths+2; i++ {
		f.EncryptedExport[fmt.Sprintf("newDocumentKey%02d", i)] = ct.Marker + "-document-level-value"
	}
	f.EncryptedExport[ct.Marker+"-document-key-that-is-no-identifier"] = ct.Marker + "-document-level-value"
	o := f.Organization(t, ct.ChosenOrganization)
	login := itemOf(t, o.PlaintextItems(), ct.ItemOrgLogin)["login"].(map[string]any)
	login["fido2Credentials"].([]any)[0].(map[string]any)["newPasskeyKey"] = ct.Marker + "-new-passkey-value"
	return f
}

var passwordProtectedExport = []byte(`{"encrypted":true,"passwordProtected":true,"salt":"c2FsdA==",` +
	`"kdfType":0,"kdfIterations":600000,"encKeyValidation_DO_NOT_EDIT":"2.a|b|c","data":"2.a|b|c"}`)
