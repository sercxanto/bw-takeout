package capture_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
)

func itemIn(t *testing.T, items []any, id string) map[string]any {
	t.Helper()
	for _, raw := range items {
		if m, ok := raw.(map[string]any); ok && m["id"] == id {
			return m
		}
	}
	t.Fatalf("no item %s", id)
	return nil
}

func encryptedItem(t *testing.T, c *capture.Capture, id string) capture.EncryptedItem {
	t.Helper()
	if c.Encrypted == nil {
		t.Fatalf("encrypted export rejected: %v", c.Errors)
	}
	for _, item := range c.Encrypted.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("encrypted export has no item %s", id)
	return capture.EncryptedItem{}
}

func mustParse(t *testing.T, f *ct.Fixture) *capture.Capture {
	t.Helper()
	c := f.Capture(t)
	if len(c.Errors) != 0 {
		t.Fatalf("capture rejected: %v", c.Errors)
	}
	return c
}

func TestParseIntact(t *testing.T) {
	f := ct.Intact(t)
	c := mustParse(t, f)

	wantEncrypted := []capture.EncryptedItem{
		{ID: ct.ItemPasskeyLogin, Type: capture.TypeLogin, Format: capture.FormatLegacy, PasskeyCount: 2},
		{ID: ct.ItemPlainLogin, Type: capture.TypeLogin, Format: capture.FormatLegacy, PasskeyCount: 0},
		{ID: ct.ItemNote, Type: 2, Format: capture.FormatLegacy, PasskeyCount: 0},
		{ID: ct.ItemArchivedLogin, Type: capture.TypeLogin, Format: capture.FormatLegacy, PasskeyCount: 0},
	}
	if !reflect.DeepEqual(c.Encrypted.Items, wantEncrypted) {
		t.Errorf("encrypted items:\n got %+v\nwant %+v", c.Encrypted.Items, wantEncrypted)
	}

	if len(c.Plaintext.Items) != 4 {
		t.Fatalf("plaintext export: %d items, want 4", len(c.Plaintext.Items))
	}
	passkeys := c.Plaintext.Items[0].Passkeys
	if c.Plaintext.Items[0].ID != ct.ItemPasskeyLogin || len(passkeys) != 2 {
		t.Fatalf("first plaintext item %s with %d passkeys", c.Plaintext.Items[0].ID, len(passkeys))
	}
	for i, p := range passkeys {
		if p.KeyType != "public-key" || p.KeyAlgorithm != "ECDSA" || p.KeyCurve != "P-256" ||
			p.KeyValue == "" || p.CredentialID == "" || !p.HasRPID || !p.HasDiscoverable ||
			!p.HasCreationDate || len(p.InvalidFields) != 0 {
			t.Errorf("passkey %d: %+v", i, p)
		}
	}
	if passkeys[0].Counter != "0" || passkeys[1].Counter != "6" {
		t.Errorf("counters %q and %q, want 0 and 6", passkeys[0].Counter, passkeys[1].Counter)
	}
	for _, item := range c.Plaintext.Items[1:] {
		if len(item.Passkeys) != 0 || item.OrganizationID != "" {
			t.Errorf("plaintext item %s: %+v", item.ID, item)
		}
	}

	wantList := []capture.ListedItem{
		{ID: ct.ItemPasskeyLogin, AttachmentIDs: []string{ct.AttachmentFirst, ct.AttachmentSecond}},
		{ID: ct.ItemPlainLogin},
		{ID: ct.ItemNote},
	}
	if !reflect.DeepEqual(c.List.Items, wantList) {
		t.Errorf("listed items:\n got %+v\nwant %+v", c.List.Items, wantList)
	}
	wantArchived := []capture.ListedItem{
		{ID: ct.ItemArchivedLogin, AttachmentIDs: []string{ct.AttachmentArchived}},
	}
	if !reflect.DeepEqual(c.ArchivedList.Items, wantArchived) {
		t.Errorf("archived listed items:\n got %+v\nwant %+v", c.ArchivedList.Items, wantArchived)
	}
	if len(c.Downloads) != 3 || c.DuplicateDownloads != nil {
		t.Errorf("downloads %d, duplicates %v", len(c.Downloads), c.DuplicateDownloads)
	}
	for _, dup := range [][]string{c.Plaintext.DuplicateIDs, c.Encrypted.DuplicateIDs, c.List.DuplicateIDs, c.ArchivedList.DuplicateIDs} {
		if dup != nil {
			t.Errorf("duplicates in the intact fixture: %v", dup)
		}
	}
}

