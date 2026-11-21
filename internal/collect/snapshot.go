package collect

import (
	"context"
	"time"

	"bw-takeout/internal/archive"
	"bw-takeout/internal/bwcli"
	"bw-takeout/internal/capture"
	"bw-takeout/internal/manifest"
)

// Snapshot is the first phase of a collection: the four documents and the
// two documents of each chosen organization as bw wrote them, their
// decryption error counts, the manifest source and the plan of attachment
// downloads. The documents are plaintext vault content. The caller clears
// them with Wipe once it no longer needs them. The file names in Plan, the
// values in Source and the organization names are strings, which Wipe
// cannot clear.
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
	// Organizations holds one entry per chosen organization, in the order
	// they were chosen. Its ID and name are part of the source of the
	// capture (design D8 of organization-export).
	Organizations []OrganizationSnapshot
}

// OrganizationSnapshot holds the two exports of one chosen organization.
type OrganizationSnapshot struct {
	ID string
	// Name is the name the accepted organization list gives, or empty. It
	// goes into the encrypted manifest only.
	Name             string
	PlaintextExport  []byte
	EncryptedExport  []byte
	DecryptionErrors capture.OrganizationDecryptionErrors
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

// Documents returns the documents for archive.NewWriter. The slices are
// shared with the snapshot, not copied.
func (s *Snapshot) Documents() archive.Documents {
	d := archive.Documents{
		PlaintextExport:  s.PlaintextExport,
		EncryptedExport:  s.EncryptedExport,
		ItemList:         s.ItemList,
		ArchivedItemList: s.ArchivedItemList,
	}
	for _, o := range s.Organizations {
		d.Organizations = append(d.Organizations, archive.OrganizationDocuments{
			ID: o.ID, PlaintextExport: o.PlaintextExport, EncryptedExport: o.EncryptedExport,
		})
	}
	return d
}

// OrganizationNames returns the name of each chosen organization by ID,
// for archive.Meta.
func (s *Snapshot) OrganizationNames() map[string]string {
	names := make(map[string]string, len(s.Organizations))
	for _, o := range s.Organizations {
		names[o.ID] = o.Name
	}
	return names
}

// Input returns the input of capture.Parse: the documents, their counts
// and the given download results. The slices are shared with the snapshot,
// not copied.
func (s *Snapshot) Input(downloads []capture.Download) capture.Input {
	in := capture.Input{
		PlaintextExport:  s.PlaintextExport,
		EncryptedExport:  s.EncryptedExport,
		ItemList:         s.ItemList,
		ArchivedItemList: s.ArchivedItemList,
		Downloads:        downloads,
		DecryptionErrors: s.DecryptionErrors,
	}
	for _, o := range s.Organizations {
		in.Organizations = append(in.Organizations, capture.OrganizationInput{
			ID: o.ID, PlaintextExport: o.PlaintextExport, EncryptedExport: o.EncryptedExport,
			DecryptionErrors: o.DecryptionErrors,
		})
	}
	return in
}

// Wipe overwrites the four documents and the organization documents with
// zero bytes (design D10). Slices that share their memory, such as those
// from Documents and Input, are cleared as well.
func (s *Snapshot) Wipe() {
	for _, b := range [][]byte{s.PlaintextExport, s.EncryptedExport, s.ItemList, s.ArchivedItemList} {
		clear(b)
	}
	for _, o := range s.Organizations {
		clear(o.PlaintextExport)
		clear(o.EncryptedExport)
	}
}

// Snapshot forces a sync, checks that the snapshot is fresh, checks the
// role in each chosen organization and collects the four documents and the
// documents of the chosen organizations (designs D3 to D6, and D2 and D3 of
// organization-export). Any failure ends the collection: no snapshot is
// returned, and documents collected before the failure are wiped. A failure
// of a bw command is the *bwcli.Error of that command, and a refused
// organization an *OrganizationError.
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

	names, err := c.checkOrganizations(ctx, v)
	if err != nil {
		return nil, err
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
	// The organization exports are server reads after the snapshot, so
	// every revision check compares the snapshot with a later read (design
	// D3 of organization-export).
	for i, id := range c.Organizations {
		s.Organizations = append(s.Organizations, OrganizationSnapshot{ID: id, Name: names[i]})
		o := &s.Organizations[i]
		for _, d := range []struct {
			op    bwcli.Op
			get   func(context.Context, string) (bwcli.Output, error)
			data  *[]byte
			count *int
		}{
			{bwcli.OpExportOrganizationJSON, v.ExportOrganizationJSON, &o.PlaintextExport, &o.DecryptionErrors.PlaintextExport},
			{bwcli.OpExportOrganizationEncrypted, v.ExportOrganizationEncryptedJSON, &o.EncryptedExport, &o.DecryptionErrors.EncryptedExport},
		} {
			out, err := d.get(ctx, id)
			if err != nil {
				clear(out.Data)
				s.Wipe()
				return nil, vaultError(d.op, err)
			}
			*d.data, *d.count = out.Data, out.DecryptionErrors
		}
	}
	s.Plan = plan(c.Organizations, s.ItemList, s.ArchivedItemList)
	return s, nil
}

// checkOrganizations reads the organization list once and checks each
// chosen organization in order: it must be listed, confirmed, and held with
// the role owner or admin (design D2 of organization-export). It returns
// the names in the order of the chosen IDs. Without a chosen organization
// it starts no command.
func (c *Collector) checkOrganizations(ctx context.Context, v Vault) ([]string, error) {
	if len(c.Organizations) == 0 {
		return nil, nil
	}
	orgs, err := v.ListOrganizations(ctx)
	if err != nil {
		return nil, vaultError(bwcli.OpListOrganizations, err)
	}
	byID := make(map[string]bwcli.Organization, len(orgs))
	for _, o := range orgs {
		if _, ok := byID[o.ID]; !ok {
			byID[o.ID] = o
		}
	}
	names := make([]string, 0, len(c.Organizations))
	for _, id := range c.Organizations {
		o, ok := byID[id]
		switch {
		case !ok:
			return nil, &OrganizationError{ID: id, Reason: OrganizationNotMember}
		case o.Status != bwcli.MembershipConfirmed:
			return nil, &OrganizationError{ID: id, Reason: OrganizationNotConfirmed}
		case o.Type != bwcli.OrganizationOwner && o.Type != bwcli.OrganizationAdmin:
			return nil, &OrganizationError{ID: id, Reason: OrganizationRole}
		}
		names = append(names, o.Name)
	}
	return names, nil
}

// plan lists the attachments of personal items and of items of the chosen
// organizations in both lists (designs D5 and D6, and D6 of
// organization-export). When either list is rejected, the plan is empty:
// the verification then fails the capture, and downloads would cost time
// for a generation that is never published.
func plan(chosen []string, itemList, archivedItemList []byte) []PlannedAttachment {
	list, err := capture.ParseItemList(itemList)
	if err != nil {
		return nil
	}
	archived, err := capture.ParseArchivedItemList(archivedItemList)
	if err != nil {
		return nil
	}
	var out []PlannedAttachment
	for _, ref := range capture.ScopedAttachments(chosen, list, archived) {
		name, ok := list.FileNames[ref]
		if !ok {
			name = archived.FileNames[ref]
		}
		out = append(out, PlannedAttachment{ItemID: ref.ItemID, AttachmentID: ref.AttachmentID, FileName: name})
	}
	return out
}
