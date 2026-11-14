// Package capturetest builds synthetic captures of a personal vault in
// memory for tests. Nothing in it comes from a real account.
//
// The documents follow the shapes recorded in the documentation of package
// capture. Their evidence level is "mock": the shapes were derived from the
// upstream source and no live capture backs them yet.
//
// Passkey keys are generated on every call, so the repository holds no key
// material. Every sensitive field of the decrypted documents carries a value
// that contains [Marker], except for the three fields whose format is fixed
// (key value, credential ID, user handle). [Fixture.Sensitive] returns all of
// them so that a test can search a result for leaks.
//
// The documents are plain JSON trees, so a variant can break any part of
// them. Rendering sorts object keys alphabetically. The CLI writes them in
// declaration order, which carries no meaning for a JSON parser.
package capturetest

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"bitwarden-backup/internal/capture"
)

// Marker is part of every sentinel value.
const Marker = "sentinel"

// IDs of the intact fixture. They are synthetic and carry no meaning.
const (
	// ItemPasskeyLogin is a login item with two passkeys and two attachments.
	ItemPasskeyLogin = "11111111-1111-4111-8111-111111111111"
	// ItemPlainLogin is a login item without passkeys and without attachments.
	ItemPlainLogin = "22222222-2222-4222-8222-222222222222"
	// ItemNote is a secure note without attachments.
	ItemNote = "33333333-3333-4333-8333-333333333333"

	// AttachmentFirst and AttachmentSecond belong to ItemPasskeyLogin and
	// share one file name.
	AttachmentFirst  = "att00000000000000000000000000001"
	AttachmentSecond = "att00000000000000000000000000002"

	// FolderID is the folder that holds ItemPlainLogin.
	FolderID = "f0000000-0000-4000-8000-000000000001"

	// AttachmentFileName is the file name both attachments share.
	AttachmentFileName = Marker + "-attachment-name.txt"
)

// Item types as the CLI numbers them.
const (
	typeLogin      = 1
	typeSecureNote = 2
)

const (
	creationDate = "2026-01-02T03:04:05.000Z"
	revisionDate = "2026-02-03T04:05:06.000Z"
)

// Fixture is one synthetic capture: the three documents, the bytes a
// successful download of each attachment returns, and the reference
// inventory the builder knows to be true.
type Fixture struct {
	PlaintextExport map[string]any
	EncryptedExport map[string]any
	ItemList        []any
	Attachments     []Attachment
	Reference       Reference
}

// Attachment is the content of one attachment, identified by ID pair.
type Attachment struct {
	ItemID       string
	AttachmentID string
	Content      []byte
}

// Reference is the inventory the fixture was built from. It is recorded
// outside the documents, so a variant that changes a document leaves it
// untouched. It holds fingerprints and never key material.
type (
	Reference           = capture.Reference
	ReferenceItem       = capture.ReferenceItem
	ReferencePasskey    = capture.ReferencePasskey
	ReferenceAttachment = capture.ReferenceAttachment
)

// Input renders the three documents and records one successful download
// result per attachment.
func (f *Fixture) Input(t testing.TB) capture.Input {
	t.Helper()
	in := capture.Input{
		PlaintextExport: f.PlaintextExportJSON(t),
		EncryptedExport: f.EncryptedExportJSON(t),
		ItemList:        f.ItemListJSON(t),
	}
	for _, a := range f.Attachments {
		sum := sha256.Sum256(a.Content)
		in.Downloads = append(in.Downloads, capture.Download{
			ItemID: a.ItemID, AttachmentID: a.AttachmentID,
			SHA256: hex.EncodeToString(sum[:]), Size: int64(len(a.Content)),
		})
	}
	return in
}

// Capture parses the rendered documents.
func (f *Fixture) Capture(t testing.TB) *capture.Capture {
	t.Helper()
	return capture.Parse(f.Input(t))
}

