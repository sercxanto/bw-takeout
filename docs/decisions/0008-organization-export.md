# Organization Backup through the Organization Export

Status: proposed

## Context

A backup covered the personal vault only. Items of organizations ended up in `documents/items.json`, as `bw list items` wrote them, but without attachments, without a second document to check them against and without any check. For an owner or admin, the vault that matters is often the organization's.

Two ways were explored on 21 November 2026 ([spike](../research/organization-export-spike-2026-11-21.md), Vaultwarden 1.37.4 with `bw` 2026.10.1):

- **The local view.** The personal exports drop every item with an organization ID. What remains of an organization in the local snapshot is its items in `bw list items` and their attachments. No second document of the same items exists, so the checks of [ADR 0004](0004-completeness-verdict.md) cannot apply.
- **The organization export.** `bw export --organizationid <id>` in the plaintext and the encrypted form fetches the whole organization from the server (`GET /organizations/{id}/export`) on every call. It does not read the local snapshot, and the client checks no permission. Owner and admin could export, and both exports agreed with each other and with `bw list items` in IDs and revision dates. For a user and a manager, Vaultwarden answered 401, and `bw` then logged itself out, so every later command failed as not logged in.

On Vaultwarden, an admin assigned to one of two collections saw the items of both locally and downloaded both attachments. On Bitwarden Cloud this depends on an organization setting, and nothing was observed there.

## Decision

- **Organizations are backed up through the organization export**, in both forms, as two documents per organization in the archive ([ADR 0005](0005-archive-format.md)). The attachments of their items are downloaded from the item lists, as those of personal items.
- **The selection is explicit.** A repeatable flag `--organization <id>` names each organization by its ID. A run without it backs up the personal vault only, and organizations that are not chosen produce no warning.
- **The account must be owner or admin** of each chosen organization with a confirmed membership. The run reads `bw list organizations` after the sync and refuses any other role, a custom role included, before it requests any document. It does not let the export fail, because the 401 logout would report a lost session instead of the missing role.
- **The organizations join the one verdict** of the capture. Each chosen organization gets the checks of the personal exports between its two documents, and each item must belong to the organization and carry the same revision date in both exports and in the item lists. Findings carry the organization ID.

## Alternatives

- **The local view only.** It needs no role and no server read, and it was rejected because it can never reach a second, independent document, and because it is not what an owner means by a backup of the organization.
- **Automatic selection of every organization of the account.** It would back up organizations the user did not ask for, cost one or two server exports per organization and run, record export events in organizations the user may not be responsible for, and end every run of a user without the role. Rejected in favor of the explicit flag.
- **Letting the export decide the permission.** It would save one command, but on Vaultwarden the refused export logs `bw` out, and the report would name a lost session.
- **A verdict per organization.** It would change the generation name, the exit status and ADR 0004. Rejected in favor of one verdict per capture.

## Consequences

- **Separate server reads.** The organization exports are not part of the snapshot that the forced sync fixes, and the two exports are reads of their own. An organization that is edited during a run fails the capture, and the user runs again. A change that leaves every revision date as it was stays invisible. Every result with a chosen organization lists this limit.
- **One login per role mistake.** The role check needs a session, so a wrong organization ID or a missing role costs one login and, with two-step login, one code.
- **An export event per run.** `bw` records `Organization_ClientExportedVault` in the organization's event log for each export, visible to its other admins. It cannot be avoided with the public commands.
- **A changed role between check and export** still reaches the 401 path. The run then fails with the organization export, the class `not_logged_in` and the organization ID. This is rare and visible.
- **Custom roles with the export permission are refused** until evidence shows how `bw list organizations` reports them. On Vaultwarden a manager also appears as type 4 and cannot export.
- **Bitwarden Cloud is unknown.** Whether an admin sees items of collections the admin is not assigned to, whether the refusal is also a 401, and whether the export is rate-limited were not observed. The documentation claims Vaultwarden 1.37.4 only.
- **The integration boundary moves.** The run now relies on a server-side export and on a role, not only on the local snapshot. Users' flags and the archive layout depend on the decision, so changing it later needs a superseding ADR.

The rules in detail are in the design of the change organization-export.
