package capturetest

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"bw-takeout/internal/capture"
)

// Every variant differs from the intact fixture in the places it declares
// and nowhere else.
func TestVariantsDifferOnlyInTheIntendedPlace(t *testing.T) {
	for _, v := range Variants() {
		t.Run(v.Name, func(t *testing.T) {
			base := Intact(t)
			changed := base.Clone()
			v.Apply(t, changed)

			want := append([]string(nil), v.Changes...)
			sort.Strings(want)
			got := Diff(t, base, changed)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("places that differ from the intact fixture:\n got %v\nwant %v", got, want)
			}
			if len(got) == 0 {
				t.Error("variant changes nothing")
			}
			if v.Models == "" {
				t.Error("variant does not say what it models")
			}

			// All documents still render.
			changed.PlaintextExportJSON(t)
			changed.EncryptedExportJSON(t)
			changed.ItemListJSON(t)
			changed.ArchivedItemListJSON(t)
		})
	}
}

// The variants cover every failure case that task 2.3 of the change
// capture-verification, design D7 of the change archived-item-list and
// design D9 of the change capture-collection name.
func TestVariantsCoverTheNamedCases(t *testing.T) {
	want := []string{
		MissingItem, DuplicateID, OnePasskeyMissing, AllPasskeysDropped,
		BlobLoginItem, UnrecognizedItemForm, EmptyKey, UnparsableKey,
		NonNumericCounter, UnknownKeyAlgorithm, SwappedCredential,
		OrganizationItems, ListedItemNotExported, ExportedItemNotListed,
		ArchiveFilterIgnored, ArchiveFilterIgnoredNoArchivedItem,
		ListsDisagreeOnAttachments, ListsDisagreeOnOrganization,
		ArchivedListDuplicateID, ArchivedListInvalidJSON, ArchivedListAbsent,
		ArchivedAttachmentDownloadMissing, DecryptionErrorLines,
	}
	seen := map[string]bool{}
	for _, v := range Variants() {
		if seen[v.Name] {
			t.Errorf("variant %s is defined twice", v.Name)
		}
		seen[v.Name] = true
	}
	for _, name := range want {
		if !seen[name] {
			t.Errorf("no variant for %s", name)
		}
		delete(seen, name)
	}
	for name := range seen {
		t.Errorf("variant %s is not in the list of named cases", name)
	}
}

