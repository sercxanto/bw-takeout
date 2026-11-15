# Architecture Status

The fixed integration boundary is the unmodified official bw CLI. Decisions and their status are in the [decision index](../decisions/README.md).

**Everything on this page is planned, except where a status says otherwise.** The code that exists is the Go module with the packages `internal/capture` and `internal/verify` from the archived change `capture-verification`, and the packages `internal/manifest`, `internal/archive` and `internal/publish` and the command `cmd/bw-takeout` with its single subcommand `verify` from the change `archive-generation`. The component model and the roadmap come from the explore session of 13 November 2026 and may change with each later change.

## Planned flow

Interactive profile → sync and inventory → JSON exports and individual attachment downloads → verification result → encrypted archive → published generation. The last two steps exist and are tested with synthetic captures. Personal and organization views are handled separately.

## Rules

1. Only one component knows `bw`. Everything else works against an interface and is testable with a fake.
2. Verification is a pure function over a captured data model.
3. Plaintext exists only in process memory and inside the encryption stream, never as a file.
4. Only one component touches the target directory, and it only creates new names.

## Components

| Component | Responsibility | Status |
|---|---|---|
| `bwcli` | Start `bw`, clean the environment, check version and hash, pass the session to child processes only, collect stdout, classify errors | planned |
| `profile` | Own CLI data directory per run, detect leftovers of earlier runs, teardown | planned |
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
3. `bw-process-adapter`: `bwcli` and `profile` against a fake `bw`, plus unauthenticated tests with the real binary. Findings of the [Vaultwarden live probe](../research/vaultwarden-live-probe-2026-11-14.md) to account for:
   - bw refuses servers without HTTPS. A self-signed server certificate needs `NODE_EXTRA_CA_CERTS` passed through to the child process (report, method "TLS").
   - `bw login --raw` without any terminal exited with code 0 and empty stdout. The adapter must validate the form of the session key instead of trusting the exit code (report, result 1).
   - On a decryption failure bw wrote error lines to stderr while exiting with 0. They name neither item nor field. They are usable as an additional warning signal, not as a classification (report, result 6).
   - The capture must merge `list items` with `list items --archived`, because the default list leaves archived items out while both exports keep them (report, added case "archived item").
4. `interactive-backup-run`: login, sync freshness, capture, teardown and user documentation. This completes the first prototype. It includes the run on a FAT or exFAT target that was deferred from `archive-generation`, see [testing](testing.md).
5. Later: platform hardening, configuration and multiple accounts, organizations, release, then batch operation and a KDBX derivative.

The live probe against a disposable Vaultwarden instance ran outside this sequence on 13 and 14 November 2026. Its results are in the [live probe report](../research/vaultwarden-live-probe-2026-11-14.md).

## Expected decisions

The change `capture-verification` added the proposed [ADR 0004](../decisions/0004-completeness-verdict.md) on the completeness verdict and its scope claim, and the change `archive-generation` the proposed [ADR 0005](../decisions/0005-archive-format.md) on archive format and generation layout. Further ADRs are expected along the roadmap: the plaintext and secret boundary, and how `bw` versions are qualified. The ADRs on Go and age stay proposed until their open points are tested. The [decision index](../decisions/README.md) lists what exists.