// The model keeps no decrypted value beyond what the passkey checks need.
func TestModelHoldsNoOtherDecryptedField(t *testing.T) {
	allowed := map[string][]string{
		"PlaintextItem": {"ID", "OrganizationID", "Passkeys"},
		"Passkey": {"CredentialID", "KeyType", "KeyAlgorithm", "KeyCurve", "KeyValue", "Counter",
			"HasRPID", "HasDiscoverable", "HasCreationDate", "InvalidFields"},
		"ListedItem":    {"ID", "OrganizationID", "AttachmentIDs"},
		"EncryptedItem": {"ID", "Type", "OrganizationID", "Format", "PasskeyCount"},
	}
	for name, typ := range map[string]reflect.Type{
		"PlaintextItem": reflect.TypeOf(capture.PlaintextItem{}),
		"Passkey":       reflect.TypeOf(capture.Passkey{}),
		"ListedItem":    reflect.TypeOf(capture.ListedItem{}),
		"EncryptedItem": reflect.TypeOf(capture.EncryptedItem{}),
	} {
		var got []string
		for i := 0; i < typ.NumField(); i++ {
			got = append(got, typ.Field(i).Name)
		}
		if !reflect.DeepEqual(got, allowed[name]) {
			t.Errorf("%s fields = %v, want %v", name, got, allowed[name])
		}
	}
}

func TestParseItemFormats(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, item map[string]any)
		want   capture.Format
		count  int
	}{
		{
			name:   "legacy login with passkeys",
			change: func(t *testing.T, item map[string]any) {},
			want:   capture.FormatLegacy, count: 2,
		},
		{
			name: "legacy login without fido2Credentials key",
			change: func(t *testing.T, item map[string]any) {
				delete(item["login"].(map[string]any), "fido2Credentials")
			},
			want: capture.FormatLegacy, count: 0,
		},
		{
			name: "legacy login with fido2Credentials null",
			change: func(t *testing.T, item map[string]any) {
				item["login"].(map[string]any)["fido2Credentials"] = nil
			},
			want: capture.FormatLegacy, count: 0,
		},
		{
			name: "legacy login with data null",
			change: func(t *testing.T, item map[string]any) {
				item["data"] = nil
			},
			want: capture.FormatLegacy, count: 2,
		},
		{
			name: "blob item",
			change: func(t *testing.T, item map[string]any) {
				seal(item, ct.BlobData)
			},
			want: capture.FormatBlob,
		},
		{
			name: "blob item with name and type object null",
			change: func(t *testing.T, item map[string]any) {
				seal(item, ct.BlobData)
				item["name"] = nil
				item["login"] = nil
			},
			want: capture.FormatBlob,
		},
		{
			name: "unrecognized sealed data",
			change: func(t *testing.T, item map[string]any) {
				seal(item, ct.UnknownSealedData)
			},
			want: capture.FormatUnknown,
		},
		{
			name: "legacy content plus data without the blob marker",
			change: func(t *testing.T, item map[string]any) {
				item["data"] = ct.UnknownSealedData
			},
			want: capture.FormatUnknown,
		},
		{
			name: "blob marker combined with legacy content",
			change: func(t *testing.T, item map[string]any) {
				item["data"] = ct.BlobData
			},
			want: capture.FormatUnknown,
		},
		{
			name: "blob marker with a name but no type object",
			change: func(t *testing.T, item map[string]any) {
				seal(item, ct.BlobData)
				item["name"] = "2.x|y|z"
			},
			want: capture.FormatUnknown,
		},
		{
			name: "data is not a string",
			change: func(t *testing.T, item map[string]any) {
				seal(item, ct.BlobData)
				item["data"] = map[string]any{"format_version": 1}
			},
			want: capture.FormatUnknown,
		},
		{
			name: "unknown item type",
			change: func(t *testing.T, item map[string]any) {
				item["type"] = 99
			},
			want: capture.FormatUnknown,
		},
		{
			name: "legacy item without name",
			change: func(t *testing.T, item map[string]any) {
				delete(item, "name")
			},
			want: capture.FormatUnknown,
		},
		{
			name: "legacy login without login object",
			change: func(t *testing.T, item map[string]any) {
				delete(item, "login")
			},
			want: capture.FormatUnknown,
		},
		{
			name: "passkey list is not an array",
			change: func(t *testing.T, item map[string]any) {
				item["login"].(map[string]any)["fido2Credentials"] = "2.x|y|z"
			},
			want: capture.FormatUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ct.Intact(t)
			tc.change(t, itemIn(t, f.EncryptedItems(), ct.ItemPasskeyLogin))
			got := encryptedItem(t, mustParse(t, f), ct.ItemPasskeyLogin)
			if got.Format != tc.want || got.PasskeyCount != tc.count {
				t.Errorf("format %s with %d passkeys, want %s with %d", got.Format, got.PasskeyCount, tc.want, tc.count)
			}
		})
	}
}

