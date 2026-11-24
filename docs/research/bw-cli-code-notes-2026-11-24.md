# Code Notes: Assumptions about bw Checked against 2026.11.1

Date: code reading and one unauthenticated run on 24 November 2026 (UTC). These notes collect the findings that the change `bw-version-2026-11` builds on: every assumption the client makes about `bw`, checked against the release source and the installed bundle of 2026.11.1, and the keys of the export models in 2026.10.1 and 2026.11.1. No account, no server of the project and no secret was involved.

## Versions and artifacts

| Component | Value |
|---|---|
| Release source, old | `bitwarden/clients` at tag `cli-v2026.10.1`, commit `91214c3b978edb5496a32a946451960fe7d44531` (the commit the GitHub release names) |
| Release source, new | `bitwarden/clients` at tag `cli-v2026.11.1`, commit `44e7e9727ca78de9ca3e9a7ecea1f187165c8ce1` (the commit the GitHub release names; `28b247ac34877882aaef4faea082a9f3bb201114` is the commit of 2026.11.0) |
| Bundle, old | `build/bw.js` of npm `@bitwarden/cli` 2026.10.1 in the dev container, 97082 lines, SHA-256 `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` |
| Bundle, new | `build/bw.js` of npm `@bitwarden/cli` 2026.11.1, installed into `.local/bw-2026.11.1/`, 104580 lines, SHA-256 `5fdd56c7497a68ef3ec782f709df6409eae9167e5babdd65747cf02e394068d3` |
| SDK, new | `@bitwarden/sdk-internal` 0.2.0-main.1214 (2026.10.1: 0.2.0-main.1126). Two WebAssembly builds: SIMD `5b760d8c99a51fe0eab0.module.wasm` (SHA-256 `3a0c239b…76a0`, chunk `685.js`), loaded when the runtime runs SIMD, and MVP `1b1feb8446c9f241e9a3.module.wasm` (SHA-256 `2e42b39f…8b2d`, chunk `916.js`) otherwise. 2026.10.1 had one build |
| Node engine range | `"node": ">=22"`, `"npm": ">=10"` in the `package.json` of 2026.11.1 |
| Vaultwarden | 1.37.4, tag commit `7c188967e7a467b19b2757bc3326f9cf3ccae327`, `src/api/core/mod.rs` and `src/config.rs` |
| Host | Dev container, Debian 12, Linux aarch64 |

`bw --version` of the new bundle printed `2026.11.1` with exit code 0 under Node 24.22.3 (`/usr/local/bin/node` of the dev container) and under Node 26.13.1, each with an empty `BITWARDENCLI_APPDATA_DIR`. It wrote the known "Could not find data file" line to standard error, as 2026.10.1 does (finding 12 of the [code notes of 15 November 2026](bw-cli-code-notes-2026-11-15.md)).

## Method

The two release trees were fetched shallowly, limited to `apps/cli/` and `libs/` without test files, and compared with `diff -r`. Every file the rows below name was compared, and each changed file was read in full in both versions. The installed bundle of 2026.11.1 was searched with `grep -n` for the literal texts and for the compiled form of each changed function. Line numbers in the column "Checked" refer to the 2026.11.1 source (`file:line`) or to its `bw.js` ("bundle"). The WebAssembly modules were searched with `grep -a` and `strings`.

Code reading shows what the code would do. It does not show what happens at run time, which server flags apply, or what the bundled SDK does inside a decryption.

## Assumption register

Results: **unchanged** (the code the assumption rests on is the same), **changed** (the code changed; the column says whether the client is affected), **not determinable** (code reading cannot settle it).

