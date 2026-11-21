# Design

## Context

See [proposal.md](proposal.md) for the motivation and the maintainer decisions of 21 November 2026. The requirements are in the spec deltas under [specs/](specs/).

The current state:

- **`bwcli.Client`** has one method per `bw` command, and every document command goes through `runDocument`. After a non-zero exit of a vault command, the client queries the status and turns the failure into `not_logged_in` or `locked` where that applies. IDs pass through `vaultid` before they become arguments (design D7 of `bw-process-adapter`).
- **`collect.Collector.Snapshot`** runs `sync --force`, `status`, the four documents and the attachment plan (`capture.PersonalAttachments`) before the run touches the target. `Collector.Attachments` then streams the downloads into the archive writer.
- **`capture`** parses each document into IDs, organization IDs, passkeys and attachment IDs. `ParsePlaintextExport` and `ParseEncryptedExport` read the top-level keys `encrypted`, `passwordProtected`, `encKeyValidation_DO_NOT_EDIT`, `items` and `data`. They ignore `folders` and `collections`. No parser keeps `revisionDate`.
- **`verify`** checks the personal exports against each other and against the combined item lists. Organization items are excluded and counted in the warning `organization_items_excluded`. Its findings carry `organization` counts only for that warning.
- **`archive`** writes the four documents, then the attachments, then the manifest in format version 1. `kindOf` knows the fixed document names. The reader compares the item count of the manifest with the plaintext export (`compareItems`).
- **`run`** orders checks, profile, version, server, login, snapshot, begin, archive, attachments, verification and commit. **`report`** prints the setup lines, the progress, the verdict and the ending.

Evidence for the behavior with organizations. The spike ran on Vaultwarden 1.37.4 linux/arm64 with `bw` 2026.10.1 (npm, `build/bw.js` SHA-256 `8a443d11…51ee9b`) during the explore session of 21 November 2026. The code reading used the same bundle.

| Statement | Source | Level |
|---|---|---|
| `bw export --organizationid` in both formats calls `GET /organizations/{id}/export` on every call and does not read the local snapshot | Bundle lines 58904, 58946, 59794 | Code reading |
| The CLI calls `getOrganizationExport` without `onlyManagedCollections`, so the whole organization is exported | Bundle lines 58643, 59868 | Code reading |
| Both organization exports leave out items with a `deletedDate` | Bundle lines 58918, 58958 | Code reading, and observed for one item in the trash |
| Export items carry `revisionDate` and `collectionIds`, but no attachments | Bundle `CipherExport.build`, line 56104 ff. | Code reading |
| The plaintext organization export has the keys `encrypted: false`, `collections` and `items`. The encrypted one has `encrypted: true`, `encKeyValidation_DO_NOT_EDIT`, `collections` and `items`, with sealed values and an item key per item | `BuildEncryptedExport`, line 59016 ff. | Code reading, and observed |
| The personal exports drop every item with an `organizationId` while building the document | Bundle line 59358 and `getEncryptedExport` | Code reading |
| `ExportCommand` checks no permission. Only the server decides | Bundle line 59830 ff. | Code reading |
| An admin with `accessAll: false`, assigned to collection C1 only, saw the items of C1 and C2 in `bw list items` and downloaded both attachments with matching SHA-256 | Spike | Observed, Vaultwarden only |
| For owner and admin, both organization exports succeeded and held the same IDs with the same `revisionDate`, which also matched `bw list items` | Spike | Observed |
| For a user and a manager, `bw export --organizationid` got 401 from Vaultwarden, `bw` retried and then logged out. Every later command printed "You are not logged in.", and `bw status` reported `unauthenticated` | Spike, run twice | Observed |
| `bw list organizations` reported the roles owner 0, admin 1 and user 2, and a manager invited with type 3 as type 4. A confirmed membership has status 2 | Spike | Observed |

The spike scripts were lost in a scratchpad cleanup. The organization setup that worked is recorded in D12, so the Vaultwarden scenario can rebuild it.

## Goals / Non-Goals

**Goals:**

- Chosen organizations get the same quality of checks as the personal vault where the documents allow it, and a clear failure where they do not.
- A run without `--organization` behaves exactly as before, apart from the removed warning and the empty organization list in the manifest.
- Every new `bw` command keeps the secret boundary of ADR 0006: the session key only in the environment, and no text of `bw` in any output.

**Non-Goals:**