// seal replaces the legacy content of an encrypted item with a data string.
func seal(item map[string]any, data string) {
	for _, key := range []string{"name", "notes", "fields", "login", "passwordHistory"} {
		delete(item, key)
	}
	item["data"] = data
}

func TestParseFixtureVariantFormats(t *testing.T) {
	for name, want := range map[string]capture.Format{
		ct.BlobLoginItem:        capture.FormatBlob,
		ct.UnrecognizedItemForm: capture.FormatUnknown,
	} {
		got := encryptedItem(t, mustParse(t, ct.VariantByName(t, name).Build(t)), ct.ItemPasskeyLogin)
		if got.Format != want {
			t.Errorf("%s: format %s, want %s", name, got.Format, want)
		}
	}
}

func TestParseNonLoginBlobItem(t *testing.T) {
	f := ct.Intact(t)
	note := itemIn(t, f.EncryptedItems(), ct.ItemNote)
	for _, key := range []string{"name", "notes", "fields", "secureNote"} {
		delete(note, key)
	}
	note["data"] = ct.BlobData
	got := encryptedItem(t, mustParse(t, f), ct.ItemNote)
	if got.Format != capture.FormatBlob || got.Type != 2 {
		t.Errorf("note: %+v", got)
	}
}

// The fixture writes the optional keys of one item as null. Setting every
// optional key of every item to null parses the same as leaving it out.
func TestParseOptionalKeysAsNull(t *testing.T) {
	base := mustParse(t, ct.Intact(t))

	f := ct.Intact(t)
	for _, items := range [][]any{f.PlaintextItems(), f.EncryptedItems(), f.ItemList, f.ArchivedItemList} {
		for _, raw := range items {
			item := raw.(map[string]any)
			for _, key := range []string{"organizationId", "folderId", "deletedDate", "archivedDate", "data"} {
				if _, ok := item[key]; !ok {
					item[key] = nil
				}
			}
			if a, ok := item["attachments"].([]any); ok && len(a) == 0 {
				item["attachments"] = nil
			}
		}
	}
	for _, raw := range f.PlaintextItems() {
		for _, p := range passkeyMaps(raw) {
			p["userHandle"] = nil
			p["userName"] = nil
			p["rpName"] = nil
			p["userDisplayName"] = nil
		}
	}
	plainLogin := itemIn(t, f.EncryptedItems(), ct.ItemPlainLogin)["login"].(map[string]any)
	plainLogin["fido2Credentials"] = nil

	got := mustParse(t, f)
	if !reflect.DeepEqual(got.Encrypted, base.Encrypted) || !reflect.DeepEqual(got.List, base.List) ||
		!reflect.DeepEqual(got.ArchivedList, base.ArchivedList) {
		t.Errorf("null keys changed the parsed encrypted export or an item list")
	}
	if len(got.Plaintext.Items) != len(base.Plaintext.Items) {
		t.Fatal("null keys changed the number of plaintext items")
	}
	for i := range got.Plaintext.Items {
		g, b := got.Plaintext.Items[i], base.Plaintext.Items[i]
		if g.ID != b.ID || g.OrganizationID != b.OrganizationID || len(g.Passkeys) != len(b.Passkeys) {
			t.Errorf("item %s differs after null keys", g.ID)
		}
	}
}

func passkeyMaps(raw any) []map[string]any {
	login, ok := raw.(map[string]any)["login"].(map[string]any)
	if !ok {
		return nil
	}
	list, _ := login["fido2Credentials"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, p.(map[string]any))
	}
	return out
}

