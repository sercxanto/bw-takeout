package capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
)

// Problem says why a document was rejected.
type Problem string

// Problems with a document. None of them carries document content.
const (
	ProblemInvalidJSON        Problem = "invalid_json"
	ProblemWrongForm          Problem = "wrong_form"
	ProblemMarkedEncrypted    Problem = "marked_encrypted"
	ProblemNotMarkedEncrypted Problem = "not_marked_encrypted"
	ProblemPasswordProtected  Problem = "password_protected"
	ProblemNotAccountBound    Problem = "not_account_bound"
	ProblemResponseEnvelope   Problem = "response_envelope"
	ProblemMalformedItem      Problem = "malformed_item"
)

// DocumentError rejects a document. It names the document, the problem and,
// when the problem lies in one identifiable item, the item ID. It never
// holds document content or parser messages, because those can quote
// secrets.
type DocumentError struct {
	Document Document
	Problem  Problem
	ItemID   string
}

func (e *DocumentError) Error() string {
	msg := string(e.Document) + ": " + string(e.Problem)
	if e.ItemID != "" {
		msg += " in item " + e.ItemID
	}
	return msg
}

// blobMarker identifies a blob item. Upstream checks for the same string
// with the quotes (see the package documentation).
const blobMarker = `"format_version"`

// typeObjectKeys maps each known item type to the key of its type-specific
// object (CipherType and CipherExport.buildContent, see the package
// documentation).
var typeObjectKeys = map[int]string{
	1: "login",
	2: "secureNote",
	3: "card",
	4: "identity",
	5: "sshKey",
	6: "bankAccount",
	7: "driversLicense",
	8: "passport",
}

// Parse parses the three documents of a capture. It never fails: a rejected
// document is recorded in Capture.Errors and left nil.
func Parse(in Input) *Capture {
	c := &Capture{Downloads: in.Downloads}
	record := func(err error) {
		var de *DocumentError
		if errors.As(err, &de) {
			c.Errors = append(c.Errors, de)
		} else if err != nil {
			panic("capture: parser returned an error that is not a DocumentError")
		}
	}
	var err error
	c.Plaintext, err = ParsePlaintextExport(in.PlaintextExport)
	record(err)
	c.Encrypted, err = ParseEncryptedExport(in.EncryptedExport)
	record(err)
	c.List, err = ParseItemList(in.ItemList)
	record(err)

	refs := make([]AttachmentRef, 0, len(in.Downloads))
	for _, d := range in.Downloads {
		refs = append(refs, d.Ref())
	}
	c.DuplicateDownloads = duplicateRefs(refs)
	return c
}

// ParsePlaintextExport parses the document of "bw export --format json".
// The error is always a *DocumentError.
func ParsePlaintextExport(raw []byte) (*PlaintextExport, error) {
	const doc = PlaintextExportDocument
	root, err := decodeObject(doc, raw)
	if err != nil {
		return nil, err
	}
	if root["passwordProtected"] == true {
		return nil, &DocumentError{Document: doc, Problem: ProblemPasswordProtected}
	}
	switch root["encrypted"] {
	case false:
	case true:
		return nil, &DocumentError{Document: doc, Problem: ProblemMarkedEncrypted}
	default:
		return nil, &DocumentError{Document: doc, Problem: ProblemWrongForm}
	}
	elements, ok := root["items"].([]any)
	if !ok {
		return nil, &DocumentError{Document: doc, Problem: ProblemWrongForm}
	}

	out := &PlaintextExport{Items: make([]PlaintextItem, 0, len(elements))}
	ids := make([]string, 0, len(elements))
	for _, raw := range elements {
		obj, id, err := itemObject(doc, raw)
		if err != nil {
			return nil, err
		}
		item := PlaintextItem{ID: id}
		if item.OrganizationID, ok = optionalString(obj, "organizationId"); !ok {
			return nil, malformed(doc, id)
		}
		if item.Passkeys, ok = plaintextPasskeys(obj); !ok {
			return nil, malformed(doc, id)
		}
		out.Items = append(out.Items, item)
		ids = append(ids, id)
	}
	out.DuplicateIDs = duplicateStrings(ids)
	return out, nil
}