// The content of each change is what the variant claims, beyond its place.
func TestVariantContent(t *testing.T) {
	build := func(name string) *Fixture { return VariantByName(t, name).Build(t) }
	secondPasskey := func(f *Fixture) map[string]any {
		return passkeysOf(t, f.PlaintextItems(), ItemPasskeyLogin)[1]
	}
	parse := func(p map[string]any) (any, error) {
		der, err := base64.RawURLEncoding.DecodeString(p["keyValue"].(string))
		if err != nil {
			return nil, err
		}
		return x509.ParsePKCS8PrivateKey(der)
	}

	t.Run(DuplicateID, func(t *testing.T) {
		count := 0
		for _, raw := range build(DuplicateID).PlaintextItems() {
			if raw.(map[string]any)["id"] == ItemNote {
				count++
			}
		}
		if count != 2 {
			t.Errorf("note appears %d times, want 2", count)
		}
	})

	t.Run(BlobLoginItem, func(t *testing.T) {
		f := build(BlobLoginItem)
		item := itemByID(f.EncryptedItems(), ItemPasskeyLogin)
		data, _ := item["data"].(string)
		if !strings.Contains(data, `"format_version"`) {
			t.Errorf("data = %q lacks the blob marker", data)
		}
		if item["type"] != typeLogin || item["key"] == nil {
			t.Error("blob item lost its type or its key")
		}
		// The decrypted view still shows both passkeys.
		if got := len(passkeysOf(t, f.PlaintextItems(), ItemPasskeyLogin)); got != 2 {
			t.Errorf("plaintext export shows %d passkeys, want 2", got)
		}
	})

	t.Run(UnrecognizedItemForm, func(t *testing.T) {
		item := itemByID(build(UnrecognizedItemForm).EncryptedItems(), ItemPasskeyLogin)
		data, _ := item["data"].(string)
		if data == "" || strings.Contains(data, `"format_version"`) {
			t.Errorf("data = %q, want a sealed string without the blob marker", data)
		}
		if _, ok := item["login"]; ok {
			t.Error("item still has legacy content")
		}
	})

	t.Run(EmptyKey, func(t *testing.T) {
		if got := secondPasskey(build(EmptyKey))["keyValue"]; got != "" {
			t.Errorf("key value = %q, want empty", got)
		}
	})

	t.Run(UnparsableKey, func(t *testing.T) {
		p := secondPasskey(build(UnparsableKey))
		if p["keyAlgorithm"] != "ECDSA" || p["keyCurve"] != "P-256" {
			t.Error("passkey no longer declares P-256")
		}
		if _, err := base64.RawURLEncoding.DecodeString(p["keyValue"].(string)); err != nil {
			t.Errorf("key value does not decode, want decodable bytes that are no key: %v", err)
		}
		if _, err := parse(p); err == nil {
			t.Error("key value parses as PKCS#8")
		}
	})

	t.Run(NonNumericCounter, func(t *testing.T) {
		counter, _ := secondPasskey(build(NonNumericCounter))["counter"].(string)
		if counter == "" || strings.Trim(counter, "0123456789") == "" {
			t.Errorf("counter = %q, want a non-numeric string", counter)
		}
	})

	t.Run(UnknownKeyAlgorithm, func(t *testing.T) {
		p := secondPasskey(build(UnknownKeyAlgorithm))
		if p["keyAlgorithm"] == "ECDSA" {
			t.Error("passkey still declares ECDSA")
		}
		key, err := parse(p)
		if err != nil {
			t.Fatalf("key value is not a well-formed key: %v", err)
		}
		if _, ok := key.(ed25519.PrivateKey); !ok {
			t.Errorf("key is %T, want an Ed25519 key", key)
		}
	})

	t.Run(SwappedCredential, func(t *testing.T) {
		f := build(SwappedCredential)
		passkeys := passkeysOf(t, f.PlaintextItems(), ItemPasskeyLogin)
		if len(passkeys) != 2 {
			t.Fatalf("%d passkeys, want the count unchanged at 2", len(passkeys))
		}
		swapped := passkeys[1]
		if swapped["credentialId"] != SwappedCredentialID || swapped["counter"] != "6" {
			t.Errorf("swapped passkey: credential %v, counter %v", swapped["credentialId"], swapped["counter"])
		}
		key, err := parse(swapped)
		if err != nil {
			t.Fatalf("swapped key does not parse: %v", err)
		}
		if _, ok := key.(*ecdsa.PrivateKey); !ok {
			t.Errorf("swapped key is %T, want a valid ECDSA key", key)
		}
		// The reference still describes the original credential.
		original := f.Reference.Items[0].Passkeys[1]
		if original.CredentialID != credentialID(2) {
			t.Errorf("reference credential = %s, want the original", original.CredentialID)
		}
		// The new key counts as sensitive as well.
		found := false
		for _, s := range f.Sensitive() {
			if s == swapped["keyValue"] {
				found = true
			}
		}
		if !found {
			t.Error("Sensitive() lacks the swapped key")
		}
	})

	t.Run(OrganizationItems, func(t *testing.T) {
		f := build(OrganizationItems)
		perOrg := map[string]int{}
		for _, raw := range f.ItemList {
			if org, ok := raw.(map[string]any)["organizationId"].(string); ok {
				perOrg[org]++
			}
		}
		if len(perOrg) != 2 || perOrg[OrganizationFirst] != 2 || perOrg[OrganizationSecond] != 1 {
			t.Errorf("organization items per organization = %v, want 2 and 1", perOrg)
		}
		attachments := itemByID(f.ItemList, ItemOrgFirstA)["attachments"].([]any)
		if len(attachments) != 1 || attachments[0].(map[string]any)["id"] != AttachmentOrg {
			t.Error("first organization item lacks its attachment")
		}
		for _, a := range f.Attachments {
			if a.AttachmentID == AttachmentOrg {
				t.Error("a download exists for the organization attachment")
			}
		}
	})

	t.Run(ListedItemNotExported, func(t *testing.T) {
		f := build(ListedItemNotExported)
		item := itemByID(f.ItemList, ItemListedOnly)
		if item == nil {
			t.Fatal("item list lacks the extra item")
		}
		if org, ok := item["organizationId"].(string); ok {
			t.Errorf("extra item belongs to organization %s, want a personal item", org)
		}
	})
}