func TestParsePasskeyFields(t *testing.T) {
	f := ct.Intact(t)
	ps := passkeyMaps(itemIn(t, f.PlaintextItems(), ct.ItemPasskeyLogin))
	ps[0]["counter"] = json.Number("6")
	ps[0]["discoverable"] = false
	ps[1]["keyValue"] = 42
	ps[1]["counter"] = []any{}
	ps[1]["discoverable"] = 1
	delete(ps[1], "rpId")
	ps[1]["creationDate"] = ""

	c := mustParse(t, f)
	got := c.Plaintext.Items[0].Passkeys
	if got[0].Counter != "6" || !got[0].HasDiscoverable || len(got[0].InvalidFields) != 0 {
		t.Errorf("first passkey: %+v", got[0])
	}
	want := []string{"counter", "discoverable", "keyValue"}
	if !reflect.DeepEqual(got[1].InvalidFields, want) {
		t.Errorf("invalid fields = %v, want %v", got[1].InvalidFields, want)
	}
	if got[1].KeyValue != "" || got[1].HasRPID || got[1].HasCreationDate || got[1].HasDiscoverable {
		t.Errorf("second passkey: %+v", got[1])
	}
}

func TestRejectWrongDocumentForms(t *testing.T) {
	passwordProtected := []byte(`{"encrypted":true,"passwordProtected":true,"salt":"c2FsdA==","kdfType":0,` +
		`"kdfIterations":600000,"encKeyValidation_DO_NOT_EDIT":"2.a|b|c","data":"2.a|b|c"}`)

	cases := []struct {
		name    string
		change  func(t *testing.T, f *ct.Fixture, in *capture.Input)
		doc     capture.Document
		problem capture.Problem
	}{
		{
			name:   "item list is not valid JSON",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) { in.ItemList = []byte(`[{"id":`) },
			doc:    capture.ItemListDocument, problem: capture.ProblemInvalidJSON,
		},
		{
			name:   "archived item list is not valid JSON",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) { in.ArchivedItemList = []byte(`[{"id":`) },
			doc:    capture.ArchivedItemListDocument, problem: capture.ProblemInvalidJSON,
		},
		{
			name:   "archived item list is absent",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) { in.ArchivedItemList = nil },
			doc:    capture.ArchivedItemListDocument, problem: capture.ProblemInvalidJSON,
		},
		{
			name: "archived item list wrapped in the BW_RESPONSE envelope",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.ArchivedItemList = []byte(`{"success":true,"data":{"object":"list","data":[]}}`)
			},
			doc: capture.ArchivedItemListDocument, problem: capture.ProblemResponseEnvelope,
		},
		{
			name: "archived item without ID",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.ArchivedItemList = []byte(`[{"name":"x"}]`)
			},
			doc: capture.ArchivedItemListDocument, problem: capture.ProblemMalformedItem,
		},
		{
			name: "plaintext export with trailing data",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.PlaintextExport = append(in.PlaintextExport, []byte(` {}`)...)
			},
			doc: capture.PlaintextExportDocument, problem: capture.ProblemInvalidJSON,
		},
		{
			name: "encrypted export is empty",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.EncryptedExport = nil
			},
			doc: capture.EncryptedExportDocument, problem: capture.ProblemInvalidJSON,
		},
		{
			name: "plaintext export marked as encrypted",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.PlaintextExport = in.EncryptedExport
			},
			doc: capture.PlaintextExportDocument, problem: capture.ProblemMarkedEncrypted,
		},
		{
			name: "plaintext export without encrypted marker",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				delete(f.PlaintextExport, "encrypted")
				in.PlaintextExport = f.PlaintextExportJSON(t)
			},
			doc: capture.PlaintextExportDocument, problem: capture.ProblemWrongForm,
		},
		{
			name: "plaintext export supplied as encrypted export",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.EncryptedExport = in.PlaintextExport
			},
			doc: capture.EncryptedExportDocument, problem: capture.ProblemNotMarkedEncrypted,
		},
		{
			name: "encrypted export without key validation",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				delete(f.EncryptedExport, "encKeyValidation_DO_NOT_EDIT")
				in.EncryptedExport = f.EncryptedExportJSON(t)
			},
			doc: capture.EncryptedExportDocument, problem: capture.ProblemNotAccountBound,
		},
		{
			name: "encrypted export with top-level data",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				f.EncryptedExport["data"] = "2.a|b|c"
				in.EncryptedExport = f.EncryptedExportJSON(t)
			},
			doc: capture.EncryptedExportDocument, problem: capture.ProblemNotAccountBound,
		},
		{
			name: "password-protected export supplied as encrypted export",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.EncryptedExport = passwordProtected
			},
			doc: capture.EncryptedExportDocument, problem: capture.ProblemPasswordProtected,
		},
		{
			name: "password-protected export supplied as plaintext export",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.PlaintextExport = passwordProtected
			},
			doc: capture.PlaintextExportDocument, problem: capture.ProblemPasswordProtected,
		},
		{
			name: "item list wrapped in the BW_RESPONSE envelope",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.ItemList = []byte(`{"success":true,"data":{"object":"list","data":` + string(in.ItemList) + `}}`)
			},
			doc: capture.ItemListDocument, problem: capture.ProblemResponseEnvelope,
		},
		{
			name: "item list is an object",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				in.ItemList = []byte(`{"items":[]}`)
			},
			doc: capture.ItemListDocument, problem: capture.ProblemWrongForm,
		},
		{
			name: "plaintext item without ID",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				delete(itemIn(t, f.PlaintextItems(), ct.ItemNote), "id")
				in.PlaintextExport = f.PlaintextExportJSON(t)
			},
			doc: capture.PlaintextExportDocument, problem: capture.ProblemMalformedItem,
		},
		{
			name: "attachment without ID",
			change: func(t *testing.T, f *ct.Fixture, in *capture.Input) {
				a := itemIn(t, f.ItemList, ct.ItemPasskeyLogin)["attachments"].([]any)
				delete(a[0].(map[string]any), "id")
				in.ItemList = f.ItemListJSON(t)
			},
			doc: capture.ItemListDocument, problem: capture.ProblemMalformedItem,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ct.Intact(t)
			in := f.Input(t)
			tc.change(t, f, &in)
			c := capture.Parse(in)
			if len(c.Errors) != 1 {
				t.Fatalf("errors = %v, want exactly one", c.Errors)
			}
			e := c.Errors[0]
			if e.Document != tc.doc || e.Problem != tc.problem {
				t.Errorf("error = %v, want %s: %s", e, tc.doc, tc.problem)
			}
			switch tc.doc {
			case capture.PlaintextExportDocument:
				if c.Plaintext != nil {
					t.Error("rejected plaintext export is not nil")
				}
			case capture.EncryptedExportDocument:
				if c.Encrypted != nil {
					t.Error("rejected encrypted export is not nil")
				}
			case capture.ItemListDocument:
				if c.List != nil {
					t.Error("rejected item list is not nil")
				}
			case capture.ArchivedItemListDocument:
				if c.ArchivedList != nil {
					t.Error("rejected archived item list is not nil")
				}
			}
		})
	}
}