- Retrying a capture whose organization changed (maintainer decision A1, 21 November 2026).
- One verdict per scope (maintainer decision B1, 21 November 2026).
- Reading organization names or roles from anywhere other than `bw list organizations`.
- Groups, collections as a unit of selection, and the managed-collections export.

## Decisions

### D1. The selection is a list of organization IDs, normalized to lowercase

`--organization` is a repeatable flag (a `flag.Value` that appends). Each value must pass the UUID rule of `vaultid`. A new function `vaultid.ValidOrganizationID` holds the same rule as item IDs. The value is lowercased, so that duplicates and the comparison with `bw list organizations` do not depend on case. `bw` prints lowercase IDs. A duplicate after lowercasing is a usage error. The order of the flags is kept everywhere: in the role check, the export order, the archive and the manifest.

`run.Config` gains `Organizations []string`. `collect` receives the same list.

*Alternative: names instead of IDs.* Rejected. Names are not unique, and choosing by name would make the scope depend on a value an admin can change.

*Alternative: silently drop a duplicate.* Rejected. A duplicate is almost always a typo for another organization.

### D2. The role check lives in the snapshot, after the freshness check

`Collector.Snapshot` gains the chosen IDs. After the accepted status and before the plaintext export, it calls a new `Vault.ListOrganizations`. For each chosen ID in order, the entry must exist, have status 2 (confirmed) and type 0 (owner) or 1 (admin). The first ID that fails ends the snapshot with a `*collect.OrganizationError{ID, Reason}`. Its reason is `not_member`, `not_confirmed` or `role`, and its sentinel is `collect.ErrOrganization`. No document has been requested yet, so nothing needs to be wiped.

The check is in `collect` because it is a rule about a capture, like the freshness rule. It cannot run before the login, because the list needs a session. A wrong ID therefore costs one login, unlike the input checks of the run.

*Alternative: let the export fail.* Rejected. On Vaultwarden the failure logs `bw` out (see Context). The run would then report `not_logged_in` for the export, which hides the cause.

*Alternative: also accept a custom role (type 4).* Rejected for now. The Bitwarden clients give a custom role an export permission in a separate `permissions` object. No evidence shows what `bw list organizations` prints for it, and on Vaultwarden a manager also appears as type 4 and cannot export. Accepting type 4 would let a manager reach the 401 path. The limitation is documented, and a later change can loosen the rule with evidence from the Cloud.

A role that changes between the check and the export still reaches the 401 path. The run then fails with the operation of the organization export, the class `not_logged_in` and the organization ID (D10). This is rare and visible.

### D3. Order of the commands

```
sync --force
status                                  freshness check (unchanged)
list organizations                      only with a chosen organization (D2)
export json | export encrypted_json      personal (unchanged)
list items | list items --archived       (unchanged)
for each chosen organization, in order:
  export --organizationid <id> --format json
  export --organizationid <id> --format encrypted_json
-- target touched: partial generation, archive writer --
get attachment ...                      personal and chosen organizations (D6)
```

The organization exports come after the item lists, so every revision check compares an earlier snapshot with a later server read. With the D5 comparison of the item lists against the plaintext organization export, this catches a change that happened after the attachment plan was made. All organization documents are collected before the target is touched, so a failed or refused export never leaves a partial directory. This is the rule of "The target is untouched until the snapshot is complete".

*Alternative: organization exports first.* Rejected. A change between export and snapshot would then also be caught, but the item lists that drive the downloads would be newer than the export they are checked against. That gives no benefit and differs from the personal order.

### D4. Capture model: one input per organization, raw revision dates

- `capture.Input` gains `Organizations []OrganizationInput{ID, PlaintextExport, EncryptedExport, DecryptionErrors OrganizationDecryptionErrors{PlaintextExport, EncryptedExport}}`.
- `capture.Capture` gains `Organizations []OrganizationCapture{ID, Plaintext *PlaintextExport, Encrypted *EncryptedExport}`.
- The two organization documents are parsed with the existing `ParsePlaintextExport` and `ParseEncryptedExport`. Their top-level form is the same as that of the personal exports, and `collections` takes the place of `folders`, which neither parser reads. A rejected organization document is a `*DocumentError` with the new documents `organization_plaintext_export` or `organization_encrypted_export` and the new field `OrganizationID`.
- `PlaintextItem`, `EncryptedItem` and `ListedItem` gain `RevisionDate string`, the raw value or empty. The parsers do not require it, so personal documents are not judged more strictly than before. The verification parses it when it compares (D5).

