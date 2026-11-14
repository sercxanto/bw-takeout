# Proposal

## Why

The project can now judge a capture, but it cannot store one. The product requirements ask for a new target per backup, no overwriting even with concurrent runs, no seemingly successful result after an abort, and an offline read test with a separately stored key. None of this depends on `bw`, so it can be built and tested now with synthetic captures, before the process adapter and the interactive run exist.

## What Changes

- Add an archive format for one backup generation: a tar stream encrypted with age, holding the captured documents unchanged, the attachments by ID, a manifest and recovery instructions.
- Encrypt to one or more age recipient keys. The writing side needs only public keys. A passphrase mode is not part of this change and can be added later without changing the format.
- Add a manifest that lists every entry with size and SHA-256, names the source and the tool versions, and embeds the verification result unchanged.
- Publish each generation as a new directory in the target. A generation in progress or left by an abort carries a visible marker. Existing content of the target is never modified.
- Let the verdict decide publication: verified is published under the plain name, unverified with a visible marker, failed not at all.
- Add the first command, `bitwarden-backup verify`, which checks a generation offline with an identity file and without `bw`.
- Keep plaintext out of the file system in both directions: writing and verifying work in memory and in the encryption stream.
- Add the first external dependency, the age library, and repair the dev container so that module downloads work for the container user.
- Record the archive format and generation layout as a proposed ADR and sharpen the proposed ADR on age.

This change adds no `bw` invocation, no login and no backup command. Archives are produced by tests from synthetic captures.

## Capabilities

### New Capabilities

- `backup-archive`: Content, structure and encryption of one backup generation, including its manifest and its recoverability with standard tools.
- `generation-publishing`: How a generation appears in the target directory: new names, no modification of existing content, markers for incomplete and unverified generations, behavior under concurrency and abort.
- `offline-verification`: Checking a published generation without network access and without `bw`, and reporting its stored verdict.

### Modified Capabilities

None. `capture-verification` is used as it is. Its result is embedded unchanged.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)):

- Each backup gets a new target, older backups are never overwritten even with concurrent runs, and aborted runs leave no seemingly successful result.
- Offline readability is tested with a separately stored key and a synthetic inventory.
- Server and account scope are attributable through the manifest.
- Export scope, errors and unverifiable areas stay visible through the embedded verdict and the marker on unverified generations.

**Code:** new internal packages for manifest, archive and publishing, and the first command under `cmd/`. New dependency `filippo.io/age` at the version the dev container already pins for the age CLI.

**Development environment:** `.devcontainer/Dockerfile`. Observed on 14 November 2026: the Go module cache and checksum database under `/go/pkg` belong to root, so `go mod tidy` fails for the container user as soon as a module must be downloaded. The first change had no dependencies and did not hit this.

**Documentation to update:**

- `docs/user/recovery.md`, `docs/user/usage.md`, `docs/user/security.md`, `docs/user/limitations.md`
- `docs/development/architecture.md`, `docs/development/testing.md`, `docs/development/getting-started.md`, `docs/development/integrations.md`
- `docs/decisions/`: new proposed ADR for archive format and generation layout, update of ADR 0003, and the index
- `README.md`: project status

**Existing ADRs:**

- [0003 age](../../../docs/decisions/0003-age.md), proposed: this change settles the recipient model and tests offline recovery, which the ADR names as open. It adds two open points and does not move the ADR to accepted.
- [0004 Completeness verdict](../../../docs/decisions/0004-completeness-verdict.md), proposed: applied as it is. The verdict decides whether and how a generation is published.
- [0002 Go](../../../docs/decisions/0002-go.md), proposed: this change adds file system behavior that differs per platform. It gives evidence for Linux only.
- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: not touched.

**Open evidence:**

- Publishing was designed for Linux, macOS and Windows, but only Linux arm64 can be run in the dev container. Directory rename, flushing and permissions on macOS and Windows are untested.
- Targets on FAT or exFAT media and on network shares are untested.
- Recipient encryption does not authenticate the sender. Anyone who knows a recipient key can create an archive that verifies. Origin is therefore not proven.
- age 1.3 added hybrid post-quantum recipients. They are not evaluated here. The archives hold passkey private keys, so this matters for archives stored outside the user's control.
- Interoperability with the independent age CLI is tested only at the pinned version 1.2.1.
- No archive has been produced from a real capture yet, and attachment sizes beyond small synthetic files are untested.
