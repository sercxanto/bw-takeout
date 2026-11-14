# Test Strategy

For the first change, define a known synthetic reference inventory outside the CLI's view. Verify two passkeys with different keys and counters, then simulate a missing credential, a missing key, and an equal count with swapped content. Download two attachments with the same name but different IDs and bytes. Make one individual download fail and check that no silent success results.

Further cases: stale local state, a restricted organization role, interrupted encryption, and two concurrent runs with the same target. Existing archives must stay byte-identical. For offline recovery, decrypt without network access and compare the contents with the reference inventory. Report mock, SDK test and authenticated end-to-end test separately. Concurrent runs, interrupted writing and offline decryption are tested with synthetic data, see [archive and publishing tests](#archive-and-publishing-tests). The tests against stale local state, organization roles and a live vault are planned.

## Synthetic Capture Fixtures

The package `internal/capture/capturetest` builds one capture of a personal vault in memory: the plaintext export, the account-bound encrypted export, the item list and the content of each attachment. The tests read no file and start no process.

**Evidence level: mock.** The document shapes were derived from the source of `bitwarden/clients` at commit `91214c3b978edb5496a32a946451960fe7d44531` (CLI 2026.10.1) and compared with the installed 2026.10.1 bundle. The fixtures are not taken from a live capture, and no test runs against one. The package documentation of `internal/capture` lists every recognized field with a link to its source, and it names what the code does not settle, for example whether an empty key is left out or written as null. The legacy shapes of the account-bound encrypted export, the plaintext export and the item list match the [Vaultwarden live probe of 14 November 2026](../research/vaultwarden-live-probe-2026-11-14.md), by comparison of the report with the parser; blob items, Bitwarden Cloud and partial items remain unconfirmed.

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

Every scenario of the capture-verification spec has one test of the same name in `internal/verify`. The mapping was checked on 14 November 2026, again after the review decisions of the same day, and is complete. Parser cases are tested in addition in `internal/capture/parse_test.go`.

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
| | Counter grew since the reference | `TestCounterGrewSinceTheReference` |
| | Counter lower than the reference | `TestCounterLowerThanTheReference` |
| Attachment accounting by ID | Two attachments with the same name | `TestTwoAttachmentsWithTheSameName` |
| | One download fails | `TestOneDownloadFails` |
| | Result missing | `TestResultMissing` |
| | Empty attachment | `TestEmptyAttachment` |
| Attachment inventory coverage | Listed item not exported | `TestListedItemNotExported` |
| | Exported item not listed | `TestExportedItemNotListed` |
| Organization items stay visible | Organization items present | `TestOrganizationItemsPresent` |
| | Exported item listed as organization item | `TestExportedItemListedAsOrganizationItem` |
| | Download result for an organization attachment | `TestDownloadResultForAnOrganizationAttachment` |
| Attributable findings | Finding for a missing passkey | `TestFindingForAMissingPasskey` |
| No secrets in results | Failing capture with sensitive content | `TestFailingCaptureWithSensitiveContent` |

## Test Run of 14 November 2026

In the dev container on Linux/arm64 with Go 1.27.3, `gofmt -l .` printed nothing, `go vet ./...` passed and `go test ./...` passed in all three packages. The key-material search above printed nothing on the same day. Evidence level: mock.

Not run: macOS, Windows, Linux/amd64, any live capture, any authenticated CLI process, and the real `bw` binary. The tests start no process and use no network.

## Archive and Publishing Tests

The change archive-generation adds tests in four packages. All of them use the synthetic fixtures above and age key pairs generated at test time. No key is committed.

- `internal/manifest`: serialization, the byte-identical embedded verification result, format version and entry validation.
- `internal/archive`: the encrypted tar stream. Malformed archives are built for the purpose by re-encrypting changed tar entries. Truncation and altered bytes are tested at several positions of an archive whose payload spans several age chunks. `TestRecoveryWithStandardTools` decrypts with the `age` command and unpacks with the `tar` command from the search path. It is skipped with a message where either is missing, and it is the only test that writes synthetic plaintext to a test directory on purpose.
- `internal/publish`: the target directory. A fault-injecting file system fails each step of design D8 in turn, and every pre-existing file in the target is hashed before and after. Eight publishers run concurrently with the same clock value and the same first random value. `TestProcessKilledWhileWriting` starts the test binary as a helper process, lets it write more than one age chunk into a new generation and kills it. `TestKillHelper` is that helper and reports a skip when run on its own.
- `cmd/bitwarden-backup`: the command is built once and run as a separate process against generations made from the fixtures, including runs with an empty search path. `TestEndToEnd` runs the whole chain from fixture capture to the verify command.

`TestLeaveGenerationForOfflineExercise` in `cmd/bitwarden-backup` writes one synthetic generation for the manual offline exercise on a second system. It runs only when both environment variables are set, otherwise it reports a skip:

```sh
age-keygen -o identity.txt
BITWARDEN_BACKUP_EXERCISE_TARGET=/path/to/target \
BITWARDEN_BACKUP_EXERCISE_RECIPIENT="$(age-keygen -y identity.txt)" \
go test -count=1 -run TestLeaveGenerationForOfflineExercise -v ./cmd/bitwarden-backup/
```

Copy the generation and the identity to the second system separately and follow [recovery](../user/recovery.md) there.

### Spec Scenario Coverage of archive-generation

Every scenario of the three specs of the change archive-generation has at least one test. The mapping was checked on 14 November 2026 and is complete.

| Spec | Scenario | Test |
|---|---|---|
| backup-archive | Content of a generation | `publish.TestNoNamesFromTheVault`, `publish.TestVerdictDecidesPublication` |
| | Recovery with the age and tar commands | `archive.TestRecoveryWithStandardTools` |
| | Two recipients | `archive.TestTwoRecipients` |
| | No recipient | `archive.TestRecipients`, `main.TestEndToEnd/no_recipient` |
| | Unsupported recipient | `archive.TestRecipients` |
| | Plaintext export round trip | `archive.TestRoundTripIntactFixture` |
| | Two attachments with the same file name | `archive.TestTwoAttachmentsWithTheSameFileName` |
| | ID with a path separator | `archive.TestIDValidation`, `main.TestEndToEnd/attachment_ID_with_a_path_separator` |
| | Attachment ID that is not a UUID | `archive.TestRoundTripIntactFixture`, `archive.TestIDValidation` |
| | Manifest covers every entry | `archive.TestRoundTripIntactFixture`, `archive.TestReaderScenarios` |
| | Verification result embedded unchanged | `manifest.TestRoundTripKeepsVerificationBytes`, `archive.TestRoundTripIntactFixture` |
| | Instructions present and free of vault data | `archive.TestRecoveryInstructions` |
| | Temporary location stays empty | `archive.TestTemporaryLocationStaysEmpty`, `publish.TestNoNamesFromTheVault` |
| generation-publishing | Two generations in the same second | `publish.TestTwoGenerationsInTheSameSecond` |
| | Name collision | `publish.TestNameCollision` |
| | Successful run next to older generations | `publish.TestSuccessfulRunNextToOlderGenerations` |
| | Failing run next to older generations | `publish.TestFailingRunNextToOlderGenerations` |
| | Concurrent runs with identical time and random source | `publish.TestConcurrentRunsWithIdenticalTimeAndRandomSource` |
| | Write error in the middle | `publish.TestWriteErrorInTheMiddle` |
| | Unfinished archive | `publish.TestUnfinishedArchiveIsNotPublished` |
| | Process killed while writing | `publish.TestProcessKilledWhileWriting` |
| | Verified capture | `publish.TestVerdictDecidesPublication/verified` |
| | Unverified capture | `publish.TestVerdictDecidesPublication/unverified` |
| | Failed capture | `publish.TestVerdictDecidesPublication/failed`, `main.TestEndToEnd/failed_capture` |
| | Leftover from an aborted run | `publish.TestLeftoverFromAnAbortedRun` |
| | Missing target | `publish.TestMissingTarget` |
| | Permissions on Linux | `publish.TestPermissionsOnLinux` |
| offline-verification | System without bw | `main.TestSystemWithoutBW`, `main.TestNoNetworkOrProcessPackage` |
| | Intact generation | `main.TestVerifiedCapture`, `main.TestEndToEnd` |
| | Entry differs from the manifest | `archive.TestReaderScenarios/entry_differs_from_the_manifest`, `main.TestFailedVerification/damaged_entry` |
| | Entry not listed | `archive.TestReaderScenarios/entry_not_listed` |
| | Listed entry missing | `archive.TestReaderScenarios/listed_entry_missing` |
| | Item count differs from the export | `archive.TestReaderScenarios/item_count_differs_from_the_export` |
| | Truncated archive file | `archive.TestTruncatedArchive`, `main.TestFailedVerification/truncated_archive` |
| | Altered byte | `archive.TestAlteredByte`, `main.TestFailedVerification/altered_byte` |
| | Identity of another key pair | `archive.TestIdentityOfAnotherKeyPair`, `main.TestFailedVerification/identity_of_another_key_pair` |
| | Partial generation | `main.TestPartialGeneration`, `main.TestDirectoryWithoutArchive` |
| | Partial generation reached through another path | `main.TestPartialGenerationByAnotherPath` |
| | Symbolic link entry | `archive.TestReaderScenarios/symbolic_link_entry` |
| | Recovery instructions not first | `archive.TestReaderScenarios/recovery_instructions_not_first` |
| | Newer format version | `archive.TestReaderScenarios/newer_format_version`, `main.TestFailedVerification/newer_format_version` |
| | Verified capture | `main.TestVerifiedCapture` |
| | Unverified capture | `main.TestUnverifiedCapture` |
| | Failed verification | `main.TestFailedVerification` |
| | Generation with sentinel values | `main.TestGenerationWithSentinelValues` |

## Test Run of 14 November 2026, archive-generation

In the dev container on Debian 12, Linux 6.8 aarch64, with Go 1.27.3, age 1.2.1, GNU tar 1.34 and OpenSpec 2.0.0: `gofmt -l .` printed nothing, and `go vet ./...`, `go test ./...` and `go test -race ./...` passed in all packages. The concurrency test passed 100 runs in a row with the race detector. `GOOS=darwin` and `GOOS=windows` builds and `go vet` passed for `./...`. The key-material search above printed nothing. The recovery commands of the [recovery page](../user/recovery.md) were run as written against a generation left by the opt-in test. Evidence level: mock.

The module cache of this container belonged to root, so the session used a `GOPATH` under the ignored `.local/` ([integrations](integrations.md)). After the image was rebuilt on 14 November 2026, `/go/pkg/mod` belongs to `vscode`, and `go mod download` and `go test ./...` passed with the default `GOPATH=/go`.

Not run: macOS, Windows, Linux/amd64, FAT or exFAT media, network shares, the manual offline exercise on a second system, attachments larger than a few hundred kilobytes, any live capture and the real `bw` binary.

## Bootstrap Check of 13 November 2026

Docker build and start as user vscode on Linux/arm64 succeeded. Go 1.27.3, bw 2026.10.1, age 1.2.1 and OpenSpec 2.0.0 were run in the image. A synthetic age round trip succeeded without network access. SHA-256 of bw's installed npm entry point (build/bw.js): `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. Package integrity is recorded separately in the npm lockfile.

OpenSpec init generated six shared skills. The Claude reference for Explore is readable in the container. openspec list reported no active changes. For a temporary change, the proposal instructions contained the project-specific documentation rule. The VS Code UI, the personal Claude feature, Linux/amd64 and authenticated vaults were not tested.
