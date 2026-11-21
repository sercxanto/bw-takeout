// Package capture holds the data model of one capture of a personal vault
// and of the chosen organizations, and the parsers that turn the documents
// the official bw CLI emitted into that model. It starts no process and
// reads no file. See the design of the change capture-verification,
// decisions D1, D3 and D4, and of the change organization-export, decision
// D4.
//
// # Evidence level
//
// Every shape below is derived from source code. Two sources were read:
//
//   - "source": bitwarden/clients at commit
//     91214c3b978edb5496a32a946451960fe7d44531, the release commit of CLI
//     2026.10.1, read on 13 November 2026. Links below point to this commit.
//   - "bundle": build/bw.js of the installed npm package @bitwarden/cli
//     2026.10.1 with SHA-256
//     8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b. The
//     blob marker, both encrypted document literals, the partial-item filter
//     and the attachment response class were found there unchanged.
//
// One live run has since observed part of these shapes: bw 2026.10.1
// against Vaultwarden 1.37.4 on 13 and 14 November 2026, reported in
// docs/research/vaultwarden-live-probe-2026-11-14.md. For a personal legacy
// login it showed:
//
//   - the legacy form of the account-bound encrypted export: per-field
//     encrypted strings, a wrapped item key, no data key, no format_version
//     marker, one fido2Credentials entry per passkey with creationDate in
//     clear (report, results 3 and 6),
//   - the plaintext export with every passkey, and an empty
//     fido2Credentials array after a passkey failed to decrypt (results 3
//     and 6),
//   - the item list with the attachment keys id, fileName, size, sizeName
//     and url, where size is the encrypted size (results 2 and 3),
//   - that the default item list leaves archived items out while both
//     exports keep them, and that the archived item list shows them (added
//     case "archived item").
//
// Still unconfirmed: the blob form, any capture from Bitwarden Cloud,
// partial items, and whether a JSON key is absent or null in a real
// document. The parsers and the fixtures of package capturetest were never
// run against a real capture; the report was only compared with them by
// reading. The parsers therefore treat an absent key and a null value alike,
// and anything they do not recognize positively lowers the verdict.
//
// # Plaintext export
//
// Produced by "bw export --format json". The document is an object
// ([types], [buildJsonExport]):
//
//	encrypted  false
//	folders    array of {id, name}
//	items      array of items
//
// An item is a CipherWithIdExport built from a decrypted view
// ([CipherWithIdExport], [CipherExport.build]):
//
//	id              string, the item ID
//	organizationId  absent or null for every item in this document, because
//	                organization items are skipped
//	folderId        string, absent or null
//	collectionIds   always null in a personal export
//	type            number: 1 login, 2 secure note, 3 card, 4 identity,
//	                5 SSH key, 6 bank account, 7 driver's license, 8 passport
//	                ([CipherType])
//	name, notes     decrypted strings
//	favorite        boolean
//	reprompt        number
//	fields          array of {name, value, type, linkedId}
//	login, secureNote, card, identity, sshKey, bankAccount, driversLicense,
//	passport        the one object that matches the type
//	passwordHistory array of {password, lastUsedDate}
//	creationDate, revisionDate, deletedDate, archivedDate  ISO 8601 strings
//
// The item key is deleted before the document is written. The login object
// holds uris, username, password, totp and fido2Credentials ([LoginExport]).
// The decrypted login view initializes fido2Credentials to an empty array
// ([LoginView]), so a login item without passkeys shows "fido2Credentials":
// [] here. The reproduced omission ends in the same empty array.
//
// A passkey is a Fido2CredentialExport ([Fido2CredentialExport]):
//
//	credentialId     string, a UUID ([createKeyView])
//	keyType          "public-key"
//	keyAlgorithm     "ECDSA"
//	keyCurve         "P-256"
//	keyValue         private key, PKCS#8 DER in base64url without padding
//	                 ([createKeyView], [arrayToString])
//	rpId             string
//	counter          string of digits; the view holds a number that the
//	                 export converts with String()
//	discoverable     "true" or "false" as a string
//	creationDate     ISO 8601 string
//	userHandle       optional, base64url without padding
//	userName, rpName, userDisplayName  optional strings
//
// Filters: deleted items, items of a restricted item type and organization
// items are left out ([getDecryptedExport], [buildJsonExport]). The export
// reads the decrypted views, which already drop partial items
// ([getAllDecrypted]). Archived items are not filtered.
//
// # Account-bound encrypted export
//
// Produced by "bw export --format encrypted_json" without a password. The
// document is an object ([types], [getEncryptedExport]):
//
//	encrypted                     true
//	encKeyValidation_DO_NOT_EDIT  encrypted string
//	folders                       array of {id, name}, name encrypted
//	items                         array of items
//
// It has no passwordProtected key and no top-level data key. Items are built
// from the encrypted items before any decryption. An encrypted string has
// the CLI notation "2.<iv>|<ciphertext>|<mac>" with base64 parts. This
// package never decrypts one.
//
// Every item carries id, type, organizationId, folderId, collectionIds
// (always null), favorite, reprompt, key (the wrapped item key, when the
// item has one) and the four dates in clear. The rest depends on the item
// format.
//
// Legacy form ([CipherExport.buildContent], [legacy test]): the item has
// the same keys as in the plaintext export, with every content value
// replaced by an encrypted string. It has a name and no data key. A login
// item has a login object. Its fido2Credentials array has one object per
// passkey, each with the keys listed above and encrypted values, so the
// number of passkeys is visible. The encrypted login omits fido2Credentials
// when the item has no passkeys ([LoginDomain], [LoginExport]), so an absent
// key on a legacy login item means zero passkeys.
//
// Blob form ([CipherExport.buildContent], [blob test]): the item has a data
// key that holds a string, and that string contains the marker
// "format_version" with the quotes ([blob marker]). The upstream test shows
// {"format_version":1,"wrapped_cek":"2.a|b|c","envelope":"g1hH"}. The item
// has no name, no fields, no passwordHistory and no type-specific object.
// The passkeys of a blob login item are sealed inside data, and their number
// is not visible. Upstream calls the marker check an implementation detail
// and tells callers not to parse data ([model contract]).
//
// Any item that matches neither form is unknown.
//
// Filters: partial items, deleted items, items of a restricted item type
// and organization items are left out ([getEncryptedExport]). Archived items
// are not filtered. The plaintext export has no partial-item filter of its
// own, but the decrypted views it reads drop partial items, so at code level
// both exports apply the same filters. In the live run both exports held the
// same items, including an archived one. Partial, deleted and restricted
// items were not tested there.
//
// # Password-protected export
//
// Produced by "bw export --format encrypted_json --password", and also by
// "--format json" or "--format zip" together with a password
// ([ExportCommand]). The document is an object ([types],
// [buildPasswordExport]):
//
//	encrypted                     true
//	passwordProtected             true
//	salt                          base64 string
//	kdfType, kdfIterations        numbers
//	kdfMemory, kdfParallelism     numbers, only for Argon2id
//	encKeyValidation_DO_NOT_EDIT  encrypted string
//	data                          one encrypted string
//
// It has no items key. Its content is the plaintext export, encrypted with a
// key derived from the password ([getPasswordProtectedExport]), so it
// inherits every omission of the plaintext export and cannot serve as the
// second inventory. A capture that supplies it as the encrypted export is
// rejected.
//
// # Item list
//
// Produced by "bw list items". Standard output is a bare JSON array
// ([processResponse]). With BW_RESPONSE=true the CLI wraps it in
// {"success", "data": {"object": "list", "data": [...]}}. This package
// expects the bare array.
//
// An element is a CipherResponse ([CipherResponse]). It has the keys of a
// plaintext export item and adds or changes:
//
//	object          "item"
//	organizationId  string for organization items, absent or null otherwise
//	collectionIds   array of strings, empty for personal items
//	attachments     array of {id, fileName, size, sizeName, url}
//	                ([AttachmentResponse]); size is a string
//	login           adds passwordRevisionDate ([LoginResponse])
//
// The list holds the same decrypted content as the plaintext export,
// including passkey private keys. This package reads only the item ID, the
// organization ID, the attachment IDs and the attachment file names from it.
//
// The file name is vault content. [ItemList.FileNames] keeps it so that the
// caller can write it into the encrypted manifest, and for no other use: no
// check reads it, and it never identifies an attachment. The first
// occurrence of an ID pair wins, and a name that is absent or not a string
// is empty without rejecting the list (design D5 of capture-collection).
// [ScopedAttachments] decides which attachments are expected, for the
// download plan and for the verification alike: those of personal items and
// of items of the chosen organizations (design D6 of capture-collection and
// of organization-export).
//
// Filters ([listCiphers]): the list reads the same decrypted views as the
// plaintext export, so partial items are missing from it
// ([getAllDecrypted]). Without options the list leaves out deleted and
// archived items and items of a restricted item type, and it includes
// organization items. With --archived it shows only archived items
// ([matchesStateOptions]). The exports do not filter archived items, so a
// default listing alone is not comparable to the exports when the vault
// holds archived items.
//
// # Archived item list
//
// Produced by "bw list items --archived". It has the same form as the item
// list and is parsed by the same rules. It is a document of its own, named
// archived_item_list, and is never merged with the default list, so that a
// rejection or a duplicate names the list it occurred in (design D1 of the
// change archived-item-list). An absent archived list is a zero-length input
// and is rejected as invalid JSON. An empty list is the two bytes "[]".
//
// Observed live (report above, added case "archived item"): the default list
// left one archived login out, both exports kept it, and the --archived list
// showed only that login. The login had no attachments.
//
// From code reading of the bundle only, recorded in
// docs/research/bw-cli-code-notes-2026-11-16.md:
//
//   - For an account without premium from any source, the CLI drops
//     --archived before filtering and returns the default list again,
//     without an error. Archived items of such an account appear in neither
//     list, while both exports still hold them.
//   - Items in the trash are in no captured document: both lists leave them
//     out, and both personal exports filter deleted items.
//
// Not observed live: an account that may not archive, the trash, and an
// archived item with an attachment.
//
// # Organization exports
//
// Produced by "bw export --organizationid <id>" with "--format json" and
// "--format encrypted_json", once per chosen organization. [Input] holds
// them in [OrganizationInput], and [Capture] the parsed form in
// [OrganizationCapture], in the order the organizations were chosen. From
// code reading of the bundle and the spike of 21 November 2026
// (docs/research/organization-export-spike-2026-11-21.md):
//
//   - Both forms are fetched from the server on every call and do not read
//     the local snapshot. The whole organization is exported, also
//     collections the account is not assigned to.
//   - The top-level form is that of the personal exports, with collections
//     in place of folders. The plaintext form has encrypted false,
//     collections and items. The encrypted form has encrypted true,
//     encKeyValidation_DO_NOT_EDIT, collections and items, with values
//     sealed with the organization key and an item key per item.
//   - Every item carries organizationId, collectionIds and revisionDate,
//     and no attachments. Deleted items are left out.
//
// [ParseOrganizationPlaintextExport] and [ParseOrganizationEncryptedExport]
// apply the rules of the personal parsers, which read neither folders nor
// collections. A rejection is a [DocumentError] with the document
// organization_plaintext_export or organization_encrypted_export and the
// organization ID. An absent export is a zero-length input and is rejected
// as invalid JSON.
//
// Not observed: blob items and passkeys in an organization export, and
// decryption error lines on one.
//
// # Revision dates
//
// [PlaintextItem], [EncryptedItem] and [ListedItem] keep the raw
// revisionDate string, which every document writes in clear. A value that
// is absent or not a string is kept as empty and never rejects a document,
// so that personal documents are judged as before. The verification parses
// the value where it compares revisions, for the chosen organizations only.
//
// # Attachment download
//
// "bw get attachment <attachment-id> --itemid <item-id>" writes the
// decrypted bytes and reports a status other than 200 as an error
// ([saveAttachmentToFile]). It emits no document. The caller records one
// download result per ID pair.
//
// # Snapshot
//
// The four documents and the attachment metadata come from one local
// snapshot. From code reading of the bundle, recorded in
// docs/research/bw-cli-code-notes-2026-11-17.md: list, export and
// get attachment read the local state and start no sync, so the forced
// sync before the first document fixes the snapshot for all of them. The
// attachment content is fetched from the server at the time of the
// download. No network trace confirms this at run time.
//
// # Decryption error counts
//
// The CLI can write decryption error lines to standard error and still exit
// with code 0. The lines name neither item nor field. The caller passes
// their number for each document in [Input.DecryptionErrors] and for each
// successful download in [Download.DecryptionErrors]. [Parse] copies the
// document counts into [Capture.DecryptionErrors] unchanged. The zero value
// means no lines, so an input without counts parses as before (design D9 of
// capture-collection). The live probe observed such lines on the plaintext
// export, the item list and attachment downloads, and none on the encrypted
// export (docs/research/vaultwarden-live-probe-2026-11-14.md, result 6).
//
// [types]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/types/bitwarden-json-export-types.ts#L14-L55
// [buildJsonExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.ts#L333-L361
// [getDecryptedExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.ts#L194-L235
// [getEncryptedExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.ts#L237-L302
// [getPasswordProtectedExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/individual-vault-export.service.ts#L74-L89
// [buildPasswordExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/tools/export-vault-core/src/services/base-vault-export.service.ts#L21-L52
// [ExportCommand]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/tools/export.command.ts#L46-L82
// [CipherWithIdExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher-with-ids.export.ts#L6-L16
// [CipherExport.build]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.ts#L199-L249
// [CipherExport.buildContent]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.ts#L251-L297
// [blob marker]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.ts#L21-L22
// [blob test]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.spec.ts#L47-L107
// [legacy test]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/cipher.export.spec.ts#L109-L144
// [model contract]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/models/domain/cipher.ts#L32-L44
// [LoginExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/login.export.ts#L51-L73
// [LoginView]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/models/view/login.view.ts#L24
// [LoginDomain]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/models/domain/login.ts#L40-L42
// [Fido2CredentialExport]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/models/export/fido2-credential.export.ts#L82-L122
// [CipherResponse]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/vault/models/cipher.response.ts#L13-L52
// [listCiphers]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/list.command.ts#L70-L157
// [matchesStateOptions]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/list.command.ts#L302-L311
// [getAllDecrypted]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/services/cipher.service.ts#L370-L378
// [processResponse]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/base-program.ts#L57-L58
// [saveAttachmentToFile]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/commands/download.command.ts#L32-L62
//
// [CipherType]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/vault/enums/cipher-type.ts#L1-L10
// [createKeyView]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/platform/services/fido2/fido2-authenticator.service.ts#L483-L505
// [arrayToString]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/libs/common/src/platform/services/fido2/fido2-utils.ts#L44-L49
// [AttachmentResponse]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/vault/models/attachment.response.ts#L5-L18
// [LoginResponse]: https://github.com/bitwarden/clients/blob/91214c3b978edb5496a32a946451960fe7d44531/apps/cli/src/models/response/login.response.ts#L6-L16
package capture
