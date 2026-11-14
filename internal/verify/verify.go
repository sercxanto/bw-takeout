package verify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"sort"
	"strconv"

	"bitwarden-backup/internal/capture"
)

// documentReasons maps each document problem to its reason code.
var documentReasons = map[capture.Problem]Reason{
	capture.ProblemInvalidJSON:        DocumentInvalidJSON,
	capture.ProblemWrongForm:          DocumentWrongForm,
	capture.ProblemMarkedEncrypted:    DocumentMarkedEncrypted,
	capture.ProblemNotMarkedEncrypted: DocumentNotMarkedEncrypted,
	capture.ProblemPasswordProtected:  DocumentPasswordProtected,
	capture.ProblemNotAccountBound:    DocumentNotAccountBound,
	capture.ProblemResponseEnvelope:   DocumentResponseEnvelope,
	capture.ProblemMalformedItem:      DocumentMalformedItem,
}

// Verify judges one capture. The reference inventory is optional. Verify
// reads nothing but its arguments and changes neither of them.
func Verify(c *capture.Capture, ref *capture.Reference) *Result {
	v := &verifier{c: c}
	if len(c.Errors) > 0 {
		// Without all three documents no comparison is meaningful.
		for _, e := range c.Errors {
			reason, ok := documentReasons[e.Problem]
			if !ok {
				reason = DocumentWrongForm
			}
			v.add(Finding{Severity: SeverityFailed, Reason: reason, Document: string(e.Document), ItemID: e.ItemID})
		}
		return v.result(ref != nil)
	}

	v.index()
	v.duplicates()
	v.identity()
	v.formats()
	v.passkeys()
	v.coverage()
	v.attachments()
	v.organizations()
	if ref != nil {
		v.reference(ref)
	}
	return v.result(ref != nil)
}

type verifier struct {
	c        *capture.Capture
	findings []Finding
	warnings []Finding

	// First occurrence of each ID. Duplicates are reported separately.
	encrypted map[string]capture.EncryptedItem
	plaintext map[string]capture.PlaintextItem
	listed    map[string]capture.ListedItem
	downloads map[capture.AttachmentRef]capture.Download

	encryptedOrder []string
	plaintextOrder []string
	listedOrder    []string
	downloadOrder  []capture.AttachmentRef
}

func (v *verifier) add(f Finding) {
	if f.Severity == SeverityWarning {
		v.warnings = append(v.warnings, f)
		return
	}
	v.findings = append(v.findings, f)
}

func (v *verifier) result(withReference bool) *Result {
	limits := []Limit{LimitServerCompleteness}
	if !withReference {
		limits = append(limits, LimitAttachmentInventory, LimitCredentialIdentity, LimitAttachmentContent)
	}
	r := &Result{
		Verdict:        verdictOf(v.findings),
		Scope:          Scope{Code: ScopeCode, Statement: ScopeStatement},
		StandingLimits: limits,
		Findings:       sortedFindings(v.findings),
		Warnings:       sortedFindings(v.warnings),
	}
	sort.Slice(r.StandingLimits, func(i, j int) bool { return r.StandingLimits[i] < r.StandingLimits[j] })
	return r
}

func (v *verifier) index() {
	v.encrypted = map[string]capture.EncryptedItem{}
	for _, item := range v.c.Encrypted.Items {
		if _, ok := v.encrypted[item.ID]; !ok {
			v.encrypted[item.ID] = item
			v.encryptedOrder = append(v.encryptedOrder, item.ID)
		}
	}
	v.plaintext = map[string]capture.PlaintextItem{}
	for _, item := range v.c.Plaintext.Items {
		if _, ok := v.plaintext[item.ID]; !ok {
			v.plaintext[item.ID] = item
			v.plaintextOrder = append(v.plaintextOrder, item.ID)
		}
	}
	v.listed = map[string]capture.ListedItem{}
	for _, item := range v.c.List.Items {
		if _, ok := v.listed[item.ID]; !ok {
			v.listed[item.ID] = item
			v.listedOrder = append(v.listedOrder, item.ID)
		}
	}
	v.downloads = map[capture.AttachmentRef]capture.Download{}
	for _, d := range v.c.Downloads {
		if _, ok := v.downloads[d.Ref()]; !ok {
			v.downloads[d.Ref()] = d
			v.downloadOrder = append(v.downloadOrder, d.Ref())
		}
	}
}

