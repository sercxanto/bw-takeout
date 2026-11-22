package capturetest

import (
	"testing"

	"bw-takeout/internal/capture"
)

// IDs of the intact organization fixture (design D13 of the change
// organization-export).
const (
	// ChosenOrganization is the one chosen organization.
	ChosenOrganization = "0c000000-0000-4000-8000-000000000001"
	// ItemOrgLogin is a login of ChosenOrganization with one passkey and
	// one attachment.
	ItemOrgLogin = "77777777-7777-4777-8777-777777777771"
	// ItemOrgNote is a secure note of ChosenOrganization.
	ItemOrgNote = "77777777-7777-4777-8777-777777777772"
	// AttachmentOrgLogin belongs to ItemOrgLogin.
	AttachmentOrgLogin = "att00000000000000000000000000007"

	// ItemOrgListedOnly is listed as an item of ChosenOrganization and is in
	// neither organization export.
	ItemOrgListedOnly = "77777777-7777-4777-8777-777777777773"
	// ItemUnchosenA and ItemUnchosenB belong to OrganizationFirst and
	// OrganizationSecond, which are not chosen. AttachmentUnchosen belongs to
	// ItemUnchosenA, and no download exists for it.
	ItemUnchosenA      = "77777777-7777-4777-8777-777777777774"
	ItemUnchosenB      = "77777777-7777-4777-8777-777777777775"
	AttachmentUnchosen = "att00000000000000000000000000008"

	// LaterRevisionDate and EarlierRevisionDate replace RevisionDate in the
	// variants that model an organization that changed during a capture.
	LaterRevisionDate   = "2026-02-03T04:05:06.500Z"
	EarlierRevisionDate = "2026-02-03T04:05:05.999Z"
)

// OrganizationFixture holds the two exports of one chosen organization as
// "bw export --organizationid" writes them.
type OrganizationFixture struct {
	ID              string
	PlaintextExport map[string]any
	EncryptedExport map[string]any
	// PlaintextExportBytes and EncryptedExportBytes, when not nil, replace
	// the rendering of the export in [Fixture.Input]. A non-nil empty slice
	// stands for an absent document.
	PlaintextExportBytes []byte
	EncryptedExportBytes []byte
	DecryptionErrors     capture.OrganizationDecryptionErrors
}

// PlaintextItems returns the items array of the plaintext organization
// export.
func (o *OrganizationFixture) PlaintextItems() []any {
	items, _ := o.PlaintextExport["items"].([]any)
	return items
}

// EncryptedItems returns the items array of the encrypted organization
// export.
func (o *OrganizationFixture) EncryptedItems() []any {
	items, _ := o.EncryptedExport["items"].([]any)
	return items
}

func (o *OrganizationFixture) input(t testing.TB) capture.OrganizationInput {
	t.Helper()
	in := capture.OrganizationInput{ID: o.ID, DecryptionErrors: o.DecryptionErrors}
	if o.PlaintextExportBytes != nil {
		in.PlaintextExport = append([]byte{}, o.PlaintextExportBytes...)
	} else {
		in.PlaintextExport = render(t, o.PlaintextExport, true)
	}
	if o.EncryptedExportBytes != nil {
		in.EncryptedExport = append([]byte{}, o.EncryptedExportBytes...)
	} else {
		in.EncryptedExport = render(t, o.EncryptedExport, true)
	}
	return in
}

func (o OrganizationFixture) clone() OrganizationFixture {
	c := OrganizationFixture{
		ID:               o.ID,
		PlaintextExport:  cloneTree(o.PlaintextExport).(map[string]any),
		EncryptedExport:  cloneTree(o.EncryptedExport).(map[string]any),
		DecryptionErrors: o.DecryptionErrors,
	}
	if o.PlaintextExportBytes != nil {
		c.PlaintextExportBytes = append([]byte{}, o.PlaintextExportBytes...)
	}
	if o.EncryptedExportBytes != nil {
		c.EncryptedExportBytes = append([]byte{}, o.EncryptedExportBytes...)
	}
	return c
}