*Alternative: separate organization parsers.* Rejected. They would duplicate the format recognition of ADR 0004 and could drift from it.

### D5. Verification per organization inside the one verdict

`Verify` runs the existing personal checks unchanged, except for the organization rules, and then one pass per chosen organization:

1. **Document form.** A rejected or absent organization document produces a failed finding with `Document` and `OrganizationID`, and its organization is skipped from the steps below.
2. **Identity, formats, passkeys.** The existing functions are factored to take a pair of parsed exports, and the organization pass calls them. The personal pass calls them as before. Each finding gets `OrganizationID`.
3. **Organization membership.** An item of either export whose `organizationId` is not the chosen ID produces the failed reason `organization_item_mismatch`. The same reason applies when the item lists show the item with another organization ID or as personal.
4. **Revisions between the exports.** For an ID in both exports, both `revisionDate` values are parsed as RFC 3339 and compared at millisecond precision. A difference produces the failed reason `organization_changed_during_capture`. A missing or unparsable value on an item of an organization export makes that document `document_malformed_item`, because every item of these exports carries one.
5. **Matching with the item lists.** The listed items of the chosen organization, from both lists counted once, are compared with its plaintext export. A listed item missing from the export gives `listed_item_missing_from_export`, failed. An exported item that neither list shows gives `exported_item_not_listed`, unverified. An item in both whose revision dates differ gives `organization_changed_during_capture`. A listed item without a parsable revision date on a chosen organization makes that list `document_malformed_item`.
6. **Attachments.** The expected downloads are the attachments of personal items and of items of chosen organizations (D6). Accounting and its reasons do not change.

Items of organizations that are not chosen are skipped by every comparison with the exports and by the attachment accounting, without a finding. They still count in the two checks of the item lists as documents, the agreement of an item that both lists show and the archive filter warning, because both lists come from one snapshot and a disagreement there is evidence about the lists themselves (review of 21 November 2026, finding C2, maintainer decision). The reason `organization_items_excluded` and the type `OrganizationCount` are removed. `attachment_download_out_of_scope` stays for downloads of organizations that are not chosen. `item_organization_mismatch` stays for personal items.

`Finding` gains `organization_id` (omitempty). `lessFinding` sorts by organization ID first, so that a result stays deterministic.

**Scope and limits.** `Scope` gains `Organizations []string` (omitempty), in the order chosen. Without a chosen organization, the code and the statement stay as they are. With one, the code is `personal_and_chosen_organization_items` and the statement names both parts. The result then also carries the new limit `organization_exports_separate_server_reads`. `LimitServerCompleteness` and the other limits apply as before. The verdict is still computed over all findings (decision B1).

**Reference inventory.** `reference` compares against the union of the personal plaintext items and the plaintext items of the chosen organizations. Item IDs are unique across a vault, so the union needs no organization key.

*Alternative: a second `Result` per organization.* Rejected by decision B1. It would change the generation name, the exit status and ADR 0004.

### D6. The attachment plan covers the chosen organizations

`capture.PersonalAttachments` becomes `capture.ScopedAttachments(chosen []string, lists ...*ItemList)`. It returns the ID pairs of personal items and of items of chosen organizations, in list order, each pair once. `collect.plan` and the verification both call it. The verification thus expects exactly the downloads the collection requested, as design D6 of `capture-collection` requires. The download path itself, with its retries and failure classes, does not change. Organization attachments need no personal premium according to the code (research summary, `bitwarden-cli-analysis`). This was not observed separately, because Vaultwarden reports premium for every user.

### D7. Archive layout, format version 1 redefined

```text
RECOVERY.txt
documents/export.json
documents/export.encrypted.json
documents/items.json
documents/items.archived.json
documents/organizations/<org-id>/export.json            (per chosen org)
documents/organizations/<org-id>/export.encrypted.json  (per chosen org)
attachments/<item-id>/<attachment-id>
manifest.json
```

