# Architecture Status

The fixed integration boundary is the unmodified official bw CLI. Decisions and their status are in the [decision index](../decisions/README.md).

**Everything on this page is planned.** The only code that exists is the Go module with the packages `internal/capture` and `internal/verify`, which the change `capture-verification` fills. The component model and the roadmap come from the explore session of 13 November 2026 and may change with each later change.

## Planned flow

Interactive profile → sync and inventory → JSON exports and individual attachment downloads → verification result → encrypted archive. Personal and organization views are handled separately.

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
| `capture` | Capture model and parsers. Later: collect the documents and attachments and compare before and after | model and parsers in progress, collection planned |
| `verify` | Turn a capture into a verdict with reasons per ID | in progress |
| `manifest` | Scope, time window, versions, hashes, verdict, unverifiable areas | planned |
| `archive` | Write and read the container, encryption as a stream | planned |
| `publish` | Create a new generation exclusively, write a partial file, sync, publish without replacing | planned |
| `report` | User output with redaction, exit codes | planned |
| `run` | Order of phases and teardown, also on abort | planned |

## Roadmap

1. `capture-verification`: verdict rules over a captured inventory. In progress.
2. `archive-generation`: container, encryption, manifest, exclusive publishing and an offline verify command.
3. `bw-process-adapter`: `bwcli` and `profile` against a fake `bw`, plus unauthenticated tests with the real binary.
4. `interactive-backup-run`: login, sync freshness, capture, teardown and user documentation. This completes the first prototype.
5. Later: platform hardening, configuration and multiple accounts, organizations, release, then batch operation and a KDBX derivative.

A live probe against a disposable Vaultwarden instance runs outside this sequence and informs steps 1 and 3.

## Expected decisions

The change `capture-verification` proposes an ADR on the completeness verdict and its scope claim. Further ADRs are expected along the roadmap: archive format and layout, the plaintext and secret boundary, and how `bw` versions are qualified. The ADRs on Go and age stay proposed until their open points are tested. The [decision index](../decisions/README.md) lists what exists.
