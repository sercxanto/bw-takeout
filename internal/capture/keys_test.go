package capture_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
	ct "bw-takeout/internal/capture/capturetest"
)

// The tests of this file cover tasks 3.1 to 3.4 of the change
// bw-version-2026-11: the known key sets of design D4, the paths and counts
// of unknown keys, the identifier rule of design D5 and the unknown item
// types.

func wantKeys(t *testing.T, what string, got []capture.UnknownKey, want ...capture.UnknownKey) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: unknown keys\n got %+v\nwant %+v", what, got, want)
	}
}

func key(path string, count int) capture.UnknownKey {
	return capture.UnknownKey{Path: path, Count: count}
}

// noUnknown fails when any accepted export of the capture records an unknown
// key path or item type.
func noUnknown(t *testing.T, c *capture.Capture) {
	t.Helper()
	if len(c.Errors) != 0 {
		t.Fatalf("capture rejected: %v", c.Errors)
	}
	check := func(name string, p *capture.PlaintextExport, e *capture.EncryptedExport) {
		t.Helper()
		wantKeys(t, name+" plaintext", p.UnknownKeys)
		wantKeys(t, name+" encrypted", e.UnknownKeys)
		if len(p.UnknownItemTypes) != 0 {
			t.Errorf("%s plaintext: unknown item types %+v", name, p.UnknownItemTypes)
		}
	}
	check("personal", c.Plaintext, c.Encrypted)
	for _, o := range c.Organizations {
		check("organization "+o.ID, o.Plaintext, o.Encrypted)
	}
}

// Task 3.1: the intact fixtures, personal and organization, use only known
// keys and item types.
func TestIntactFixturesHoldOnlyKnownKeys(t *testing.T) {
	noUnknown(t, ct.Intact(t).Capture(t))
	c := ct.IntactOrganization(t).Capture(t)
	if len(c.Organizations) != 1 {
		t.Fatalf("%d organizations", len(c.Organizations))
	}
	noUnknown(t, c)
}

// Every variant that the parser accepts keeps to the known keys, except
// those whose documents are replaced by bytes. A blob item with data and a
// legacy item with key pass alike.
func TestVariantsHoldOnlyKnownKeys(t *testing.T) {
	for _, v := range append(ct.Variants(), ct.OrganizationVariants()...) {
		t.Run(v.Name, func(t *testing.T) {
			c := v.Build(t).Capture(t)
			if c.Plaintext != nil {
				wantKeys(t, "plaintext", c.Plaintext.UnknownKeys)
			}
			if c.Encrypted != nil {
				wantKeys(t, "encrypted", c.Encrypted.UnknownKeys)
			}
			for _, o := range c.Organizations {
				if o.Plaintext != nil {
					wantKeys(t, "organization plaintext", o.Plaintext.UnknownKeys)
				}
				if o.Encrypted != nil {
					wantKeys(t, "organization encrypted", o.Encrypted.UnknownKeys)
				}
			}
		})
	}
}

// Task 3.2: an unknown key on one item.
func TestUnknownKeyOnOneItem(t *testing.T) {
	f := ct.Intact(t)
	itemIn(t, f.PlaintextItems(), ct.ItemNote)["newKey"] = "value"
	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys, key("items[].newKey", 1))
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys)
}

// Task 3.2: the same key on three items is one path with the count 3.
func TestSameUnknownKeyOnThreeItems(t *testing.T) {
	f := ct.Intact(t)
	for _, id := range []string{ct.ItemPasskeyLogin, ct.ItemPlainLogin, ct.ItemNote} {
		itemIn(t, f.EncryptedItems(), id)["newKey"] = "2.a|b|c"
	}
	c := mustParse(t, f)
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys, key("items[].newKey", 3))
}

// An item counts once for a path, also when two of its passkeys carry the
// key.
func TestUnknownKeyCountsEachItemOnce(t *testing.T) {
	f := ct.Intact(t)
	login := itemIn(t, f.PlaintextItems(), ct.ItemPasskeyLogin)["login"].(map[string]any)
	for _, p := range login["fido2Credentials"].([]any) {
		p.(map[string]any)["newPasskeyKey"] = "value"
	}
	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys, key("items[].login.fido2Credentials[].newPasskeyKey", 1))
}

