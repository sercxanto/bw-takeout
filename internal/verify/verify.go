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
	"strings"
	"time"

	"bw-takeout/internal/capture"
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
//
// The chosen organizations are those of c.Organizations. Their checks run
// after the personal ones, inside the one verdict (design D5 of the change
// organization-export).
func Verify(c *capture.Capture, ref *capture.Reference) *Result {
	v := &verifier{c: c, inScope: map[string]bool{"": true}}
	for _, o := range c.Organizations {
		v.chosen = append(v.chosen, o.ID)
		v.inScope[o.ID] = true
	}
	// The lines are reported whatever else the capture shows, also when a
	// document was rejected.
	v.decryptionErrors()
	personalRejected := false
	for _, e := range c.Errors {
		reason, ok := documentReasons[e.Problem]
		if !ok {
			reason = DocumentWrongForm
		}
		v.add(Finding{Severity: SeverityFailed, Reason: reason, Document: string(e.Document),
			OrganizationID: e.OrganizationID, ItemID: e.ItemID})
		if e.OrganizationID == "" {
			personalRejected = true
		}
	}
	if personalRejected {
		// Without all four documents no comparison is meaningful.
		return v.result(ref != nil)
	}

	v.index()
	v.duplicates()
	v.identity(v.own)
	v.formats(v.own)
	v.passkeys(v.own)
	v.coverage()
	v.attachments()
	v.archiveFilter()
	for _, o := range c.Organizations {
		// A rejected organization document is reported above, and its
		// organization is skipped from the comparisons (step 1).
		if o.Plaintext == nil || o.Encrypted == nil {
			continue
		}
		p := newExportPair(o.ID, capture.OrganizationPlaintextExportDocument, capture.OrganizationEncryptedExportDocument,
			o.Plaintext, o.Encrypted)
		v.orgs = append(v.orgs, p)
		v.exportDuplicates(p)
		v.identity(p)
		v.formats(p)
		v.passkeys(p)
		v.membership(p)
		v.revisions(p)
		v.matchLists(p)
	}
	if ref != nil {
		v.reference(ref)
	}
	return v.result(ref != nil)
}

// exportPair is the plaintext and the encrypted export of one scope: the
// personal vault, with an empty organization ID, or one chosen
// organization. The checks that both scopes share take a pair, so that they
// apply the same rules (design D5 of organization-export, step 2).
type exportPair struct {
	org              string
	plainDoc, encDoc capture.Document
	plain            *capture.PlaintextExport
	enc              *capture.EncryptedExport
	plaintext        map[string]capture.PlaintextItem
	encrypted        map[string]capture.EncryptedItem
	plaintextOrder   []string
	encryptedOrder   []string
}

// newExportPair indexes the first occurrence of each ID. Duplicates are
// reported separately.
func newExportPair(org string, plainDoc, encDoc capture.Document, plain *capture.PlaintextExport, enc *capture.EncryptedExport) *exportPair {
	p := &exportPair{org: org, plainDoc: plainDoc, encDoc: encDoc, plain: plain, enc: enc,
		plaintext: map[string]capture.PlaintextItem{}, encrypted: map[string]capture.EncryptedItem{}}
	for _, item := range enc.Items {
		if _, ok := p.encrypted[item.ID]; !ok {
			p.encrypted[item.ID] = item
			p.encryptedOrder = append(p.encryptedOrder, item.ID)
		}
	}
	for _, item := range plain.Items {
		if _, ok := p.plaintext[item.ID]; !ok {
			p.plaintext[item.ID] = item
			p.plaintextOrder = append(p.plaintextOrder, item.ID)
		}
	}
	return p
}