// Organization returns the fixture of the organization with the given ID
// and fails the test when there is none.
func (f *Fixture) Organization(t testing.TB, id string) *OrganizationFixture {
	t.Helper()
	for i := range f.Organizations {
		if f.Organizations[i].ID == id {
			return &f.Organizations[i]
		}
	}
	t.Fatalf("fixture has no organization %s", id)
	return nil
}

// collectionOf returns the synthetic collection ID of an organization.
func collectionOf(organizationID string) string {
	return "c011ec71-0000-4000-8000-" + organizationID[len(organizationID)-12:]
}

// IntactOrganization returns the intact fixture with one chosen
// organization: ChosenOrganization holds a login with one passkey and one
// attachment, and a secure note. The default item list shows both, both
// organization exports hold both with the same revision dates, and the
// reference inventory includes them.
func IntactOrganization(t testing.TB) *Fixture {
	t.Helper()
	f := Intact(t)
	specs := []itemSpec{
		{
			id: ItemOrgLogin, label: "org-login", typ: typeLogin, organizationID: ChosenOrganization,
			passkeys: []passkeySpec{newP256Passkey(t, 3, "4")},
			attachments: []attachmentSpec{
				{id: AttachmentOrgLogin, n: 7, content: []byte(Marker + " attachment content of the organization login")},
			},
		},
		{id: ItemOrgNote, label: "org-note", typ: typeSecureNote, organizationID: ChosenOrganization},
	}
	collection := collectionOf(ChosenOrganization)
	o := OrganizationFixture{
		ID: ChosenOrganization,
		PlaintextExport: map[string]any{
			"encrypted": false,
			"collections": []any{map[string]any{
				"id": collection, "organizationId": ChosenOrganization, "name": sentinel("collection-name"),
				"externalId": nil,
			}},
			"items": []any{},
		},
		EncryptedExport: map[string]any{
			"encrypted":                    true,
			"encKeyValidation_DO_NOT_EDIT": enc("organization-key-validation"),
			"collections": []any{map[string]any{
				"id": collection, "organizationId": ChosenOrganization, "name": enc("collection-name"),
				"externalId": nil,
			}},
			"items": []any{},
		},
	}
	for _, s := range specs {
		plain, sealed := plainItem(s), encryptedItem(s)
		plain["collectionIds"] = []any{collection}
		sealed["collectionIds"] = []any{collection}
		o.PlaintextExport["items"] = append(o.PlaintextItems(), plain)
		o.EncryptedExport["items"] = append(o.EncryptedItems(), sealed)
		f.ItemList = append(f.ItemList, listItem(s))

		ref := ReferenceItem{ItemID: s.id}
		for _, p := range s.passkeys {
			ref.Passkeys = append(ref.Passkeys, p.reference(t))
		}
		for _, a := range s.attachments {
			ref.Attachments = append(ref.Attachments, referenceAttachment(a))
			f.Attachments = append(f.Attachments, Attachment{ItemID: s.id, AttachmentID: a.id, Content: a.content})
		}
		f.Reference.Items = append(f.Reference.Items, ref)
	}
	f.Organizations = append(f.Organizations, o)
	return f
}

// Names of the variants of the intact organization fixture (design D13 of
// the change organization-export).
const (
	OrgItemMissingFromPlaintextExport = "org-item-missing-from-plaintext-export"
	OrgExportHoldsOtherOrganization   = "org-export-holds-other-organization"
	OrgItemListedAsPersonal           = "org-item-listed-as-personal"
	OrgRevisionDiffersBetweenExports  = "org-revision-differs-between-exports"
	OrgRevisionDiffersFromList        = "org-revision-differs-from-list"
	OrgListedItemNotExported          = "org-listed-item-not-exported"
	OrgExportedItemNotListed          = "org-exported-item-not-listed"
	OrgBlobLoginItem                  = "org-blob-login-item"
	OrgEmptyPasskeyKey                = "org-empty-passkey-key"
	OrgPasswordProtectedExport        = "org-password-protected-export"
	OrgDocumentAbsent                 = "org-document-absent"
	OrgDecryptionErrorLines           = "org-decryption-error-lines"
	OrgAttachmentDownloadMissing      = "org-attachment-download-missing"
	OrgNotChosenItems                 = "org-not-chosen-items"
)