// duplicates reports every ID and ID pair that occurs more than once in one
// input.
func (v *verifier) duplicates() {
	for doc, ids := range map[capture.Document][]string{
		capture.PlaintextExportDocument: v.c.Plaintext.DuplicateIDs,
		capture.EncryptedExportDocument: v.c.Encrypted.DuplicateIDs,
		capture.ItemListDocument:        v.c.List.DuplicateIDs,
	} {
		for _, id := range ids {
			v.add(Finding{Severity: SeverityFailed, Reason: DuplicateItemID, Document: string(doc), ItemID: id})
		}
	}
	for _, r := range v.c.List.DuplicateAttachments {
		v.add(Finding{Severity: SeverityFailed, Reason: DuplicateAttachmentID,
			Document: string(capture.ItemListDocument), ItemID: r.ItemID, AttachmentID: r.AttachmentID})
	}
	for _, r := range v.c.DuplicateDownloads {
		v.add(Finding{Severity: SeverityFailed, Reason: DuplicateDownloadResult,
			Document: string(capture.DownloadsDocument), ItemID: r.ItemID, AttachmentID: r.AttachmentID})
	}
}

// identity compares the item IDs of the two exports in both directions
// (design D5).
func (v *verifier) identity() {
	for _, id := range v.encryptedOrder {
		if _, ok := v.plaintext[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ItemMissingFromPlaintextExport,
				Document: string(capture.PlaintextExportDocument), ItemID: id})
		}
	}
	for _, id := range v.plaintextOrder {
		if _, ok := v.encrypted[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ItemMissingFromEncryptedExport,
				Document: string(capture.EncryptedExportDocument), ItemID: id})
		}
	}
}

// formats compares passkey counts where the encrypted export shows them and
// reports every item where it does not (design D4).
func (v *verifier) formats() {
	for _, id := range v.encryptedOrder {
		item := v.encrypted[id]
		switch item.Format {
		case capture.FormatLegacy:
			plain, ok := v.plaintext[id]
			if !ok {
				continue
			}
			if found := len(plain.Passkeys); found != item.PasskeyCount {
				v.add(Finding{Severity: SeverityFailed, Reason: PasskeyCountMismatch, ItemID: id,
					Expected: intp(item.PasskeyCount), Found: intp(found)})
			}
		case capture.FormatBlob:
			if item.Type == capture.TypeLogin {
				v.add(Finding{Severity: SeverityUnverified, Reason: PasskeyCountUnverifiable, ItemID: id})
			}
		default:
			v.add(Finding{Severity: SeverityUnverified, Reason: ItemFormatUnrecognized,
				Document: string(capture.EncryptedExportDocument), ItemID: id})
		}
	}
}

// The passkey form the verification knows (design D6).
const (
	knownKeyType      = "public-key"
	knownKeyAlgorithm = "ECDSA"
	knownKeyCurve     = "P-256"
)

// passkeys checks the structure of every passkey in the plaintext export
// (design D6).
func (v *verifier) passkeys() {
	for _, id := range v.plaintextOrder {
		for i, p := range v.plaintext[id].Passkeys {
			v.passkey(id, i, p)
		}
	}
}