- `archive.Documents` gains `Organizations []OrganizationDocuments{ID, PlaintextExport, EncryptedExport}`. `NewWriter` refuses an organization ID that fails `vaultid.ValidOrganizationID`, a duplicate ID and an empty document. It writes the organization documents after the four, in the order given. It counts the items of each plaintext organization export as it counts the personal ones.
- `archive.Meta` gains the names. The manifest gains the required key `organizations`, an array of `{id, name, items}` placed after `counts`, empty without a chosen organization. `counts.items` stays the count of the personal plaintext export.
- `kindOf` accepts `documents/organizations/<uuid>/export.json` and `.../export.encrypted.json` as documents and returns the organization ID.
- The format version stays 1. `manifest.Parse` requires the key `organizations` and refuses a manifest without it as invalid. The reader refuses an organization entry whose ID the manifest does not list. It also requires both documents for every listed organization and compares each organization's `items` with its plaintext export, in the same way as `compareItems`.
- `RECOVERY.txt` explains the organization documents. The plaintext organization export can be read directly. The encrypted one is sealed with the organization key and can only be opened by importing it into that organization with a member's account.

**Why version 1 stays.** The tool is not in use yet, and backward compatibility is not needed (maintainer decision, 21 November 2026). A second version would oblige the reader to keep the earlier layout readable for archives that nobody relies on. This is the reasoning of design D5 of `archived-item-list`, which redefined version 1 the first time. The generations written so far come from exercises with synthetic accounts. Their manifest has no organization list, so the new reader refuses them. The `organizations` key is required rather than read as empty when absent, for the same reason: an absent key would be a compatibility rule for those archives. The rule of ADR 0005, that every layout change needs a new format version from the first release on, does not change.

*Alternative: format version 2 with a reader for version 1.* Rejected by the maintainer decision. It adds a reader path for archives nobody relies on.

*Alternative: the organization name in the entry path.* Rejected. Names are vault-adjacent data and can hold any character. Entry names are built from validated IDs only (`backup-archive`).

### D8. Organization names go into the manifest only

The name comes from `bw list organizations`, the only source of names for the chosen organizations. It helps a user years later to tell which organization a directory holds. It is stored only in the encrypted manifest, like attachment file names. The report, the verification result and the output of `bw-takeout verify` never print it. `collect.Snapshot` carries the names as strings in its source, which `Wipe` cannot clear. This is the same as the file names in the plan.

### D9. `bwcli`: three commands and the organization list form

| Op | Arguments | Time limit | Result |
|---|---|---|---|
| `list_organizations` | `list organizations --raw` | Long | `[]Organization{ID, Name, Type, Status}` |
| `export_organization_json` | `export --organizationid <id> --format json --raw` | Long | `Output` |
| `export_organization_encrypted_json` | `export --organizationid <id> --format encrypted_json --raw` | Long | `Output` |

- All three are vault commands with the session. A non-zero exit is therefore followed by the status query, which yields `not_logged_in` after the 401 logout.
- The organization ID is checked with `vaultid.ValidOrganizationID` before the command is built. It is kept in `bwcli.Error` as `OrganizationID`, just as the download keeps its IDs.
- `ListOrganizations` decodes the output in memory. The output must be a JSON array, and every entry must have an `id` that passes the UUID rule and numeric `type` and `status`. Anything else is the class `output`. `name` is optional and passed on as it is. The raw output is wiped after decoding.
- The fake `bw` of `bwclitest` gains fixed answers for these argument lists. A test hook lets one answer log the fake out, so that later commands see an unauthenticated status. This models the observed 401 behavior.

### D10. Run and report

- `run.Config.Organizations` passes through to `collect`. Phases do not change: the role check and the organization exports belong to `PhaseSnapshot`.
- `Snapshot.Wipe` clears the organization documents too.
- `report.Setup` gains the chosen IDs. A line `Organizations: <id>, <id>` follows the recipients line, and it is omitted when none is chosen, so the golden report of a run without organizations keeps its setup lines.
- A `collect.OrganizationError` ends as `Failed: snapshot: organization <id>: <reason in words>`. A `bwcli.Error` with an organization ID prints `organization=<id>` with the operation and class.
- A finding line prints `organization=<id>` when it has one. The reason `organization_changed_during_capture` gets the hint "run the backup again".
- Values pass through `printable` as before. The exit status rules do not change.

### D11. Documentation and ADRs