| # | Assumption | Source of the assumption | Depends on it | Checked in 2026.11.1 | Result |
|---|---|---|---|---|---|
| 1 | Any non-empty `BW_CLEANEXIT` turns a failure into exit code 0, and `BW_NOINTERACTION=true` stops the prompts | [code notes 15 Nov](bw-cli-code-notes-2026-11-15.md), findings 2 and 4 | `bwcli` environment | `apps/cli/src/base-program.ts:33` and `:167`, file unchanged; bundle 33375, 33503 | unchanged |
| 2 | Prompts go to standard error. `login --raw` prints only the session key, the base64 form of a 64-byte key | code notes 15 Nov, finding 4; [live probe](vaultwarden-live-probe-2026-11-14.md) | `bwcli.Login` | `apps/cli/src/auth/` unchanged, session key at `login.command.ts:444`; bundle 33314, 33330, 39869, 39892, every prompt with `output: process.stderr` | unchanged |
| 3 | The status document and its three states | `bwcli/status.go` | `bwcli/status.go` | `apps/cli/src/commands/status.command.ts:67` to `:71`, file unchanged; bundle 40312 to 40318 | unchanged |
| 4 | A decryption error line contains `ERROR bitwarden_crypto::` and goes to standard error, one line per failure | live probe, result 6; `bwcli/stream.go` | `bwcli/stream.go`, decryption findings | Marker form: see [Decryption error lines](#decryption-error-lines). The texts `bitwarden_crypto::store::context`, `bitwarden_crypto::traits::decryptable` and `The decryption operation failed` and the level label `%cERROR%c %s` are in both WebAssembly builds; the SDK log bindings still call `console.error` (bundle 10045 to 10061). One line per failure: decided inside the SDK | form unchanged; one line per failure not determinable (D3 of the change tests it) |
| 5 | The premium sentence of the attachment download is "Premium status is required to use this feature." | code notes 15 Nov, findings 5 and 9; `bwcli/stream.go` | `bwcli/stream.go` | `apps/cli/src/commands/get.command.ts:394`, `getAttachment` unchanged; bundle 59063 | unchanged |
| 6 | `list items` leaves out deleted and archived items, `--archived` returns only archived items, and for an account without premium from any source `--archived` is dropped | [code notes 16 Nov](bw-cli-code-notes-2026-11-16.md), findings 1 to 4 | `collect`, `archive_filter_not_applied` | `listCiphers` byte-identical in both versions; `list.command.ts:85`, `:327`; bundle 59401, 59402, 59599, 59600 | unchanged |
| 7 | `list`, `export` and `get attachment` start no sync; every `fullSync` call lies in login, unlock, `sync`, import, the organization API service and the migrations | [code notes 17 Nov](bw-cli-code-notes-2026-11-17.md) | the freshness rule of `collect` | Both bundles have 17 occurrences of `fullSync(` and none in the chunks. The source adds two callers, both outside the CLI: `libs/vault/src/routing/vault-scope.guard.ts` (a web router guard) and `libs/importer/.../present-import-outcome.ts` (import UI). `syncUpsertCipher(` occurs once (bundle 80047), the two interval timers are unchanged (bundle 36263, 75510) | unchanged |
| 8 | Export forms, the blob marker `"format_version"`, and the export filters (deleted, partial, restricted item types, organization) | package documentation of `capture` | `capture`, `verify` | `individual-vault-export.service.ts` and `bitwarden-json-export-types.ts` unchanged; marker at `cipher.export.ts:22` and bundle 57197; filters at `individual-vault-export.service.ts:212` and `:259`, bundle 60420, 60453. See [Restricted items](#restricted-gated-items) | unchanged |
| 9 | The organization export reads from the server on every call, exports the whole organization and checks no permission in the client | [organization spike](organization-export-spike-2026-11-21.md) | `organization-export` | `getOrganizationDecryptedExport` and `getOrganizationEncryptedExport` (`org-vault-export.service.ts:128`, `:191`) unchanged, both call `vaultExportApiService.getOrganizationExport`; `onlyManagedCollections = false` (`vault-export.service.ts:68`); bundle 60053 to 60114. Only the managed-collections export, which the CLI does not call, changed | unchanged |
| 10 | `get attachment` writes the decrypted bytes and turns a status other than 200 into an error | package documentation of `capture` | `collect` | `download.command.ts` unchanged, `:41`; bundle 50929 to 50932 | unchanged |
| 11 | The path of `get attachment` before and after the removal of the flag `pm-28192-cipher-attachment-ops-to-sdk`, and whether the premium sentence, the failure texts and the decryption error lines stay the same on it | design D2 of the change | `bwcli` download classes, `attachment_decryption_errors` | See [Attachment download](#attachment-download) | unchanged: the download never read the flag |
| 12 | The new exit after a command flushes standard output before it exits | release notes, PM-45308 | `bwcli` output reading | `apps/cli/src/exit-after-command.ts:12`, called at `bw.ts:42`; bundle 12380 to 12384, 104572. See [Exit after a command](#exit-after-a-command) | changed; the client is not affected |
| 13 | The key sets of the export models | design D4 of the change | `capture` | See [Export keys](#export-keys) | unchanged: no key added or removed |
| 14 | `bw list items` and the plaintext personal export read the decrypted views, which leave out every item that fails to decrypt as a whole | package documentation of `capture` | `item_missing_from_plaintext_export` | `cipher.service.ts:360`, `:417`; bundle 85405 to 85470. See [Decryption path](#decryption-path) | changed; the client is not affected |
| 15 | The export command writes the document after it collected the export event | code notes 17 Nov, finding 7 | none | `export.command.ts:87`; bundle 61045. See [Export event](#export-event) | changed; the client is not affected |
| 16 | The object names the client passes (`items`, `organizations`, `attachment`) keep their meaning | `bwcli/commands.go` | `bwcli` commands | `list.command.ts:56` to `:62`, `get.command.ts:104` to `:113`. See ["Shared folder" alias](#shared-folder-alias) | unchanged |

No row reads "changed" in a way that affects the client.

## Details

### Decryption error lines

The SDK formats a log record as `%cERROR%c %s` with two CSS strings, and `__wbg_error_1eece6b0039034ce` passes the four values to `console.error` (bundle 10045). Node drops the CSS strings for `%c` and prints `ERROR <target>: <fields>` to standard error. The format string, the target names `bitwarden_crypto::store::context` and `bitwarden_crypto::traits::decryptable`, and the text `The decryption operation failed` are in the old build and in both new builds. So a line that the 2026.10.1 SDK wrote as `ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed` keeps that form in 2026.11.1. The live probe observed exactly this form.

The SDK sets the CLI log level to `Error` before `init` (`cli-sdk-load.service.ts`, unchanged apart from the choice of build at `:9`). The literal marker that the client counts, `ERROR bitwarden_crypto::`, is therefore unchanged at code level.

Whether a field that fails to decrypt still yields one line, and whether the SDK of 0.2.0-main.1214 logs at the same places, is decided inside the WebAssembly code and cannot be read from the bundle. The tampered scenario of the Vaultwarden suite requires the finding from this change on (design D3).

### Decryption path

`getAllDecrypted`, which `list items` and the plaintext personal export read, changed in three ways:

- **One path for every server.** 2026.10.1 chose between the SDK bulk call `cipherSdkService.getAllDecrypted` (flag `pm-27632-cipher-crud-operations-to-sdk` on) and `decryptManyLegacy` (flag off), which decrypts one cipher at a time with `vault().ciphers().decrypt()`. 2026.11.1 removed the flag and always runs `temporary_get_all_decrypted` (`cipher.service.ts:417`), which also decrypts one cipher at a time with the same SDK call (`default-cipher-encryption.service.ts:140`). Vaultwarden 1.37.4 never sends that flag (see [Feature flags of Vaultwarden](#feature-flags-of-vaultwarden-1374)), so on Vaultwarden the decryption path is the same as before. On Bitwarden Cloud, where the flag may have been on, 2026.11.1 moves from the bulk call to the per-cipher call.
- **Failure of a whole cipher.** In both versions a cipher that fails to decrypt as a whole goes to the failure list and is left out of the decrypted views, so it is missing from `list items` and the plaintext export while the encrypted export still holds it. The verification reports this as `item_missing_from_plaintext_export`. In 2026.10.1 on Vaultwarden a failure of the SDK client itself rejected the whole call and failed the export. In 2026.11.1 each cipher fails on its own, so the same failure leaves an empty plaintext export with exit code 0. The verification reports each item as missing from the plaintext export, so the verdict is not weakened. Each failure writes `Failed to decrypt cipher <error>` through the log service to standard error (bundle 86761); 2026.10.1 wrote `Failed to decrypt cipher <id>: <error>`. The client keeps neither text.
- **Ciphers without an organization key.** `decryptManyLegacy` in 2026.10.1 first dropped ciphers whose organization key was missing (`excludeCiphersMissingOrgKey`). The new serial path does not, so such a cipher now fails to decrypt and is left out in the same way. After the forced sync of a capture, the stored ciphers belong to the organizations of the account, whose keys are present, so this state is not expected in a capture.

The batching of PM-46036 (`DECRYPT_BATCH_SIZE = 250`, `default-cipher-encryption.service.ts:16`, bundle 86614) applies to `decryptManyWithFailures`, which serves the list views of the web vault. No command the client runs reads it.

### Restricted ("gated") items

`CipherViewLikeUtils.isPartial` was renamed to `isRestricted` with the same body, `"partial" in cipher ? !!cipher.partial : false` (`cipher-view-like-utils.ts:424`, bundle 61697), and `isPartial` stays as an alias. `getAllDecrypted` filters with it as before (bundle 85410). The domain getter `Cipher.isPartial` is unchanged (`partialData != null`).

- **Personal exports:** unchanged. The plaintext export reads the decrypted views, which drop restricted items, and filters deleted items and restricted item types (`individual-vault-export.service.ts:212`). The encrypted export filters `!f.isPartial`, deleted items and restricted item types (`:259`).
- **Organization exports:** unchanged. The full export, which the CLI runs, drops `decCipher.partial` in plaintext and `cipher.isPartial` in the encrypted form (bundle 60080, 60114 ff.). The managed-collections export gained `getManagedExportGatedItemCount` (`org-vault-export.service.ts:269`) for a warning in the web export page. The CLI never calls it.
- **`list items`:** unchanged. It reads `getAllDecrypted`, so restricted items are not listed. The vault lists of the web client read the new `cipherListViewsIncludingRestricted$`, which the CLI does not use.

Gated items therefore stay in no captured document, as in 2026.10.1. They belong to the existing limit "items the CLI leaves out of every document". Vaultwarden has no PAM features, so none of this was observed.

### "Shared folder" alias

`list` accepts `my-folders`, `shared-folders` and `org-shared-folders` next to `folders`, `collections` and `org-collections`. `get` accepts `my-folder`, `shared-folder` and `org-shared-folder`, and `list` accepts `--sharedfolderid` (`list.command.ts:56` to `:62`, `get.command.ts:104` to `:113`). The flag `vfo-1-foundation` (`FeatureFlag.VFO1Foundation`) only switches the help texts and the term in folder and collection responses. The objects `items`, `organizations` and `attachment` and the item and organization responses are unchanged.

### Exit after a command

2026.10.1 let the process end when its event loop was empty. 2026.11.1 calls `exitAfterCommand()` after the command and the disposal of the service container (`bw.ts:42`):

```ts
process.stdout.write("", () => {
  process.stderr.write("", () => process.exit());
});
```

The callback of an empty write runs once every earlier write to the stream has been handed to the system, so standard output and then standard error are flushed before the exit. The upstream comment names the case: standard output to a pipe is asynchronous on macOS, so an immediate exit could cut off a large `list` output. Without an argument `process.exit` uses `process.exitCode`, which `processResponse` sets to 0, or to 1 or 0 under `BW_CLEANEXIT` (`base-program.ts:33`, unchanged). Commands that call `process.exit` directly do so as before.

Storage writes are not cut off: the CLI stores its state with lowdb's `FileSync` adapter, which writes synchronously (`lowdb-storage.service.ts`).

The client reads standard output until end of file and then waits for the exit code. Neither changes. The flush matters only where a write to a pipe completes asynchronously, which the upstream comment names for macOS.

### Export event

The export command now awaits `eventCollectionService.collect(eventType, null, true, organizationId)` inside its `try` before it writes the document (`export.command.ts:87`). In 2026.10.1 the call was not awaited. `collect` uploads at once, and `uploadEvents` catches a failed upload, logs it and puts the events back into state (`libs/common/src/dirt/event-logs/services/event-upload.service.ts`, unchanged). So a failed upload does not fail the export. The document is now written after the upload has finished.

### Attachment download

`get attachment` runs `getAttachment` (`get.command.ts:348`), which is unchanged: premium check, `apiService.getAttachmentData`, then `DownloadCommand.saveAttachmentToFile` with `cipherService.getDecryptedAttachmentBuffer`. That method is the same in both versions and decrypts with `cipherEncryptionService.decryptAttachmentContent` (SDK).

In 2026.10.1 the flag `pm-28192-cipher-attachment-ops-to-sdk` was read in `saveAttachmentRawWithServer` (upload), `deleteAttachmentWithServer` and `upgradeOldCipherAttachments`, and in the web download component, and never on the path of `get attachment`. 2026.11.1 removed the flag and always takes the SDK branch of those three methods. The download path, the premium sentence and the decryption call are therefore the same before and after the removal. The failure texts of a download (`saveAttachmentToFile`, `download.command.ts:41`) are unchanged.

The SDK branch is now always taken for `create attachment`, `create item` and `edit item` as well. The client never runs these, but the seed of the Vaultwarden suite does. Whether Vaultwarden 1.37.4 serves the endpoints of the SDK branch is shown only by a run.

### Feature flags of Vaultwarden 1.37.4

`GET /api/config` returns `featureStates` built from the setting `experimental_client_feature_flags`, parsed with `FeatureFlagFilter::ValidOnly`, plus `pm-19148-innovation-archive` set to true (`src/api/core/mod.rs`, function `config`). `ValidOnly` keeps only the flags in `SUPPORTED_FEATURE_FLAGS` (`src/config.rs`): `pm-5594-safari-account-switching`, `pm-32413-multi-client-password-management`, `undetermined-cipher-scenario-logic`, `enable-basic-auth-response`, `ssh-agent-v2`, `windows-native-credential-sync`, `pm-34171-card-scanner`, `pm-30529-webauthn-related-origins` and `pm-32009-new-item-types`. Neither `pm-28192-cipher-attachment-ops-to-sdk` nor `pm-27632-cipher-crud-operations-to-sdk` can be set, and the client default of both was false in 2026.10.1. So with Vaultwarden 1.37.4, `bw` 2026.10.1 downloaded, created and decrypted on the old paths.

## Export keys

Each place of design D4 with the keys that the export model classes and the document types write. The model classes are `libs/common/src/models/export/*.export.ts`; the document types are `libs/tools/export-vault-core/src/types/bitwarden-json-export-types.ts` and the `jsonDoc` literals of `individual-vault-export.service.ts` and `org-vault-export.service.ts`. The lists were taken from the property declarations of each class, and no class assigns a property it does not declare. The two versions differ in `cipher.export.ts` and `ssh-key.export.ts` only in `toView`, which imports and writes no document.

| Place | 2026.10.1 | 2026.11.1 |
|---|---|---|
| Personal plaintext export, document | `encrypted`, `folders`, `items` | same |
| Personal encrypted export, document | `encrypted`, `encKeyValidation_DO_NOT_EDIT`, `folders`, `items` | same |
| Organization plaintext export, document | `encrypted`, `collections`, `items` | same |
| Organization encrypted export, document | `encrypted`, `encKeyValidation_DO_NOT_EDIT`, `collections`, `items` | same |
| Element of `folders` (`FolderWithIdExport`) | `id`, `name` | same |
| Element of `collections` (`CollectionWithIdExport`) | `id`, `organizationId`, `name`, `externalId` | same |
| Item (`CipherWithIdExport`) | `id`, `collectionIds`, `type`, `folderId`, `organizationId`, `name`, `notes`, `favorite`, `fields`, `login`, `secureNote`, `card`, `identity`, `sshKey`, `bankAccount`, `driversLicense`, `passport`, `reprompt`, `passwordHistory`, `revisionDate`, `creationDate`, `deletedDate`, `archivedDate`, `key`, `data` | same |
| `login` (`LoginExport`) | `uris`, `username`, `password`, `totp`, `fido2Credentials` | same |
| `secureNote` | `type` | same |
| `card` | `cardholderName`, `brand`, `number`, `expMonth`, `expYear`, `code` | same |
| `identity` | `title`, `firstName`, `middleName`, `lastName`, `address1`, `address2`, `address3`, `city`, `state`, `postalCode`, `country`, `company`, `email`, `phone`, `ssn`, `username`, `passportNumber`, `licenseNumber` | same |
| `sshKey` | `privateKey`, `publicKey`, `keyFingerprint` | same |
| `bankAccount` | `bankName`, `nameOnAccount`, `accountType`, `accountNumber`, `routingNumber`, `branchNumber`, `pin`, `swiftCode`, `iban`, `bankContactPhone` | same |
| `driversLicense` | `firstName`, `middleName`, `lastName`, `dateOfBirth`, `licenseNumber`, `issuingCountry`, `issuingState`, `issueDate`, `expirationDate`, `issuingAuthority`, `licenseClass` | same |
| `passport` | `surname`, `givenName`, `dateOfBirth`, `sex`, `birthPlace`, `nationality`, `issuingCountry`, `passportNumber`, `passportType`, `nationalIdentificationNumber`, `issuingAuthority`, `issueDate`, `expirationDate` | same |
| Element of `login.uris` (`LoginUriExport`) | `uri`, `uriChecksum`, `match` | same |
| Element of `login.fido2Credentials` (`Fido2CredentialExport`) | `credentialId`, `keyType`, `keyAlgorithm`, `keyCurve`, `keyValue`, `rpId`, `userHandle`, `userName`, `counter`, `rpName`, `userDisplayName`, `discoverable`, `creationDate` | same |
| Element of `fields` (`FieldExport`) | `name`, `value`, `type`, `linkedId` | same |
| Element of `passwordHistory` (`PasswordHistoryExport`) | `password`, `lastUsedDate` | same |

Added in 2026.11.1: none. Removed in 2026.11.1: none.

The item list is one set for both forms, as design D4 says. The plaintext exports delete `key` from each item before writing (`individual-vault-export.service.ts:356`, `org-vault-export.service.ts:408`), and `data` is written only for a blob item of the encrypted export (`cipher.export.ts`, `buildContent`).

The login domain and view gained `httpAuthenticationEnabled` (`libs/common/src/vault/models/domain/login.ts:21`), which the server sends and the SDK carries. `LoginExport` does not write it, so it is in no export. The item list (`CipherResponse`, `LoginResponse`) is unchanged as well.

## Scope

- The findings hold for these two release trees and the one installed bundle. A later `bw` version can change any of them.
- The commercial single-file builds were not read. They are built from the same tree with the Bitwarden-licensed additions, and their zips are renamed to `bw-bit-<os>[-arm64]-<version>.zip`.
- The Vaultwarden findings come from reading two source files at the tag, not from a running server.

## Not tested

- Any authenticated run of 2026.11.1. The Vaultwarden suite of the change covers it.
- What the SDK of 0.2.0-main.1214 logs for a field that fails to decrypt (row 4).
- The per-cipher decryption on Bitwarden Cloud, where 2026.10.1 may have used the SDK bulk call.
- Gated items, which neither Vaultwarden nor a test account of this project can produce.
- The exit after a command on macOS and Windows, where standard output to a pipe is asynchronous.
