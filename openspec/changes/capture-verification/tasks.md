# Tasks

## 1. Module and architecture baseline

- [x] 1.1 Create the Go module `bitwarden-backup` at the repository root with the Go version of the dev container and the empty packages `internal/capture` and `internal/verify`; verify `go build ./...` and `go vet ./...` succeed
- [x] 1.2 Write the component model, the four architecture rules, the roadmap and the expected ADRs from design.md into `docs/development/architecture.md`, marked as planned; verify the page names every component and links to the decision index
- [x] 1.3 Add the commands `gofmt -l .`, `go vet ./...` and `go test ./...` to `docs/development/getting-started.md`; verify each documented command runs as written in the dev container

## 2. Input shapes and synthetic fixtures

- [x] 2.1 Read the export models and their tests in `bitwarden/clients` at commit `91214c3b978edb5496a32a946451960fe7d44531` (`cipher.export.ts`, `cipher.export.spec.ts`, `fido2-credential.export.ts`, `individual-vault-export.service.ts`, `cipher.response.ts`) and record the legacy form, the blob form, the account-bound and the password-protected document form with source links and evidence level in the package documentation of `internal/capture`; verify every recognized field cites its source
- [x] 2.2 Build test helpers that create the plaintext export, the account-bound encrypted export and the item list in memory, generate P-256 passkey keys at test time and put a sentinel value in every sensitive field; verify a helper test produces the intact fixture with two passkeys (counters 0 and 6) and two attachments with the same file name, and that `git grep` finds no key material in the repository
- [x] 2.3 Add fixture variants for every failure case in the spec: missing item, duplicate ID, one passkey missing, all passkeys dropped, blob login item, unrecognized item form, empty key, unparsable key, non-numeric counter, unknown key algorithm, swapped credential with equal count, organization items, listed item not exported, exported item not listed; verify each variant differs from the intact fixture only in the intended place
- [x] 2.4 Describe the fixtures, their evidence level "mock" and their sources in `docs/development/testing.md`; verify the section states that no live capture backs the shapes yet
- [x] 2.5 Correct the package documentation of `internal/capture`: the decrypted views drop partial items (with source link), `--archived` shows only archived items and is ignored when the account cannot archive; verify its filter paragraphs agree with design.md

## 3. Capture model and parsers

- [x] 3.1 Define the capture model in `internal/capture`: items with ID, type, organization ID, recognized format and passkey count from the encrypted export; items with passkeys from the plaintext export; items with attachment IDs from the item list; download results per ID pair with either an error class or a content hash and byte count; verify the package compiles and the model holds no decrypted field other than what the passkey checks need
- [x] 3.2 Implement the three document parsers with positive format recognition; verify tests cover the intact fixture, a legacy item, a legacy login without a `fido2Credentials` key (zero passkeys), a blob item, an unrecognized item, an item with legacy content plus a `data` field without the blob marker (unknown), an item that combines the blob marker with legacy content (unknown), and optional keys given as null
- [x] 3.3 Reject wrong document forms; verify tests fail the capture for invalid JSON, a plaintext export marked as encrypted, an encrypted export without the account-bound form, a password-protected export and an item list wrapped in the `BW_RESPONSE` envelope
- [x] 3.4 Detect duplicate item IDs per document and duplicate attachment ID pairs; verify one test per document

## 4. Verdict rules