// PasswordProtectedExport is the form of an export with a password. It
// holds no items key and is not the second inventory.
const PasswordProtectedExport = `{"encrypted":true,"passwordProtected":true,"salt":"c2FsdA==",` +
	`"kdfType":0,"kdfIterations":600000,"encKeyValidation_DO_NOT_EDIT":"2.a|b|c","data":"2.a|b|c"}`

// Paths of the organization documents in the notation of [Diff].
const (
	orgPath      = "organization[" + ChosenOrganization + "]"
	orgPlaintext = orgPath + "/plaintext/items"
	orgEncrypted = orgPath + "/encrypted/items"
)

// OrganizationVariants returns the variants of the intact organization
// fixture in a fixed order. Each one breaks it in one place.
func OrganizationVariants() []Variant {
	vs := []Variant{
		{
			Name:    OrgItemMissingFromPlaintextExport,
			Models:  "The note of the chosen organization is missing from its plaintext export, while its encrypted export and the item list hold it.",
			Changes: []string{"- " + orgPlaintext + "[" + ItemOrgNote + "]"},
			apply: func(t testing.TB, f *Fixture) {
				o := f.Organization(t, ChosenOrganization)
				o.PlaintextExport["items"] = withoutItem(o.PlaintextItems(), ItemOrgNote)
			},
		},
		{
			Name:    OrgExportHoldsOtherOrganization,
			Models:  "The plaintext export of the chosen organization holds its note with the ID of another organization.",
			Changes: []string{"~ " + orgPlaintext + "[" + ItemOrgNote + "]/organizationId"},
			apply: func(t testing.TB, f *Fixture) {
				itemByID(f.Organization(t, ChosenOrganization).PlaintextItems(), ItemOrgNote)["organizationId"] = OrganizationFirst
			},
		},
		{
			Name:   OrgItemListedAsPersonal,
			Models: "The item list shows the note of the chosen organization as a personal item.",
			Changes: []string{
				"- list[" + ItemOrgNote + "]/organizationId",
				"- list[" + ItemOrgNote + "]/collectionIds/0",
			},
			apply: func(t testing.TB, f *Fixture) {
				item := itemByID(f.ItemList, ItemOrgNote)
				delete(item, "organizationId")
				item["collectionIds"] = []any{}
			},
		},
		{
			Name:    OrgRevisionDiffersBetweenExports,
			Models:  "The note of the chosen organization was edited between its two exports, so the encrypted export shows a later revision.",
			Changes: []string{"~ " + orgEncrypted + "[" + ItemOrgNote + "]/revisionDate"},
			apply: func(t testing.TB, f *Fixture) {
				itemByID(f.Organization(t, ChosenOrganization).EncryptedItems(), ItemOrgNote)["revisionDate"] = LaterRevisionDate
			},
		},
		{
			Name:    OrgRevisionDiffersFromList,
			Models:  "The note of the chosen organization was edited after the snapshot, so the item list shows an earlier revision than both exports.",
			Changes: []string{"~ list[" + ItemOrgNote + "]/revisionDate"},
			apply: func(t testing.TB, f *Fixture) {
				itemByID(f.ItemList, ItemOrgNote)["revisionDate"] = EarlierRevisionDate
			},
		},
		{
			Name:    OrgListedItemNotExported,
			Models:  "The item list shows an item of the chosen organization that neither organization export contains.",
			Changes: []string{"+ list[" + ItemOrgListedOnly + "]"},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = append(f.ItemList, listItem(itemSpec{
					id: ItemOrgListedOnly, label: "org-listed-only", typ: typeSecureNote, organizationID: ChosenOrganization,
				}))
			},
		},
		{
			Name:    OrgExportedItemNotListed,
			Models:  "Both exports of the chosen organization hold its note, and neither item list shows it.",
			Changes: []string{"- list[" + ItemOrgNote + "]"},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = withoutItem(f.ItemList, ItemOrgNote)
			},
		},
		{
			Name:   OrgBlobLoginItem,
			Models: "The encrypted export of the chosen organization shows its login in blob form, so its passkey count is sealed.",
			Changes: []string{
				"+ " + orgEncrypted + "[" + ItemOrgLogin + "]/data",
				"- " + orgEncrypted + "[" + ItemOrgLogin + "]/fields",
				"- " + orgEncrypted + "[" + ItemOrgLogin + "]/login",
				"- " + orgEncrypted + "[" + ItemOrgLogin + "]/name",
				"- " + orgEncrypted + "[" + ItemOrgLogin + "]/notes",
				"- " + orgEncrypted + "[" + ItemOrgLogin + "]/passwordHistory",
			},
			apply: func(t testing.TB, f *Fixture) {
				item := itemByID(f.Organization(t, ChosenOrganization).EncryptedItems(), ItemOrgLogin)
				for _, key := range []string{"name", "notes", "fields", "login", "passwordHistory"} {
					delete(item, key)
				}
				item["data"] = BlobData
			},
		},
		{
			Name:    OrgEmptyPasskeyKey,
			Models:  "The passkey of the organization login has an empty private key in the plaintext organization export.",
			Changes: []string{"~ " + orgPlaintext + "[" + ItemOrgLogin + "]/login/fido2Credentials/0/keyValue"},
			apply: func(t testing.TB, f *Fixture) {
				item := itemByID(f.Organization(t, ChosenOrganization).PlaintextItems(), ItemOrgLogin)
				passkeys := item["login"].(map[string]any)["fido2Credentials"].([]any)
				passkeys[0].(map[string]any)["keyValue"] = ""
			},
		},
		{
			Name:    OrgPasswordProtectedExport,
			Models:  "The encrypted export of the chosen organization is protected by a password.",
			Changes: []string{"~ " + orgPath + "/encrypted-bytes"},
			apply: func(t testing.TB, f *Fixture) {
				f.Organization(t, ChosenOrganization).EncryptedExportBytes = []byte(PasswordProtectedExport)
			},
		},
		{
			Name:    OrgDocumentAbsent,
			Models:  "The capture holds no plaintext export for the chosen organization.",
			Changes: []string{"~ " + orgPath + "/plaintext-bytes"},
			apply: func(t testing.TB, f *Fixture) {
				f.Organization(t, ChosenOrganization).PlaintextExportBytes = []byte{}
			},
		},
		{
			Name:    OrgDecryptionErrorLines,
			Models:  "The CLI wrote two decryption error lines with the plaintext export of the chosen organization and exited with code 0.",
			Changes: []string{"~ " + orgPath + "/decryption-errors"},
			apply: func(t testing.TB, f *Fixture) {
				f.Organization(t, ChosenOrganization).DecryptionErrors.PlaintextExport = 2
			},
		},
		{
			Name:    OrgAttachmentDownloadMissing,
			Models:  "No download result exists for the attachment of the organization login.",
			Changes: []string{"~ attachments"},
			apply: func(t testing.TB, f *Fixture) {
				f.Attachments = withoutAttachmentsOf(f.Attachments, ItemOrgLogin)
			},
		},
		{
			Name:   OrgNotChosenItems,
			Models: "The item list shows an item of each of two organizations that are not chosen, one with an attachment that was not downloaded.",
			Changes: []string{
				"+ list[" + ItemUnchosenA + "]",
				"+ list[" + ItemUnchosenB + "]",
			},
			apply: func(t testing.TB, f *Fixture) {
				f.ItemList = append(f.ItemList,
					listItem(itemSpec{
						id: ItemUnchosenA, label: "unchosen-a", typ: typeLogin, organizationID: OrganizationFirst,
						attachments: []attachmentSpec{{id: AttachmentUnchosen, n: 8}},
					}),
					listItem(itemSpec{id: ItemUnchosenB, label: "unchosen-b", typ: typeSecureNote, organizationID: OrganizationSecond}),
				)
			},
		},
	}
	for i := range vs {
		vs[i].base = IntactOrganization
	}
	return vs
}
