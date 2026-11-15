# Architecture Status

The fixed integration boundary is the unmodified official bw CLI. Decisions and their status are in the [decision index](../decisions/README.md).

**Everything on this page is planned, except where a status says otherwise.** The code that exists is the Go module with the packages `internal/capture` and `internal/verify` from the archived change `capture-verification`, the packages `internal/manifest`, `internal/archive` and `internal/publish` and the command `cmd/bw-takeout` with its single subcommand `verify` from the change `archive-generation`, and the packages `internal/bwcli`, `internal/profile` and `internal/vaultid` with the development tool `internal/tools/loginprobe` from the change `bw-process-adapter`. The component model and the roadmap come from the explore session of 13 November 2026 and may change with each later change.

## Planned flow

Interactive profile → sync and inventory → JSON exports and individual attachment downloads → verification result → encrypted archive → published generation. The last two steps exist and are tested with synthetic captures. Starting `bw`, the interactive login, the single commands of the second and third steps and the profile exist and are tested against a fake `bw`. Collecting a capture from them does not exist yet. Personal and organization views are handled separately.

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
| `capture` | Capture model and parsers. Later: collect the documents and attachments and compare before and after | model and parsers implemented, collection planned |
| `verify` | Turn a capture into a verdict with reasons per ID | implemented |
| `manifest` | Format version, creation time, source, versions, entry sizes and hashes, embedded verification result | implemented |
| `archive` | Write and read the age-encrypted tar stream, recovery instructions, recipient and identity parsing | implemented |
| `publish` | Create a new partial generation directory exclusively, flush, publish by verdict without replacing, list leftovers | implemented, tested on Linux only |
| `report` | User output with redaction, exit codes | planned. The `verify` command prints its own redacted output and exit codes |
| `run` | Order of phases and teardown, also on abort | planned |

## Roadmap

1. `capture-verification`: verdict rules over a captured inventory. Done, archived on 14 November 2026. The agreed behavior is in `openspec/specs/capture-verification/`.
2. `archive-generation`: container, encryption, manifest, exclusive publishing and an offline verify command. Done, archived on 14 November 2026. The agreed behavior is in `openspec/specs/backup-archive/`, `openspec/specs/generation-publishing/` and `openspec/specs/offline-verification/`.
3. `bw-process-adapter`: `bwcli` and `profile` against a fake `bw`, unauthenticated tests with the real binary and a manual login exercise against Vaultwarden on Linux and macOS. Done on 15 November 2026, to be archived after review. The behavior is in the specs `bw-invocation` and `bw-profile` of the change. Results are in the [code notes](../research/bw-cli-code-notes-2026-11-15.md) and the [login exercise](../research/login-exercise-2026-11-15.md).
4. `interactive-backup-run`: login, capture collection, sync freshness, teardown and user documentation. This completes the first prototype. It must account for:
   - The capture must merge `list items` with `list items --archived`, because the default list leaves archived items out while both exports keep them ([live probe](../research/vaultwarden-live-probe-2026-11-14.md), added case "archived item").
   - The freshness rule: `lastSync` must be later than the start of the run before the capture. The login ignores a failed sync, so a session key alone does not show that the local state is current.
   - Teardown on abort of the whole run, with a logout after every login attempt, also a failed one, because `bw` can be logged in although the client refused its session key (review of 15 November 2026).
   - What a premium failure of an attachment download means for the verdict. It must never be more lenient than any other failed download.
   - The run on a FAT or exFAT target that was deferred from `archive-generation`, see [testing](testing.md).
5. Follow-ups recorded by `bw-process-adapter`, not yet ordered:
   - A separate explore session on an automated Vaultwarden test setup, including account creation without a person.
   - A small change that reports export fields unknown to this project at run time.
   - Reusing one profile across runs in a logged-out state, so that the server keeps one device entry. To be looked at before a first release.
   - Comparing passkey counters with an earlier generation. Reading an earlier generation needs an identity, which ADR 0003 keeps off the backing-up system, so this fits an offline comparison of two generations.
6. Later: platform hardening, configuration and multiple accounts, organizations, release, then batch operation and a KDBX derivative.

The live probe against a disposable Vaultwarden instance ran outside this sequence on 13 and 14 November 2026. Its results are in the [live probe report](../research/vaultwarden-live-probe-2026-11-14.md).

## Expected decisions

The change `capture-verification` added the proposed [ADR 0004](../decisions/0004-completeness-verdict.md) on the completeness verdict and its scope claim, and the change `archive-generation` the proposed [ADR 0005](../decisions/0005-archive-format.md) on archive format and generation layout. The change `bw-process-adapter` added the proposed [ADR 0006](../decisions/0006-secret-boundary.md) on the secret boundary around the `bw` process and the proposed [ADR 0007](../decisions/0007-bw-versions.md) on the handling of `bw` versions. The ADRs on Go and age stay proposed until their open points are tested. ADR 0002 on Go now has evidence for Linux and macOS, and Windows is open. The [decision index](../decisions/README.md) lists what exists.
