# Code Notes: Archive and Trash Filters in bw 2026.10.1

Date: code reading on 16 November 2026 (UTC). These notes collect the findings that the change `archived-item-list` builds on. No account, no server and no secret was involved, and `bw` was not started.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package `@bitwarden/cli` in the dev container |
| Bundle | `build/bw.js`, 97082 lines, SHA-256 `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` |

The SHA-256 matches the one in the package documentation of `internal/capture` and in the [code notes of 15 November 2026](bw-cli-code-notes-2026-11-15.md).

## Method

The installed bundle was read with a text editor. Each quoted expression below was looked up with `grep -n` on 16 November 2026 and found at the line given. Code reading shows what the code would do. It does not show what happens at run time, which account state applies, or what other versions do.

## Findings

Evidence level of every finding: **code reading**.

| # | Finding | Quoted expression | Line |
|---|---|---|---|
| 1 | `bw list items` filters by trash and archive state. The trash option is false unless `--trash` is given, so the default list leaves deleted items out | `options.trash = options.trash \|\| false;` and `const passesTrashFilter = options.trash === c.isDeleted;` | 58259, 58454 |
| 2 | An item passes the archive filter only when its archive state equals the option. The default list therefore leaves archived items out, and `--archived` returns only archived items | `const passesArchivedFilter = options.archived === c.isArchived;` | 58455 |
| 3 | For an account that may not archive, `--archived` is dropped before filtering. `bw list items --archived` then returns the default list again, without an error | `options.archived = userCanArchive && options.archived;` | 58260 |
| 4 | Whether an account may archive is read from premium from any source, personal or through an organization | `userCanArchive$(userId) {` followed by `.hasPremiumFromAnySource$(userId)` | 79027 to 79029, used at 58258 |
| 5 | The plaintext personal export (`getDecryptedExport`) leaves deleted items out | `decCiphers = ciphers.filter((f) => f.deletedDate == null &&` | 59248 |
| 6 | The account-bound encrypted personal export (`getEncryptedExport`) leaves deleted items out | `f.deletedDate == null &&` within the filter that starts with `!f.isPartial &&` | 59282 |

The archive and trash state of an item are getters of the decrypted view: `isArchived` is `this.archivedDate != null`, `isDeleted` is `this.deletedDate != null` (48944 to 48949).

Consequences for a capture:

- Archived items are in neither list for an account that archived items while it had premium and has none at the time of the capture (findings 2 to 4). Both exports still hold them, because neither export filters by archive state.
- Items in the trash are in no captured document: not in the default list (finding 1), the archived list (finding 1, the trash option stays false) or either export (findings 5 and 6).

## Scope

- The findings hold for this one bundle. A later `bw` version can change any of them.
- Findings 5 and 6 apply to the personal exports only. The organization exports at 58985 and 59009 were not examined for this note.

## Not observed live

- An account that may not archive. The [live probe](vaultwarden-live-probe-2026-11-14.md) ran on Vaultwarden, where archiving was available. Whether Vaultwarden or Bitwarden Cloud report premium for a given account, and what `bw` then returns for `--archived`, was not run.
- The trash. The live probe lists it as not examined.
- An archived item with an attachment. The live probe archived one login without attachments.
