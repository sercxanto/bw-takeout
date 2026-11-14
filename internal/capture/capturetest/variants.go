package capturetest

import (
	"encoding/base64"
	"testing"
)

// Names of the variants. Each one breaks the intact fixture in one place.
const (
	MissingItem           = "missing-item"
	DuplicateID           = "duplicate-id"
	OnePasskeyMissing     = "one-passkey-missing"
	AllPasskeysDropped    = "all-passkeys-dropped"
	BlobLoginItem         = "blob-login-item"
	UnrecognizedItemForm  = "unrecognized-item-form"
	EmptyKey              = "empty-key"
	UnparsableKey         = "unparsable-key"
	NonNumericCounter     = "non-numeric-counter"
	UnknownKeyAlgorithm   = "unknown-key-algorithm"
	SwappedCredential     = "swapped-credential"
	OrganizationItems     = "organization-items"
	ListedItemNotExported = "listed-item-not-exported"
	ExportedItemNotListed = "exported-item-not-listed"
)

// IDs that only variants use.
const (
	// OrganizationFirst holds two items, OrganizationSecond one.
	OrganizationFirst  = "0a000000-0000-4000-8000-000000000001"
	OrganizationSecond = "0a000000-0000-4000-8000-000000000002"

	ItemOrgFirstA  = "44444444-4444-4444-8444-444444444441"
	ItemOrgFirstB  = "44444444-4444-4444-8444-444444444442"
	ItemOrgSecondA = "44444444-4444-4444-8444-444444444443"

	// AttachmentOrg belongs to ItemOrgFirstA. No download exists for it.
	AttachmentOrg = "att00000000000000000000000000009"

	// ItemListedOnly appears in the item list and in no export.
	ItemListedOnly = "55555555-5555-4555-8555-555555555555"

	// SwappedCredentialID identifies the passkey that the variant
	// SwappedCredential puts in place of the second one.
	SwappedCredentialID = "c0000000-0000-4000-8000-000000000009"
)

// BlobData is the sealed content of the upstream blob test. The string
// contains the marker "format_version" that identifies the blob form.
const BlobData = `{"format_version":1,"wrapped_cek":"2.a|b|c","envelope":"g1hH"}`

// UnknownSealedData stands for a future sealed form. It lacks the blob
// marker.
const UnknownSealedData = `{"v":2,"sealed":"` + Marker + `-unknown-sealed-content"}`

// Variant is a named deviation from the intact fixture.
type Variant struct {
	Name string
	// Models says in one sentence what the variant stands for.
	Models string
	// Changes lists every place where the variant differs from the intact
	// fixture, in the notation of [Diff].
	Changes []string
	apply   func(t testing.TB, f *Fixture)
}

// Apply changes f in place.
func (v Variant) Apply(t testing.TB, f *Fixture) {
	t.Helper()
	v.apply(t, f)
}

// Build returns a fresh intact fixture with the variant applied.
func (v Variant) Build(t testing.TB) *Fixture {
	t.Helper()
	f := Intact(t)
	v.apply(t, f)
	return f
}

// VariantByName returns the variant with the given name and fails the test
// when there is none.
func VariantByName(t testing.TB, name string) Variant {
	t.Helper()
	for _, v := range Variants() {
		if v.Name == name {
			return v
		}
	}
	t.Fatalf("no fixture variant named %q", name)
	return Variant{}
}

// The plaintext export and the item list are two renderings of the same
// decrypted view. A variant that changes decrypted content therefore changes
// both. The encrypted export is built before decryption and stays as it is.
const (
	plainPasskeys  = "plaintext/items[" + ItemPasskeyLogin + "]/login/fido2Credentials"
	listPasskeys   = "list[" + ItemPasskeyLogin + "]/login/fido2Credentials"
	sealedItemPath = "encrypted/items[" + ItemPasskeyLogin + "]"
)