type verifier struct {
	c        *capture.Capture
	findings []Finding
	warnings []Finding

	// chosen lists the chosen organizations in order. inScope holds them
	// and the empty ID of personal items.
	chosen  []string
	inScope map[string]bool

	// own is the pair of personal exports, orgs the pairs of the chosen
	// organizations whose documents were accepted.
	own  *exportPair
	orgs []*exportPair
	// listed combines the default and the archived item list (design D2 of
	// archived-item-list). An ID in both lists enters once, from the
	// default list. listedDoc names the list each entry came from.
	listed    map[string]capture.ListedItem
	listedDoc map[string]capture.Document
	// shared lists, in order, the item IDs that both lists show.
	shared []string
	// scoped holds, per item ID, the attachment IDs of every entry in scope
	// of the item in both lists, in the order of capture.ScopedAttachments.
	// Every per-item view of attachments reads it (design D6 of
	// capture-collection and of organization-export).
	scoped    map[string][]string
	downloads map[capture.AttachmentRef]capture.Download

	listedOrder   []string
	downloadOrder []capture.AttachmentRef
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
	scope := Scope{Code: ScopeCode, Statement: ScopeStatement}
	if len(v.chosen) > 0 {
		scope = Scope{Code: ScopeCodeWithOrganizations, Statement: ScopeStatementWithOrganizations,
			Organizations: append([]string(nil), v.chosen...)}
		limits = append(limits, LimitOrganizationExportsSeparateReads)
	}
	r := &Result{
		Verdict:        verdictOf(v.findings),
		Scope:          scope,
		StandingLimits: limits,
		Findings:       sortedFindings(v.findings),
		Warnings:       sortedFindings(v.warnings),
	}
	sort.Slice(r.StandingLimits, func(i, j int) bool { return r.StandingLimits[i] < r.StandingLimits[j] })
	return r
}

func (v *verifier) index() {
	v.own = newExportPair("", capture.PlaintextExportDocument, capture.EncryptedExportDocument, v.c.Plaintext, v.c.Encrypted)
	v.listed = map[string]capture.ListedItem{}
	v.listedDoc = map[string]capture.Document{}
	for _, item := range v.c.List.Items {
		if _, ok := v.listed[item.ID]; !ok {
			v.listed[item.ID] = item
			v.listedDoc[item.ID] = capture.ItemListDocument
			v.listedOrder = append(v.listedOrder, item.ID)
		}
	}
	compared := map[string]bool{}
	for _, item := range v.c.ArchivedList.Items {
		first, ok := v.listed[item.ID]
		switch {
		case !ok:
			v.listed[item.ID] = item
			v.listedDoc[item.ID] = capture.ArchivedItemListDocument
			v.listedOrder = append(v.listedOrder, item.ID)
		case v.listedDoc[item.ID] == capture.ItemListDocument && !compared[item.ID]:
			// A repeated ID within the archived list is a duplicate of that
			// list and is compared only once.
			compared[item.ID] = true
			v.shared = append(v.shared, item.ID)
			if !sameListing(first, item) {
				v.add(Finding{Severity: SeverityFailed, Reason: ListedItemDiffersBetweenLists, ItemID: item.ID})
			}
		}
	}
	v.scoped = map[string][]string{}
	for _, ref := range capture.ScopedAttachments(v.chosen, v.c.List, v.c.ArchivedList) {
		v.scoped[ref.ItemID] = append(v.scoped[ref.ItemID], ref.AttachmentID)
	}
	v.downloads = map[capture.AttachmentRef]capture.Download{}
	for _, d := range v.c.Downloads {
		if _, ok := v.downloads[d.Ref()]; !ok {
			v.downloads[d.Ref()] = d
			v.downloadOrder = append(v.downloadOrder, d.Ref())
		}
	}
}

// sameListing reports whether two entries of one item agree in
// organization ID and in the set of attachment IDs (design D3 of
// archived-item-list). The order of attachments carries no meaning.
func sameListing(a, b capture.ListedItem) bool {
	if a.OrganizationID != b.OrganizationID {
		return false
	}
	set := func(ids []string) map[string]bool {
		m := make(map[string]bool, len(ids))
		for _, id := range ids {
			m[id] = true
		}
		return m
	}
	as, bs := set(a.AttachmentIDs), set(b.AttachmentIDs)
	if len(as) != len(bs) {
		return false
	}
	for id := range as {
		if !bs[id] {
			return false
		}
	}
	return true
}

