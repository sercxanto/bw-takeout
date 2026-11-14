# Test Strategy

For the first change, define a known synthetic reference inventory outside the CLI's view. Verify two passkeys with different keys and counters, then simulate a missing credential, a missing key, and an equal count with swapped content. Download two attachments with the same name but different IDs and bytes. Make one individual download fail and check that no silent success results.

Further cases: stale local state, a restricted organization role, interrupted encryption, and two concurrent runs with the same target. Existing archives must stay byte-identical. For offline recovery, decrypt without network access and compare the contents with the reference inventory. Report mock, SDK test and authenticated end-to-end test separately. These product tests are planned.

## Synthetic Capture Fixtures

The package `internal/capture/capturetest` builds one capture of a personal vault in memory: the plaintext export, the account-bound encrypted export, the item list and the content of each attachment. The tests read no file and start no process.

**Evidence level: mock.** The document shapes were derived from the source of `bitwarden/clients` at commit `91214c3b978edb5496a32a946451960fe7d44531` (CLI 2026.10.1) and compared with the installed 2026.10.1 bundle. No live capture backs the shapes yet. No authenticated CLI process has produced any of these documents for this project. The package documentation of `internal/capture` lists every recognized field with a link to its source, and it names what the code does not settle, for example whether an empty key is left out or written as null. A live probe against a disposable Vaultwarden instance is planned and may change the fixtures.

The intact fixture holds three personal items:

- a login with two passkeys (counters 0 and 6) and two attachments that share one file name and differ in ID and content,
- a login without passkeys, in a folder, with its empty optional keys written as null,
- a secure note.

Rules the fixtures follow:

- Passkey keys are generated with the Go standard library on every test run. The repository contains no key material. Search for it with `git grep --untracked -I -E 'BEGIN [A-Z ]*PRIVATE KEY|MIGHAgEAMBMGByqGSM49|MC4CAQAwBQYDK2Vw|AGE-SECRET-KEY-1' -- . ':!docs/development/testing.md'`. The two base64 patterns are the fixed first bytes of a P-256 and an Ed25519 key in PKCS#8. This page is excluded because it names the patterns. The search printed nothing on 13 November 2026.
- Every sensitive field of the decrypted documents carries a value that contains the word `sentinel`. Key value, credential ID and user handle have a fixed format and are tracked by value instead. `Fixture.Sensitive` returns all of them, the encrypted placeholder strings and the attachment contents, so that a test can search a result for leaks.
- Encrypted fields are opaque placeholders in the CLI's encrypted-string notation. They are not ciphertext.
- The fixture records a reference inventory outside the documents: credential ID, key fingerprint and counter per passkey, and a content hash per attachment. A variant that changes a document leaves the reference untouched.

Fourteen variants each break the intact fixture in one place: missing item, duplicate ID, one passkey missing, all passkeys dropped, blob login item, unrecognized item form, empty key, unparsable key, non-numeric counter, unknown key algorithm, swapped credential with equal count, organization items, listed item not exported, exported item not listed. Every variant declares the places it changes, and a test compares that list with the actual difference to the intact fixture. The plaintext export and the item list render the same decrypted view, so a variant that changes decrypted content changes both.

## Verification Tests

`internal/capture` tests the parsers: format recognition, rejected document forms and duplicate detection. `internal/verify` tests the verdict rules. Each test of a spec scenario carries the scenario's name, for example `TestOneDownloadFails`. The cross-cutting tests run every fixture variant and every additional failure case, with and without the reference inventory, and check that the serialized result contains no sensitive value, that every finding carries a severity, a known reason code and the IDs it refers to, and that two runs produce byte-identical output.

### Spec Scenario Coverage

Every scenario of the capture-verification spec has one test of the same name in `internal/verify`. The mapping was checked on 14 November 2026 and is complete. Parser cases are tested in addition in `internal/capture/parse_test.go`.

| Requirement | Scenario | Test |
|---|---|---|
| Three-state verdict | Intact capture | `TestIntactCapture` |
| | Failure outranks unverifiable items | `TestFailureOutranksUnverifiableItems` |
| Scope statement and standing limits | Verified result without reference inventory | `TestVerifiedResultWithoutReferenceInventory` |
| Document form validation | Password-protected export supplied as inventory | `TestPasswordProtectedExportSuppliedAsInventory` |
| | Unparsable document | `TestUnparsableDocument` |
| Item identity comparison | Item missing from plaintext export | `TestItemMissingFromPlaintextExport` |
| | Duplicate item ID | `TestDuplicateItemID` |
| Passkey count comparison | Legacy login without passkey list | `TestLegacyLoginWithoutPasskeyList` |
| | One passkey missing | `TestOnePasskeyMissing` |
| | All passkeys dropped | `TestAllPasskeysDropped` |
| Unverifiable item formats | Blob-format login item | `TestBlobFormatLoginItem` |
| | Unrecognized format | `TestUnrecognizedFormat` |
| | Legacy content with unmarked sealed data | `TestLegacyContentWithUnmarkedSealedData` |
| Passkey structure validation | Two valid passkeys with different keys and counters | `TestTwoValidPasskeysWithDifferentKeysAndCounters` |
| | Empty key | `TestEmptyKey` |
| | Key does not parse | `TestKeyDoesNotParse` |
| | Unknown key form | `TestUnknownKeyForm` |
| Reference inventory comparison | Swapped credential without reference | `TestSwappedCredentialWithoutReference` |
| | Swapped credential with reference | `TestSwappedCredentialWithReference` |
| | Attachment content differs from reference | `TestAttachmentContentDiffersFromReference` |
| Attachment accounting by ID | Two attachments with the same name | `TestTwoAttachmentsWithTheSameName` |
| | One download fails | `TestOneDownloadFails` |
| | Result missing | `TestResultMissing` |
| | Empty attachment | `TestEmptyAttachment` |
| Attachment inventory coverage | Listed item not exported | `TestListedItemNotExported` |
| | Exported item not listed | `TestExportedItemNotListed` |
| Organization items stay visible | Organization items present | `TestOrganizationItemsPresent` |
| Attributable findings | Finding for a missing passkey | `TestFindingForAMissingPasskey` |
| No secrets in results | Failing capture with sensitive content | `TestFailingCaptureWithSensitiveContent` |

## Test Run of 14 November 2026

In the dev container on Linux/arm64 with Go 1.27.3, `gofmt -l .` printed nothing, `go vet ./...` passed and `go test ./...` passed in all three packages. The key-material search above printed nothing on the same day. Evidence level: mock.

Not run: macOS, Windows, Linux/amd64, any live capture, any authenticated CLI process, and the real `bw` binary. The tests start no process and use no network.

## Bootstrap Check of 13 November 2026

Docker build and start as user vscode on Linux/arm64 succeeded. Go 1.27.3, bw 2026.10.1, age 1.2.1 and OpenSpec 2.0.0 were run in the image. A synthetic age round trip succeeded without network access. SHA-256 of bw's installed npm entry point (build/bw.js): `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. Package integrity is recorded separately in the npm lockfile.

OpenSpec init generated six shared skills. The Claude reference for Explore is readable in the container. openspec list reported no active changes. For a temporary change, the proposal instructions contained the project-specific documentation rule. The VS Code UI, the personal Claude feature, Linux/amd64 and authenticated vaults were not tested.
