package verify

import (
	"encoding/json"
	"sort"
)

// Verdict is the outcome of a verification (design D2).
type Verdict string

// The verdicts, from strongest to weakest.
const (
	// Failed means a proven deviation or broken input.
	Failed Verdict = "failed"
	// Unverified means a required check was not possible.
	Unverified Verdict = "unverified"
	// Verified means all checks for the stated scope passed.
	Verified Verdict = "verified"
)

// Severity classifies a finding. A failed or unverified finding lowers the
// verdict. A warning never does.
type Severity string

// The severities.
const (
	SeverityFailed     Severity = "failed"
	SeverityUnverified Severity = "unverified"
	SeverityWarning    Severity = "warning"
)

// Reason is a stable, machine-readable reason code.
type Reason string

// Reason codes for documents.
const (
	DocumentInvalidJSON        Reason = "document_invalid_json"
	DocumentWrongForm          Reason = "document_wrong_form"
	DocumentMarkedEncrypted    Reason = "document_marked_encrypted"
	DocumentNotMarkedEncrypted Reason = "document_not_marked_encrypted"
	DocumentPasswordProtected  Reason = "document_password_protected"
	DocumentNotAccountBound    Reason = "document_not_account_bound"
	DocumentResponseEnvelope   Reason = "document_response_envelope"
	DocumentMalformedItem      Reason = "document_malformed_item"
)

// Reason codes for item identity, format and coverage.
const (
	DuplicateItemID                Reason = "duplicate_item_id"
	ItemMissingFromPlaintextExport Reason = "item_missing_from_plaintext_export"
	ItemMissingFromEncryptedExport Reason = "item_missing_from_encrypted_export"
	ItemFormatUnrecognized         Reason = "item_format_unrecognized"
	ListedItemMissingFromExport    Reason = "listed_item_missing_from_export"
	ExportedItemNotListed          Reason = "exported_item_not_listed"
	OrganizationItemsExcluded      Reason = "organization_items_excluded"
	// ItemOrganizationMismatch: both exports hold the item as a personal
	// item, but the item list shows it as an organization item.
	ItemOrganizationMismatch Reason = "item_organization_mismatch"
	// ListedItemDiffersBetweenLists: the default and the archived item list
	// show the same item ID with a different organization ID or a
	// different set of attachment IDs.
	ListedItemDiffersBetweenLists Reason = "listed_item_differs_between_lists"
	// ArchiveFilterNotApplied: at least one item ID is in both item lists,
	// so the CLI did not apply --archived. A warning with the number of
	// shared item IDs.
	ArchiveFilterNotApplied Reason = "archive_filter_not_applied"
)

// Reason codes for passkeys.
const (
	PasskeyCountMismatch     Reason = "passkey_count_mismatch"
	PasskeyCountUnverifiable Reason = "passkey_count_unverifiable"
	PasskeyFieldMissing      Reason = "passkey_field_missing"
	PasskeyFieldInvalid      Reason = "passkey_field_invalid"
	PasskeyKeyInvalid        Reason = "passkey_key_invalid"
	PasskeyKeyFormUnknown    Reason = "passkey_key_form_unknown"
	PasskeyCounterInvalid    Reason = "passkey_counter_invalid"
)

// Reason codes for attachments.
const (
	DuplicateAttachmentID         Reason = "duplicate_attachment_id"
	DuplicateDownloadResult       Reason = "duplicate_download_result"
	AttachmentDownloadFailed      Reason = "attachment_download_failed"
	AttachmentDownloadMissing     Reason = "attachment_download_missing"
	AttachmentDownloadInvalid     Reason = "attachment_download_invalid"
	AttachmentDownloadNotExpected Reason = "attachment_download_not_expected"
	AttachmentEmpty               Reason = "attachment_empty"
	// AttachmentDownloadOutOfScope: a download result for an attachment
	// that the item list shows on an organization item. A warning.
	AttachmentDownloadOutOfScope Reason = "attachment_download_out_of_scope"
)

// Reason codes for the comparison with a reference inventory.
const (
	ReferenceItemMissing            Reason = "reference_item_missing"
	ReferenceItemNotExpected        Reason = "reference_item_not_expected"
	ReferencePasskeyMissing         Reason = "reference_passkey_missing"
	ReferencePasskeyNotExpected     Reason = "reference_passkey_not_expected"
	ReferencePasskeyKeyMismatch     Reason = "reference_passkey_key_mismatch"
	ReferencePasskeyCounterMismatch Reason = "reference_passkey_counter_mismatch"
	ReferenceAttachmentMissing      Reason = "reference_attachment_missing"
	ReferenceAttachmentNotExpected  Reason = "reference_attachment_not_expected"
	ReferenceAttachmentMismatch     Reason = "reference_attachment_content_mismatch"
)

// Limit names a check that the verification could not perform. It is part
// of every result it applies to, whatever the verdict.
type Limit string

// The standing limits.
const (
	// LimitServerCompleteness: the capture is compared with itself, never
	// with an authoritative server inventory. An omission in a step that
	// all documents share stays invisible.
	LimitServerCompleteness Limit = "server_completeness_not_checked"
	// LimitAttachmentInventory: the two item lists are the only source of
	// attachment metadata. An attachment missing from an otherwise intact
	// list entry stays undetected.
	LimitAttachmentInventory Limit = "attachment_inventory_single_source"
	// LimitCredentialIdentity: without a reference inventory, a passkey
	// swapped for another valid passkey and a well-formed but wrong key
	// pass.
	LimitCredentialIdentity Limit = "credential_identity_not_checked"
	// LimitAttachmentContent: without a reference inventory, the content
	// of a downloaded attachment is not compared with anything.
	LimitAttachmentContent Limit = "attachment_content_not_checked"
)

