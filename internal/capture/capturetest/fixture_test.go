package capturetest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// decode parses a rendered document back into a JSON tree.
func decode(t *testing.T, raw []byte) any {
	t.Helper()
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("rendered document is not valid JSON: %v", err)
	}
	return out
}

func passkeysOf(t *testing.T, items []any, itemID string) []map[string]any {
	t.Helper()
	item := itemByID(items, itemID)
	if item == nil {
		t.Fatalf("no item %s", itemID)
	}
	login, ok := item["login"].(map[string]any)
	if !ok {
		t.Fatalf("item %s has no login", itemID)
	}
	raw, _ := login["fido2Credentials"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		out = append(out, p.(map[string]any))
	}
	return out
}

func TestIntactDocumentForms(t *testing.T) {
	f := Intact(t)

	plaintext := decode(t, f.PlaintextExportJSON(t)).(map[string]any)
	if plaintext["encrypted"] != false {
		t.Errorf("plaintext export: encrypted = %v, want false", plaintext["encrypted"])
	}
	if got := len(plaintext["items"].([]any)); got != 3 {
		t.Errorf("plaintext export: %d items, want 3", got)
	}

	encrypted := decode(t, f.EncryptedExportJSON(t)).(map[string]any)
	if encrypted["encrypted"] != true {
		t.Errorf("encrypted export: encrypted = %v, want true", encrypted["encrypted"])
	}
	if _, ok := encrypted["encKeyValidation_DO_NOT_EDIT"].(string); !ok {
		t.Error("encrypted export: no key validation field")
	}
	for _, key := range []string{"passwordProtected", "data"} {
		if _, ok := encrypted[key]; ok {
			t.Errorf("encrypted export: has %q, which only the password-protected form has", key)
		}
	}
	if got := len(encrypted["items"].([]any)); got != 3 {
		t.Errorf("encrypted export: %d items, want 3", got)
	}

	list, ok := decode(t, f.ItemListJSON(t)).([]any)
	if !ok {
		t.Fatal("item list is not a bare array")
	}
	if len(list) != 3 {
		t.Errorf("item list: %d items, want 3", len(list))
	}
	for _, raw := range list {
		if raw.(map[string]any)["object"] != "item" {
			t.Errorf("item list: element without object \"item\"")
		}
	}
}

func TestIntactPasskeys(t *testing.T) {
	f := Intact(t)

	plain := passkeysOf(t, f.PlaintextItems(), ItemPasskeyLogin)
	if len(plain) != 2 {
		t.Fatalf("plaintext export: %d passkeys, want 2", len(plain))
	}
	if plain[0]["counter"] != "0" || plain[1]["counter"] != "6" {
		t.Errorf("counters = %v and %v, want \"0\" and \"6\"", plain[0]["counter"], plain[1]["counter"])
	}
	if plain[0]["keyValue"] == plain[1]["keyValue"] {
		t.Error("both passkeys hold the same key")
	}
	if plain[0]["credentialId"] == plain[1]["credentialId"] {
		t.Error("both passkeys share one credential ID")
	}

	ref := f.Reference.Items[0]
	if ref.ItemID != ItemPasskeyLogin || len(ref.Passkeys) != 2 {
		t.Fatalf("reference: first item %s with %d passkeys", ref.ItemID, len(ref.Passkeys))
	}
	for i, p := range plain {
		der, err := base64.RawURLEncoding.DecodeString(p["keyValue"].(string))
		if err != nil {
			t.Fatalf("passkey %d: key value is not base64url without padding: %v", i, err)
		}
		parsed, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			t.Fatalf("passkey %d: key value is not PKCS#8: %v", i, err)
		}
		key, ok := parsed.(*ecdsa.PrivateKey)
		if !ok || key.Curve != elliptic.P256() {
			t.Errorf("passkey %d: key is not a P-256 ECDSA key", i)
		}
		sum := sha256.Sum256(der)
		if ref.Passkeys[i].KeySHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("passkey %d: reference fingerprint does not match the exported key", i)
		}
		if ref.Passkeys[i].CredentialID != p["credentialId"] {
			t.Errorf("passkey %d: reference credential ID does not match", i)
		}
	}
	if ref.Passkeys[0].Counter != 0 || ref.Passkeys[1].Counter != 6 {
		t.Errorf("reference counters = %d and %d, want 0 and 6", ref.Passkeys[0].Counter, ref.Passkeys[1].Counter)
	}

	// The encrypted export shows the count and nothing readable.
	sealed := passkeysOf(t, f.EncryptedItems(), ItemPasskeyLogin)
	if len(sealed) != 2 {
		t.Fatalf("encrypted export: %d passkeys, want 2", len(sealed))
	}
	for i, p := range sealed {
		for key, value := range p {
			if key == "creationDate" {
				continue
			}
			if s, _ := value.(string); !strings.HasPrefix(s, "2.") || strings.Count(s, "|") != 2 {
				t.Errorf("encrypted passkey %d: %s is not in encrypted-string notation", i, key)
			}
		}
	}

	// A login without passkeys: empty array when decrypted, no key when
	// encrypted.
	if got := passkeysOf(t, f.PlaintextItems(), ItemPlainLogin); len(got) != 0 {
		t.Errorf("plain login has %d passkeys in the plaintext export", len(got))
	}
	login := itemByID(f.EncryptedItems(), ItemPlainLogin)["login"].(map[string]any)
	if _, ok := login["fido2Credentials"]; ok {
		t.Error("encrypted plain login carries fido2Credentials, want the key left out")
	}
}

