package archive

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"filippo.io/age"

	"bw-takeout/internal/capture"
	"bw-takeout/internal/manifest"
	"bw-takeout/internal/vaultid"
)

// Documents are the four captured documents and the documents of the
// chosen organizations, byte for byte as captured. Each is required. An
// empty archived item list is the two bytes "[]".
type Documents struct {
	PlaintextExport  []byte
	EncryptedExport  []byte
	ItemList         []byte
	ArchivedItemList []byte
	// Organizations holds the two exports of each chosen organization, in
	// the order they were chosen (design D7 of organization-export).
	Organizations []OrganizationDocuments
}

// OrganizationDocuments are the two exports of one chosen organization.
type OrganizationDocuments struct {
	ID              string
	PlaintextExport []byte
	EncryptedExport []byte
}

// ErrDocumentAbsent is wrapped by the refusal of a captured document that
// has no bytes. The error names the entry the document would have, never
// content.
var ErrDocumentAbsent = errors.New("captured document is absent")

// Attachment is one downloaded attachment.
type Attachment struct {
	ItemID       string
	AttachmentID string
	// FileName is the original file name. It is stored in the manifest
	// only.
	FileName string
	Content  []byte
}

// Meta is what the manifest states besides the entries. Its values are
// known only after all attachments were written.
type Meta struct {
	Source manifest.Source
	Tool   manifest.Tool
	BW     manifest.BW
	// Verification is the serialized verification result. It is embedded
	// byte for byte.
	Verification json.RawMessage
	// OrganizationNames maps each chosen organization ID to the name the
	// organization list gave. A missing name is stored as empty. The names
	// go into the encrypted manifest only (design D8 of
	// organization-export).
	OrganizationNames map[string]string
}

// ErrWriterFailed is returned by every call after a call of the same writer
// failed. Such an archive is never finished.
var ErrWriterFailed = errors.New("archive writer failed earlier")

// Writer streams one archive into an io.Writer. The caller writes the
// attachments one at a time and finishes with the manifest. An archive whose
// writer was not finished lacks its manifest and its final encryption
// chunk, so the reader refuses it.
type Writer struct {
	enc     io.WriteCloser
	tw      *tar.Writer
	created time.Time
	items   int
	// orgs holds the chosen organizations in order, with the item count
	// of each plaintext organization export.
	orgs    []manifest.Organization
	entries []manifest.Entry
	seen    map[string]bool
	err     error
	done    bool
}