// Variants returns all variants in a fixed order.
func Variants() []Variant {
	return []Variant{
		{
			Name:   MissingItem,
			Models: "An item failed to decrypt and vanished from the plaintext export and the item list.",
			Changes: []string{
				"- plaintext/items[" + ItemPlainLogin + "]",
				"- list[" + ItemPlainLogin + "]",
			},
			apply: func(t testing.TB, f *Fixture) {
				f.PlaintextExport["items"] = withoutItem(f.PlaintextItems(), ItemPlainLogin)
				f.ItemList = withoutItem(f.ItemList, ItemPlainLogin)
			},
		},
		{
			Name:    DuplicateID,
			Models:  "The plaintext export contains the same item ID twice.",
			Changes: []string{"+ plaintext/items/3"},
			apply: func(t testing.TB, f *Fixture) {
				note := itemByID(f.PlaintextItems(), ItemNote)
				f.PlaintextExport["items"] = append(f.PlaintextItems(), cloneTree(note))
			},
		},
		{
			Name:    OnePasskeyMissing,
			Models:  "The second of two passkeys is missing from the decrypted view.",
			Changes: []string{"- " + plainPasskeys + "/1", "- " + listPasskeys + "/1"},
			apply: func(t testing.TB, f *Fixture) {
				f.eachDecryptedLogin(t, ItemPasskeyLogin, func(login map[string]any) {
					login["fido2Credentials"] = login["fido2Credentials"].([]any)[:1]
				})
			},
		},
		{
			Name:   AllPasskeysDropped,
			Models: "Tolerant decryption returned the item with an empty passkey list.",
			Changes: []string{
				"- " + plainPasskeys + "/0", "- " + plainPasskeys + "/1",
				"- " + listPasskeys + "/0", "- " + listPasskeys + "/1",
			},
			apply: func(t testing.TB, f *Fixture) {
				f.eachDecryptedLogin(t, ItemPasskeyLogin, func(login map[string]any) {
					login["fido2Credentials"] = []any{}
				})
			},
		},
		{
			Name:    BlobLoginItem,
			Models:  "The encrypted export shows the passkey login in blob form, so its passkey count is sealed.",
			Changes: sealedChanges(),
			apply: func(t testing.TB, f *Fixture) {
				seal(t, f, BlobData)
			},
		},
		{
			Name:    UnrecognizedItemForm,
			Models:  "The encrypted export shows the passkey login in a form that is neither legacy nor blob.",
			Changes: sealedChanges(),
			apply: func(t testing.TB, f *Fixture) {
				seal(t, f, UnknownSealedData)
			},
		},
		{
			Name:    EmptyKey,
			Models:  "The second passkey has an empty private key.",
			Changes: passkeyChanges(1, "keyValue"),
			apply: func(t testing.TB, f *Fixture) {
				f.eachDecryptedPasskey(t, ItemPasskeyLogin, 1, func(p map[string]any) {
					p["keyValue"] = ""
				})
			},
		},
		{
			Name:    UnparsableKey,
			Models:  "The second passkey declares P-256, but its key value is not a PKCS#8 key.",
			Changes: passkeyChanges(1, "keyValue"),
			apply: func(t testing.TB, f *Fixture) {
				garbage := base64.RawURLEncoding.EncodeToString([]byte(Marker + " bytes that are not a PKCS#8 key"))
				f.eachDecryptedPasskey(t, ItemPasskeyLogin, 1, func(p map[string]any) {
					p["keyValue"] = garbage
				})
			},
		},
		{
			Name:    NonNumericCounter,
			Models:  "The second passkey has a counter that is not a number.",
			Changes: passkeyChanges(1, "counter"),
			apply: func(t testing.TB, f *Fixture) {
				f.eachDecryptedPasskey(t, ItemPasskeyLogin, 1, func(p map[string]any) {
					p["counter"] = "x6"
				})
			},
		},
		{
			Name:    UnknownKeyAlgorithm,
			Models:  "The second passkey declares a key form the verification does not know (Ed25519).",
			Changes: passkeyChanges(1, "keyAlgorithm", "keyCurve", "keyValue"),
			apply: func(t testing.TB, f *Fixture) {
				keyValue := newEd25519KeyValue(t)
				f.eachDecryptedPasskey(t, ItemPasskeyLogin, 1, func(p map[string]any) {
					p["keyAlgorithm"] = "EdDSA"
					p["keyCurve"] = "Ed25519"
					p["keyValue"] = keyValue
				})
			},
		},
		{
			Name:    SwappedCredential,
			Models:  "The second passkey was replaced by a different valid passkey. The count is unchanged.",
			Changes: passkeyChanges(1, "credentialId", "keyValue"),
			apply: func(t testing.TB, f *Fixture) {
				other := newP256Passkey(t, 9, "6")
				f.eachDecryptedPasskey(t, ItemPasskeyLogin, 1, func(p map[string]any) {
					p["credentialId"] = SwappedCredentialID
					p["keyValue"] = other.keyValue
				})
			},
		},
		{
			Name:   OrganizationItems,
			Models: "The item list shows three items across two organizations. One of them has an attachment.",
			Changes: []string{
				"+ list[" + ItemOrgFirstA + "]",
				"+ list[" + ItemOrgFirstB + "]",
				"+ list[" + ItemOrgSecondA + "]",
			},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = append(f.ItemList,
					listItem(itemSpec{
						id: ItemOrgFirstA, label: "org-first-a", typ: typeLogin, organizationID: OrganizationFirst,
						attachments: []attachmentSpec{{id: AttachmentOrg, n: 9}},
					}),
					listItem(itemSpec{id: ItemOrgFirstB, label: "org-first-b", typ: typeSecureNote, organizationID: OrganizationFirst}),
					listItem(itemSpec{id: ItemOrgSecondA, label: "org-second-a", typ: typeLogin, organizationID: OrganizationSecond}),
				)
			},
		},
		{
			Name:    ListedItemNotExported,
			Models:  "The item list shows a personal item that neither export contains.",
			Changes: []string{"+ list[" + ItemListedOnly + "]"},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = append(f.ItemList, listItem(itemSpec{id: ItemListedOnly, label: "listed-only", typ: typeSecureNote}))
			},
		},
		{
			Name:    ExportedItemNotListed,
			Models:  "Both exports contain an item that the item list does not show, as an archived item would be in a default listing.",
			Changes: []string{"- list[" + ItemPlainLogin + "]"},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = withoutItem(f.ItemList, ItemPlainLogin)
			},
		},
	}
}