func TestIntactAttachments(t *testing.T) {
	f := Intact(t)

	listed := itemByID(f.ItemList, ItemPasskeyLogin)["attachments"].([]any)
	if len(listed) != 2 {
		t.Fatalf("item list: %d attachments, want 2", len(listed))
	}
	first, second := listed[0].(map[string]any), listed[1].(map[string]any)
	if first["fileName"] != AttachmentFileName || second["fileName"] != AttachmentFileName {
		t.Errorf("file names = %v and %v, want both %q", first["fileName"], second["fileName"], AttachmentFileName)
	}
	if first["id"] != AttachmentFirst || second["id"] != AttachmentSecond {
		t.Errorf("attachment IDs = %v and %v", first["id"], second["id"])
	}
	if _, ok := first["size"].(string); !ok {
		t.Error("attachment size is not a string")
	}

	if len(f.Attachments) != 2 {
		t.Fatalf("%d attachment contents, want 2", len(f.Attachments))
	}
	if string(f.Attachments[0].Content) == string(f.Attachments[1].Content) {
		t.Error("both attachments hold the same content")
	}
	ref := f.Reference.Items[0].Attachments
	for i, a := range f.Attachments {
		if a.ItemID != ItemPasskeyLogin {
			t.Errorf("attachment %d belongs to %s", i, a.ItemID)
		}
		sum := sha256.Sum256(a.Content)
		if ref[i].AttachmentID != a.AttachmentID || ref[i].SHA256 != hex.EncodeToString(sum[:]) || ref[i].Size != int64(len(a.Content)) {
			t.Errorf("attachment %d: reference does not match the content", i)
		}
	}

	// The exports carry no attachment inventory.
	for name, items := range map[string][]any{"plaintext": f.PlaintextItems(), "encrypted": f.EncryptedItems()} {
		if _, ok := itemByID(items, ItemPasskeyLogin)["attachments"]; ok {
			t.Errorf("%s export lists attachments", name)
		}
	}
}

func TestSentinelInEverySensitiveField(t *testing.T) {
	f := Intact(t)

	sawDecrypted := false
	for _, l := range f.sensitiveLeaves() {
		if l.value == "" {
			t.Errorf("%s: %s is empty", l.document, l.key)
		}
		if l.document == "encrypted" {
			if l.key != "data" && !strings.HasPrefix(l.value, "2.") {
				t.Errorf("encrypted export: %s holds a readable value", l.key)
			}
			continue
		}
		sawDecrypted = true
		if formatBoundKeys[l.key] {
			if len(l.value) < 20 {
				t.Errorf("%s: %s is too short to search for", l.document, l.key)
			}
			continue
		}
		if !hasMarker(l.value) {
			t.Errorf("%s: %s = %q carries no sentinel", l.document, l.key, l.value)
		}
	}
	if !sawDecrypted {
		t.Fatal("no sensitive field found in the decrypted documents")
	}

	sensitive := f.Sensitive()
	want := []string{
		AttachmentFileName,
		sentinel("passkey-login-name"),
		sentinel("passkey-login-password"),
		sentinel("note-notes"),
		string(f.Attachments[0].Content),
		passkeysOf(t, f.PlaintextItems(), ItemPasskeyLogin)[1]["keyValue"].(string),
		credentialID(2),
	}
	for _, w := range want {
		found := false
		for _, s := range sensitive {
			if s == w {
				found = true
			}
		}
		if !found {
			t.Errorf("Sensitive() lacks %q", w)
		}
	}
	// IDs identify findings and must stay out of the list.
	for _, s := range sensitive {
		for _, id := range []string{ItemPasskeyLogin, ItemPlainLogin, ItemNote, AttachmentFirst, AttachmentSecond, FolderID} {
			if s == id {
				t.Errorf("Sensitive() contains the ID %s", id)
			}
		}
	}
}

func TestKeysAreGeneratedPerFixture(t *testing.T) {
	a := passkeysOf(t, Intact(t).PlaintextItems(), ItemPasskeyLogin)
	b := passkeysOf(t, Intact(t).PlaintextItems(), ItemPasskeyLogin)
	for i := range a {
		if a[i]["keyValue"] == b[i]["keyValue"] {
			t.Errorf("passkey %d: two fixtures share one key, so the key is not generated at test time", i)
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	f := Intact(t)
	c := f.Clone()
	if d := Diff(t, f, c); len(d) != 0 {
		t.Fatalf("clone differs from its source: %v", d)
	}

	itemByID(c.PlaintextItems(), ItemNote)["name"] = "changed"
	c.Attachments[0].Content[0] = 'X'
	c.Reference.Items[0].Passkeys[0].Counter = 99

	want := []string{"~ attachments", "~ plaintext/items[" + ItemNote + "]/name", "~ reference"}
	if got := Diff(t, f, c); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diff after changing the clone:\n got %v\nwant %v", got, want)
	}
}

// Two fresh fixtures differ in the generated keys and in nothing else.
func TestFixturesDifferOnlyInKeys(t *testing.T) {
	got := Diff(t, Intact(t), Intact(t))
	want := append(passkeyChanges(0, "keyValue"), passkeyChanges(1, "keyValue")...)
	want = append(want, "~ reference")
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diff between two fresh fixtures:\n got %v\nwant %v", got, want)
	}
}
