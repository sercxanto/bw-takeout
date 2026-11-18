# Proposal

## Why

Each step around a capture already exists: `bwcli` starts the single `bw` commands, and `verify`, `archive` and `publish` turn a capture into a generation. What is missing is the step in between, which collects a capture from `bw`. Every test today feeds the chain from synthetic fixtures. Roadmap item 4 B fills this gap so that change C, the `backup` command, only has to order the phases. The explore session of 17 November 2026 settled the open questions that kept B from being created.

## What Changes

- A new collection step works on a logged-in client. It forces one sync and checks that the sync is fresh. It then collects the four documents, downloads the attachments of personal items one at a time, hands each one to the archive writer and records its download result.
- **Freshness rule.** The collection starts with `bw sync --force`. It then requires `bw status` to report the state unlocked and a `lastSync` at or after the moment the sync was started. A failed sync, another state or a stale or missing `lastSync` ends the collection without a capture, so no generation is written. This is a maintainer decision from 17 November 2026. A fresh profile per run has no older snapshot, so a failed sync would leave the documents empty or partial.
- **No comparison before and after.** The comparison of the inventory before and after the capture, planned in the roadmap, is dropped. `list`, `export` and `get` start no sync of their own, so the one forced sync fixes the local snapshot for every later command. This rests on a first code search of 17 November 2026, and a task of this change confirms it in the `bw` bundle.
- **Documents.** The plaintext export, the encrypted export, the default item list and the archived item list are collected in this order. A failure of any of these commands ends the collection without a capture.
- **Attachments.** The attachments of the personal items in the combined listing are downloaded, once for each ID pair. Their file names come from the item list and go into the manifest only.
- **Failures of a download.**
  - A timeout or a non-zero exit code is retried. If the last attempt also fails, the download is recorded as failed.
  - A download whose output passes the output limit is not retried and is recorded as failed, because a second attempt meets the same limit (maintainer decision, 18 November 2026).
  - A premium failure is not retried and is recorded as failed. It is never treated more leniently than any other failed download.
  - Not logged in, locked, cancelled, failure to start, an unused profile and a stray file end the collection without a capture. These failures concern the run as a whole, not one attachment.
  - The remaining failures map to the three download error classes. The classes do not change the verdict.
- **Attachment time limit.** The default time limit for one attachment download drops from 30 to 5 minutes (maintainer decision, 17 November 2026). With three attempts, a hanging download then costs at most 15 minutes instead of 90. A very large attachment over a slow connection can exceed the limit on every attempt.
- **Decryption error lines.** The collection passes on the number of decryption error lines of every document and every successful download.
  - Lines on the plaintext export, the encrypted export or either item list make the verdict at least unverified. The finding names the document and the number of lines. This is a maintainer decision from 17 November 2026.
  - Lines on an attachment download only produce a warning, because the content is covered by size and SHA-256.
- **Manifest source.** The collection takes the server URL, account e-mail and account ID for the manifest from the status after the sync.
- `capture.Input` gains the decryption error counts, `capture.Download` gains a decryption error count, and the parsed item list keeps the attachment file names. The new fields default to zero and empty, so existing callers keep working. The archive format does not change.

Not part of this change: the `backup` command, the login, logout and teardown, the order of the phases around the target directory, the report and its exit codes (all change C), and publishing failed captures under a marker. A later explore session may take up that last point, which is not on the roadmap. Memory hardening also stays with change C.

## Capabilities

### New Capabilities

- `capture-collection`: collecting one capture from a logged-in `bw` client. It covers the forced sync and the freshness rule, the four documents, the plan and streaming of the attachment downloads, retries, the failures that end a collection, the mapping to download error classes, the decryption error counts and the manifest source.

### Modified Capabilities

- `capture-verification`: a new requirement turns decryption error lines into an unverified finding for a document and a warning for an attachment. The existing requirements keep their text.

## Impact

**Code.**

- New package `internal/collect`, working against an interface that `*bwcli.Client` satisfies.
- `internal/capture`: input and download fields, attachment file names in the parsed item list, package documentation.
- `internal/capture/capturetest`.
- `internal/verify`: two reason codes.
- Tests that wire collection, verification, archive and publishing against the fake `bw`.

- `internal/bwcli`: the default attachment time limit, and the output buffer, which now clears its old array when it grows (review of 18 November 2026).

`internal/archive`, `internal/publish` and `internal/manifest` do not change.

**Requirements affected.**

- `capture-verification` gains one requirement. "Attachment accounting by ID" and "Three-state verdict" are read together with it.
- `bw-invocation`, "Decryption error lines are counted": its count now reaches the verification.
- `backup-archive`, "Attachments are stored by ID": collection supplies the file name this requirement keeps in the manifest.
- `generation-publishing`, "The verdict decides publication": unchanged. A failed download still means no generation.

**Documentation.**

- `docs/user/limitations.md`: decryption error lines, sync failure, retries, and the time limit for large attachments.
- `docs/development/architecture.md`: component table with the new package, roadmap item B.
- `docs/development/testing.md`: collection scenarios against the fake `bw`.
- A new research note with the code reading of 17 November 2026 on the sync behavior of `list`, `export` and `get`.

**ADRs.**

- [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md) (proposed) is updated in place: decryption error lines on a document are a check that did not pass, so they are a reason for unverified.
- [ADR 0006](../../../docs/decisions/0006-secret-boundary.md) (proposed) is checked: it already counts the lines and keeps no text, and the collection keeps to it.
- No new ADR. The freshness rule, the retries and the abort classes are details of this change and can be reversed, and they move no integration boundary.

**Open evidence.**

- The claim that `list`, `export` and `get` start no sync comes from a search of the 2026.10.1 bundle for `fullSync` calls only. A task reads the commands in full before the comparison is dropped for good.
- A premium failure of an attachment download was not observed at run time.
- No archived item with an attachment was observed live.
- Decryption error lines were observed on the plaintext export, the default item list, `get item` and attachment downloads. They were not observed on the archived item list. The encrypted export printed none.
- Evidence level of all tests in this change: mock, with the fake `bw` built from the observations above.