- **ADR 0008, proposed:** "Organization backup through the organization export". The context is the two ways explored on 21 November 2026. The decision is the organization export, an explicit selection and the owner or admin role. The alternatives are the local view only and automatic selection. The consequences are the separate server reads, one login per role mistake, the export event in the organization's event log, and that the Cloud is unknown.
- **ADR 0004** is updated in place with the extended scope, the new limit and the organization findings inside the one verdict.
- **ADR 0005** is updated in place with the layout of D7 and the second redefinition of format version 1 before any release, next to the first one in its consequences.
- **User guide:**
  - `usage.md`: the flag and the role.
  - `configuration.md`: the flag as configuration.
  - `limitations.md`: replaces "Organization items are not checked" with a section on organizations. It covers the separate reads, the revision rule, the 401 logout, custom roles, the Cloud gap, the items of organizations that are not chosen in `items.json`, and the admin's local view on Vaultwarden.
  - `recovery.md`: the organization documents.
  - `security.md`: organization data in the archive, the role the backing-up account needs, and the export event in the organization's event log (`Organization_ClientExportedVault`, recorded by `ExportCommand`).
- **Development:**
  - `architecture.md`: the components and the roadmap item.
  - `testing.md`: the new tests and the Vaultwarden scenario.
- **Research:** a note on the spike of 21 November 2026, with the code reading of the organization export, and a row in `research-summary.md`.

### D12. Vaultwarden scenario: organization setup over the API

The suite in `internal/e2e` gains a scenario that rebuilds the spike. These calls worked against Vaultwarden 1.37.4 on 21 November 2026:

1. Register an owner, an admin and a user account as today.
2. As the owner, call `POST /api/organizations` with `name`, `billingEmail`, `collectionName`, `key`, `keys{publicKey, encryptedPrivateKey}` and `planType: 0`:
   - `collectionName` is an EncString of type 2 under a new 64-byte organization key.
   - `key` is that organization key wrapped for the owner's public key as an EncString of type 4: RSA-OAEP with SHA-1, `4.<base64>`.
   - `keys` is a new RSA key pair of the organization. Its private key is PKCS#8 under the organization key.
3. Find the default collection C1 with `GET /api/organizations/{id}/collections`. Create C2 with `POST /api/organizations/{id}/collections` and the body `{name, groups: [], users: []}`.
4. Invite each member with `POST /api/organizations/{id}/users/invite`:
   - the body is `{emails, groups: [], type, accessAll: false, permissions: {}, collections: [{id: C1, readOnly: false, hidePasswords: false, manage: false}]}`, with type 1 for admin and 2 for user;
   - without SMTP, an existing account becomes status 1 (accepted).
5. Read the members with `GET /api/organizations/{id}/users` and confirm each with `POST /api/organizations/{id}/users/{memberId}/confirm` and `{key}`. `key` is the organization key wrapped for that member's public key.
6. As the owner, through `bw`:
   - create one login item in C1 and one in C2, each with `organizationId` and `collectionIds`, and an attachment of random bytes;
   - create a third item in C1 and delete it to the trash;
   - sync.

The scenario then runs the backup twice:

- **As the admin with `--organization`:** the generation must be verified, and it must hold both organization documents and both attachments with their SHA-256. The trash item must be in neither document.
- **As the user:** the run must end in the snapshot phase with the organization reason `role`, exit status 1 and an unchanged target. The user's `bw` must not have been asked for an organization export.

The reference inventory of the scenario includes both organization items.

*Alternative: a manual exercise in the web vault.* Rejected in the explore session: hard to repeat, and it cannot run in the workflow.

### D13. Tests below the end-to-end level

- **`capturetest`:** an intact fixture with one chosen organization, two items, one attachment and one passkey. Variants each break it in one place:
  - item missing from the organization plaintext export,
  - item of another organization in the export,
  - organization item listed as personal,
  - revision differs between the two exports,
  - revision differs between the list and the export,
  - listed organization item not exported,
  - exported organization item not listed,
  - blob login item in the organization encrypted export,
  - empty passkey key in the organization export,
  - password-protected organization export,
  - absent organization document,
  - decryption error lines on the organization plaintext export,
  - missing download of an organization attachment,
  - items of an organization that is not chosen, which must give no finding and no warning.
- **`collect` against the fake `bw`:**
  - each role outcome of D2,
  - the list failure,
  - the order of D3,
  - a failed organization export with the class `not_logged_in` after the logout hook,
  - the plan with chosen and other organizations.
- **`archive` and `offline`:**
  - round trip of the organization documents,
  - refusal of an invalid or duplicate organization ID,
  - a manifest without the organization list, which is refused,
  - an archive in the layout before this change, which is refused,
  - an entry of an organization the manifest does not list,
  - a missing organization document,
  - a wrong organization item count.
