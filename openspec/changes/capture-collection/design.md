# Design

## Context

See [proposal.md](proposal.md) for the motivation and the decisions of the explore session of 17 November 2026. The requirements are in [specs/capture-collection/spec.md](specs/capture-collection/spec.md) and [specs/capture-verification/spec.md](specs/capture-verification/spec.md).

The current state:

- **`bwcli.Client`** has `Sync`, `Status`, `ExportJSON`, `ExportEncryptedJSON`, `ListItems`, `ListArchivedItems` and `Attachment`.
  - Each document command returns an `Output` with the raw bytes and the count of decryption error lines.
  - Each failure is a `*bwcli.Error` with a class and no CLI text.
  - After a non-zero exit of a vault command, the client queries the status. This turns the failure into `not_logged_in` or `locked` where that applies (design D7 of `bw-process-adapter`).
  - `Status` returns the state, server URL, user e-mail, user ID and `lastSync`.
- **`capture.Input`** takes four raw documents and a list of `capture.Download`. `capture.ParseItemList` reduces a list to item IDs, organization IDs and attachment IDs. The attachment file name is dropped.
- **`archive.NewWriter`** takes the four documents before any attachment. `AddAttachment` takes one attachment with its file name, and `Finish` takes the manifest metadata and the serialized verification result.
- **`publish.Publish`** creates the partial generation directory, calls a `WriteFunc` and publishes by the verdict it returns.
- **The fake `bw`** answers each command with a fixed behavior, looked up by the full argument list. It cannot answer the same command differently on a second call.

Evidence for the behavior of `bw` 2026.10.1:

| Statement | Source | Level |
|---|---|---|
| `lastSync` is written only after a successful sync. Its value is taken before the request is sent | [Code notes 15 November](../../../docs/research/bw-cli-code-notes-2026-11-15.md), finding 6 | Code reading |
| `lastSync` has millisecond precision and lay inside the wall-clock window of `bw sync --force` | [Live probe](../../../docs/research/vaultwarden-live-probe-2026-11-14.md), result 5 | Observed |
| The login ignores a failed sync, and `bw sync --force` propagates it | Code notes, finding 7 | Code reading |
| No call of `fullSync` lies inside `ListCommand`, `ExportCommand` or `GetCommand`. The calls are in login, unlock, `sync`, import, create and edit, and the account migrations | Bundle `build/bw.js`, search of 17 November 2026 | Code search, to be confirmed (task 1.1) |
| Decryption error lines appear with exit code 0 on the plaintext export, `list items`, `get item` and attachment downloads, and not on the encrypted export. The attachment bytes were still exact | Live probe, result 6 | Observed |
| `get attachment` requires premium for an attachment of a personal item | Code notes, finding 9 | Code reading |

## Goals / Non-Goals

**Goals:**

- Change C can build the `backup` command from `bwcli`, `profile`, the collection, `verify`, `archive` and `publish` without new rules about `bw`.
- The two phases can be placed around the target directory. Freshness and documents are settled before a partial directory exists. Attachments stream into an open archive writer.
- Every outcome of a `bw` command maps to exactly one consequence: a document, a download result, or the end of the collection.

**Non-Goals:**

- Login, logout, profile teardown and their order. These are in change C. The collection takes a client that is already logged in, and returns.
- Report wording and exit codes.
- Any comparison of two snapshots, and any inventory from another source than the four documents.
- Changing `archive`, `publish` or the fake `bw`. The only change to `bwcli` is the default time limit of D12.

## Decisions

### D1. A new package `internal/collect` against an interface

The collection lives in a new package `internal/collect`. It works against an interface `Vault` that holds the seven methods listed in Context, with the signatures `*bwcli.Client` already has. It uses `bwcli.Output`, `bwcli.Status` and `bwcli.ClassOf`, but starts no process itself. Rule 1 of the architecture page stays true: only `bwcli` knows how `bw` is started.

*Alternative: inside `internal/capture`.* Rejected. `capture` is the pure model that `verify` and tests import. With the collection inside it, the model would depend on the process adapter.

*Alternative: inside `internal/bwcli`.* Rejected. `bwcli` reports outcomes without judging them (ADR 0007, design D7 of `bw-process-adapter`). Retries, abort rules and the download plan are judgments about a capture.

### D2. Two phases: snapshot, then attachments

```
Snapshot(ctx, vault)                       -> *Snapshot, error
   sync --force, status, 4 documents
   Snapshot: Documents (raw), DecryptionErrors per document,
             Source, Plan (ID pairs + file names)

Attachments(ctx, vault, snap, receive)     -> []capture.Download, error
   for each planned pair: download (with retries), hash, receive, wipe
```

Change C calls `Snapshot` before it touches the target. It calls `Attachments` inside the `publish.WriteFunc`, after `archive.NewWriter` has taken the documents, and passes `(*archive.Writer).AddAttachment` as `receive`. The capture for `verify` is then `capture.Parse` of the snapshot documents with the downloads and the counts.