// Task 3.2: an unknown key in a passkey of an organization plaintext export.
func TestUnknownKeyInAnOrganizationPasskey(t *testing.T) {
	f := ct.IntactOrganization(t)
	o := f.Organization(t, ct.ChosenOrganization)
	login := itemIn(t, o.PlaintextItems(), ct.ItemOrgLogin)["login"].(map[string]any)
	login["fido2Credentials"].([]any)[0].(map[string]any)["newPasskeyKey"] = "value"
	c := mustParse(t, f)
	wantKeys(t, "organization plaintext", c.Organizations[0].Plaintext.UnknownKeys,
		key("items[].login.fido2Credentials[].newPasskeyKey", 1))
	wantKeys(t, "organization encrypted", c.Organizations[0].Encrypted.UnknownKeys)
	wantKeys(t, "personal plaintext", c.Plaintext.UnknownKeys)
}

// Task 3.2: an unknown key at document level has the count 1.
func TestUnknownDocumentLevelKey(t *testing.T) {
	f := ct.Intact(t)
	f.PlaintextExport["newTopKey"] = []any{"a", "b"}
	f.EncryptedExport["collections"] = []any{map[string]any{"id": "x"}}
	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys, key("newTopKey", 1))
	// Collections belong to the organization exports. In a personal export
	// they are unknown, and their elements are not walked.
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys, key("collections", 1))
}

// Each place of design D4 has its own set: folders, collections, fields,
// password history, URIs and every type-specific object.
func TestUnknownKeysAtEachPlace(t *testing.T) {
	f := ct.IntactOrganization(t)
	f.PlaintextExport["folders"].([]any)[0].(map[string]any)["newFolderKey"] = 1
	o := f.Organization(t, ct.ChosenOrganization)
	o.EncryptedExport["collections"].([]any)[0].(map[string]any)["newCollectionKey"] = 1
	login := itemIn(t, f.PlaintextItems(), ct.ItemPasskeyLogin)
	login["fields"].([]any)[0].(map[string]any)["newFieldKey"] = 1
	login["passwordHistory"].([]any)[0].(map[string]any)["newHistoryKey"] = 1
	login["login"].(map[string]any)["uris"].([]any)[0].(map[string]any)["newUriKey"] = 1
	login["login"].(map[string]any)["newLoginKey"] = 1
	note := itemIn(t, f.EncryptedItems(), ct.ItemNote)
	note["secureNote"].(map[string]any)["newNoteKey"] = 1
	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys,
		key("folders[].newFolderKey", 1),
		key("items[].fields[].newFieldKey", 1),
		key("items[].login.newLoginKey", 1),
		key("items[].login.uris[].newUriKey", 1),
		key("items[].passwordHistory[].newHistoryKey", 1),
	)
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys, key("items[].secureNote.newNoteKey", 1))
	wantKeys(t, "organization encrypted", c.Organizations[0].Encrypted.UnknownKeys, key("collections[].newCollectionKey", 1))
}

// The other type-specific objects are known with their keys.
func TestTypeSpecificObjectsAreKnown(t *testing.T) {
	objects := map[string][]string{
		"card":           {"cardholderName", "brand", "number", "expMonth", "expYear", "code"},
		"identity":       {"title", "firstName", "middleName", "lastName", "address1", "address2", "address3", "city", "state", "postalCode", "country", "company", "email", "phone", "ssn", "username", "passportNumber", "licenseNumber"},
		"sshKey":         {"privateKey", "publicKey", "keyFingerprint"},
		"bankAccount":    {"bankName", "nameOnAccount", "accountType", "accountNumber", "routingNumber", "branchNumber", "pin", "swiftCode", "iban", "bankContactPhone"},
		"driversLicense": {"firstName", "middleName", "lastName", "dateOfBirth", "licenseNumber", "issuingCountry", "issuingState", "issueDate", "expirationDate", "issuingAuthority", "licenseClass"},
		"passport":       {"surname", "givenName", "dateOfBirth", "sex", "birthPlace", "nationality", "issuingCountry", "passportNumber", "passportType", "nationalIdentificationNumber", "issuingAuthority", "issueDate", "expirationDate"},
	}
	f := ct.Intact(t)
	note := itemIn(t, f.PlaintextItems(), ct.ItemNote)
	for object, keys := range objects {
		m := map[string]any{"newObjectKey": 1}
		for _, k := range keys {
			m[k] = "value"
		}
		note[object] = m
	}
	c := mustParse(t, f)
	var want []capture.UnknownKey
	for _, object := range []string{"bankAccount", "card", "driversLicense", "identity", "passport", "sshKey"} {
		want = append(want, key("items[]."+object+".newObjectKey", 1))
	}
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys, want...)
}

