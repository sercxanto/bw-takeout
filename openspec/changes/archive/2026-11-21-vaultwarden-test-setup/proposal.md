# Proposal

## Why

The verdict rules have never run against a live capture that a test checks. Every run against a real server so far needed a person: the maintainer registered the account in the web vault, seeded it by hand and typed the login ([backup exercise of 18 November 2026](../../../docs/development/testing.md#backup-exercise-of-18-november-2026)). The roadmap lists an automated Vaultwarden test setup "including account creation without a person" as a follow-up ([architecture](../../../docs/development/architecture.md#roadmap), item 5). The design of `bw-process-adapter` and the testing page defer every automated test with a login to it. A spike in the explore session of 21 November 2026 showed that an account can be registered on the pinned Vaultwarden 1.37.4 over HTTP alone, and that the pinned `bw` 2026.10.1 logs in to it, unlocks the vault and seeds it with a passkey, attachments and an archived item.

## What Changes

- Record the spike of 21 November 2026 as a research note with version, method, results and limits, and add it to the research overview.
- Add an end-to-end test suite in a new package `internal/e2e`, behind the build tag `vaultwarden`, so that `go test ./...` and the skip report never see it. It runs on Linux only and:
  - fetches the pinned Vaultwarden 1.37.4 image index by digest, selects the manifest of the running architecture (arm64 or amd64) and checks the server binary against a recorded SHA-256 per architecture,
  - starts the server on loopback with TLS and a fresh certificate, in a temporary data directory,
  - registers a synthetic account over the HTTP API with a test-only helper that derives the account keys itself,
  - seeds the vault through the official `bw` in a separate seed profile,
  - runs the built `bw-takeout backup` under a pseudo-terminal that answers the prompts of `bw`, and checks the report, the exit status and the generation with `bw-takeout verify`.
- Two scenarios in this first change:
  - **Intact vault**: the three items of the backup exercise of 18 November 2026 (a login with a passkey, a login with an attachment, an archived login with an attachment) give exit status 0, the verdict `verified`, a generation that `verify` accepts and that holds the attachment of the archived item, and an empty profile location.
  - **Tampered passkey**: after the helper replaces the encrypted `keyValue` of the passkey through the Vaultwarden API with a well-formed ciphertext under a foreign key, the backup ends with exit status 1, a failed verdict that names the passkey count, and no generation in the target. This is the false success of the [live probe](../../../docs/research/vaultwarden-live-probe-2026-11-14.md), reproduced end to end through the product.
- Add a GitHub Actions workflow `vaultwarden.yml` that starts only on `workflow_dispatch`. It builds the dev container image on `ubuntu-24.04` (amd64) and runs the suite inside it. It needs no secrets, uploads no artifacts and never blocks a merge.
- Add `go vet -tags vaultwarden ./...` to `checks.yml`, so that the tagged code is checked on every change although no regular run compiles it.
- Update the test strategy, the integration notes, the architecture roadmap and the release notes.

Out of scope, recorded as a follow-up change once the setup has run successfully in the dev container and in CI: an account with two-step login (TOTP), a deleted or corrupted attachment file on the server, the login cases of `bw-process-adapter` against a server, an interrupt during a live backup, organizations, macOS and Windows. Blob-format items cannot be produced with this setup, because `bw` 2026.10.1 wrote legacy items against Vaultwarden in the spike. The lapsed-premium and the archive-filter cases cannot be produced either, because Vaultwarden 1.37.4 reports premium for every user.

This change adds no product feature and changes no command.

## Capabilities

### New Capabilities

None. The change adds development tooling, test code and documentation. It sets `skip_specs: true`, because no behavior of the program changes. The acceptance criteria of the suite and the workflow are in the design and the tasks.

### Modified Capabilities

None.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)): none directly. The suite gives the first automated evidence for the existing requirements on visible export scope and on runs that must not leave a seemingly successful result, against a real server.

**Code:** new package `internal/e2e` with test files only, all behind the build tag `vaultwarden`. New workflow `.github/workflows/vaultwarden.yml`, one more step in `checks.yml`, and an extended `TestWorkflowStructure` in `internal/tools/skipreport`. No change to `cmd/bw-takeout` or to any product package under `internal/`.

**Dependencies:** none new. The helper uses the Go standard library (`crypto/pbkdf2`, `crypto/hkdf`, AES, HMAC, RSA, `crypto/x509`), and the pseudo-terminal uses `golang.org/x/sys`, which is already a direct dependency.

**External contact during a run:** Docker Hub, to fetch the pinned image, and `https://api.bitwarden.com`, once for each fresh `bw` profile ([finding 12 of the code notes](../../../docs/research/bw-cli-code-notes-2026-11-15.md)).

**Documentation to update:**

- `docs/research/vaultwarden-registration-spike-2026-11-21.md` (new) and `docs/research/research-summary.md`
- `docs/development/testing.md`: a section on the Vaultwarden suite, how to run it, its scenarios, what it does not cover, and the first runs. The manual exercises stay for macOS, TOTP and the interrupt cases
- `docs/development/integrations.md`: the Vaultwarden pins per architecture, the new workflow and its pins, the tested `bw` row
- `docs/development/architecture.md`: the roadmap item of the automated setup, and the follow-up change for TOTP and the further cases
- `docs/development/releases.md`: a passing dispatched run on the release commit before a release
- `docs/user/limitations.md`: only if the first runs contradict a statement there

**Existing ADRs:**

- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: the product keeps using only the official `bw`. The registration helper derives account keys in test code that is never linked into the program. It produces server-side input for a test, and `bw` rejects the account if the helper is wrong. The design justifies why this stays outside the ADR. No change.
- [0004 Completeness verdict](../../../docs/decisions/0004-completeness-verdict.md), proposed: the tampered-passkey scenario is its first end-to-end evidence. No change unless a run contradicts it.
- [0007 Handling of bw versions](../../../docs/decisions/0007-bw-versions.md), proposed: the suite is a means it names to find behavior changes of a new `bw`. No change.
- 0002, 0003, 0005 and 0006 are not touched.

**Open evidence:**

- Whether Vaultwarden accepts a `PUT` of a cipher with a changed `keyValue` without further checks. Unverified. The fallback is the database edit of the live probe.
- Whether `bw` 2026.10.1 drops only the passkey or the whole item when the `keyValue` is under a foreign key, with one passkey per item. The live probe saw 0 of 2 passkeys with the item kept.
- The amd64 server binary was hashed in the spike but never run.
- Driving the prompts of `bw` through a pseudo-terminal was not part of the spike.
- Anonymous Docker Hub pulls are rate-limited, and GitHub runners share addresses. Whether that affects a dispatched run is unknown.
- Bitwarden Cloud remains untested. Results of the suite are Vaultwarden evidence only.
