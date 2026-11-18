package collect

import (
	"context"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/manifest"
)

// Snapshot is the first phase of a collection: the four documents as bw
// wrote them, their decryption error counts, the manifest source and the
// plan of attachment downloads. The documents are plaintext vault content.
// The caller clears them with Wipe once it no longer needs them. The file
// names in Plan and the values in Source are strings, which Wipe cannot
// clear.
type Snapshot struct {
	PlaintextExport  []byte
	EncryptedExport  []byte
	ItemList         []byte
	ArchivedItemList []byte
	DecryptionErrors capture.DecryptionErrors
	// Source is taken from the accepted status. A value the status does
	// not hold is empty.
	Source manifest.Source
	// Plan lists the attachments to download, in order (design D6).
	Plan []PlannedAttachment
}

// PlannedAttachment is one attachment to download. The file name is vault
// content and goes into the encrypted manifest only.
type PlannedAttachment struct {
	ItemID       string
	AttachmentID string
	FileName     string
}

// Ref returns the ID pair of the attachment.
func (p PlannedAttachment) Ref() capture.AttachmentRef {
	return capture.AttachmentRef{ItemID: p.ItemID, AttachmentID: p.AttachmentID}
}

// Documents returns the four documents for archive.NewWriter. The slices
// are shared with the snapshot, not copied.
func (s *Snapshot) Documents() archive.Documents {
	return archive.Documents{
		PlaintextExport:  s.PlaintextExport,
		EncryptedExport:  s.EncryptedExport,
		ItemList:         s.ItemList,
		ArchivedItemList: s.ArchivedItemList,
	}
}

// Input returns the input of capture.Parse: the four documents, their
// counts and the given download results. The slices are shared with the
// snapshot, not copied.
func (s *Snapshot) Input(downloads []capture.Download) capture.Input {
	return capture.Input{
		PlaintextExport:  s.PlaintextExport,
		EncryptedExport:  s.EncryptedExport,
		ItemList:         s.ItemList,
		ArchivedItemList: s.ArchivedItemList,
		Downloads:        downloads,
		DecryptionErrors: s.DecryptionErrors,
	}
}

// Wipe overwrites the four documents with zero bytes (design D10). Slices
// that share their memory, such as those from Documents and Input, are
// cleared as well.
func (s *Snapshot) Wipe() {
	for _, b := range [][]byte{s.PlaintextExport, s.EncryptedExport, s.ItemList, s.ArchivedItemList} {
		clear(b)
	}
}

// Snapshot forces a sync, checks that the snapshot is fresh and collects
// the four documents (designs D3 to D6). Any failure ends the collection:
// no snapshot is returned, and documents collected before the failure are
// wiped. A failure of a bw command is the *bwcli.Error of that command.
func (c *Collector) Snapshot(ctx context.Context, v Vault) (*Snapshot, error) {
	// bw writes lastSync with millisecond precision. Truncating also drops
	// the monotonic reading, so the comparison uses the wall clock as bw
	// does.
	t0 := c.now().Truncate(time.Millisecond)
	if err := v.Sync(ctx); err != nil {
		return nil, vaultError(bwcli.OpSync, err)
	}
	st, err := v.Status(ctx)
	if err != nil {
		return nil, vaultError(bwcli.OpStatus, err)
	}
	if st.State != bwcli.StateUnlocked {
		return nil, ErrNotUnlocked
	}
	if st.LastSync == nil || st.LastSync.Truncate(time.Millisecond).Before(t0) {
		return nil, ErrNotFresh
	}

	s := &Snapshot{Source: manifest.Source{
		ServerURL:    st.ServerURL,
		AccountEmail: st.UserEmail,
		AccountID:    st.UserID,
	}}
	for _, d := range []struct {
		op    bwcli.Op
		get   func(context.Context) (bwcli.Output, error)
		data  *[]byte
		count *int
	}{
		{bwcli.OpExportJSON, v.ExportJSON, &s.PlaintextExport, &s.DecryptionErrors.PlaintextExport},
		{bwcli.OpExportEncrypted, v.ExportEncryptedJSON, &s.EncryptedExport, &s.DecryptionErrors.EncryptedExport},
		{bwcli.OpListItems, v.ListItems, &s.ItemList, &s.DecryptionErrors.ItemList},
		{bwcli.OpListArchived, v.ListArchivedItems, &s.ArchivedItemList, &s.DecryptionErrors.ArchivedItemList},
	} {
		out, err := d.get(ctx)
		if err != nil {
			clear(out.Data)
			s.Wipe()
			return nil, vaultError(d.op, err)
		}
		*d.data, *d.count = out.Data, out.DecryptionErrors
	}
	s.Plan = plan(s.ItemList, s.ArchivedItemList)
	return s, nil
}

// plan lists the attachments of personal items in both lists (designs D5
// and D6). When either list is rejected, the plan is empty: the
// verification then fails the capture, and downloads would cost time for a
// generation that is never published.
func plan(itemList, archivedItemList []byte) []PlannedAttachment {
	list, err := capture.ParseItemList(itemList)
	if err != nil {
		return nil
	}
	archived, err := capture.ParseArchivedItemList(archivedItemList)
	if err != nil {
		return nil
	}
	var out []PlannedAttachment
	for _, ref := range capture.PersonalAttachments(list, archived) {
		name, ok := list.FileNames[ref]
		if !ok {
			name = archived.FileNames[ref]
		}
		out = append(out, PlannedAttachment{ItemID: ref.ItemID, AttachmentID: ref.AttachmentID, FileName: name})
	}
	return out
}