// Intact returns a capture that is complete and consistent: three personal
// items, one of them a login with two passkeys (counters 0 and 6) and two
// attachments with the same file name.
func Intact(t testing.TB) *Fixture {
	t.Helper()

	first := newP256Passkey(t, 1, "0")
	second := newP256Passkey(t, 2, "6")

	specs := []itemSpec{
		{
			id: ItemPasskeyLogin, label: "passkey-login", typ: typeLogin,
			passkeys: []passkeySpec{first, second},
			attachments: []attachmentSpec{
				{id: AttachmentFirst, n: 1, content: []byte(Marker + " attachment content one")},
				{id: AttachmentSecond, n: 2, content: []byte(Marker + " attachment content two, longer")},
			},
		},
		// This item writes its empty optional keys as null. The other two
		// leave them out. Both spellings are possible at code level.
		{id: ItemPlainLogin, label: "plain-login", typ: typeLogin, folderID: FolderID, explicitNulls: true},
		{id: ItemNote, label: "note", typ: typeSecureNote},
	}

	f := &Fixture{
		PlaintextExport: map[string]any{
			"encrypted": false,
			"folders":   []any{map[string]any{"id": FolderID, "name": sentinel("folder-name")}},
			"items":     []any{},
		},
		EncryptedExport: map[string]any{
			"encrypted":                    true,
			"encKeyValidation_DO_NOT_EDIT": enc("key-validation"),
			"folders":                      []any{map[string]any{"id": FolderID, "name": enc("folder-name")}},
			"items":                        []any{},
		},
		ItemList: []any{},
	}
	for _, s := range specs {
		f.PlaintextExport["items"] = append(f.PlaintextItems(), plainItem(s))
		f.EncryptedExport["items"] = append(f.EncryptedItems(), encryptedItem(s))
		f.ItemList = append(f.ItemList, listItem(s))

		ref := ReferenceItem{ItemID: s.id}
		for _, p := range s.passkeys {
			ref.Passkeys = append(ref.Passkeys, p.reference(t))
		}
		for _, a := range s.attachments {
			sum := sha256.Sum256(a.content)
			ref.Attachments = append(ref.Attachments, ReferenceAttachment{
				AttachmentID: a.id, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(a.content)),
			})
			f.Attachments = append(f.Attachments, Attachment{ItemID: s.id, AttachmentID: a.id, Content: a.content})
		}
		f.Reference.Items = append(f.Reference.Items, ref)
	}
	return f
}

// PlaintextItems returns the items array of the plaintext export.
func (f *Fixture) PlaintextItems() []any {
	items, _ := f.PlaintextExport["items"].([]any)
	return items
}

// EncryptedItems returns the items array of the encrypted export.
func (f *Fixture) EncryptedItems() []any {
	items, _ := f.EncryptedExport["items"].([]any)
	return items
}

// PlaintextExportJSON renders the plaintext export the way
// "bw export --format json" lays it out, indented by two spaces.
func (f *Fixture) PlaintextExportJSON(t testing.TB) []byte {
	t.Helper()
	return render(t, f.PlaintextExport, true)
}

// EncryptedExportJSON renders the account-bound encrypted export.
func (f *Fixture) EncryptedExportJSON(t testing.TB) []byte {
	t.Helper()
	return render(t, f.EncryptedExport, true)
}

// ItemListJSON renders the item list as the bare array that
// "bw list items" prints.
func (f *Fixture) ItemListJSON(t testing.TB) []byte {
	t.Helper()
	return render(t, f.ItemList, false)
}

// Clone returns a deep copy.
func (f *Fixture) Clone() *Fixture {
	c := &Fixture{
		PlaintextExport: cloneTree(f.PlaintextExport).(map[string]any),
		EncryptedExport: cloneTree(f.EncryptedExport).(map[string]any),
		ItemList:        cloneTree(f.ItemList).([]any),
	}
	for _, a := range f.Attachments {
		a.Content = append([]byte(nil), a.Content...)
		c.Attachments = append(c.Attachments, a)
	}
	for _, item := range f.Reference.Items {
		item.Passkeys = append([]ReferencePasskey(nil), item.Passkeys...)
		item.Attachments = append([]ReferenceAttachment(nil), item.Attachments...)
		c.Reference.Items = append(c.Reference.Items, item)
	}
	return c
}

