# Proposal

## Why

A backup today covers the personal vault only. Items of organizations end up in `documents/items.json` in plaintext, but without attachments, without a second document to check them against and without any check, and the report calls them excluded. For an owner or admin, the vault they care about is often the organization's. The [product requirements](../../../docs/product/requirements.md) name organizations as part of the expansion goal. The [roadmap](../../../docs/development/architecture.md#roadmap) lists them after configuration and multiple accounts, but the maintainer moved them ahead in the explore session of 21 November 2026.

## What Changes

- **Organization export from an admin's view** (maintainer decision, 21 November 2026). A run can back up chosen organizations through `bw export --organizationid`. The run uses both the plaintext and the encrypted form. Unlike the personal exports, each of these calls fetches the organization's items from the server (`GET /organizations/{id}/export`) and does not read the local snapshot. The alternative was to back up only the organization items that the account sees locally. It was rejected because it can never reach a second, independent document, and because it is not what an owner means by a backup of the organization.
- **Explicit selection.** A new repeatable flag `--organization <id>` names each organization to back up. The ID must have the UUID form. A run without the flag behaves as today. Organizations that are not chosen produce no warning (maintainer decision, 21 November 2026). The existing warning that counts organization items per organization ID is removed.
- **Role check before the documents.** After the sync and the freshness check, the run reads `bw list organizations`. Every chosen organization must be there with a confirmed membership and the role owner or admin. Otherwise the run ends with no generation before any document is requested, and the report names the organization ID and the reason. The run makes this check itself because the organization export of an account without that role made Vaultwarden answer 401, and `bw` then logged itself out. Every later command failed as not logged in, so the report would have shown a lost session instead of the missing permission (spike of 21 November 2026).
- **Two documents per organization**, collected after the four personal documents and stored byte for byte in the archive under `documents/organizations/<org-id>/`. A failure of either export ends the run without a generation, like a failure of any other document.
- **Attachments of chosen organizations** are downloaded from the item lists, like those of personal items. Attachments of organizations that are not chosen are still not requested.
- **Verification of each chosen organization** with the checks of the personal exports. The plaintext and the encrypted organization export must hold the same item IDs, the same passkey counts and well-formed passkeys. Each item must belong to the organization. The two exports are separate server reads, and so are the item lists, so each item must also carry the same `revisionDate` in both exports and in the item lists. A difference is a failed finding that says the organization changed during the run, and the user can run again (maintainer decision, 21 November 2026: no automatic retry). An exported item that the item lists do not show is unverified, because its attachments are unknown (maintainer decision, 21 November 2026).
- **One verdict for the whole capture** (maintainer decision, 21 November 2026). The worst finding decides, as today. Findings about an organization carry its ID. The scope statement names the chosen organizations, and a new standing limit says that their exports are separate server reads.
- **Archive layout.** The archive gains two documents per chosen organization, and the manifest lists the chosen organizations with ID, name and item count. The format version stays 1 and is redefined a second time before any release, because the tool is not in use yet and no backward compatibility is needed (maintainer decision, 21 November 2026). Archives of the earlier layout, whose manifest has no organization list, are refused.
- The research note of the spike of 21 November 2026 records how organization roles, exports and attachments behaved on Vaultwarden 1.37.4 with `bw` 2026.10.1.

Not part of this change:

- Bitwarden Cloud, whose behavior is unknown. On the Cloud, whether an admin sees items of collections they are not assigned to depends on an organization setting.
- Custom roles with an export permission. These accounts are refused, because no evidence shows how `bw list organizations` reports them.
- The managed-collections export, which the CLI does not offer.
- Organization items that `documents/items.json` holds for organizations that are not chosen. They stay there, because the item list is stored as `bw` wrote it.
- A configuration file. The selection is a flag until the configuration format exists.

## Capabilities

### New Capabilities

None. Organization export extends the existing capture, verification, archive and run capabilities, and a separate capability would split requirements that must change together.

### Modified Capabilities

- `capture-collection`:
  - the role check of the chosen organizations,
  - two organization documents per organization,
  - attachments of chosen organizations,
  - decryption error counts of the new documents.
