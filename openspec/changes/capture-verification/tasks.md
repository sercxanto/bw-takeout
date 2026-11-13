# Tasks

## 1. Module and architecture baseline

- [ ] 1.1 Create the Go module `bitwarden-backup` at the repository root with the Go version of the dev container and the empty packages `internal/capture` and `internal/verify`; verify `go build ./...` and `go vet ./...` succeed
- [ ] 1.2 Write the component model, the four architecture rules, the roadmap and the expected ADRs from design.md into `docs/development/architecture.md`, marked as planned; verify the page names every component and links to the decision index
- [ ] 1.3 Add the commands `gofmt -l .`, `go vet ./...` and `go test ./...` to `docs/development/getting-started.md`; verify each documented command runs as written in the dev container

## 2. Input shapes and synthetic fixtures

- [ ] 2.1 Read the export models and their tests in `bitwarden/clients` at commit `91214c3b978edb5496a32a946451960fe7d44531` (`cipher.export.ts`, `cipher.export.spec.ts`, `fido2-credential.export.ts`, `individual-vault-export.service.ts`, `cipher.response.ts`) and record the legacy form, the blob form, the account-bound and the password-protected document form with source links and evidence level in the package documentation of `internal/capture`; verify every recognized field cites its source
- [ ] 2.2 Build test helpers that create the plaintext export, the account-bound encrypted export and the item list in memory, generate P-256 passkey keys at test time and put a sentinel value in every sensitive field; verify a helper test produces the intact fixture with two passkeys (counters 0 and 6) and two attachments with the same file name, and that `git grep` finds no key material in the repository
- [ ] 2.3 Add fixture variants for every failure case in the spec: missing item, duplicate ID, one passkey missing, all passkeys dropped, blob login item, unrecognized item form, empty key, unparsable key, non-numeric counter, unknown key algorithm, swapped credential with equal count, organization items, listed item not exported, exported item not listed; verify each variant differs from the intact fixture only in the intended place
- [ ] 2.4 Describe the fixtures, their evidence level "mock" and their sources in `docs/development/testing.md`; verify the section states that no live capture backs the shapes yet

## 3. Capture model and parsers

- [ ] 3.1 Define the capture model in `internal/capture`: items with ID, type, organization ID, recognized format and passkey count from the encrypted export; items with passkeys from the plaintext export; items with attachment IDs from the item list; download results per ID pair with either an error class or a content hash and byte count; verify the package compiles and the model holds no decrypted field other than what the passkey checks need
- [ ] 3.2 Implement the three document parsers with positive format recognition; verify tests cover the intact fixture, a legacy item, a blob item, an unrecognized item and a legacy item that carries a `data` field without the blob marker
- [ ] 3.3 Reject wrong document forms; verify tests fail the capture for invalid JSON, a plaintext export marked as encrypted, an encrypted export without the account-bound form and a password-protected export
- [ ] 3.4 Detect duplicate item IDs per document and duplicate attachment ID pairs; verify one test per document

## 4. Verdict rules

- [ ] 4.1 Define the result type in `internal/verify` with verdict, scope statement, standing limits, findings and warnings, and the reason code constants; verify a test asserts the precedence failed over unverified over verified and that warnings do not change the verdict
- [ ] 4.2 Implement the item identity comparison in both directions; verify the spec scenarios "Item missing from plaintext export" and "Duplicate item ID" and the reverse direction as tests
- [ ] 4.3 Implement the passkey count comparison for legacy items and the unverified finding for blob login items and unrecognized items; verify the scenarios "One passkey missing", "All passkeys dropped", "Blob-format login item" and "Unrecognized format", and that a non-login blob item produces no finding
- [ ] 4.4 Implement the passkey structure validation; verify the scenarios "Two valid passkeys with different keys and counters", "Empty key", "Key does not parse" and "Unknown key form", plus a missing required field and a non-numeric counter
- [ ] 4.5 Implement attachment accounting by ID pair and the three-way item coverage; verify the scenarios "Two attachments with the same name", "One download fails", "Result missing", "Empty attachment", "Listed item not exported" and "Exported item not listed", plus a result for an unexpected ID pair
- [ ] 4.6 Implement the organization warning with counts per organization ID; verify the scenario "Organization items present" and that organization items never appear as missing
- [ ] 4.7 Implement the optional reference inventory comparison; verify the scenarios "Swapped credential without reference", "Swapped credential with reference" and "Attachment content differs from reference", and that the result without a reference lists credential identity and attachment content as not checked
- [ ] 4.8 Write `docs/decisions/0004-completeness-verdict.md` with status proposed, covering context, alternatives and consequences as outlined in design.md D13, and add it to `docs/decisions/README.md`; verify the index lists it as proposed
- [ ] 4.9 Update `docs/user/limitations.md` with what a verdict can and cannot detect: blob items are unverified, an equal-count swap and a well-formed wrong key pass without a reference, attachment inventory has a single source, verified never means server-complete; verify each limit corresponds to a test or a standing limit in the result

## 5. Result serialization and redaction

- [ ] 5.1 Serialize the result to JSON deterministically, sorted by item ID, attachment ID and reason code; verify two runs over the same capture produce byte-identical output
- [ ] 5.2 Add the redaction test: run every failure fixture, serialize the result and search it for every sentinel value, key value and attachment file name; verify the scenario "Failing capture with sensitive content" passes and that the test fails when a finding is given a name on purpose
- [ ] 5.3 Verify the scenario "Finding for a missing passkey": every finding in every fixture run carries a severity, a known reason code and the IDs it refers to

## 6. Integration check

- [ ] 6.1 Run `gofmt -l .`, `go vet ./...` and `go test ./...` in the dev container; verify no output from gofmt and all tests pass, and record platform, Go version and date in `docs/development/testing.md` together with what was not run (macOS, Windows, live capture)
- [ ] 6.2 Compare every scenario in `specs/capture-verification/spec.md` with the test that covers it; verify the mapping is complete and note it in the PR description
- [ ] 6.3 Run `openspec validate capture-verification --strict`; verify it passes

## Workflow follow-up

- Review scope, risks and acceptance criteria before implementing, as CONTRIBUTING requires.
- After the live probe, compare its captured shapes with the fixtures and open a follow-up change if they differ.
- Archive the change after review and verify the archived result.