// duplicates reports every ID and ID pair that occurs more than once in one
// personal input.
func (v *verifier) duplicates() {
	v.exportDuplicates(v.own)
	for doc, ids := range map[capture.Document][]string{
		capture.ItemListDocument:         v.c.List.DuplicateIDs,
		capture.ArchivedItemListDocument: v.c.ArchivedList.DuplicateIDs,
	} {
		for _, id := range ids {
			v.add(Finding{Severity: SeverityFailed, Reason: DuplicateItemID, Document: string(doc), ItemID: id})
		}
	}
	for doc, refs := range map[capture.Document][]capture.AttachmentRef{
		capture.ItemListDocument:         v.c.List.DuplicateAttachments,
		capture.ArchivedItemListDocument: v.c.ArchivedList.DuplicateAttachments,
	} {
		for _, r := range refs {
			v.add(Finding{Severity: SeverityFailed, Reason: DuplicateAttachmentID,
				Document: string(doc), ItemID: r.ItemID, AttachmentID: r.AttachmentID})
		}
	}
	for _, r := range v.c.DuplicateDownloads {
		v.add(Finding{Severity: SeverityFailed, Reason: DuplicateDownloadResult,
			Document: string(capture.DownloadsDocument), ItemID: r.ItemID, AttachmentID: r.AttachmentID})
	}
}

// exportDuplicates reports every item ID that occurs more than once in one
// export of the pair.
func (v *verifier) exportDuplicates(p *exportPair) {
	for _, d := range []struct {
		doc capture.Document
		ids []string
	}{{p.plainDoc, p.plain.DuplicateIDs}, {p.encDoc, p.enc.DuplicateIDs}} {
		for _, id := range d.ids {
			v.add(Finding{Severity: SeverityFailed, Reason: DuplicateItemID, Document: string(d.doc),
				OrganizationID: p.org, ItemID: id})
		}
	}
}

// identity compares the item IDs of the two exports in both directions
// (design D5).
func (v *verifier) identity(p *exportPair) {
	for _, id := range p.encryptedOrder {
		if _, ok := p.plaintext[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ItemMissingFromPlaintextExport,
				Document: string(p.plainDoc), OrganizationID: p.org, ItemID: id})
		}
	}
	for _, id := range p.plaintextOrder {
		if _, ok := p.encrypted[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ItemMissingFromEncryptedExport,
				Document: string(p.encDoc), OrganizationID: p.org, ItemID: id})
		}
	}
}