// Task 3.2: the value of an unknown key is not walked, and a known key with
// a value of an unexpected type is not walked either.
func TestUnknownValueIsNotWalked(t *testing.T) {
	f := ct.Intact(t)
	itemIn(t, f.PlaintextItems(), ct.ItemNote)["newObject"] = map[string]any{
		"inner": 1, "deeper": map[string]any{"innermost": []any{map[string]any{"x": 1}}},
	}
	note := itemIn(t, f.EncryptedItems(), ct.ItemNote)
	note["fields"] = map[string]any{"notAnArray": 1}
	note["login"] = "not an object"
	note["passwordHistory"] = []any{"not an object", map[string]any{"newHistoryKey": 1}}
	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys, key("items[].newObject", 1))
	// The array element that is an object is walked.
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys, key("items[].passwordHistory[].newHistoryKey", 1))
}

// Task 3.2: a blob item with data and a legacy item with key record nothing.
func TestBlobAndLegacyItemsHoldKnownKeys(t *testing.T) {
	f := ct.VariantByName(t, ct.BlobLoginItem).Build(t)
	c := mustParse(t, f)
	if encryptedItem(t, c, ct.ItemPasskeyLogin).Format != capture.FormatBlob {
		t.Fatal("variant has no blob item")
	}
	if _, ok := itemIn(t, f.EncryptedItems(), ct.ItemPlainLogin)["key"]; !ok {
		t.Fatal("legacy item has no key")
	}
	wantKeys(t, "encrypted", c.Encrypted.UnknownKeys)
}

// Task 3.2: an item that lacks a known key records nothing.
func TestAbsentKnownKeyRecordsNothing(t *testing.T) {
	f := ct.Intact(t)
	for _, items := range [][]any{f.PlaintextItems(), f.EncryptedItems()} {
		item := itemIn(t, items, ct.ItemNote)
		for _, k := range []string{"notes", "favorite", "fields", "reprompt", "creationDate"} {
			delete(item, k)
		}
	}
	delete(f.PlaintextExport, "folders")
	noUnknown(t, mustParse(t, f))
}

// Task 3.3: a key that is not an identifier is recorded as the placeholder
// at its place, and its text is not in the parse result.
func TestKeysThatAreNotIdentifiers(t *testing.T) {
	sentinelKey := ct.Marker + " unknown key @ example"
	sentinelValue := ct.Marker + " value of an unknown key @ example"
	long := strings.Repeat("k", 65)
	digit := "9lives"
	nonASCII := "clé"
	f := ct.Intact(t)
	f.PlaintextExport[nonASCII] = sentinelValue
	item := itemIn(t, f.PlaintextItems(), ct.ItemPasskeyLogin)
	item[sentinelKey] = sentinelValue
	login := item["login"].(map[string]any)
	login[long] = sentinelValue
	login["fido2Credentials"].([]any)[0].(map[string]any)[digit] = sentinelValue
	// The longest identifier passes.
	item[strings.Repeat("k", 64)] = sentinelValue

	c := mustParse(t, f)
	wantKeys(t, "plaintext", c.Plaintext.UnknownKeys,
		key(capture.OtherKey, 1),
		key("items[]."+capture.OtherKey, 1),
		key("items[]."+strings.Repeat("k", 64), 1),
		key("items[].login."+capture.OtherKey, 1),
		key("items[].login.fido2Credentials[]."+capture.OtherKey, 1),
	)
	// The item lists are not walked for keys. Their map of file names has
	// keys that JSON cannot serialize, so the exports and errors are
	// serialized instead.
	out, err := json.Marshal([]any{c.Plaintext, c.Encrypted, c.Errors, c.Organizations})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{sentinelKey, sentinelValue, long, digit, nonASCII, "unknown key @"} {
		if strings.Contains(string(out), s) {
			t.Errorf("parse result contains %q", s)
		}
	}
}

