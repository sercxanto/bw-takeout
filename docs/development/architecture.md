# Architecture Status

The fixed integration boundary is the unmodified official bw CLI. Decisions and their status are in the [decision index](../decisions/README.md).

**Everything on this page is planned, except where a status says otherwise.** The code that exists is the Go module with the packages `internal/capture` and `internal/verify` from the archived change `capture-verification`, the packages `internal/manifest`, `internal/archive` and `internal/publish` and the command `cmd/bw-takeout` with its single subcommand `verify` from the change `archive-generation`, and the packages `internal/bwcli`, `internal/profile` and `internal/vaultid` with the development tool `internal/tools/loginprobe` from the change `bw-process-adapter`. The change `archived-item-list` extended `internal/capture`, `internal/verify` and `internal/archive` to a second item list. The change `capture-collection` added the package `internal/collect` and extended `internal/capture` and `internal/verify` by decryption error counts and attachment file names. The component model and the roadmap come from the explore session of 13 November 2026 and may change with each later change.

## Planned flow

Interactive profile → sync and inventory → JSON exports and individual attachment downloads → verification result → encrypted archive → published generation. The last two steps exist and are tested with synthetic captures. Starting `bw`, the interactive login, the single commands of the second and third steps and the profile exist and are tested against a fake `bw`.

Collecting a capture exists in two phases, tested against an in-memory vault and the fake `bw`:

- **Snapshot**, before the target directory is touched: one forced sync, the freshness check, the four documents with their decryption error counts, the source for the manifest and the plan of attachment downloads.
- **Attachments**, inside the write function of the publishing step: one download at a time, each handed to the archive writer and then cleared, with one download result per planned attachment.

The `backup` command that orders the phases, logs in and tears down does not exist yet (change C). Personal and organization views are handled separately.

## Rules

1. Only one component knows `bw`. Everything else works against an interface and is testable with a fake.
2. Verification is a pure function over a captured data model.
3. Plaintext exists only in process memory and inside the encryption stream, never as a file.
4. Only one component touches the target directory, and it only creates new names.

## Components

| Component | Responsibility | Status |
|---|---|---|
| `bwcli` | Start `bw`, build the child environment from an allowlist, report the version and the SHA-256 of the executable without judging them ([ADR 0007](../decisions/0007-bw-versions.md)), pass the session key to child processes only, collect stdout in memory, count decryption error lines, classify failures without CLI text, run the interactive login | implemented, tested against a fake `bw` in the dev container and in CI on Linux, macOS and Windows, login exercised by hand against Vaultwarden on Linux and macOS |
| `profile` | Own CLI data directory per run, list leftovers of earlier runs without removing them, teardown also when the logout fails | implemented, tested in the dev container and in CI on Linux, macOS and Windows. A fresh profile per run is a maintainer decision for the prototype (15 November 2026) |
| `capture` | Capture model and parsers for four documents, with the default and the archived item list as separate documents, decryption error counts per document and download, attachment file names for the manifest, and the one function that decides which attachments are expected | implemented |
| `collect` | Collect one capture from a logged-in client in two phases: forced sync, freshness check, documents, download plan, then attachment downloads with retries and classification of failures. Works against an interface that `bwcli.Client` satisfies | implemented, tested against an in-memory vault and the fake `bw` |
| `verify` | Turn a capture into a verdict with reasons per ID, over the combined listing of both item lists, including decryption error lines | implemented |
| `manifest` | Format version, creation time, source, versions, entry sizes and hashes, embedded verification result | implemented |
| `archive` | Write and read the age-encrypted tar stream with four captured documents, recovery instructions, recipient and identity parsing | implemented |
| `publish` | Create a new partial generation directory exclusively, flush, publish by verdict without replacing, list leftovers | implemented, tested on Linux only |
| `report` | User output with redaction, exit codes | planned. The `verify` command prints its own redacted output and exit codes |
| `run` | Order of phases and teardown, also on abort | planned |

## Roadmap