// Keys whose string values are identifiers, dates or fixed vocabulary. Every
// other string in a document counts as sensitive.
var (
	safeDecryptedKeys = keySet(
		"id", "organizationId", "folderId", "collectionIds", "object",
		"keyType", "keyAlgorithm", "keyCurve", "counter", "discoverable",
		"creationDate", "revisionDate", "deletedDate", "archivedDate",
		"lastUsedDate", "passwordRevisionDate", "size", "sizeName",
	)
	// In the encrypted export the passkey vocabulary is ciphertext too.
	safeEncryptedKeys = keySet(
		"id", "organizationId", "folderId", "collectionIds",
		"creationDate", "revisionDate", "deletedDate", "archivedDate",
		"lastUsedDate",
	)
)

// formatBoundKeys name the sensitive fields whose value has a fixed format
// and therefore cannot contain [Marker] literally.
var formatBoundKeys = keySet("keyValue", "credentialId", "userHandle")

// Sensitive returns every value that must never show up in a verification
// result: all strings of the three documents that are not identifiers, dates
// or fixed vocabulary, and the attachment contents. The list is sorted and
// free of duplicates and empty strings.
func (f *Fixture) Sensitive() []string {
	seen := map[string]bool{}
	for _, leaf := range f.sensitiveLeaves() {
		seen[leaf.value] = true
	}
	for _, a := range f.Attachments {
		seen[string(a.Content)] = true
	}
	delete(seen, "")
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// leaf is one string value of a document together with the key it sits
// under and the document it comes from.
type leaf struct {
	document string
	key      string
	value    string
}

func (f *Fixture) sensitiveLeaves() []leaf {
	var out []leaf
	collectLeaves("plaintext", "", f.PlaintextExport, safeDecryptedKeys, &out)
	collectLeaves("list", "", f.ItemList, safeDecryptedKeys, &out)
	collectLeaves("encrypted", "", f.EncryptedExport, safeEncryptedKeys, &out)
	return out
}

func collectLeaves(document, key string, node any, safe map[string]bool, out *[]leaf) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			collectLeaves(document, k, child, safe, out)
		}
	case []any:
		for _, child := range v {
			collectLeaves(document, key, child, safe, out)
		}
	case string:
		if !safe[key] {
			*out = append(*out, leaf{document: document, key: key, value: v})
		}
	}
}

// itemSpec describes one item. The three documents are rendered from it.
type itemSpec struct {
	id, label      string
	typ            int
	folderID       string
	organizationID string
	passkeys       []passkeySpec
	attachments    []attachmentSpec
	// explicitNulls writes empty optional keys as null instead of leaving
	// them out.
	explicitNulls bool
}

type attachmentSpec struct {
	id      string
	n       int
	content []byte
}

// passkeySpec holds the decrypted fields of one passkey.
type passkeySpec struct {
	n            int
	credentialID string
	keyAlgorithm string
	keyCurve     string
	keyValue     string
	counter      string
}

func (p passkeySpec) reference(t testing.TB) ReferencePasskey {
	t.Helper()
	der, err := base64.RawURLEncoding.DecodeString(p.keyValue)
	if err != nil {
		t.Fatalf("decode key value of passkey %d: %v", p.n, err)
	}
	sum := sha256.Sum256(der)
	var counter uint64
	for _, c := range p.counter {
		counter = counter*10 + uint64(c-'0')
	}
	return ReferencePasskey{CredentialID: p.credentialID, KeySHA256: hex.EncodeToString(sum[:]), Counter: counter}
}

// newP256Passkey generates a fresh P-256 key and returns a passkey that
// holds it the way the CLI exports it: PKCS#8 DER in base64url without
// padding.
func newP256Passkey(t testing.TB, n int, counter string) passkeySpec {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate P-256 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal P-256 key: %v", err)
	}
	return passkeySpec{
		n:            n,
		credentialID: credentialID(n),
		keyAlgorithm: "ECDSA",
		keyCurve:     "P-256",
		keyValue:     base64.RawURLEncoding.EncodeToString(der),
		counter:      counter,
	}
}