func TestDocumentErrorCarriesNoContent(t *testing.T) {
	_, err := capture.ParseItemList([]byte(`[{"id":"x","name":"` + ct.Marker + `"`))
	var de *capture.DocumentError
	if !errors.As(err, &de) {
		t.Fatalf("error %v is not a DocumentError", err)
	}
	if got := err.Error(); got != "item_list: invalid_json" {
		t.Errorf("message = %q", got)
	}
}

func TestDuplicateItemIDs(t *testing.T) {
	t.Run("plaintext export", func(t *testing.T) {
		c := mustParse(t, ct.VariantByName(t, ct.DuplicateID).Build(t))
		if !reflect.DeepEqual(c.Plaintext.DuplicateIDs, []string{ct.ItemNote}) {
			t.Errorf("duplicates = %v", c.Plaintext.DuplicateIDs)
		}
		if len(c.Plaintext.Items) != 5 {
			t.Errorf("%d items, want the duplicate kept", len(c.Plaintext.Items))
		}
	})
	t.Run("encrypted export", func(t *testing.T) {
		f := ct.Intact(t)
		f.EncryptedExport["items"] = append(f.EncryptedItems(), itemIn(t, f.EncryptedItems(), ct.ItemPlainLogin))
		c := mustParse(t, f)
		if !reflect.DeepEqual(c.Encrypted.DuplicateIDs, []string{ct.ItemPlainLogin}) {
			t.Errorf("duplicates = %v", c.Encrypted.DuplicateIDs)
		}
	})
	t.Run("item list", func(t *testing.T) {
		f := ct.Intact(t)
		f.ItemList = append(f.ItemList, itemIn(t, f.ItemList, ct.ItemPasskeyLogin))
		c := mustParse(t, f)
		if !reflect.DeepEqual(c.List.DuplicateIDs, []string{ct.ItemPasskeyLogin}) {
			t.Errorf("duplicates = %v", c.List.DuplicateIDs)
		}
		// The repeated item repeats its attachments. That is a duplicate
		// item, not a duplicate attachment.
		if c.List.DuplicateAttachments != nil {
			t.Errorf("duplicate attachments = %v", c.List.DuplicateAttachments)
		}
	})
}