// The variants of the change archived-item-list hold what they claim.
func TestArchivedVariantContent(t *testing.T) {
	build := func(name string) *Fixture { return VariantByName(t, name).Build(t) }
	ids := func(items []any) []string {
		var out []string
		for _, raw := range items {
			out = append(out, raw.(map[string]any)["id"].(string))
		}
		return out
	}
	hasAttachmentOf := func(f *Fixture, itemID string) bool {
		for _, a := range f.Attachments {
			if a.ItemID == itemID {
				return true
			}
		}
		return false
	}

	for _, name := range []string{ArchiveFilterIgnored, ArchiveFilterIgnoredNoArchivedItem} {
		t.Run(name, func(t *testing.T) {
			f := build(name)
			if got, want := strings.Join(ids(f.ArchivedItemList), ","), strings.Join(ids(f.ItemList), ","); got != want {
				t.Errorf("archived list %s, want the default list %s", got, want)
			}
			if hasAttachmentOf(f, ItemArchivedLogin) {
				t.Error("a download exists for the attachment of the archived item")
			}
			inExport := itemByID(f.PlaintextItems(), ItemArchivedLogin) != nil
			if inExport != (name == ArchiveFilterIgnored) {
				t.Errorf("archived item in the plaintext export: %v", inExport)
			}
		})
	}

	t.Run(ListsDisagreeOnAttachments, func(t *testing.T) {
		f := build(ListsDisagreeOnAttachments)
		def := itemByID(f.ItemList, ItemPasskeyLogin)["attachments"].([]any)
		arc := itemByID(f.ArchivedItemList, ItemPasskeyLogin)["attachments"].([]any)
		if len(def) != 2 || len(arc) != 1 {
			t.Errorf("attachments %d in the default and %d in the archived list, want 2 and 1", len(def), len(arc))
		}
	})

	t.Run(ListsDisagreeOnOrganization, func(t *testing.T) {
		f := build(ListsDisagreeOnOrganization)
		if org := itemByID(f.ItemList, ItemNote)["organizationId"]; org != nil {
			t.Errorf("default list shows the note in organization %v", org)
		}
		if org := itemByID(f.ArchivedItemList, ItemNote)["organizationId"]; org != OrganizationFirst {
			t.Errorf("archived list shows the note in organization %v", org)
		}
	})

	t.Run(ArchivedListInvalidJSON, func(t *testing.T) {
		raw := build(ArchivedListInvalidJSON).ArchivedItemListJSON(t)
		var v any
		if len(raw) == 0 || json.Unmarshal(raw, &v) == nil {
			t.Errorf("archived list %q, want non-empty invalid JSON", raw)
		}
	})

	t.Run(ArchivedListAbsent, func(t *testing.T) {
		if raw := build(ArchivedListAbsent).ArchivedItemListJSON(t); len(raw) != 0 {
			t.Errorf("archived list %q, want zero bytes", raw)
		}
	})

	t.Run(DecryptionErrorLines, func(t *testing.T) {
		in := build(DecryptionErrorLines).Input(t)
		want := capture.DecryptionErrors{PlaintextExport: 3}
		if in.DecryptionErrors != want {
			t.Errorf("document counts = %+v, want %+v", in.DecryptionErrors, want)
		}
		for _, d := range in.Downloads {
			if got, want := d.DecryptionErrors, map[bool]int{true: 3}[d.AttachmentID == AttachmentFirst]; got != want {
				t.Errorf("download %s: count %d, want %d", d.AttachmentID, got, want)
			}
		}
	})

	t.Run(ArchivedAttachmentDownloadMissing, func(t *testing.T) {
		f := build(ArchivedAttachmentDownloadMissing)
		if hasAttachmentOf(f, ItemArchivedLogin) {
			t.Error("a download exists for the attachment of the archived item")
		}
		if itemByID(f.ArchivedItemList, ItemArchivedLogin) == nil {
			t.Error("archived list lost the archived item")
		}
	})
}