// ParseEncryptedExport parses the document of
// "bw export --format encrypted_json" without a password and recognizes the
// format of every item. The error is always a *DocumentError.
func ParseEncryptedExport(raw []byte) (*EncryptedExport, error) {
	const doc = EncryptedExportDocument
	root, err := decodeObject(doc, raw)
	if err != nil {
		return nil, err
	}
	if root["passwordProtected"] == true {
		return nil, &DocumentError{Document: doc, Problem: ProblemPasswordProtected}
	}
	if root["encrypted"] != true {
		return nil, &DocumentError{Document: doc, Problem: ProblemNotMarkedEncrypted}
	}
	validation, _ := root["encKeyValidation_DO_NOT_EDIT"].(string)
	elements, hasItems := root["items"].([]any)
	if validation == "" || !hasItems || !absent(root, "data") {
		return nil, &DocumentError{Document: doc, Problem: ProblemNotAccountBound}
	}

	out := &EncryptedExport{Items: make([]EncryptedItem, 0, len(elements))}
	ids := make([]string, 0, len(elements))
	for _, raw := range elements {
		obj, id, err := itemObject(doc, raw)
		if err != nil {
			return nil, err
		}
		item := EncryptedItem{ID: id}
		var ok bool
		if item.OrganizationID, ok = optionalString(obj, "organizationId"); !ok {
			return nil, malformed(doc, id)
		}
		if typ, ok := intValue(obj["type"]); ok {
			item.Type = typ
		}
		item.Format, item.PasskeyCount = classify(obj, item.Type)
		out.Items = append(out.Items, item)
		ids = append(ids, id)
	}
	out.DuplicateIDs = duplicateStrings(ids)
	return out, nil
}

// ParseItemList parses the bare JSON array of "bw list items". The error is
// always a *DocumentError.
func ParseItemList(raw []byte) (*ItemList, error) {
	const doc = ItemListDocument
	root, ok := decode(raw)
	if !ok {
		return nil, &DocumentError{Document: doc, Problem: ProblemInvalidJSON}
	}
	elements, ok := root.([]any)
	if !ok {
		if obj, isObject := root.(map[string]any); isObject && !absent(obj, "success") {
			return nil, &DocumentError{Document: doc, Problem: ProblemResponseEnvelope}
		}
		return nil, &DocumentError{Document: doc, Problem: ProblemWrongForm}
	}

	out := &ItemList{Items: make([]ListedItem, 0, len(elements))}
	ids := make([]string, 0, len(elements))
	for _, raw := range elements {
		obj, id, err := itemObject(doc, raw)
		if err != nil {
			return nil, err
		}
		item := ListedItem{ID: id}
		if item.OrganizationID, ok = optionalString(obj, "organizationId"); !ok {
			return nil, malformed(doc, id)
		}
		attachments, ok := optionalArray(obj, "attachments")
		if !ok {
			return nil, malformed(doc, id)
		}
		for _, a := range attachments {
			m, ok := a.(map[string]any)
			if !ok {
				return nil, malformed(doc, id)
			}
			attachmentID, _ := m["id"].(string)
			if attachmentID == "" {
				return nil, malformed(doc, id)
			}
			item.AttachmentIDs = append(item.AttachmentIDs, attachmentID)
		}
		out.Items = append(out.Items, item)
		ids = append(ids, id)
	}
	out.DuplicateIDs = duplicateStrings(ids)
	// A duplicated item repeats its attachments. That is reported as a
	// duplicate item, so only pairs repeated within one element count here.
	out.DuplicateAttachments = duplicateRefs(dedupePerItem(out.Items))
	return out, nil
}

// classify recognizes the format of an encrypted item (design D4) and, for
// a legacy item, counts its passkeys.
func classify(obj map[string]any, typ int) (Format, int) {
	typeKey, knownType := typeObjectKeys[typ]
	if !knownType {
		return FormatUnknown, 0
	}
	if !absent(obj, "data") {
		data, ok := obj["data"].(string)
		if !ok || !strings.Contains(data, blobMarker) || !absent(obj, "name") {
			return FormatUnknown, 0
		}
		for _, key := range typeObjectKeys {
			if !absent(obj, key) {
				return FormatUnknown, 0
			}
		}
		return FormatBlob, 0
	}

	if _, ok := obj["name"].(string); !ok {
		return FormatUnknown, 0
	}
	content, ok := obj[typeKey].(map[string]any)
	if !ok {
		return FormatUnknown, 0
	}
	if typ != TypeLogin {
		return FormatLegacy, 0
	}
	// The encrypted login leaves fido2Credentials out when there are no
	// passkeys.
	passkeys, ok := optionalArray(content, "fido2Credentials")
	if !ok {
		return FormatUnknown, 0
	}
	for _, p := range passkeys {
		if _, ok := p.(map[string]any); !ok {
			return FormatUnknown, 0
		}
	}
	return FormatLegacy, len(passkeys)
}