- [x] 4.1 Define the result type in `internal/verify` with verdict, scope statement, standing limits, findings and warnings, and the reason code constants; verify a test asserts the precedence failed over unverified over verified and that warnings do not change the verdict
- [x] 4.2 Implement the item identity comparison in both directions; verify the spec scenarios "Item missing from plaintext export" and "Duplicate item ID" and the reverse direction as tests
- [x] 4.3 Implement the passkey count comparison for legacy items and the unverified finding for blob login items and unrecognized items; verify the scenarios "Legacy login without passkey list", "One passkey missing", "All passkeys dropped", "Blob-format login item", "Unrecognized format" and "Legacy content with unmarked sealed data", and that a non-login blob item produces no finding
- [x] 4.4 Implement the passkey structure validation; verify the scenarios "Two valid passkeys with different keys and counters", "Empty key", "Key does not parse" and "Unknown key form", plus a missing required field and a non-numeric counter
- [x] 4.5 Implement attachment accounting by ID pair and the three-way item coverage; verify the scenarios "Two attachments with the same name", "One download fails", "Result missing", "Empty attachment", "Listed item not exported" and "Exported item not listed", plus a result for an unexpected ID pair
- [x] 4.6 Implement the organization warning with counts per organization ID; verify the scenario "Organization items present" and that organization items never appear as missing
- [x] 4.7 Implement the optional reference inventory comparison; verify the scenarios "Swapped credential without reference", "Swapped credential with reference" and "Attachment content differs from reference", and that the result without a reference lists credential identity and attachment content as not checked
- [x] 4.8 Write `docs/decisions/0004-completeness-verdict.md` with status proposed, covering context, alternatives and consequences as outlined in design.md D13, and add it to `docs/decisions/README.md`; verify the index lists it as proposed
- [x] 4.9 Update `docs/user/limitations.md` with what a verdict can and cannot detect: blob items are unverified, an equal-count swap and a well-formed wrong key pass without a reference, attachment inventory has a single source, items the item list does not show, such as archived items in a default listing, are unverified, verified never means server-complete; verify each limit corresponds to a test or a standing limit in the result

## 5. Result serialization and redaction

- [x] 5.1 Serialize the result to JSON deterministically, sorted by item ID, attachment ID and reason code; verify two runs over the same capture produce byte-identical output
- [x] 5.2 Add the redaction test: run every failure fixture, serialize the result and search it for every sentinel value, key value and attachment file name; verify the scenario "Failing capture with sensitive content" passes and that the test fails when a finding is given a name on purpose
- [x] 5.3 Verify the scenario "Finding for a missing passkey": every finding in every fixture run carries a severity, a known reason code and the IDs it refers to

## 6. Integration check

- [x] 6.1 Run `gofmt -l .`, `go vet ./...` and `go test ./...` in the dev container; verify no output from gofmt and all tests pass, and record platform, Go version and date in `docs/development/testing.md` together with what was not run (macOS, Windows, live capture)
- [x] 6.2 Compare every scenario in `specs/capture-verification/spec.md` with the test that covers it; verify the mapping is complete and record it in `docs/development/testing.md`, because the repository has no pull request workflow yet
- [x] 6.3 Run `openspec validate capture-verification --strict`; verify it passes

## 7. Review decisions of 14 November 2026

- [x] 7.1 Record the finding fields, the additional reason codes and the handling of rejected documents in design.md D2 and D10; verify the design names every field of `verify.Finding`
- [x] 7.2 Accept a padded base64url key value that parses; verify a test passes a padded key with and without the reference and fails a wrongly padded one
- [x] 7.3 Report a download result for an attachment of an organization item as a warning; verify the scenario "Download result for an organization attachment" and that an attachment ID the organization item does not show still fails
- [x] 7.4 Fail an item that both exports hold as personal item when the item list shows it as an organization item; verify the scenario "Exported item listed as organization item"
- [x] 7.5 Let a passkey counter higher than the reference pass and a lower one fail; verify the scenarios "Counter grew since the reference" and "Counter lower than the reference"
- [x] 7.6 Record the key values held as strings in the capture model as a risk in design.md
- [x] 7.7 Rerun `gofmt -l .`, `go vet ./...`, `go test ./...` and `openspec validate capture-verification --strict`, and extend the scenario mapping in `docs/development/testing.md`; verify every scenario has a test

## Workflow follow-up

- Review scope, risks and acceptance criteria before implementing, as CONTRIBUTING requires.
- After the live probe, compare its captured shapes with the fixtures and open a follow-up change if they differ.
- After the live probe, compare its archived-item results with D8 and its filter results with D5.
- Archive the change after review and verify the archived result.
