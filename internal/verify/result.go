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
	// DocumentDecryptionErrors: the CLI wrote decryption error lines with
	// the document. The lines name no item, so the check that every item
	// decrypted did not pass. Unverified, with the document and the number
	// of lines (design D9 of capture-collection).
	DocumentDecryptionErrors Reason = "document_decryption_errors"
)

// Reason codes for the chosen organizations (design D5 of the change
// organization-export).
const (
	// OrganizationItemMismatch: an item of an organization export does not
	// carry the ID of that organization, or the item lists show it with
	// another organization ID or as a personal item. Failed.
	OrganizationItemMismatch Reason = "organization_item_mismatch"
	// OrganizationChangedDuringCapture: an item of a chosen organization has
	// different revision dates in its two exports, or in its plaintext
	// export and the item lists. The exports are separate reads from the
	// server, so the organization changed during the run. Failed; running
	// the backup again is the remedy.
	OrganizationChangedDuringCapture Reason = "organization_changed_during_capture"
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
	// that the item list shows on an item of an organization that is not
	// chosen. A warning.
	AttachmentDownloadOutOfScope Reason = "attachment_download_out_of_scope"
	// AttachmentDecryptionErrors: the CLI wrote decryption error lines
	// with a successful download. A warning with the number of lines,
	// because size and SHA-256 cover the content (design D9 of
	// capture-collection).
	AttachmentDecryptionErrors Reason = "attachment_decryption_errors"
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
	// LimitOrganizationExportsSeparateReads: each organization export is a
	// read from the server of its own, separate from the local snapshot and
	// from the other export. A change between the reads shows up only for
	// items whose revision date changed. Part of every result with a
	// chosen organization (design D5 of organization-export).
	LimitOrganizationExportsSeparateReads Limit = "organization_exports_separate_server_reads"
)

// ScopeCode names the scope of a verdict without a chosen organization.
const ScopeCode = "personal_items_in_captured_snapshot"

// ScopeStatement says in words what a verdict without a chosen organization
// covers.
const ScopeStatement = "The verdict covers the personal items in the local snapshot the CLI " +
	"exported during this capture. It does not cover items on the server that the snapshot " +
	"does not hold, organization items, deleted items, or items the CLI leaves out of every " +
	"document."

// ScopeCodeWithOrganizations names the scope of a verdict with at least one
// chosen organization.
const ScopeCodeWithOrganizations = "personal_and_chosen_organization_items"

// ScopeStatementWithOrganizations says in words what a verdict with chosen
// organizations covers. The organizations are listed by ID in the scope.
const ScopeStatementWithOrganizations = "The verdict covers the personal items in the local " +
	"snapshot the CLI exported during this capture and, for each chosen organization, the items " +
	"of its organization exports, which the CLI read from the server. It does not cover items on " +
	"the server that the snapshot does not hold, items of organizations that were not chosen, " +
	"deleted items, or items the CLI leaves out of every document."

// Scope states what a verdict refers to.
type Scope struct {
	Code      string `json:"code"`
	Statement string `json:"statement"`
	// Organizations lists the chosen organizations by ID, in the order they
	// were chosen. It is absent without one.
	Organizations []string `json:"organizations,omitempty"`
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
	// OrganizationID names the chosen organization a finding about its
	// exports or its items refers to.
	OrganizationID string `json:"organization_id,omitempty"`
	ItemID         string `json:"item_id,omitempty"`
	// AttachmentID is set together with ItemID.
	AttachmentID string `json:"attachment_id,omitempty"`
	// Passkey is the zero-based position of the passkey within its item in
	// the plaintext export.
	Passkey *int `json:"passkey,omitempty"`
	// Field names a passkey field by its JSON key.
	Field    string `json:"field,omitempty"`
	Expected *int   `json:"expected,omitempty"`
	Found    *int   `json:"found,omitempty"`
	// Count is the number of items a warning summarizes, or the number of
	// decryption error lines a finding reports.
	Count *int `json:"count,omitempty"`
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
		ItemOrganizationMismatch,
		ListedItemDiffersBetweenLists, ArchiveFilterNotApplied,
		DocumentDecryptionErrors,
		OrganizationItemMismatch, OrganizationChangedDuringCapture,
		PasskeyCountMismatch, PasskeyCountUnverifiable, PasskeyFieldMissing,
		PasskeyFieldInvalid, PasskeyKeyInvalid, PasskeyKeyFormUnknown,
		PasskeyCounterInvalid,
		DuplicateAttachmentID, DuplicateDownloadResult, AttachmentDownloadFailed,
		AttachmentDownloadMissing, AttachmentDownloadInvalid,
		AttachmentDownloadNotExpected, AttachmentEmpty, AttachmentDownloadOutOfScope,
		AttachmentDecryptionErrors,
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
// sorted by organization ID, item ID, attachment ID and reason code, and
// the standing limits by code. Two results with equal content give byte-identical output.
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
	if a.OrganizationID != b.OrganizationID {
		return a.OrganizationID < b.OrganizationID
	}
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