// plaintextPasskeys reads the passkeys of a decrypted item. An item without
// a login object has none.
func plaintextPasskeys(obj map[string]any) ([]Passkey, bool) {
	if absent(obj, "login") {
		return nil, true
	}
	login, ok := obj["login"].(map[string]any)
	if !ok {
		return nil, false
	}
	raw, ok := optionalArray(login, "fido2Credentials")
	if !ok {
		return nil, false
	}
	out := make([]Passkey, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, passkey(m))
	}
	return out, true
}

func passkey(m map[string]any) Passkey {
	var p Passkey
	var invalid []string
	text := func(key string) string {
		v, ok := optionalString(m, key)
		if !ok {
			invalid = append(invalid, key)
		}
		return v
	}
	present := func(key string) bool {
		return text(key) != ""
	}
	p.CredentialID = text("credentialId")
	p.KeyType = text("keyType")
	p.KeyAlgorithm = text("keyAlgorithm")
	p.KeyCurve = text("keyCurve")
	p.KeyValue = text("keyValue")
	p.HasRPID = present("rpId")
	p.HasCreationDate = present("creationDate")

	switch v := m["counter"].(type) {
	case nil:
	case string:
		p.Counter = v
	case json.Number:
		p.Counter = v.String()
	default:
		invalid = append(invalid, "counter")
	}

	// The export writes discoverable as a string. The view holds a
	// boolean, so a boolean is accepted as well.
	switch v := m["discoverable"].(type) {
	case nil:
	case string:
		p.HasDiscoverable = v != ""
	case bool:
		p.HasDiscoverable = true
	default:
		invalid = append(invalid, "discoverable")
	}

	sort.Strings(invalid)
	p.InvalidFields = invalid
	return p
}

// decode parses one JSON value and rejects trailing data. Numbers stay
// json.Number.
func decode(raw []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

func decodeObject(doc Document, raw []byte) (map[string]any, error) {
	v, ok := decode(raw)
	if !ok {
		return nil, &DocumentError{Document: doc, Problem: ProblemInvalidJSON}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, &DocumentError{Document: doc, Problem: ProblemWrongForm}
	}
	return obj, nil
}

// itemObject checks that an array element is an object with a non-empty
// string ID.
func itemObject(doc Document, raw any) (map[string]any, string, error) {
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, "", malformed(doc, "")
	}
	id, _ := obj["id"].(string)
	if id == "" {
		return nil, "", malformed(doc, "")
	}
	return obj, id, nil
}

func malformed(doc Document, itemID string) *DocumentError {
	return &DocumentError{Document: doc, Problem: ProblemMalformedItem, ItemID: itemID}
}

// absent reports whether a key is missing or null. The parsers treat both
// alike, because the code does not settle which one a real document uses.
func absent(obj map[string]any, key string) bool {
	v, ok := obj[key]
	return !ok || v == nil
}

// optionalString returns the string under key, or "" when the key is
// absent or null. The second result is false for any other JSON type.
func optionalString(obj map[string]any, key string) (string, bool) {
	if absent(obj, key) {
		return "", true
	}
	s, ok := obj[key].(string)
	return s, ok
}

// optionalArray returns the array under key, or nil when the key is absent
// or null. The second result is false for any other JSON type.
func optionalArray(obj map[string]any, key string) ([]any, bool) {
	if absent(obj, key) {
		return nil, true
	}
	a, ok := obj[key].([]any)
	return a, ok
}

func intValue(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	if err != nil || i < 0 || i > 1<<16 {
		return 0, false
	}
	return int(i), true
}

func duplicateStrings(values []string) []string {
	seen := make(map[string]int, len(values))
	var out []string
	for _, v := range values {
		seen[v]++
		if seen[v] == 2 {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func duplicateRefs(refs []AttachmentRef) []AttachmentRef {
	seen := make(map[AttachmentRef]int, len(refs))
	var out []AttachmentRef
	for _, r := range refs {
		seen[r]++
		if seen[r] == 2 {
			out = append(out, r)
		}
	}
	SortRefs(out)
	return out
}

// dedupePerItem returns the attachment pairs of every element, so that a
// pair repeated within one element appears twice and a pair repeated only
// because its item is duplicated appears once.
func dedupePerItem(items []ListedItem) []AttachmentRef {
	seen := map[string]bool{}
	var out []AttachmentRef
	for _, item := range items {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		for _, a := range item.AttachmentIDs {
			out = append(out, AttachmentRef{ItemID: item.ID, AttachmentID: a})
		}
	}
	return out
}

// SortRefs sorts ID pairs by item ID, then attachment ID.
func SortRefs(refs []AttachmentRef) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].ItemID != refs[j].ItemID {
			return refs[i].ItemID < refs[j].ItemID
		}
		return refs[i].AttachmentID < refs[j].AttachmentID
	})
}