- `capture-verification`:
  - the scope statement and the new standing limit,
  - the checks of the organization exports, their organization and their revisions,
  - attachment accounting for chosen organizations,
  - the removal of the warning about organization items,
  - organization IDs in findings,
  - comparing the reference inventory across all exports in scope.
- `backup-run`:
  - the `--organization` flag,
  - the point up to which the target stays untouched,
  - the new documents in memory,
  - setup and failure lines of the report.
- `backup-archive`:
  - the organization documents,
  - the organization ID in entry names,
  - the manifest section for organizations.
- `offline-verification`:
  - organization entries and their counts,
  - the required organization list, which refuses archives of the earlier layout,
  - the output per organization.
- `bw-invocation`:
  - the commands `list organizations` and `export --organizationid`,
  - the check of the organization ID before it becomes an argument.

## Impact

**Code.**

- `internal/bwcli`: three new commands, the organization ID check, and the fake `bw` of `bwclitest`.
- `internal/collect`: the role check, the organization documents and the attachment plan.
- `internal/capture`: organization inputs, and `revisionDate` in the parsed exports and item lists.
- `internal/verify`: the organization checks, new reason codes, a new limit and a dynamic scope.
- `internal/manifest`: the organization section, required under format version 1.
- `internal/archive`: the new entry names and `RECOVERY.txt`.
- `internal/offline`: the organization entries and the counts per organization.
- `internal/run`, `internal/report` and `cmd/bw-takeout`: the flag, the phase order and the report lines.
- `internal/e2e`: an organization scenario against Vaultwarden.

**Requirements affected.**

- Modified as listed above.
- `generation-publishing` and `bw-profile` do not change.
- "Three-state verdict" and "Exit status of a backup" keep their text and now also apply to organization findings.

**Documentation.**

- User guide: `usage.md`, `configuration.md`, `limitations.md`, `recovery.md` and `security.md`. The new content covers the flag, the role, the 401 logout, the export event in the organization's event log, the Cloud gap, the archive layout and the recovery of organization documents.
- Development: `architecture.md`, with the roadmap and components, and `testing.md`, with the new tests and the Vaultwarden scenario.
- Research: a new research note and a row in `research-summary.md`.

**ADRs.**

- [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md) (proposed) is updated in place: the scope claim grows by the chosen organizations, and their findings join the single verdict.
- [ADR 0005](../../../docs/decisions/0005-archive-format.md) (proposed) is updated in place: the layout, and format version 1 redefined a second time before any release. Its rule that every layout change needs a new format version from the first release on stays.
- A new ADR 0008, proposed, records the organization backup through the organization export with an owner or admin role. The decision is hard to reverse, because users' flags and archives depend on it. It also moves an integration boundary: the run now relies on a server-side export and on a role, not only on the local snapshot.
- [ADR 0006](../../../docs/decisions/0006-secret-boundary.md) is checked: the new commands keep the session key in the environment and copy no text of `bw`.

**Open evidence.**

- The behavior with organizations was observed only on Vaultwarden 1.37.4 with `bw` 2026.10.1 on Linux arm64, in the spike of 21 November 2026. The spike observed:
  - An admin assigned to one of two collections saw both locally and downloaded both attachments.
  - Owner and admin could export, and both exports agreed in IDs and `revisionDate`.
  - Both exports left out an item in the trash.
  - A user and a manager got 401, and `bw` then logged out.
- The spike scripts were lost in a scratchpad cleanup. Its results are those recorded in the explore session. The Vaultwarden scenario of this change rebuilds the setup.
- Not observed:
  - Bitwarden Cloud,
  - custom roles,
  - organization items in blob format or with passkeys,
  - an organization export with decryption error lines,
  - an organization that changes during a run.
- Code reading of `bw` 2026.10.1 shows:
  - the CLI never sets `onlyManagedCollections`,
  - exports carry no attachment inventory,
  - the client checks no export permission itself.

Evidence level of the new tests: mock against the fake `bw`, and automated end-to-end against Vaultwarden for the organization scenario.