// ScopeCode names the scope every verdict refers to.
const ScopeCode = "personal_items_in_captured_snapshot"

// ScopeStatement says in words what a verdict covers.
const ScopeStatement = "The verdict covers the personal items in the local snapshot the CLI " +
	"exported during this capture. It does not cover items on the server that the snapshot " +
	"does not hold, organization items, deleted items, or items the CLI leaves out of every " +
	"document."

// Scope states what a verdict refers to.
type Scope struct {
	Code      string `json:"code"`
	Statement string `json:"statement"`
}

// Result is the outcome of one verification. It holds IDs, positions,
// counts and reason codes, never content (design D10).
type Result struct {
	Verdict        Verdict   `json:"verdict"`
	Scope          Scope     `json:"scope"`
	StandingLimits []Limit   `json:"standing_limits"`
	Findings       []Finding `json:"findings"`
	Warnings       []Finding `json:"warnings"`
}

// Finding is one deviation, one check that was not possible, or one
// warning.
type Finding struct {
	Severity Severity `json:"severity"`
	Reason   Reason   `json:"reason"`
	// Document names the input the finding refers to, when it refers to
	// one document.
	Document string `json:"document,omitempty"`
	ItemID   string `json:"item_id,omitempty"`
	// AttachmentID is set together with ItemID.
	AttachmentID string `json:"attachment_id,omitempty"`
	// Passkey is the zero-based position of the passkey within its item in
	// the plaintext export.
	Passkey *int `json:"passkey,omitempty"`
	// Field names a passkey field by its JSON key.
	Field    string `json:"field,omitempty"`
	Expected *int   `json:"expected,omitempty"`
	Found    *int   `json:"found,omitempty"`
	// Count is the number of items a warning summarizes.
	Count *int `json:"count,omitempty"`
	// Organizations breaks Count down per organization ID.
	Organizations []OrganizationCount `json:"organizations,omitempty"`
}

// OrganizationCount is the number of items of one organization.
type OrganizationCount struct {
	OrganizationID string `json:"organization_id"`
	Count          int    `json:"count"`
}

// Reasons returns every reason code.
func Reasons() []Reason {
	return []Reason{
		DocumentInvalidJSON, DocumentWrongForm, DocumentMarkedEncrypted,
		DocumentNotMarkedEncrypted, DocumentPasswordProtected,
		DocumentNotAccountBound, DocumentResponseEnvelope, DocumentMalformedItem,
		DuplicateItemID, ItemMissingFromPlaintextExport,
		ItemMissingFromEncryptedExport, ItemFormatUnrecognized,
		ListedItemMissingFromExport, ExportedItemNotListed,
		OrganizationItemsExcluded, ItemOrganizationMismatch,
		ListedItemDiffersBetweenLists, ArchiveFilterNotApplied,
		PasskeyCountMismatch, PasskeyCountUnverifiable, PasskeyFieldMissing,
		PasskeyFieldInvalid, PasskeyKeyInvalid, PasskeyKeyFormUnknown,
		PasskeyCounterInvalid,
		DuplicateAttachmentID, DuplicateDownloadResult, AttachmentDownloadFailed,
		AttachmentDownloadMissing, AttachmentDownloadInvalid,
		AttachmentDownloadNotExpected, AttachmentEmpty, AttachmentDownloadOutOfScope,
		ReferenceItemMissing, ReferenceItemNotExpected, ReferencePasskeyMissing,
		ReferencePasskeyNotExpected, ReferencePasskeyKeyMismatch,
		ReferencePasskeyCounterMismatch, ReferenceAttachmentMissing,
		ReferenceAttachmentNotExpected, ReferenceAttachmentMismatch,
	}
}

// verdictOf applies the precedence failed over unverified over verified.
// Warnings are not passed in, so they cannot change the verdict.
func verdictOf(findings []Finding) Verdict {
	v := Verified
	for _, f := range findings {
		switch f.Severity {
		case SeverityFailed:
			return Failed
		case SeverityUnverified:
			v = Unverified
		}
	}
	return v
}

// JSON serializes the result deterministically: findings and warnings are
// sorted by item ID, attachment ID and reason code, and the standing limits
// by code. Two results with equal content give byte-identical output.
func (r *Result) JSON() ([]byte, error) {
	c := *r
	c.Findings = sortedFindings(r.Findings)
	c.Warnings = sortedFindings(r.Warnings)
	c.StandingLimits = append([]Limit{}, r.StandingLimits...)
	sort.Slice(c.StandingLimits, func(i, j int) bool { return c.StandingLimits[i] < c.StandingLimits[j] })
	return json.MarshalIndent(c, "", "  ")
}

func sortedFindings(in []Finding) []Finding {
	out := append([]Finding{}, in...)
	sort.SliceStable(out, func(i, j int) bool { return lessFinding(out[i], out[j]) })
	return out
}

func lessFinding(a, b Finding) bool {
	if a.ItemID != b.ItemID {
		return a.ItemID < b.ItemID
	}
	if a.AttachmentID != b.AttachmentID {
		return a.AttachmentID < b.AttachmentID
	}
	if a.Reason != b.Reason {
		return a.Reason < b.Reason
	}
	if a.Document != b.Document {
		return a.Document < b.Document
	}
	if pa, pb := position(a.Passkey), position(b.Passkey); pa != pb {
		return pa < pb
	}
	return a.Field < b.Field
}

func position(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