// formats compares passkey counts where the encrypted export shows them and
// reports every item where it does not (design D4).
func (v *verifier) formats(p *exportPair) {
	for _, id := range p.encryptedOrder {
		item := p.encrypted[id]
		switch item.Format {
		case capture.FormatLegacy:
			plain, ok := p.plaintext[id]
			if !ok {
				continue
			}
			if found := len(plain.Passkeys); found != item.PasskeyCount {
				v.add(Finding{Severity: SeverityFailed, Reason: PasskeyCountMismatch, OrganizationID: p.org, ItemID: id,
					Expected: intp(item.PasskeyCount), Found: intp(found)})
			}
		case capture.FormatBlob:
			if item.Type == capture.TypeLogin {
				v.add(Finding{Severity: SeverityUnverified, Reason: PasskeyCountUnverifiable, OrganizationID: p.org, ItemID: id})
			}
		default:
			v.add(Finding{Severity: SeverityUnverified, Reason: ItemFormatUnrecognized,
				Document: string(p.encDoc), OrganizationID: p.org, ItemID: id})
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
func (v *verifier) passkeys(pair *exportPair) {
	for _, id := range pair.plaintextOrder {
		for i, p := range pair.plaintext[id].Passkeys {
			v.passkey(pair.org, id, i, p)
		}
	}
}

func (v *verifier) passkey(org, itemID string, position int, p capture.Passkey) {
	failed := func(reason Reason, field string) {
		v.add(Finding{Severity: SeverityFailed, Reason: reason, OrganizationID: org, ItemID: itemID, Passkey: intp(position), Field: field})
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
		v.add(Finding{Severity: SeverityUnverified, Reason: PasskeyKeyFormUnknown, OrganizationID: org, ItemID: itemID, Passkey: intp(position)})
		return
	}
	if p.KeyValue != "" && !isP256Key(p.KeyValue) {
		failed(PasskeyKeyInvalid, "keyValue")
	}
}

// decodeKeyValue decodes a key value as base64url. The export writes it
// without padding. A padded value is accepted as well, because padding
// changes the encoding and not the key.
func decodeKeyValue(keyValue string) ([]byte, error) {
	if strings.HasSuffix(keyValue, "=") {
		return base64.URLEncoding.DecodeString(keyValue)
	}
	return base64.RawURLEncoding.DecodeString(keyValue)
}

// isP256Key reports whether a key value decodes as base64url and parses as
// a PKCS#8 P-256 private key. The parsed key is dropped at once and never
// used.
func isP256Key(keyValue string) bool {
	der, err := decodeKeyValue(keyValue)
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

// coverage compares the personal items of both item lists with the
// plaintext export (design D8). Items of chosen organizations are matched by
// matchLists, and items of other organizations are outside the scope.
func (v *verifier) coverage() {
	for _, id := range v.listedOrder {
		if v.listed[id].OrganizationID != "" {
			continue
		}
		if _, ok := v.own.plaintext[id]; !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ListedItemMissingFromExport,
				Document: string(capture.PlaintextExportDocument), ItemID: id})
		}
	}
	for _, id := range v.own.plaintextOrder {
		item, ok := v.listed[id]
		switch {
		case !ok:
			// The item is in neither list, so the finding names no single
			// document (design D2 of archived-item-list).
			v.add(Finding{Severity: SeverityUnverified, Reason: ExportedItemNotListed, ItemID: id})
		case item.OrganizationID != "":
			// The personal exports leave organization items out, so the
			// documents disagree about the same snapshot (design D5, D8).
			v.add(Finding{Severity: SeverityFailed, Reason: ItemOrganizationMismatch,
				Document: string(v.listedDoc[id]), ItemID: id})
		}
	}
}

// attachments accounts for every attachment in scope by ID pair (design
// D7): those of personal items and of items of the chosen organizations.
// The expected set comes from capture.ScopedAttachments, the function the
// collection plans its downloads with (design D6 of capture-collection and
// of organization-export).
func (v *verifier) attachments() {
	expected := map[capture.AttachmentRef]bool{}
	for _, ref := range capture.ScopedAttachments(v.chosen, v.c.List, v.c.ArchivedList) {
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
	for _, ref := range v.downloadOrder {
		if expected[ref] {
			continue
		}
		f := attachmentFinding(SeverityFailed, AttachmentDownloadNotExpected, ref)
		if v.outOfScopeAttachment(ref) {
			f.Severity, f.Reason = SeverityWarning, AttachmentDownloadOutOfScope
		}
		f.Document = string(capture.DownloadsDocument)
		v.add(f)
	}
}

// decryptionErrors reports the decryption error lines of every document as
// unverified and those of every successful download as a warning (design
// D9 of capture-collection). A count of zero or less reports nothing. A
// repeated download result counts once, from its first occurrence, like
// every other check of the downloads.
func (v *verifier) decryptionErrors() {
	counts := v.c.DecryptionErrors
	for _, doc := range []struct {
		document capture.Document
		count    int
	}{
		{capture.PlaintextExportDocument, counts.PlaintextExport},
		{capture.EncryptedExportDocument, counts.EncryptedExport},
		{capture.ItemListDocument, counts.ItemList},
		{capture.ArchivedItemListDocument, counts.ArchivedItemList},
	} {
		if doc.count > 0 {
			v.add(Finding{Severity: SeverityUnverified, Reason: DocumentDecryptionErrors,
				Document: string(doc.document), Count: intp(doc.count)})
		}
	}
	for _, o := range v.c.Organizations {
		for _, doc := range []struct {
			document capture.Document
			count    int
		}{
			{capture.OrganizationPlaintextExportDocument, o.DecryptionErrors.PlaintextExport},
			{capture.OrganizationEncryptedExportDocument, o.DecryptionErrors.EncryptedExport},
		} {
			if doc.count > 0 {
				v.add(Finding{Severity: SeverityUnverified, Reason: DocumentDecryptionErrors,
					Document: string(doc.document), OrganizationID: o.ID, Count: intp(doc.count)})
			}
		}
	}
	seen := map[capture.AttachmentRef]bool{}
	for _, d := range v.c.Downloads {
		if seen[d.Ref()] {
			continue
		}
		seen[d.Ref()] = true
		if d.DecryptionErrors > 0 && validDownload(d) {
			f := attachmentFinding(SeverityWarning, AttachmentDecryptionErrors, d.Ref())
			f.Count = intp(d.DecryptionErrors)
			v.add(f)
		}
	}
}

// outOfScopeAttachment reports whether an entry of either item list shows
// the attachment on an item of an organization that is not chosen. Every
// entry counts, as in capture.ScopedAttachments.
func (v *verifier) outOfScopeAttachment(ref capture.AttachmentRef) bool {
	for _, list := range []*capture.ItemList{v.c.List, v.c.ArchivedList} {
		for _, item := range list.Items {
			if item.ID != ref.ItemID || v.inScope[item.OrganizationID] {
				continue
			}
			for _, id := range item.AttachmentIDs {
				if id == ref.AttachmentID {
					return true
				}
			}
		}
	}
	return false
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

// membership reports every item of an organization export that does not
// carry the ID of its organization, and every such item that the item lists
// show with another organization ID or as a personal item (design D5 of
// organization-export, step 3).
func (v *verifier) membership(p *exportPair) {
	for _, d := range []struct {
		doc   capture.Document
		order []string
		org   func(id string) string
	}{
		{p.plainDoc, p.plaintextOrder, func(id string) string { return p.plaintext[id].OrganizationID }},
		{p.encDoc, p.encryptedOrder, func(id string) string { return p.encrypted[id].OrganizationID }},
	} {
		for _, id := range d.order {
			if d.org(id) != p.org {
				v.add(Finding{Severity: SeverityFailed, Reason: OrganizationItemMismatch,
					Document: string(d.doc), OrganizationID: p.org, ItemID: id})
			}
		}
	}
	for _, id := range p.ids() {
		if item, ok := v.listed[id]; ok && item.OrganizationID != p.org {
			v.add(Finding{Severity: SeverityFailed, Reason: OrganizationItemMismatch,
				Document: string(v.listedDoc[id]), OrganizationID: p.org, ItemID: id})
		}
	}
}

// ids returns the item IDs of both exports of the pair, each once, those of
// the plaintext export first.
func (p *exportPair) ids() []string {
	out := append([]string(nil), p.plaintextOrder...)
	for _, id := range p.encryptedOrder {
		if _, ok := p.plaintext[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// revisions requires a revision date on every item of both organization
// exports and compares the two dates of every item ID that both hold
// (design D5 of organization-export, step 4). The two exports are separate
// reads from the server, so a difference means that the organization
// changed between them.
func (v *verifier) revisions(p *exportPair) {
	plain := map[string]time.Time{}
	for _, id := range p.plaintextOrder {
		if t, ok := revision(p.plaintext[id].RevisionDate); ok {
			plain[id] = t
		} else {
			v.add(Finding{Severity: SeverityFailed, Reason: DocumentMalformedItem,
				Document: string(p.plainDoc), OrganizationID: p.org, ItemID: id})
		}
	}
	for _, id := range p.encryptedOrder {
		sealed, ok := revision(p.encrypted[id].RevisionDate)
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: DocumentMalformedItem,
				Document: string(p.encDoc), OrganizationID: p.org, ItemID: id})
			continue
		}
		if t, ok := plain[id]; ok && !t.Equal(sealed) {
			v.add(Finding{Severity: SeverityFailed, Reason: OrganizationChangedDuringCapture,
				OrganizationID: p.org, ItemID: id})
		}
	}
}

// matchLists compares the items both item lists show for the organization,
// each once, with its plaintext export (design D5 of organization-export,
// step 5). The item lists come from the snapshot and the export from a
// later read of the server.
func (v *verifier) matchLists(p *exportPair) {
	for _, id := range v.listedOrder {
		listed := v.listed[id]
		if listed.OrganizationID != p.org {
			continue
		}
		plain, ok := p.plaintext[id]
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ListedItemMissingFromExport,
				Document: string(p.plainDoc), OrganizationID: p.org, ItemID: id})
			continue
		}
		listedAt, ok := revision(listed.RevisionDate)
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: DocumentMalformedItem,
				Document: string(v.listedDoc[id]), OrganizationID: p.org, ItemID: id})
			continue
		}
		// A missing date on the export item is reported by revisions.
		if exportedAt, ok := revision(plain.RevisionDate); ok && !listedAt.Equal(exportedAt) {
			v.add(Finding{Severity: SeverityFailed, Reason: OrganizationChangedDuringCapture,
				OrganizationID: p.org, ItemID: id})
		}
	}
	for _, id := range p.plaintextOrder {
		if _, ok := v.listed[id]; !ok {
			// Its attachments are unknown, because only the item lists name
			// them.
			v.add(Finding{Severity: SeverityUnverified, Reason: ExportedItemNotListed,
				OrganizationID: p.org, ItemID: id})
		}
	}
}