func TestDuplicateAttachmentPairs(t *testing.T) {
	t.Run("item list", func(t *testing.T) {
		f := ct.Intact(t)
		item := itemIn(t, f.ItemList, ct.ItemPasskeyLogin)
		attachments := item["attachments"].([]any)
		item["attachments"] = append(attachments, attachments[1])
		c := mustParse(t, f)
		want := []capture.AttachmentRef{{ItemID: ct.ItemPasskeyLogin, AttachmentID: ct.AttachmentSecond}}
		if !reflect.DeepEqual(c.List.DuplicateAttachments, want) {
			t.Errorf("duplicate attachments = %v", c.List.DuplicateAttachments)
		}
	})
	t.Run("downloads", func(t *testing.T) {
		f := ct.Intact(t)
		in := f.Input(t)
		in.Downloads = append(in.Downloads, in.Downloads[0], in.Downloads[0])
		c := capture.Parse(in)
		want := []capture.AttachmentRef{{ItemID: ct.ItemPasskeyLogin, AttachmentID: ct.AttachmentFirst}}
		if !reflect.DeepEqual(c.DuplicateDownloads, want) {
			t.Errorf("duplicate downloads = %v", c.DuplicateDownloads)
		}
	})
}

// The archived item list is parsed by the same rules as the default list,
// and every rejection names the archived list (design D1 of
// archived-item-list).
func TestParseArchivedItemList(t *testing.T) {
	t.Run("valid archived list", func(t *testing.T) {
		c := mustParse(t, ct.Intact(t))
		if c.ArchivedList == nil || len(c.ArchivedList.Items) != 1 || c.ArchivedList.Items[0].ID != ct.ItemArchivedLogin {
			t.Errorf("archived list = %+v", c.ArchivedList)
		}
		for _, item := range c.List.Items {
			if item.ID == ct.ItemArchivedLogin {
				t.Error("default list holds the archived item")
			}
		}
	})
	t.Run("empty list", func(t *testing.T) {
		list, err := capture.ParseArchivedItemList([]byte(`[]`))
		if err != nil || list == nil || len(list.Items) != 0 {
			t.Errorf("list = %+v, err = %v", list, err)
		}
	})
	for name, raw := range map[string][]byte{
		"invalid JSON":      []byte(`[{"id":`),
		"zero-length input": {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := capture.ParseArchivedItemList(raw)
			var de *capture.DocumentError
			if !errors.As(err, &de) || de.Document != capture.ArchivedItemListDocument || de.Problem != capture.ProblemInvalidJSON {
				t.Errorf("error = %v, want archived_item_list: invalid_json", err)
			}
			in := ct.Intact(t).Input(t)
			in.ArchivedItemList = raw
			c := capture.Parse(in)
			if len(c.Errors) != 1 || c.Errors[0].Document != capture.ArchivedItemListDocument || c.ArchivedList != nil {
				t.Errorf("errors = %v, archived list = %v", c.Errors, c.ArchivedList)
			}
		})
	}
	t.Run("duplicate ID", func(t *testing.T) {
		c := mustParse(t, ct.VariantByName(t, ct.ArchivedListDuplicateID).Build(t))
		if !reflect.DeepEqual(c.ArchivedList.DuplicateIDs, []string{ct.ItemArchivedLogin}) {
			t.Errorf("archived duplicates = %v", c.ArchivedList.DuplicateIDs)
		}
		if c.List.DuplicateIDs != nil {
			t.Errorf("default list duplicates = %v", c.List.DuplicateIDs)
		}
	})
}