func (v *verifier) passkey(itemID string, position int, p capture.Passkey) {
	failed := func(reason Reason, field string) {
		v.add(Finding{Severity: SeverityFailed, Reason: reason, ItemID: itemID, Passkey: intp(position), Field: field})
	}
	invalid := map[string]bool{}
	for _, field := range p.InvalidFields {
		invalid[field] = true
		failed(PasskeyFieldInvalid, field)
	}
	required := []struct {
		field   string
		present bool
	}{
		{"credentialId", p.CredentialID != ""},
		{"keyType", p.KeyType != ""},
		{"keyAlgorithm", p.KeyAlgorithm != ""},
		{"keyCurve", p.KeyCurve != ""},
		{"keyValue", p.KeyValue != ""},
		{"rpId", p.HasRPID},
		{"counter", p.Counter != ""},
		{"discoverable", p.HasDiscoverable},
		{"creationDate", p.HasCreationDate},
	}
	for _, r := range required {
		if !r.present && !invalid[r.field] {
			failed(PasskeyFieldMissing, r.field)
		}
	}

	if p.Counter != "" {
		if _, err := strconv.ParseUint(p.Counter, 10, 64); err != nil {
			failed(PasskeyCounterInvalid, "counter")
		}
	}

	if p.KeyType == "" || p.KeyAlgorithm == "" || p.KeyCurve == "" {
		return
	}
	if p.KeyType != knownKeyType || p.KeyAlgorithm != knownKeyAlgorithm || p.KeyCurve != knownKeyCurve {
		v.add(Finding{Severity: SeverityUnverified, Reason: PasskeyKeyFormUnknown, ItemID: itemID, Passkey: intp(position)})
		return
	}
	if p.KeyValue != "" && !isP256Key(p.KeyValue) {
		failed(PasskeyKeyInvalid, "keyValue")
	}
}

// isP256Key reports whether a key value decodes as base64url without
// padding and parses as a PKCS#8 P-256 private key. The parsed key is
// dropped at once and never used.
func isP256Key(keyValue string) bool {
	der, err := base64.RawURLEncoding.DecodeString(keyValue)
	if err != nil {
		return false
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return false
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	return ok && key.Curve == elliptic.P256()
}

// coverage compares the personal items of the item list with the plaintext
// export (design D8).
func (v *verifier) coverage() {
	for _, id := range v.listedOrder {
		if v.listed[id].OrganizationID != "" {
			continue
		}
		if _, ok := v.plaintext[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ListedItemMissingFromExport,
				Document: string(capture.PlaintextExportDocument), ItemID: id})
		}
	}
	for _, id := range v.plaintextOrder {
		if item, ok := v.listed[id]; !ok || item.OrganizationID != "" {
			v.add(Finding{Severity: SeverityUnverified, Reason: ExportedItemNotListed,
				Document: string(capture.ItemListDocument), ItemID: id})
		}
	}
}

// attachments accounts for every attachment of a personal item by ID pair
// (design D7).
func (v *verifier) attachments() {
	expected := map[capture.AttachmentRef]bool{}
	for _, id := range v.listedOrder {
		item := v.listed[id]
		if item.OrganizationID != "" {
			continue
		}
		for _, attachmentID := range item.AttachmentIDs {
			ref := capture.AttachmentRef{ItemID: id, AttachmentID: attachmentID}
			if expected[ref] {
				continue
			}
			expected[ref] = true
			d, ok := v.downloads[ref]
			if !ok {
				v.add(attachmentFinding(SeverityFailed, AttachmentDownloadMissing, ref))
				continue
			}
			switch {
			case d.Error != "" && d.SHA256 == "":
				v.add(attachmentFinding(SeverityFailed, AttachmentDownloadFailed, ref))
			case !validDownload(d):
				v.add(attachmentFinding(SeverityFailed, AttachmentDownloadInvalid, ref))
			case d.Size == 0:
				v.add(attachmentFinding(SeverityWarning, AttachmentEmpty, ref))
			}
		}
	}
	for _, ref := range v.downloadOrder {
		if !expected[ref] {
			f := attachmentFinding(SeverityFailed, AttachmentDownloadNotExpected, ref)
			f.Document = string(capture.DownloadsDocument)
			v.add(f)
		}
	}
}

func attachmentFinding(s Severity, r Reason, ref capture.AttachmentRef) Finding {
	return Finding{Severity: s, Reason: r, ItemID: ref.ItemID, AttachmentID: ref.AttachmentID}
}

// validDownload reports whether a download result is a well-formed success:
// no error class, a hex SHA-256 and a non-negative size.
func validDownload(d capture.Download) bool {
	if d.Error != "" || d.Size < 0 || len(d.SHA256) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(d.SHA256)
	return err == nil
}