*Alternative: one `Collect` call with a callback that opens the writer.* Rejected. It would hide inside the collection the point where the target directory is created, and that order belongs to change C. Two calls make the order visible at the call site and testable on its own.

### D3. Freshness check

`Snapshot` takes the moment `t0` from an injectable clock, right before it calls `Sync`, and truncates it to milliseconds. After a successful sync it calls `Status` and accepts the snapshot only when:

- the state is `unlocked`,
- `lastSync` is set,
- `lastSync` is not before `t0`.

Otherwise it returns `ErrNotFresh` or `ErrNotUnlocked`. Both are errors of this package and carry no value from the status.

The comparison is "at or after", not "after", and is done at millisecond precision:

- `bw` takes the value after its own start and before the request (finding 6), so it cannot lie before `t0` unless the clock moved backwards.
- `bw` writes milliseconds. Comparing an untruncated `t0` with nanoseconds could reject a sync that started within the same millisecond.

*Alternative: compare with the start of the run, before the login.* Rejected. The login's own sync ignores errors (finding 7). Its `lastSync` proves nothing about the explicit sync. Taking `t0` right before the forced sync ties the check to the sync whose failure is propagated.

*Alternative: a stale snapshot lowers the verdict.* Rejected by the maintainer on 17 November 2026, see the proposal.

### D4. Document failures end the collection

The documents are requested in the order plaintext export, encrypted export, default item list, archived item list. The order carries no meaning, because no command after the sync starts a sync (task 1.1). A failure of any command is returned as it is: a `*bwcli.Error`, which names the operation and the class and holds no text.

*Alternative: record the failed document as absent and let `verify` fail it.* Rejected. The verdict would be failed in both cases, so no generation is written either way. A document failure is a fact about the run, not about the inventory, so the report in change C should show the operation and its class, not a document finding. `archive.NewWriter` also refuses an absent document.

### D5. Attachment file names in the parsed item list

`capture.ItemList` gains `FileNames map[AttachmentRef]string`. The first occurrence of an ID pair in a list wins. A `fileName` that is absent or not a string gives an empty name and no rejection, because no check relies on the name. `verify` does not read the field. The name is vault content: it stays in memory and goes into the encrypted manifest only, as the spec `backup-archive` already requires.

*Alternative: a parallel slice next to `AttachmentIDs`.* Rejected, because two slices can drift apart.

*Alternative: a second parser in `collect` for the file names.* Rejected, because a second reading of the same document can disagree with the first.

### D6. One function decides which attachments are expected

A new function `capture.PersonalAttachments(lists ...*ItemList) []AttachmentRef` returns the ID pairs of attachments on personal items. It reads the lists in order, takes items in document order and attachments in item order, and keeps each pair once. `verify` uses it to build its expected set instead of its own loop. `collect` uses it for the plan, and takes file names from the default list first, then the archived list.

With one function, the downloads the collection makes are exactly the results the verification expects. A planned download can never be reported as unexpected, and an expected one can never be skipped by the plan.

If `ParseItemList` rejects either list, the plan is empty. The capture still goes to `verify`, where the document finding makes the verdict failed (spec scenario "Unparsable item list"). Downloading the attachments of the other list would cost time for a generation that is never published.

### D7. Classification of a failed download

| `bwcli` class | Retried | Consequence | Download error class |
|---|---|---|---|
| `timeout` | yes | failed download after the last attempt | `process` |
| `exit` | yes | failed download after the last attempt | `status` |
| `output` | yes | failed download after the last attempt | `other` |
| `premium_required` | no | failed download | `status` |
| `invalid_argument` | no | failed download | `other` |
| `not_logged_in`, `locked`, `cancelled`, `start`, `profile_not_used`, `stray_file` | no | collection ends | — |
| `no_terminal`, an error that is not a `*bwcli.Error` | no | collection ends | — |

Notes on the table:

- **`exit`** is what remains after `bwcli` has ruled out not logged in and locked with its status query. It can be a server or network error that a second attempt survives.
- **`invalid_argument`** means that the item list holds an ID of a form the archive would refuse. This is a fact about one attachment.
- **The last row** cannot occur on a download. Ending the collection is the strictest answer, so an unexpected case never ends more leniently than a known one.
- **Download error classes** are kept only for the report. `verify` treats every class the same (spec `capture-verification`, "Attachment accounting by ID").

### D8. Retry policy

There are at most three attempts per attachment. The waits between them are 2 and 5 seconds and come from an injectable function, so that tests do not wait. A cancelled context during a wait ends the collection as a cancelled run. A failed attempt leaves no data, because `bwcli` returns output only on success.

*Alternative: a configurable number of attempts.* Deferred. The configuration format is not designed yet (roadmap item 6), and three attempts are enough for a prototype.

### D9. Decryption error counts in the capture

- `capture.Input` gains `DecryptionErrors`, a struct with one count per document: `PlaintextExport`, `EncryptedExport`, `ItemList` and `ArchivedItemList`.
- `capture.Download` gains `DecryptionErrors int`, set on successful downloads only.
- `capture.Parse` copies the counts into `capture.Capture`.

`verify` adds two reason codes:

- `document_decryption_errors`: unverified, with `Document` and `Count`.
- `attachment_decryption_errors`: a warning, with `ItemID`, `AttachmentID` and `Count`.

The comment on `Finding.Count` is widened from "number of items" to "number of items or lines". A count of zero or less produces nothing.

The finding is unverified rather than failed, and the reasons are in the proposal (maintainer decision, 17 November 2026). In terms of [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md), the lines show that a decrypt step reported an error. They do not prove which item lacks what, so the check "every item decrypted" did not pass, and that is unverified. A proven deviation, such as a passkey count mismatch, still makes the verdict failed. ADR 0004 is proposed and is updated in place with one sentence.

### D10. Plaintext lifetime

The collection holds the four raw documents in the `Snapshot` until change C has passed them to `archive.NewWriter` and parsed them. `Snapshot` offers a `Wipe` method that clears the document bytes, and change C calls it after both steps.

During `Attachments`, the content of one attachment exists while it is hashed and handed to `receive`. The collection clears the slice after `receive` returns, whether it succeeded or not. No step writes a file. The point in the roadmap about plaintext copies is met without a change to the writer, as the explore session of 16 November 2026 expected.

### D11. Tests

There are two layers:

- **Unit tests in `internal/collect`** use an in-memory `Vault` that answers each call from a queue and records the calls, a fixed clock and an empty wait. They cover every scenario of the spec, including:
  - retries with a later success,
  - the session lost in the middle of the downloads,
  - cancellation during a wait,
  - every row of the table in D7.
- **Integration tests** start the real `bwcli.Client` against the fake `bw`, with documents from `capturetest`. They cover the cases a fixed answer per command allows:
  - an intact run from snapshot to a published generation that `archive` reads back with its identity,
  - a stale `lastSync`,
  - a failing sync,
  - decryption error lines on a document and on a download,
  - the premium sentence on one download,
  - a sentinel value in an error text.

  A retry with a later success needs a second answer to the same command, so it stays at the unit level. The fake is not extended for it.

The sentinel test of the spec reuses the fixture marker of `capturetest`.

### D12. Default attachment time limit of 5 minutes

`bwcli.DefaultAttachmentTimeout` changes from 30 to 5 minutes (maintainer decision, 17 November 2026). The 30 minutes come from design D4 of `bw-process-adapter`, which gives no reason for the value. `bw` downloads, decrypts and only then writes an attachment, so the limit covers the whole transfer. Five minutes carry 100 MB at about 3 Mbit/s and 500 MB at about 14 Mbit/s. As far as known, but not recorded in this repository, Bitwarden Cloud accepts attachments of up to 500 MB, and Vaultwarden makes the limit configurable. With the three attempts of D8, a hanging download costs at most 15 minutes.

The limit stays a field of `bwcli.Timeouts`, which the caller can set. The collection does not set it, so the default applies until the configuration format exists.

*Alternative: keep 30 minutes.* Rejected. A hanging download would hold a run for up to 90 minutes, and the attachments the product expects are documents of a few megabytes.

*Alternative: no retry after a timeout.* Rejected. A retry helps against a download that hangs. It does not help a download that is too slow for the limit, which fails on every attempt.

## Risks / Trade-offs

- **Hidden syncs.** If `list`, `export` or `get` start a sync after all, the snapshot can change between the documents. → Task 1.1 reads the three commands and the services they call before any code is written. If it finds a sync, the work stops and the design returns to the comparison before and after.
- **Large attachments over slow connections.** With a limit of 5 minutes (D12), an attachment that takes longer to transfer fails on every attempt and in every run, so the account gets no generation, as in the premium case. → Documented in `docs/user/limitations.md` with the bandwidth needed. Making the limit configurable belongs to the configuration format (roadmap item 6). The report in change C shows which download timed out.
- **Long runs on a hanging attachment.** Three attempts take up to 15 minutes per attachment. → Accepted for the prototype.
- **Accounts with lapsed premium never get a generation.** A premium failure stays a failed download, and a failed capture is not published. → Accepted and documented in `docs/user/limitations.md`. Publishing failed captures under a marker is not part of this change.
- **Decryption error lines on a download are only a warning.** The probe showed exact bytes with error lines, but whether lines on a download can ever come with wrong content was not tested. → The warning keeps the case visible. Without a reference inventory, attachment content stays a standing limit of every result.
- **Clock moved backwards during the sync.** `lastSync` can then lie before `t0`. → The collection ends, which is the safe direction. The user runs again.
- **All four documents in memory at once.** Their size is bounded by the output limit of `bwcli`, which is 1 GiB per command. → Accepted for the prototype. Hardening is in roadmap item 6.
- **Empty server URL in the manifest.** `bw status` may report no server URL for the default cloud server. This was not observed. → The source then holds an empty server URL, and the manifest accepts it. Change C decides how the report names the default.

## Migration Plan

None. The new fields of `capture.Input`, `capture.Download` and `capture.ItemList` have zero values that mean "no lines" and "no names", so existing fixtures and tests stay valid and only gain new cases. No archive format changes, so the format version stays 1.
