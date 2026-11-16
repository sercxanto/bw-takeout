# Proposal

## Why

`bw list items` leaves archived items out, while both exports keep them. The capture model, the verification and the archive know exactly one item list, so every vault with an archived item ends as unverified although nothing is missing, and the attachments of archived items are never known. The roadmap assigned this to `interactive-backup-run`. That change is too large, so this one takes the part that needs no process and no terminal, and it changes the archive layout while no real generation exists yet.

## What Changes

- A capture holds two item lists as separate documents: the default list and the list from `bw list items --archived`.
- The verification treats the items of both lists as one listing. All existing rules about listed items, attachments and organization items apply to it.
- An item that both lists show is accepted when both entries agree, and fails when they name different attachments or a different organization.
- A warning reports that the archive filter did not apply when the two lists overlap. `bw` ignores `--archived` without an error for an account that may not archive, and then returns the default list again.
- An exported item that neither list shows stays unverified, as today.
- **BREAKING** The archive holds a fourth document, `documents/items.archived.json`, stored byte for byte. It is mandatory. The format version stays 1 and is redefined, because no `backup` command exists and no generation was ever written outside the tests. Archives written by earlier test code are refused.
- The recovery instructions inside the archive describe the fourth document.
- The trash is recorded as a limitation. Both personal exports and the default list leave out deleted items. Capturing the trash is not planned for the prototype (maintainer decision, 16 November 2026).

Not part of this change: starting `bw`, collecting a capture, the `backup` command, and any check against `archivedDate` of the exports.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `capture-verification`: new requirements for combining the default and the archived item list and for reporting an archive filter that did not apply. The existing requirements keep their text and now read "the item list" as the combined listing.
- `backup-archive`: the requirement "Captured documents are stored unchanged" covers four documents.
- `offline-verification`: the requirement "Unsafe or unknown archive structure is rejected" also rejects an archive that lacks a document the format requires.

## Impact

**Code.** `internal/capture` (model, parser entry point, package documentation), `internal/capture/capturetest` (fixture and variants), `internal/verify` (combined listing, two reason codes), `internal/archive` (writer, reader, entry names, recovery text), `internal/archive/archivetest`, and the tests of `cmd/bw-takeout`. `internal/bwcli` already has `ListArchivedItems` and does not change.

**Documentation.** `docs/user/limitations.md` (trash, account that may not archive, the sentence on archived items), `docs/user/recovery.md` where it names the documents, `docs/development/testing.md` (fixtures, variants, scenario coverage), `docs/development/architecture.md` (component status and roadmap, see below), and a new research note for the code reading of 16 November 2026.

**ADRs.** [ADR 0005](../../../docs/decisions/0005-archive-format.md) (proposed) lists the layout and says "three captured documents". It is updated in place, because it is not accepted yet. [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md) (proposed) is checked for statements about the item list and updated where it names one list. No new ADR: the decision extends the format of ADR 0005 and adds no new integration boundary or security assumption.

**Open evidence.**

- That `bw` ignores `--archived` for an account that may not archive comes from reading the bundle of `bw` 2026.10.1 only. The live probe ran on Vaultwarden, where archiving was available. The case is tested with synthetic documents.
- That the exports and the default list leave out the trash comes from the same code reading. The live probe lists the trash as not examined.
- An archived item with an attachment was not observed live. The live probe archived a login without attachments.
- Evidence level of all tests in this change: mock.

## Split of `interactive-backup-run`

The explore session of 16 November 2026 cut roadmap item 4 into four changes. This change is A. The others are not created yet, because their open questions are not settled and B builds on the model of A. A task of this change moves this section into the roadmap.

| | Change | Content |
|---|---|---|
| A | `archived-item-list` | This change |
| B | `capture-collection` | Collect a capture through `bwcli`: sync, freshness rule, documents, attachments streamed one at a time into the archive writer, mapping of failure classes to download results, source for the manifest. Tested against the fake `bw` |
| C | `interactive-backup-run` | The `backup` command, order of phases, teardown and logout on abort, report and exit codes, user documentation. Tested against the fake `bw` and by hand against Vaultwarden |
| D | FAT and exFAT run | The run of `publish` on a FAT or exFAT target that was deferred from `archive-generation`. Independent of `bw` |

Constraints the session recorded for the later changes:

- **B, plaintext in memory.** Keep the number of plaintext copies small and their lifetime short: one attachment at a time, and nothing held longer than the step that needs it. `capture.Download` carries only hash and size, and the archive writer takes attachments one at a time, so no design change is needed for this.
- **B, before and after.** Question the planned comparison of the inventory before and after the capture. The profile belongs to the run, and after the one sync every command reads local state. If `export` and `list` start no sync, the freshness rule is enough. This has to be checked against the code of `bw`.
- **B, open questions.** Whether a stale `lastSync` aborts the run or lowers the verdict, what happens with the count of decryption error lines that `bwcli` reports and `verify` cannot receive, and how the `bwcli` failure classes map to the three download error classes. A premium failure stays a failed download.
- **C, memory hardening.** Disable core dumps for the run. On Unix the limit is inherited by the `bw` processes. The way on Windows is open. Document swap, hibernation and core dumps as residual risk in `docs/user/security.md`, with the advice to use encrypted swap or an encrypted disk, and correct rule 3 of the architecture page, which says that plaintext never exists as a file.
- **Later, platform hardening.** Locking memory. It does not reach the `bw` processes, common limits are smaller than a vault with attachments, and Go gives little control over copies on the heap. An experiment per platform comes first.
