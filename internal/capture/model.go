package capture

// Document names one input of a capture.
type Document string

// The inputs of a capture.
const (
	PlaintextExportDocument Document = "plaintext_export"
	EncryptedExportDocument Document = "encrypted_export"
	ItemListDocument        Document = "item_list"
	// ArchivedItemListDocument is the output of "bw list items --archived".
	ArchivedItemListDocument Document = "archived_item_list"
	DownloadsDocument        Document = "downloads"
)

// Format is the form in which the encrypted export shows an item (design
// D4).
type Format string

// The recognized item formats.
const (
	// FormatLegacy items show their content as separate encrypted fields,
	// so the number of passkeys is visible.
	FormatLegacy Format = "legacy"
	// FormatBlob items seal their content in one data string, so the number
	// of passkeys is not visible.
	FormatBlob Format = "blob"
	// FormatUnknown items match neither form.
	FormatUnknown Format = "unknown"
)

// TypeLogin is the CLI's number for a login item, the only type that can
// hold passkeys.
const TypeLogin = 1

// Input holds the raw documents of one capture and the attachment download
// results the caller recorded.
type Input struct {
	PlaintextExport []byte
	EncryptedExport []byte
	ItemList        []byte
	// ArchivedItemList is the output of "bw list items --archived". An
	// absent list is a zero-length input and is rejected like any other
	// document that does not parse (design D1 of archived-item-list).
	ArchivedItemList []byte
	Downloads        []Download
}

// Capture is the parsed form of one capture. A document that was rejected
// is nil, and Errors says why. Items keep the order of their document,
// including duplicates.
type Capture struct {
	Plaintext *PlaintextExport
	Encrypted *EncryptedExport
	List      *ItemList
	// ArchivedList is the parsed archived item list.
	ArchivedList *ItemList
	Downloads    []Download
	// DuplicateDownloads lists every ID pair that has more than one
	// download result, sorted.
	DuplicateDownloads []AttachmentRef
	Errors             []*DocumentError
}

// EncryptedExport is what the account-bound encrypted export shows. It
// holds no encrypted value.
type EncryptedExport struct {
	Items []EncryptedItem
	// DuplicateIDs lists every item ID that occurs more than once, sorted.
	DuplicateIDs []string
}

// EncryptedItem is one item of the encrypted export.
type EncryptedItem struct {
	ID             string
	Type           int
	OrganizationID string
	Format         Format
	// PasskeyCount is the number of passkeys. It is meaningful only for
	// FormatLegacy.
	PasskeyCount int
}

// PlaintextExport is what the plaintext export shows, reduced to what the
// passkey checks need.
type PlaintextExport struct {
	Items        []PlaintextItem
	DuplicateIDs []string
}

// PlaintextItem is one item of the plaintext export.
type PlaintextItem struct {
	ID             string
	OrganizationID string
	Passkeys       []Passkey
}

// Passkey holds the decrypted fields of one passkey that the structure and
// reference checks read. For the other required fields it records only
// whether they hold a value. The optional fields are not kept.
type Passkey struct {
	CredentialID string
	KeyType      string
	KeyAlgorithm string
	KeyCurve     string
	KeyValue     string
	// Counter is the counter as text: the string the export wrote, or the
	// literal of a JSON number.
	Counter         string
	HasRPID         bool
	HasDiscoverable bool
	HasCreationDate bool
	// InvalidFields names, sorted, the keys whose value has a JSON type the
	// export never writes for them. Their value above is empty.
	InvalidFields []string
}

// ItemList is what the item list shows, reduced to IDs.
type ItemList struct {
	Items        []ListedItem
	DuplicateIDs []string
	// DuplicateAttachments lists every ID pair that occurs more than once
	// on one item, sorted.
	DuplicateAttachments []AttachmentRef
}

// ListedItem is one element of the item list.
type ListedItem struct {
	ID string
	// OrganizationID is empty for a personal item.
	OrganizationID string
	AttachmentIDs  []string
}

// AttachmentRef identifies one attachment by its item ID and attachment ID.
// File names never identify an attachment.
type AttachmentRef struct {
	ItemID       string
	AttachmentID string
}

// Download is the result of downloading one attachment. Exactly one of
// Error and SHA256 is set.
type Download struct {
	ItemID       string
	AttachmentID string
	// Error classifies a failed download. The caller chooses the class and
	// never copies CLI error text into it, because that text can contain
	// names.
	Error ErrorClass
	// SHA256 is the hex SHA-256 of the downloaded content.
	SHA256 string
	Size   int64
}

// Ref returns the ID pair of the download.
func (d Download) Ref() AttachmentRef {
	return AttachmentRef{ItemID: d.ItemID, AttachmentID: d.AttachmentID}
}

// ErrorClass classifies a failed download.
type ErrorClass string

// Error classes. The verification treats every non-empty class as a failed
// download and does not copy the class into its result.
const (
	ErrorProcess ErrorClass = "process"
	ErrorStatus  ErrorClass = "status"
	ErrorOther   ErrorClass = "other"
)

// Reference is an inventory known to be true independently of the CLI, for
// example the one a test fixture was built from (design D9). It holds
// fingerprints and never key material.
type Reference struct {
	Items []ReferenceItem
}

// ReferenceItem lists what one item is known to hold.
type ReferenceItem struct {
	ItemID      string
	Passkeys    []ReferencePasskey
	Attachments []ReferenceAttachment
}

// ReferencePasskey identifies one passkey without its key.
type ReferencePasskey struct {
	CredentialID string
	// KeySHA256 is the hex SHA-256 of the decoded key value.
	KeySHA256 string
	Counter   uint64
}

// ReferenceAttachment identifies the content of one attachment.
type ReferenceAttachment struct {
	AttachmentID string
	// SHA256 is the hex SHA-256 of the content.
	SHA256 string
	Size   int64
}