// NewWriter checks the recipients and the documents, starts the
// encryption, and writes the recovery instructions, the four documents and
// the documents of each chosen organization in the order given. Without a
// valid recipient, with a zero-length document, or with an organization ID
// that is not a UUID or is given twice, it fails before writing anything to
// dst.
func NewWriter(dst io.Writer, recipients []string, created time.Time, docs Documents) (*Writer, error) {
	rs, err := ParseRecipients(recipients)
	if err != nil {
		return nil, err
	}
	documents := []struct {
		path    string
		content []byte
	}{
		{PlaintextExportPath, docs.PlaintextExport},
		{EncryptedExportPath, docs.EncryptedExport},
		{ItemListPath, docs.ItemList},
		{ArchivedItemListPath, docs.ArchivedItemList},
	}
	for _, d := range documents {
		if len(d.content) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrDocumentAbsent, d.path)
		}
	}
	orgs := make([]manifest.Organization, 0, len(docs.Organizations))
	seen := map[string]bool{}
	for i, o := range docs.Organizations {
		if !vaultid.ValidOrganizationID(o.ID) {
			return nil, fmt.Errorf("organization %d refused: its ID is not a UUID", i+1)
		}
		if seen[o.ID] {
			return nil, fmt.Errorf("organization %s refused: it is given twice", o.ID)
		}
		seen[o.ID] = true
		for _, d := range []struct {
			path    string
			content []byte
		}{
			{OrganizationPlaintextExportPath(o.ID), o.PlaintextExport},
			{OrganizationEncryptedExportPath(o.ID), o.EncryptedExport},
		} {
			if len(d.content) == 0 {
				return nil, fmt.Errorf("%w: %s", ErrDocumentAbsent, d.path)
			}
			documents = append(documents, d)
		}
		orgPlain, err := capture.ParseOrganizationPlaintextExport(o.ID, o.PlaintextExport)
		if err != nil {
			return nil, fmt.Errorf("plaintext export of organization %s cannot be read, so its items cannot be counted", o.ID)
		}
		orgs = append(orgs, manifest.Organization{ID: o.ID, Items: len(orgPlain.Items)})
	}
	plain, err := capture.ParsePlaintextExport(docs.PlaintextExport)
	if err != nil {
		return nil, errors.New("plaintext export cannot be read, so its items cannot be counted")
	}
	enc, err := age.Encrypt(dst, rs...)
	if err != nil {
		return nil, fmt.Errorf("start encryption: %w", err)
	}
	w := &Writer{
		enc:     enc,
		tw:      tar.NewWriter(enc),
		created: created.UTC().Truncate(time.Second),
		items:   len(plain.Items),
		orgs:    orgs,
		seen:    map[string]bool{},
	}
	if err := w.entry(manifest.Entry{Path: RecoveryPath, Kind: manifest.KindRecovery}, []byte(RecoveryText)); err != nil {
		return nil, err
	}
	for _, d := range documents {
		if err := w.entry(manifest.Entry{Path: d.path, Kind: manifest.KindDocument}, d.content); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// AddAttachment writes one attachment under its item ID and attachment ID.
// IDs outside the forms of design D3 and a second attachment with the same
// IDs are refused, and the writer then fails for good.
func (w *Writer) AddAttachment(a Attachment) error {
	if err := w.usable(); err != nil {
		return err
	}
	if !ValidItemID(a.ItemID) {
		return w.fail(errors.New("attachment refused: item ID is not a UUID"))
	}
	if !ValidAttachmentID(a.AttachmentID) {
		return w.fail(fmt.Errorf("attachment of item %s refused: attachment ID is not 1 to 64 ASCII letters and digits", a.ItemID))
	}
	return w.entry(manifest.Entry{
		Path:         AttachmentPath(a.ItemID, a.AttachmentID),
		Kind:         manifest.KindAttachment,
		ItemID:       a.ItemID,
		AttachmentID: a.AttachmentID,
		FileName:     a.FileName,
	}, a.Content)
}

// Finish writes the manifest as the last entry and closes the tar stream and
// the encryption. Only a finished archive is complete.
//
// It returns the verification result exactly as the manifest embeds it, and
// only when the archive is finished. Publishing takes the verdict from these
// bytes (design D9), so they are what a publish.WriteFunc returns.
func (w *Writer) Finish(meta Meta) ([]byte, error) {
	if err := w.finish(meta); err != nil {
		return nil, err
	}
	return bytes.Clone(meta.Verification), nil
}

func (w *Writer) finish(meta Meta) error {
	if err := w.usable(); err != nil {
		return err
	}
	attachments := 0
	for _, e := range w.entries {
		if e.Kind == manifest.KindAttachment {
			attachments++
		}
	}
	orgs := make([]manifest.Organization, len(w.orgs))
	for i, o := range w.orgs {
		o.Name = meta.OrganizationNames[o.ID]
		orgs[i] = o
	}
	data, err := manifest.Marshal(&manifest.Manifest{
		Created:       w.created,
		Source:        meta.Source,
		Tool:          meta.Tool,
		BW:            meta.BW,
		Counts:        manifest.Counts{Items: w.items, Attachments: attachments},
		Organizations: orgs,
		Verification:  meta.Verification,
		Entries:       w.entries,
	})
	if err != nil {
		return w.fail(err)
	}
	if err := w.header(ManifestPath, int64(len(data))); err != nil {
		return w.fail(err)
	}
	if _, err := w.tw.Write(data); err != nil {
		return w.fail(fmt.Errorf("write manifest: %w", err))
	}
	if err := w.tw.Close(); err != nil {
		return w.fail(fmt.Errorf("close tar stream: %w", err))
	}
	if err := w.enc.Close(); err != nil {
		return w.fail(fmt.Errorf("close encryption: %w", err))
	}
	w.done = true
	return nil
}

func (w *Writer) usable() error {
	if w.err != nil {
		return ErrWriterFailed
	}
	if w.done {
		return errors.New("archive writer already finished")
	}
	return nil
}

func (w *Writer) fail(err error) error {
	w.err = err
	return err
}

func (w *Writer) entry(e manifest.Entry, content []byte) error {
	if w.seen[e.Path] {
		return w.fail(fmt.Errorf("entry %s written twice", e.Path))
	}
	w.seen[e.Path] = true
	if err := w.header(e.Path, int64(len(content))); err != nil {
		return w.fail(err)
	}
	if _, err := w.tw.Write(content); err != nil {
		return w.fail(fmt.Errorf("write entry %s: %w", e.Path, err))
	}
	sum := sha256.Sum256(content)
	e.Size = int64(len(content))
	e.SHA256 = hex.EncodeToString(sum[:])
	w.entries = append(w.entries, e)
	return nil
}

// header writes a tar header with the attributes of design D2: a regular
// file, mode 0600, owner and group 0 without names, and the creation time.
func (w *Writer) header(name string, size int64) error {
	err := w.tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Size:     size,
		Mode:     0o600,
		ModTime:  w.created,
	})
	if err != nil {
		return fmt.Errorf("write header of %s: %w", name, err)
	}
	return nil
}