// revision parses a revision date as RFC 3339 at millisecond precision,
// the precision the server stores.
func revision(raw string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC().Truncate(time.Millisecond), true
}

// archiveFilter reports that the CLI did not apply --archived when both
// lists show at least one item ID (design D4 of archived-item-list). The
// warning names no item and leaves the verdict unchanged.
func (v *verifier) archiveFilter() {
	if len(v.shared) == 0 {
		return
	}
	v.add(Finding{Severity: SeverityWarning, Reason: ArchiveFilterNotApplied, Count: intp(len(v.shared))})
}

// reference compares the capture with a reference inventory (design D9).
// Every difference fails, except a passkey counter that grew. The capture
// side is the union of the personal plaintext export and the plaintext
// exports of the chosen organizations; item IDs are unique across a vault
// (design D5 of organization-export).
func (v *verifier) reference(ref *capture.Reference) {
	plaintext := map[string]capture.PlaintextItem{}
	var order []string
	for _, p := range append([]*exportPair{v.own}, v.orgs...) {
		for _, id := range p.plaintextOrder {
			if _, ok := plaintext[id]; !ok {
				plaintext[id] = p.plaintext[id]
				order = append(order, id)
			}
		}
	}
	known := map[string]capture.ReferenceItem{}
	for _, item := range ref.Items {
		known[item.ItemID] = item
		plain, ok := plaintext[item.ItemID]
		if !ok {
			v.add(Finding{Severity: SeverityFailed, Reason: ReferenceItemMissing, ItemID: item.ItemID})
			continue
		}
		v.referencePasskeys(item, plain)
		v.referenceAttachments(item)
	}
	for _, id := range order {
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
		// Counters grow with use, so a higher counter than the reference
		// passes. A lower one points to a reset passkey.
		if counter, err := strconv.ParseUint(got.Counter, 10, 64); err != nil || counter < want.Counter {
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
	der, err := decodeKeyValue(keyValue)
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
	for _, id := range v.scoped[item.ItemID] {
		listed[id] = true
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
	for _, id := range v.scoped[item.ItemID] {
		if !expected[id] {
			v.add(attachmentFinding(SeverityFailed, ReferenceAttachmentNotExpected,
				capture.AttachmentRef{ItemID: item.ItemID, AttachmentID: id}))
		}
	}
}

func intp(n int) *int {
	return &n
}