// newEd25519KeyValue generates a fresh Ed25519 key in the same encoding. The
// CLI does not create such passkeys. The value serves the variant with an
// unknown key algorithm.
func newEd25519KeyValue(t testing.TB) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal Ed25519 key: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(der)
}

// credentialID returns a synthetic UUID for passkey n.
func credentialID(n int) string {
	return "c0000000-0000-4000-8000-00000000000" + string(rune('0'+n))
}

func sentinel(label string) string {
	return Marker + "-" + label
}

// enc returns an opaque placeholder in the CLI's encrypted-string notation.
// It is not ciphertext and nothing can decrypt it.
func enc(label string) string {
	part := func(kind string) string {
		return base64.StdEncoding.EncodeToString([]byte(Marker + " " + kind + " " + label))
	}
	return "2." + part("iv") + "|" + part("ct") + "|" + part("mac")
}

// plainItem renders an item as the plaintext export shows it.
func plainItem(s itemSpec) map[string]any {
	m := map[string]any{
		"id":            s.id,
		"type":          s.typ,
		"reprompt":      0,
		"favorite":      false,
		"collectionIds": nil,
		"name":          sentinel(s.label + "-name"),
		"notes":         sentinel(s.label + "-notes"),
		"fields": []any{map[string]any{
			"name":     sentinel(s.label + "-field-name"),
			"value":    sentinel(s.label + "-field-value"),
			"type":     1,
			"linkedId": nil,
		}},
		"creationDate": creationDate,
		"revisionDate": revisionDate,
	}
	setOptional(m, s)
	switch s.typ {
	case typeLogin:
		passkeys := []any{}
		for _, p := range s.passkeys {
			passkeys = append(passkeys, plainPasskey(p))
		}
		m["login"] = map[string]any{
			"uris": []any{map[string]any{
				"uri":   "https://" + sentinel(s.label+"-uri") + ".invalid/login",
				"match": nil,
			}},
			"username":         sentinel(s.label + "-username"),
			"password":         sentinel(s.label + "-password"),
			"totp":             sentinel(s.label + "-totp"),
			"fido2Credentials": passkeys,
		}
		m["passwordHistory"] = []any{map[string]any{
			"password":     sentinel(s.label + "-old-password"),
			"lastUsedDate": creationDate,
		}}
	case typeSecureNote:
		m["secureNote"] = map[string]any{"type": 0}
	}
	return m
}

func plainPasskey(p passkeySpec) map[string]any {
	label := "passkey-" + string(rune('0'+p.n))
	return map[string]any{
		"credentialId":    p.credentialID,
		"keyType":         "public-key",
		"keyAlgorithm":    p.keyAlgorithm,
		"keyCurve":        p.keyCurve,
		"keyValue":        p.keyValue,
		"rpId":            sentinel(label+"-rp-id") + ".invalid",
		"userHandle":      base64.RawURLEncoding.EncodeToString([]byte(sentinel(label + "-user-handle"))),
		"userName":        sentinel(label + "-user-name"),
		"counter":         p.counter,
		"rpName":          sentinel(label + "-rp-name"),
		"userDisplayName": sentinel(label + "-user-display-name"),
		"discoverable":    "true",
		"creationDate":    creationDate,
	}
}

// listItem renders an item as "bw list items" shows it.
func listItem(s itemSpec) map[string]any {
	m := plainItem(s)
	m["object"] = "item"
	m["collectionIds"] = []any{}
	if s.organizationID != "" {
		m["collectionIds"] = []any{"c011ec71-0000-4000-8000-" + s.organizationID[len(s.organizationID)-12:]}
	}
	attachments := []any{}
	for _, a := range s.attachments {
		attachments = append(attachments, map[string]any{
			"id":       a.id,
			"fileName": AttachmentFileName,
			// The size metadata may describe ciphertext. It deliberately
			// differs from the content length.
			"size":     "1337",
			"sizeName": "1.31 KB",
			"url":      "https://" + sentinel("attachment-url") + ".invalid/" + a.id + "?token=" + sentinel("url-token-"+string(rune('0'+a.n))),
		})
	}
	m["attachments"] = attachments
	if login, ok := m["login"].(map[string]any); ok {
		login["passwordRevisionDate"] = nil
	}
	return m
}