1. `capture-verification`: verdict rules over a captured inventory. Done, archived on 14 November 2026. The agreed behavior is in `openspec/specs/capture-verification/`.
2. `archive-generation`: container, encryption, manifest, exclusive publishing and an offline verify command. Done, archived on 14 November 2026. The agreed behavior is in `openspec/specs/backup-archive/`, `openspec/specs/generation-publishing/` and `openspec/specs/offline-verification/`.
3. `bw-process-adapter`: `bwcli` and `profile` against a fake `bw`, unauthenticated tests with the real binary and a manual login exercise against Vaultwarden on Linux and macOS. Done, archived on 15 November 2026. The agreed behavior is in `openspec/specs/bw-invocation/` and `openspec/specs/bw-profile/`. Results are in the [code notes](../research/bw-cli-code-notes-2026-11-15.md) and the [login exercise](../research/login-exercise-2026-11-15.md).
4. The former item `interactive-backup-run` was split into four changes in the explore session of 16 November 2026. Together they complete the first prototype. B builds on the model of A. C and D are not created yet, because their open questions are not settled.
   - **A. `archived-item-list`**: the capture holds the default and the archived item list as two documents, the verification treats them as one listing and reports an archive filter that did not apply, and the archive stores the fourth document `documents/items.archived.json`. Done, archived on 16 November 2026. The agreed behavior is in `openspec/specs/capture-verification/`, `openspec/specs/backup-archive/` and `openspec/specs/offline-verification/`. This takes the obligation that the capture must include `list items --archived`, because the default list leaves archived items out while both exports keep them ([live probe](../research/vaultwarden-live-probe-2026-11-14.md), added case "archived item"; [code notes of 16 November 2026](../research/bw-cli-code-notes-2026-11-16.md)).
   - **B. `capture-collection`**: collect a capture through `bwcli` in two phases, snapshot and attachments, against an interface that `bwcli.Client` satisfies. Tested against an in-memory vault and the fake `bw`. Done, archived on 18 November 2026. The agreed behavior is in `openspec/specs/capture-collection/` and `openspec/specs/capture-verification/`. The decisions of 17 November 2026:
     - Freshness rule: the run forces one sync and requires `bw status` to report an unlocked vault and a `lastSync` at or after the start of that sync, compared at millisecond precision. A failed or stale sync ends the run without a generation. It does not lower the verdict.
     - No comparison of the inventory before and after the capture. `list`, `export` and `get` start no sync of their own, so the one forced sync fixes the snapshot for every later command ([code notes of 17 November 2026](../research/bw-cli-code-notes-2026-11-17.md), code reading only).
     - Premium failure: a failed download without a retry, never more lenient than any other failed download. An account with lapsed premium and personal attachments gets no generation ([limitations](../user/limitations.md)).
     - Download failures: a timeout or a non-zero exit code is tried up to three times, with waits of 2 and 5 seconds. Output beyond the output limit of 1 GiB is a failed download without a retry (review of 18 November 2026). Not logged in, locked, cancelled, failure to start, an unused profile and a stray file end the run. The remaining failures map to the three download error classes.
     - Decryption error lines: lines on any of the four documents make the verdict at least unverified, lines on a successful download give a warning. [ADR 0004](../decisions/0004-completeness-verdict.md) stays proposed and was updated in place.
     - Default time limit of one attachment download: 5 minutes instead of 30.
     - Plaintext in memory: one attachment at a time, cleared after the archive writer took it, a `Wipe` method for the four documents, and an output buffer in `bwcli` that clears what it leaves behind when it grows. No step writes a file. Decoded JSON trees, file names and other strings are not cleared and stay until the garbage collector frees them; this residual risk goes to the memory hardening of C.
   - **C. `interactive-backup-run`**: the `backup` command, order of phases, the login, teardown and logout on abort, report and exit codes, user documentation. Tested against the fake `bw` and by hand against Vaultwarden. It must account for:
     - The order of the two collection phases around the target directory, and the call of `Snapshot.Wipe` once the documents were handed to the archive writer and parsed.
     - How the report names attachments by ID and the `bw` command and class of a failure that ended the run, and how it names the default server when `bw status` reports no server URL.
     - Teardown on abort of the whole run, with a logout after every login attempt, also a failed one, because `bw` can be logged in although the client refused its session key (review of 15 November 2026).
     - Memory hardening: disable core dumps for the run. On Unix the limit is inherited by the `bw` processes. The way on Windows is open. Document swap, hibernation and core dumps as residual risk in `docs/user/security.md`, with the advice to use encrypted swap or an encrypted disk, and correct rule 3 above, which says that plaintext never exists as a file.
     - The next live run should include an archived item with an attachment, which was not observed yet.
   - **D. FAT and exFAT run**: the run of `publish` on a FAT or exFAT target that was deferred from `archive-generation`, see [testing](testing.md). Independent of `bw`.
5. Follow-ups recorded by `bw-process-adapter` and `capture-collection`, not yet ordered:
   - A separate explore session on an automated Vaultwarden test setup, including account creation without a person.
   - A small change that reports export fields unknown to this project at run time.
   - Reusing one profile across runs in a logged-out state, so that the server keeps one device entry. To be looked at before a first release.
   - Publishing failed captures under a marker, raised by `capture-collection` for accounts whose premium lapsed. A later explore session may take it up.
   - Comparing passkey counters with an earlier generation. Reading an earlier generation needs an identity, which ADR 0003 keeps off the backing-up system, so this fits an offline comparison of two generations.
6. Later: platform hardening, configuration and multiple accounts (including the attachment time limit and the number of download attempts that `capture-collection` fixed), organizations, release, then batch operation and a KDBX derivative. Platform hardening starts with locking memory: it does not reach the `bw` processes, common limits are smaller than a vault with attachments, and Go gives little control over copies on the heap, so an experiment per platform comes first.

Not planned at all: capturing the trash, which is a [non-goal](../product/non-goals.md) of the product. Both personal exports and both item lists leave deleted items out, so items in the trash are not backed up (maintainer decision, 16 November 2026, see [limitations](../user/limitations.md)).

The live probe against a disposable Vaultwarden instance ran outside this sequence on 13 and 14 November 2026. Its results are in the [live probe report](../research/vaultwarden-live-probe-2026-11-14.md).

## Expected decisions

The change `capture-verification` added the proposed [ADR 0004](../decisions/0004-completeness-verdict.md) on the completeness verdict and its scope claim, and the change `archive-generation` the proposed [ADR 0005](../decisions/0005-archive-format.md) on archive format and generation layout. The change `bw-process-adapter` added the proposed [ADR 0006](../decisions/0006-secret-boundary.md) on the secret boundary around the `bw` process and the proposed [ADR 0007](../decisions/0007-bw-versions.md) on the handling of `bw` versions. The ADRs on Go and age stay proposed until their open points are tested. ADR 0002 on Go now has evidence for Linux and macOS, and Windows is open. The [decision index](../decisions/README.md) lists what exists.