func TestIdentifierRule(t *testing.T) {
	for k, want := range map[string]bool{
		"a": true, "Z": true, "newKey": true, "key2": true, strings.Repeat("a", 64): true,
		"": false, "2key": false, "new_key": false, "new-key": false, "new key": false,
		"a@b": false, "ключ": false, "é": false, strings.Repeat("a", 65): false, "a.b": false,
		"a[]": false, "<other>": false,
	} {
		f := ct.Intact(t)
		f.EncryptedExport[k] = 1
		c := mustParse(t, f)
		wantPath := capture.OtherKey
		if want {
			wantPath = k
		}
		wantKeys(t, "key "+k, c.Encrypted.UnknownKeys, key(wantPath, 1))
	}
}

// Task 3.4: an unknown item type on two items is one entry with the count 2.
func TestUnknownItemTypeCounted(t *testing.T) {
	f := ct.Intact(t)
	for _, id := range []string{ct.ItemPlainLogin, ct.ItemNote} {
		itemIn(t, f.PlaintextItems(), id)["type"] = 9
	}
	itemIn(t, f.PlaintextItems(), ct.ItemArchivedLogin)["type"] = 12
	c := mustParse(t, f)
	want := []capture.UnknownItemType{{Type: 9, Count: 2}, {Type: 12, Count: 1}}
	if !reflect.DeepEqual(c.Plaintext.UnknownItemTypes, want) {
		t.Errorf("unknown item types %+v, want %+v", c.Plaintext.UnknownItemTypes, want)
	}
}

// Every integer type counts, also one outside the range the encrypted
// parser accepts as a type (review of 24 November 2026, finding C3).
func TestUnknownItemTypeOfAnyInteger(t *testing.T) {
	f := ct.Intact(t)
	itemIn(t, f.PlaintextItems(), ct.ItemPlainLogin)["type"] = -1
	itemIn(t, f.PlaintextItems(), ct.ItemNote)["type"] = 70000
	itemIn(t, f.PlaintextItems(), ct.ItemArchivedLogin)["type"] = 0
	c := mustParse(t, f)
	want := []capture.UnknownItemType{{Type: -1, Count: 1}, {Type: 0, Count: 1}, {Type: 70000, Count: 1}}
	if !reflect.DeepEqual(c.Plaintext.UnknownItemTypes, want) {
		t.Errorf("unknown item types %+v, want %+v", c.Plaintext.UnknownItemTypes, want)
	}
}

// Task 3.4: the known types 1 to 8 record nothing, and a type that is not
// an integer is not counted as a type.
func TestKnownItemTypesRecordNothing(t *testing.T) {
	for _, typ := range []any{1, 2, 3, 4, 5, 6, 7, 8, "9", 9.5, nil} {
		f := ct.Intact(t)
		itemIn(t, f.PlaintextItems(), ct.ItemNote)["type"] = typ
		c := mustParse(t, f)
		if len(c.Plaintext.UnknownItemTypes) != 0 {
			t.Errorf("type %v: unknown item types %+v", typ, c.Plaintext.UnknownItemTypes)
		}
	}
}

// Task 3.4: per organization, and the encrypted export keeps its rule.
func TestUnknownItemTypeInOrganizationAndEncryptedExport(t *testing.T) {
	f := ct.IntactOrganization(t)
	itemIn(t, f.Organization(t, ct.ChosenOrganization).PlaintextItems(), ct.ItemOrgNote)["type"] = 9
	itemIn(t, f.EncryptedItems(), ct.ItemNote)["type"] = 9
	c := mustParse(t, f)
	want := []capture.UnknownItemType{{Type: 9, Count: 1}}
	if got := c.Organizations[0].Plaintext.UnknownItemTypes; !reflect.DeepEqual(got, want) {
		t.Errorf("organization plaintext: %+v, want %+v", got, want)
	}
	if len(c.Plaintext.UnknownItemTypes) != 0 {
		t.Errorf("personal plaintext: %+v", c.Plaintext.UnknownItemTypes)
	}
	if got := encryptedItem(t, c, ct.ItemNote).Format; got != capture.FormatUnknown {
		t.Errorf("encrypted item of type 9: format %s, want %s", got, capture.FormatUnknown)
	}
}

// A rejected document records nothing, because it is nil.
func TestRejectedDocumentRecordsNothing(t *testing.T) {
	f := ct.Intact(t)
	f.PlaintextExport["newTopKey"] = 1
	f.PlaintextExport["encrypted"] = true
	c := f.Capture(t)
	if c.Plaintext != nil {
		t.Fatal("plaintext export accepted")
	}
}