// encryptedItem renders an item in the legacy form of the account-bound
// encrypted export.
func encryptedItem(s itemSpec) map[string]any {
	m := map[string]any{
		"id":            s.id,
		"type":          s.typ,
		"reprompt":      0,
		"favorite":      false,
		"collectionIds": nil,
		"key":           enc(s.label + "-key"),
		"name":          enc(s.label + "-name"),
		"notes":         enc(s.label + "-notes"),
		"fields": []any{map[string]any{
			"name":     enc(s.label + "-field-name"),
			"value":    enc(s.label + "-field-value"),
			"type":     1,
			"linkedId": nil,
		}},
		"creationDate": creationDate,
		"revisionDate": revisionDate,
	}
	setOptional(m, s)
	switch s.typ {
	case typeLogin:
		login := map[string]any{
			"uris": []any{map[string]any{
				"uri":         enc(s.label + "-uri"),
				"uriChecksum": enc(s.label + "-uri-checksum"),
				"match":       nil,
			}},
			"username": enc(s.label + "-username"),
			"password": enc(s.label + "-password"),
			"totp":     enc(s.label + "-totp"),
		}
		// The encrypted login leaves the key out when there are no passkeys.
		if len(s.passkeys) > 0 {
			passkeys := []any{}
			for _, p := range s.passkeys {
				passkeys = append(passkeys, encryptedPasskey(p))
			}
			login["fido2Credentials"] = passkeys
		}
		m["login"] = login
		m["passwordHistory"] = []any{map[string]any{
			"password":     enc(s.label + "-old-password"),
			"lastUsedDate": creationDate,
		}}
	case typeSecureNote:
		m["secureNote"] = map[string]any{"type": 0}
	}
	return m
}

func encryptedPasskey(p passkeySpec) map[string]any {
	label := "passkey-" + string(rune('0'+p.n))
	m := map[string]any{"creationDate": creationDate}
	for _, key := range []string{
		"credentialId", "keyType", "keyAlgorithm", "keyCurve", "keyValue",
		"rpId", "userHandle", "userName", "counter", "rpName",
		"userDisplayName", "discoverable",
	} {
		m[key] = enc(label + "-" + key)
	}
	return m
}

// setOptional writes the organization ID, the folder ID and the two optional
// dates.
func setOptional(m map[string]any, s itemSpec) {
	if s.explicitNulls {
		m["organizationId"] = nil
		m["folderId"] = nil
		m["deletedDate"] = nil
		m["archivedDate"] = nil
	}
	if s.organizationID != "" {
		m["organizationId"] = s.organizationID
	}
	if s.folderID != "" {
		m["folderId"] = s.folderID
	}
}

func render(t testing.TB, doc any, indent bool) []byte {
	t.Helper()
	var (
		out []byte
		err error
	)
	if indent {
		out, err = json.MarshalIndent(doc, "", "  ")
	} else {
		out, err = json.Marshal(doc)
	}
	if err != nil {
		t.Fatalf("render fixture document: %v", err)
	}
	return out
}

func cloneTree(node any) any {
	switch v := node.(type) {
	case map[string]any:
		c := make(map[string]any, len(v))
		for k, child := range v {
			c[k] = cloneTree(child)
		}
		return c
	case []any:
		c := make([]any, len(v))
		for i, child := range v {
			c[i] = cloneTree(child)
		}
		return c
	default:
		return v
	}
}

func keySet(keys ...string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

// itemByID returns the first item with the given ID, or nil.
func itemByID(items []any, id string) map[string]any {
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && item["id"] == id {
			return item
		}
	}
	return nil
}

// withoutItem returns the items without those that have the given ID.
func withoutItem(items []any, id string) []any {
	out := []any{}
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok && item["id"] == id {
			continue
		}
		out = append(out, raw)
	}
	return out
}

// hasMarker reports whether a value carries the sentinel marker.
func hasMarker(value string) bool {
	return strings.Contains(value, Marker)
}