// organizations counts the organization items of the item list per
// organization ID. They are excluded from every other check.
func (v *verifier) organizations() {
	perOrg := map[string]int{}
	total := 0
	for _, id := range v.listedOrder {
		if org := v.listed[id].OrganizationID; org != "" {
			perOrg[org]++
			total++
		}
	}
	if total == 0 {
		return
	}
	counts := make([]OrganizationCount, 0, len(perOrg))
	for org, n := range perOrg {
		counts = append(counts, OrganizationCount{OrganizationID: org, Count: n})
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].OrganizationID < counts[j].OrganizationID })
	v.add(Finding{Severity: SeverityWarning, Reason: OrganizationItemsExcluded,
		Document: string(capture.ItemListDocument), Count: intp(total), Organizations: counts})
}

// reference compares the capture with a reference inventory (design D9).
// Every difference fails.
func (v *verifier) reference(ref *capture.Reference) {
	known := map[string]capture.ReferenceItem{}
	for _, item := range ref.Items {
		known[item.ItemID] = item
		plain, ok := v.plaintext[item.ItemID]
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferenceItemMissing, ItemID: item.ItemID})
			continue
		}
		v.referencePasskeys(item, plain)
		v.referenceAttachments(item)
	}
	for _, id := range v.plaintextOrder {
		if _, ok := known[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferenceItemNotExpected, ItemID: id})
		}
	}
}

// referencePasskeys matches passkeys by credential ID. The credential ID
// itself never enters a finding.
func (v *verifier) referencePasskeys(item capture.ReferenceItem, plain capture.PlaintextItem) {
	exported := map[string]int{}
	for i, p := range plain.Passkeys {
		if _, ok := exported[p.CredentialID]; !ok {
			exported[p.CredentialID] = i
		}
	}
	expected := map[string]bool{}
	for _, want := range item.Passkeys {
		expected[want.CredentialID] = true
		i, ok := exported[want.CredentialID]
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferencePasskeyMissing, ItemID: item.ItemID})
			continue
		}
		got := plain.Passkeys[i]
		if keyFingerprint(got.KeyValue) != want.KeySHA256 {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferencePasskeyKeyMismatch, ItemID: item.ItemID, Passkey: intp(i)})
		}
		if counter, err := strconv.ParseUint(got.Counter, 10, 64); err != nil || counter != want.Counter {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferencePasskeyCounterMismatch, ItemID: item.ItemID, Passkey: intp(i)})
		}
	}
	for i, p := range plain.Passkeys {
		if !expected[p.CredentialID] {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferencePasskeyNotExpected, ItemID: item.ItemID, Passkey: intp(i)})
		}
	}
}

// keyFingerprint returns the hex SHA-256 of the decoded key value, or "" when
// it does not decode.
func keyFingerprint(keyValue string) string {
	der, err := base64.RawURLEncoding.DecodeString(keyValue)
	if err != nil || len(der) == 0 {
		return ""
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// referenceAttachments compares the attachment IDs the item list shows and
// the content of every successful download with the reference. A failed or
// missing download is already reported by the attachment accounting.
func (v *verifier) referenceAttachments(item capture.ReferenceItem) {
	listed := map[string]bool{}
	if l, ok := v.listed[item.ItemID]; ok && l.OrganizationID == "" {
		for _, id := range l.AttachmentIDs {
			listed[id] = true
		}
	}
	expected := map[string]bool{}
	for _, want := range item.Attachments {
		expected[want.AttachmentID] = true
		ref := capture.AttachmentRef{ItemID: item.ItemID, AttachmentID: want.AttachmentID}
		if !listed[want.AttachmentID] {
			v.add(attachmentFinding(SeverityFailed, ReferenceAttachmentMissing, ref))
			continue
		}
		d, ok := v.downloads[ref]
		if !ok || !validDownload(d) {
			continue
		}
		if d.SHA256 != want.SHA256 || d.Size != want.Size {
			v.add(attachmentFinding(SeverityFailed, ReferenceAttachmentMismatch, ref))
		}
	}
	if l, ok := v.listed[item.ItemID]; ok && l.OrganizationID == "" {
		seen := map[string]bool{}
		for _, id := range l.AttachmentIDs {
			if !expected[id] && !seen[id] {
				seen[id] = true
				v.add(attachmentFinding(SeverityFailed, ReferenceAttachmentNotExpected,
					capture.AttachmentRef{ItemID: item.ItemID, AttachmentID: id}))
			}
		}
	}
}

func intp(n int) *int {
	return &n
}