- **`run` and `report`:**
  - the flag errors,
  - the setup line,
  - the failure lines,
  - the finding line with the organization,
  - a sentinel in an organization name that appears nowhere in the output,
  - the empty temporary directory with an organization.

## Risks / Trade-offs

- [The behavior on Bitwarden Cloud is unknown: admin visibility depends on an organization setting, the denial may not be 401, and rate limits on the export are unknown] → The documentation claims Vaultwarden only. An admin who does not see an item locally gets an unverified finding, not a silent gap.
- [An organization that is edited during the run fails the capture, and a busy organization may fail repeatedly] → The finding names the item and suggests running again (D10). A retry is deferred by decision A1 and can be added without changing the specs of other checks.
- [A role that changes between the check and the export logs `bw` out] → The failure names the organization export, the class and the ID. No generation is written.
- [Owners with a custom role that has the export permission cannot use the feature] → Documented in the limitations. D2 names how to loosen the rule.
- [Organization exports are held in memory, twice per organization, within the output limit of 1 GiB per command] → Same bound as the personal exports. Very large organizations are a known limit of the in-memory design.
- [Each run records an export event in the organization's event log, visible to other admins] → Documented in `security.md`. It is the behavior of `bw` and cannot be avoided with its public commands.
- [Generations written before this change are refused by the new reader] → Accepted by the maintainer decision. They come from exercises with synthetic accounts, and the tool is not in use.
- [`items.json` still holds plaintext organization items of organizations that are not chosen] → They are inside the encryption, as today. The limitations page says so.
- [The spike scripts are lost] → D12 records the calls that worked. If an API detail differs, the scenario fails loudly at setup, not with a wrong result.

## Migration Plan

No migration of data, because the tool is not in use. Generations written before this change are refused by the new reader. A run without `--organization` writes the same content as before plus an empty organization list in the manifest. Rolling back the change makes the older build refuse archives with organization entries as unexpected entries.

## Open Questions

- What does `bw list organizations` print on Bitwarden Cloud for a custom role with the export permission? This decides only whether D2 can later accept such a role.
- Does a disabled organization (`enabled: false`) still export? Today an export failure ends the run either way, so the answer changes only the wording of the report.

## Review of 21 November 2026

An independent code review and an independent security review ran after the last task, each in a fresh context. Every finding was checked against the code and the files.

- **S1 = C1 (confirmed, fixed).** An untracked `bw-takeout` binary, a build of the dirty working tree, sat at the repository root, and `.gitignore` did not cover it. A `git add -A` would have published it. It was removed; `.gitignore` stays unchanged (maintainer decision). Review fix R1.
- **C2 (confirmed, spec clarified).** The delta of `capture-verification` said that items of organizations that are not chosen are excluded from all comparisons and produce no warning, while the code still counts them in the agreement of items that both lists show and in the archive filter warning, as before this change. The maintainer kept the behavior and the requirement "Organization items outside the scope" was narrowed to the comparisons with the exports, next to a new requirement "Item lists are checked as documents in every scope" with the scenario "Unchosen organization item in both lists" for `TestUnchosenOrganizationItemInBothLists`. See D5. Review fix R4.
- **C3 (confirmed, fixed).** Two sentences in `docs/development/testing.md` and `docs/research/research-summary.md` still called organization roles and live tests planned or untested. Review fix R2.
- **C4 (confirmed, fixed).** A test comment named the removed `capture.PersonalAttachments`. Review fix R3.

No finding was rejected. The security review found nothing beyond S1: it checked the ID validation before arguments, the strict organization list decoding and its wipe, the role check failing closed, the wipe of the organization documents, the archive entry names, the reader rules and the absence of organization names and `bw` text in every output.

A second round with a fresh code reviewer checked the review fixes and found three more prose findings, all confirmed and fixed. No security review ran in that round, because no fix touched code or documentation relevant to security.

- **C1 (confirmed, fixed).** The reworded sentence of R2 claimed an owner run against Vaultwarden, which D12 does not define. Review fix R5.
- **C2 (confirmed, fixed).** The paragraph above the `capture-verification` table did not name the requirement added by R4. Review fix R6.
- **C3 (confirmed, spec clarified).** A duplicate ID of an unchosen organization item within one list, and its differing attachments between the lists, still give failed findings, which the delta did not cover. The clarification of C2 was extended to the checks of the item lists as documents, with a scenario and two tests. No code change. Review fix R7.