// eachDecryptedLogin calls fn with the login object of the item in the
// plaintext export and in the item list.
func (f *Fixture) eachDecryptedLogin(t testing.TB, itemID string, fn func(login map[string]any)) {
	t.Helper()
	for _, items := range [][]any{f.PlaintextItems(), f.ItemList} {
		item := itemByID(items, itemID)
		if item == nil {
			t.Fatalf("fixture has no item %s", itemID)
		}
		login, ok := item["login"].(map[string]any)
		if !ok {
			t.Fatalf("fixture item %s has no login", itemID)
		}
		fn(login)
	}
}

// eachDecryptedPasskey calls fn with the passkey at the given position in
// the plaintext export and in the item list.
func (f *Fixture) eachDecryptedPasskey(t testing.TB, itemID string, position int, fn func(passkey map[string]any)) {
	t.Helper()
	f.eachDecryptedLogin(t, itemID, func(login map[string]any) {
		passkeys, _ := login["fido2Credentials"].([]any)
		if position >= len(passkeys) {
			t.Fatalf("fixture item %s has no passkey at position %d", itemID, position)
		}
		fn(passkeys[position].(map[string]any))
	})
}

// seal replaces the legacy content of the passkey login in the encrypted
// export with one sealed data string, as the upstream exporter does for a
// blob item.
func seal(t testing.TB, f *Fixture, data string) {
	t.Helper()
	item := itemByID(f.EncryptedItems(), ItemPasskeyLogin)
	if item == nil {
		t.Fatalf("fixture has no encrypted item %s", ItemPasskeyLogin)
	}
	for _, key := range []string{"name", "notes", "fields", "login", "passwordHistory"} {
		delete(item, key)
	}
	item["data"] = data
}

func sealedChanges() []string {
	return []string{
		"+ " + sealedItemPath + "/data",
		"- " + sealedItemPath + "/fields",
		"- " + sealedItemPath + "/login",
		"- " + sealedItemPath + "/name",
		"- " + sealedItemPath + "/notes",
		"- " + sealedItemPath + "/passwordHistory",
	}
}

// passkeyChanges lists the given keys of one passkey in both decrypted
// documents.
func passkeyChanges(position int, keys ...string) []string {
	var out []string
	for _, base := range []string{plainPasskeys, listPasskeys} {
		for _, key := range keys {
			out = append(out, "~ "+base+"/"+string(rune('0'+position))+"/"+key)
		}
	}
	return out
}
