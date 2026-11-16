# Test Strategy

For the first change, define a known synthetic reference inventory outside the CLI's view. Verify two passkeys with different keys and counters, then simulate a missing credential, a missing key, and an equal count with swapped content. Download two attachments with the same name but different IDs and bytes. Make one individual download fail and check that no silent success results.

Further cases: stale local state, a restricted organization role, interrupted encryption, and two concurrent runs with the same target. Existing archives must stay byte-identical. For offline recovery, decrypt without network access and compare the contents with the reference inventory. Report mock, SDK test and authenticated end-to-end test separately. Concurrent runs, interrupted writing and offline decryption are tested with synthetic data, see [archive and publishing tests](#archive-and-publishing-tests). The tests against stale local state, organization roles and a live vault are planned.

## Synthetic Capture Fixtures

The package `internal/capture/capturetest` builds one capture of a personal vault in memory: the plaintext export, the account-bound encrypted export, the default item list, the archived item list and the content of each attachment. The tests read no file and start no process.

**Evidence level: mock.** The document shapes were derived from the source of `bitwarden/clients` at commit `91214c3b978edb5496a32a946451960fe7d44531` (CLI 2026.10.1) and compared with the installed 2026.10.1 bundle. The fixtures are not taken from a live capture, and no test runs against one. The package documentation of `internal/capture` lists every recognized field with a link to its source, and it names what the code does not settle, for example whether an empty key is left out or written as null. The legacy shapes of the account-bound encrypted export, the plaintext export and the item list match the [Vaultwarden live probe of 14 November 2026](../research/vaultwarden-live-probe-2026-11-14.md), by comparison of the report with the parser; blob items, Bitwarden Cloud and partial items remain unconfirmed.

The intact fixture holds four personal items:

- a login with two passkeys (counters 0 and 6) and two attachments that share one file name and differ in ID and content,
- a login without passkeys, in a folder, with its empty optional keys written as null,
- a secure note,
- an archived login with one attachment. Both exports hold it with an archive date, and only the archived item list shows it. Its attachment is in the reference inventory.

The first three items are in the default item list. Every test therefore runs with two non-empty lists.

Rules the fixtures follow:

- Passkey keys are generated with the Go standard library on every test run. The repository contains no key material. Search for it with `git grep --untracked -I -E 'BEGIN [A-Z ]*PRIVATE KEY|MIGHAgEAMBMGByqGSM49|MC4CAQAwBQYDK2Vw|AGE-SECRET-KEY-1' -- . ':!docs/development/testing.md' ':!.github/workflows/checks.yml'`. The two base64 patterns are the fixed first bytes of a P-256 and an Ed25519 key in PKCS#8. This page and the workflow `checks.yml` are excluded because they name the patterns; that workflow runs the same search on every change. The search printed nothing on 13 November 2026.
- Every sensitive field of the decrypted documents carries a value that contains the word `sentinel`. Key value, credential ID and user handle have a fixed format and are tracked by value instead. `Fixture.Sensitive` returns all of them, the encrypted placeholder strings and the attachment contents, so that a test can search a result for leaks.
- Encrypted fields are opaque placeholders in the CLI's encrypted-string notation. They are not ciphertext.
- The fixture records a reference inventory outside the documents: credential ID, key fingerprint and counter per passkey, and a content hash per attachment. A variant that changes a document leaves the reference untouched.

Twenty-two variants each break the intact fixture in one place: missing item, duplicate ID, one passkey missing, all passkeys dropped, blob login item, unrecognized item form, empty key, unparsable key, non-numeric counter, unknown key algorithm, swapped credential with equal count, organization items, listed item not exported, exported item not listed (removed from both lists), and the eight variants of the change archived-item-list: archive filter ignored with the archived item in neither list, archive filter ignored in a vault without archived items, an item in both lists with different attachment IDs, an item in both lists with a different organization, a duplicate ID within the archived list, an archived list that is not valid JSON, an absent archived list, and a missing download for the attachment of the archived item. The last two document variants replace the rendered archived list with a byte override. Every variant declares the places it changes, and a test compares that list with the actual difference to the intact fixture. The plaintext export and the item lists render the same decrypted view, so a variant that changes decrypted content changes both.

## Verification Tests

`internal/capture` tests the parsers: format recognition, rejected document forms, duplicate detection, and the archived item list as a document of its own. `internal/verify` tests the verdict rules. Each test of a spec scenario carries the scenario's name, for example `TestOneDownloadFails`. The cross-cutting tests run every fixture variant and every additional failure case, with and without the reference inventory, and check that the serialized result contains no sensitive value, that every finding carries a severity, a known reason code and the IDs it refers to, and that two runs produce byte-identical output.

### Spec Scenario Coverage

Every scenario of the capture-verification spec has one test of the same name in `internal/verify`. The mapping was checked on 14 November 2026, again after the review decisions of the same day, and is complete. The rows of the three requirements of the change archived-item-list were added and checked with `go test -list` on 16 November 2026. `TestArchiveFilterWarningLeavesTheVerdict` checks in addition that the filter warning leaves a failed, an unverified and a verified verdict unchanged. Parser cases are tested in addition in `internal/capture/parse_test.go`.

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
| Default and archived item lists form one listing | Archived item with an attachment | `TestArchivedItemWithAnAttachment` |
| | Archived item in neither list | `TestArchivedItemInNeitherList` |
| | Download missing for an archived item | `TestDownloadMissingForAnArchivedItem` |
| | Duplicate ID within the archived item list | `TestDuplicateIDWithinTheArchivedItemList` |
| | Archived item list cannot be parsed | `TestArchivedItemListCannotBeParsed` (invalid and absent) |
| | Organization item in both lists | `TestOrganizationItemInBothLists` |
| Item shown by both lists must agree | Same item with the same attachments | `TestSameItemWithTheSameAttachments`, order in `TestSameAttachmentsInADifferentOrder` |
| | Same item with different attachments | `TestSameItemWithDifferentAttachments` |
| | Same item with a different organization | `TestSameItemWithADifferentOrganization` |
| Archive filter that did not apply is reported | Archived list repeats the default list | `TestArchivedListRepeatsTheDefaultList` |
| | Disjoint lists | `TestDisjointLists` |
| | Both lists empty | `TestBothListsEmpty` |

## Test Run of 14 November 2026

In the dev container on Linux/arm64 with Go 1.27.3, `gofmt -l .` printed nothing, `go vet ./...` passed and `go test ./...` passed in all three packages. The key-material search above printed nothing on the same day. Evidence level: mock.

Not run: macOS, Windows, Linux/amd64, any live capture, any authenticated CLI process, and the real `bw` binary. The tests start no process and use no network.

## Archive and Publishing Tests

The change archive-generation adds tests in four packages. All of them use the synthetic fixtures above and age key pairs generated at test time. No key is committed.

- `internal/manifest`: serialization, the byte-identical embedded verification result, format version and entry validation.
- `internal/archive`: the encrypted tar stream. Malformed archives are built for the purpose by re-encrypting changed tar entries. Truncation and altered bytes are tested at several positions of an archive whose payload spans several age chunks. `TestRecoveryWithStandardTools` decrypts with the `age` command and unpacks with the `tar` command from the search path. It is skipped with a message where either is missing, and it is the only test that writes synthetic plaintext to a test directory on purpose.
- `internal/publish`: the target directory. A fault-injecting file system fails each step of design D8 in turn, and every pre-existing file in the target is hashed before and after. Eight publishers run concurrently with the same clock value and the same first random value. `TestProcessKilledWhileWriting` starts the test binary as a helper process, lets it write more than one age chunk into a new generation and kills it. `TestKillHelper` is that helper and reports a skip when run on its own.
- `cmd/bw-takeout`: the command is built once and run as a separate process against generations made from the fixtures, including runs with an empty search path. `TestEndToEnd` runs the whole chain from fixture capture to the verify command.

`TestLeaveGenerationForOfflineExercise` in `cmd/bw-takeout` writes one synthetic generation for the manual offline exercise on a second system. It runs only when both environment variables are set, otherwise it reports a skip:

```sh
age-keygen -o identity.txt
BW_TAKEOUT_EXERCISE_TARGET=/path/to/target \
BW_TAKEOUT_EXERCISE_RECIPIENT="$(age-keygen -y identity.txt)" \
go test -count=1 -run TestLeaveGenerationForOfflineExercise -v ./cmd/bw-takeout/
```

Copy the generation and the identity to the second system separately and follow [recovery](../user/recovery.md) there.

### Spec Scenario Coverage of archive-generation

Every scenario of the three specs of the change archive-generation has at least one test. The mapping was checked on 14 November 2026 and is complete. The change archived-item-list added the rows for `documents/items.archived.json` and checked them with `go test -list` on 16 November 2026.

| Spec | Scenario | Test |
|---|---|---|
| backup-archive | Content of a generation | `publish.TestNoNamesFromTheVault`, `publish.TestVerdictDecidesPublication` |
| | Recovery with the age and tar commands | `archive.TestRecoveryWithStandardTools` |
| | Two recipients | `archive.TestTwoRecipients` |
| | No recipient | `archive.TestRecipients`, `main.TestEndToEnd/no_recipient` |
| | Unsupported recipient | `archive.TestRecipients` |
| | Plaintext export round trip | `archive.TestRoundTripIntactFixture` |
| | Both item lists round trip | `archive.TestBothItemListsRoundTrip` |
| | Empty archived item list | `archive.TestEmptyArchivedItemList` |
| | Archived item list absent | `archive.TestArchivedItemListAbsent` (also each of the other three documents) |
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
| | Entry differs from the manifest | `archive.TestReaderScenarios/entry_differs_from_the_manifest`, `archive.TestReaderScenariosForTheArchivedItemList`, `main.TestFailedVerification/damaged_entry` |
| | Entry not listed | `archive.TestReaderScenarios/entry_not_listed`, `archive.TestReaderScenariosForTheArchivedItemList` |
| | Listed entry missing | `archive.TestReaderScenarios/listed_entry_missing`, `archive.TestReaderScenariosForTheArchivedItemList` |
| | Item count differs from the export | `archive.TestReaderScenarios/item_count_differs_from_the_export` |
| | Truncated archive file | `archive.TestTruncatedArchive`, `main.TestFailedVerification/truncated_archive` |
| | Altered byte | `archive.TestAlteredByte`, `main.TestFailedVerification/altered_byte` |
| | Identity of another key pair | `archive.TestIdentityOfAnotherKeyPair`, `main.TestFailedVerification/identity_of_another_key_pair` |
| | Partial generation | `main.TestPartialGeneration`, `main.TestDirectoryWithoutArchive` |
| | Partial generation reached through another path | `main.TestPartialGenerationByAnotherPath` |
| | Symbolic link entry | `archive.TestReaderScenarios/symbolic_link_entry` |
| | Recovery instructions not first | `archive.TestReaderScenarios/recovery_instructions_not_first` |
| | Newer format version | `archive.TestReaderScenarios/newer_format_version`, `main.TestFailedVerification/newer_format_version` |
| | Archive without the archived item list | `archive.TestArchiveWithoutTheArchivedItemList` |
| | Verified capture | `main.TestVerifiedCapture` |
| | Unverified capture | `main.TestUnverifiedCapture` |
| | Failed verification | `main.TestFailedVerification` |
| | Generation with sentinel values | `main.TestGenerationWithSentinelValues` |

## Test Run of 14 November 2026, archive-generation

In the dev container on Debian 12, Linux 6.8 aarch64, with Go 1.27.3, age 1.2.1, GNU tar 1.34 and OpenSpec 2.0.0: `gofmt -l .` printed nothing, and `go vet ./...`, `go test ./...` and `go test -race ./...` passed in all packages. The concurrency test passed 100 runs in a row with the race detector. `GOOS=darwin` and `GOOS=windows` builds and `go vet` passed for `./...`. The key-material search above printed nothing. The recovery commands of the [recovery page](../user/recovery.md) were run as written against a generation left by the opt-in test. Evidence level: mock.

The module cache of this container belonged to root, so the session used a `GOPATH` under the ignored `.local/` ([integrations](integrations.md)). After the image was rebuilt on 14 November 2026, `/go/pkg/mod` belongs to `vscode`, and `go mod download` and `go test ./...` passed with the default `GOPATH=/go`.

Not run: macOS, Windows, Linux/amd64, FAT or exFAT media, network shares, the manual offline exercise on a second system, attachments larger than a few hundred kilobytes, any live capture and the real `bw` binary.

## bw Process and Profile Tests

The change bw-process-adapter adds tests in two packages. Neither needs a network, an account or a `bw` installation in its default run.

- `internal/bwcli`: the client runs against a fake `bw`, the `main` package `internal/bwcli/fakebw`. `TestMain` builds it once per package run into a temporary directory, so it works on Linux, macOS and Windows without a shell or Node. The fake reads a scenario file from the data directory it receives in `BITWARDENCLI_APPDATA_DIR` and appends one record per start: arguments, the full environment, the working directory and whether standard input was empty. Tests check the allowlist, the session key rule and the arguments from these records. Each behavior of the fake names its evidence in the package comment: the [code notes](../research/bw-cli-code-notes-2026-11-15.md), the [live probe](../research/vaultwarden-live-probe-2026-11-14.md) or an unauthenticated run. The login tests use a fake terminal, because `go test` has none.
- `internal/profile`: temporary directories as profile location, an injected clock, random source and removal function, and a logout function supplied by the test. Every pre-existing file in the location is hashed before and after the listing of leftovers.

The fake can drift from the real `bw`. The tests `TestRealBW…` in `internal/bwcli` compare the cases that need no login with the real program. They run only when `BW_TAKEOUT_REAL_BW` holds the absolute path of a `bw` executable, and report a skip otherwise. Run them by hand in the dev container:

```sh
BW_TAKEOUT_REAL_BW="$(command -v bw)" go test -count=1 -run TestRealBW -v ./internal/bwcli/
```

They are not offline. Each test creates a fresh profile, and the first start of a fresh profile sends one anonymous configuration request to `https://api.bitwarden.com` (finding 12 of the code notes). `bw` has no switch to turn this off. CI does not run them, because CI has no `bw`. Cases with a login wait for an automated Vaultwarden test setup.

| Test | Real behavior checked | Fake behavior it corresponds to |
|---|---|---|
| `TestRealBWVersionAndHash` | version line, SHA-256 of the resolved file, `data.json` created in the given directory | version line and `data.json` |
| `TestRealBWStatusWithoutLogin` | status `unauthenticated` without `lastSync` | status `unauthenticated` without `lastSync` |
| `TestRealBWNotLoggedIn` | class `not_logged_in` for sync, item list and plaintext export | failure followed by an unauthenticated status |
| `TestRealBWCleanExitInCallingEnvironment` | `BW_CLEANEXIT=true` in the calling environment does not reach `bw`, the sync still fails | exit code 0 when `BW_CLEANEXIT` is present |
| `TestRealBWLoginWithoutTerminal` | the real terminal check refuses the login with `/dev/null` on standard input | login refused by the fake terminal |

### Run of 15 November 2026 with the real bw

Dev container, Debian 12, Linux aarch64, Go 1.27.3, `bw` 2026.10.1 in the npm layout, SHA-256 of `bw.js` `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. All five tests passed, in 19.6 s together. Each sent one configuration request to Bitwarden Cloud. Not run: the official single-file `bw` for macOS or Windows, and any case with a login.

### Manual Login Exercise

The login cannot run in `go test`, because it needs a terminal and a person who types. The development tool `internal/tools/loginprobe` runs it by hand against a Vaultwarden instance with a synthetic account. It creates a profile, queries the version, sets the server, logs in, reads the status, syncs, reads the status again, logs out and tears down. It prints each step with ok or the failure class, the form of the session key, the status values, whether `lastSync` is later than the start, and the number of `user_` entries left in the data file after the logout. It never prints the session key or any text of `bw`. The logout runs after every login attempt, also a failed one, because `bw` can be logged in although the client refused its session key. The tool did this only after a successful login until the review of 15 November 2026, so the runs of that day read the data file after successful logins only.

The procedure follows the [live probe](../research/vaultwarden-live-probe-2026-11-14.md). Vaultwarden runs inside the dev container on loopback, and the port is forwarded to the host. All files go to the ignored `.local/` directory. Run every command from the repository root.

**1. Fetch Vaultwarden 1.37.4.** The image index is pinned by digest. The manifest for the architecture of the container is taken from the index, and every blob is checked against its digest before it is used. Only the server binary `vaultwarden` and the directory `web-vault/` are unpacked, not the Alpine system around them. Docker is not needed.

```sh
VW="$PWD/.local/vaultwarden"
mkdir -p "$VW/blobs" "$VW/root" "$VW/data"
REPO=vaultwarden/server
INDEX=sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb
ARCH=$(go env GOARCH)
TOKEN=$(curl -fsS "https://auth.docker.io/token?service=registry.docker.io&scope=repository:$REPO:pull" | jq -r .token)
fetch() {
  curl -fsSL -H "Authorization: Bearer $TOKEN" -H "Accept: $3" \
    -o "$VW/blobs/${2#sha256:}" "https://registry-1.docker.io/v2/$REPO/$1/$2" &&
  echo "${2#sha256:}  $VW/blobs/${2#sha256:}" | sha256sum -c --quiet
}
fetch manifests "$INDEX" application/vnd.oci.image.index.v1+json
MANIFEST=$(jq -r --arg a "$ARCH" '.manifests[] | select(.platform.os == "linux" and .platform.architecture == $a) | .digest' "$VW/blobs/${INDEX#sha256:}")
fetch manifests "$MANIFEST" application/vnd.oci.image.manifest.v1+json
for d in $(jq -r '.config.digest, .layers[].digest' "$VW/blobs/${MANIFEST#sha256:}"); do fetch blobs "$d" '*/*'; done
for d in $(jq -r '.layers[].digest' "$VW/blobs/${MANIFEST#sha256:}"); do
  tar -tzf "$VW/blobs/${d#sha256:}" | grep -E '^(vaultwarden$|web-vault/)' >"$VW/members" || continue
  tar -xzf "$VW/blobs/${d#sha256:}" -C "$VW/root" --no-recursion -T "$VW/members"
done
"$VW/root/vaultwarden" --version
sha256sum "$VW/root/vaultwarden"
cat "$VW/root/web-vault/vw-version.json"
```

On Linux arm64 the manifest is `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4`, the binary prints `Vaultwarden 1.37.4` and has the SHA-256 of the live probe, `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d`, and the web vault file names version 2026.7.0. A different value stops the exercise. The line `Web-Vault Version file missing` after the version is expected, because the binary looks for the web vault in its default place.

**2. Create a certificate for `localhost`,** valid for seven days. `bw` refuses servers without HTTPS and trusts this certificate through `NODE_EXTRA_CA_CERTS`, which the tool sets from `-ca`.

```sh
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 7 \
  -subj /CN=localhost -addext subjectAltName=DNS:localhost,IP:127.0.0.1 \
  -keyout "$VW/key.pem" -out "$VW/cert.pem"
chmod 600 "$VW/key.pem"
```

**3. Start the server** in its own terminal of the dev container. Sign-ups are open, SMTP is not configured and icon downloads are off. Stop it later with Ctrl-C.

```sh
VW="$PWD/.local/vaultwarden"
DATA_FOLDER="$VW/data" WEB_VAULT_FOLDER="$VW/root/web-vault" \
ROCKET_ADDRESS=127.0.0.1 ROCKET_PORT=8087 \
ROCKET_TLS="{certs=\"$VW/cert.pem\",key=\"$VW/key.pem\"}" \
DOMAIN=https://localhost:8087 SIGNUPS_ALLOWED=true DISABLE_ICON_DOWNLOAD=true \
"$VW/root/vaultwarden"
```

Check it from another terminal with `curl -fsS --cacert "$VW/cert.pem" https://localhost:8087/alive`, which prints a time.

**4. Forward the port.** VS Code forwards port 8087 to the host when the server starts listening. Otherwise add it in the Ports view. The local address must be `localhost:8087`, because the certificate and `DOMAIN` name it. The instance is then reachable from the host only.

**5. Register the account.** The maintainer opens `https://localhost:8087` in a browser on the host, accepts the self-signed certificate, registers a synthetic account with an address under `example.invalid` and a password used nowhere else, and enables two-step login with an authenticator app. Nothing else goes into the vault. The password and the TOTP seed never go into the repository, a file or the chat.

**6. Build the tool** for the dev container and for macOS:

```sh
go build -o .local/bin/loginprobe ./internal/tools/loginprobe
GOOS=darwin GOARCH=arm64 go build -o .local/bin/loginprobe-darwin-arm64 ./internal/tools/loginprobe
```

**7. Run the five cases in a terminal of the dev container** (Linux arm64, `bw` in the npm layout). The terminal must be a real one, not the output panel of a task. Use a second terminal, because the server runs in the first. Define the call once from the repository root, then run the cases one after the other:

```sh
VW="$PWD/.local/vaultwarden"
probe() { .local/bin/loginprobe -bw "$(command -v bw)" -server https://localhost:8087 -ca "$VW/cert.pem"; }
echoed() { stty -a | tr ' ;' '\n\n' | grep -x -- '-\{0,1\}echo'; }
```

| Case | Command | What the person does |
|---|---|---|
| 1. Successful login with TOTP | `probe; echo "exit $?"; echoed` | Enter address, password and code |
| 2. Wrong password | `probe; echo "exit $?"; echoed` | Enter address and a wrong password |
| 3. Interrupt at the password prompt | `probe; echo "exit $?"; echoed` | Enter the address, then press Ctrl-C at the password prompt |
| 4. Standard input from `/dev/null` | `probe </dev/null; echo "exit $?"; echoed` | Nothing. No prompt may appear |
| 5. Standard output of the tool to a file | `probe >"$VW/probe-out.txt"; echo "exit $?"; echoed; cat "$VW/probe-out.txt"; grep -cE '[A-Za-z0-9+/]{80,}' "$VW/probe-out.txt"` | As in case 1. The prompts still appear, because they go to standard error |

`echoed` prints `echo` when the terminal echoes typed characters and `-echo` when it does not. In case 5 the `grep` counts lines that could hold a session key and must print 0.

**8. Run the two terminal-dependent cases on macOS** with the official single-file `bw`, which is not the npm package under Node, and with the tool built for darwin/arm64. Saving and restoring the terminal state uses other system calls there. In a folder outside the project, in Terminal on the Mac:

```sh
curl -fLO https://github.com/bitwarden/clients/releases/download/cli-v2026.10.1/bw-macos-arm64-2026.10.1.zip
shasum -a 256 bw-macos-arm64-2026.10.1.zip
unzip bw-macos-arm64-2026.10.1.zip
./bw --version
```

The hash must be `4e595f9da80ef8b30abca808d0c78a1560d9589466d1bccd2b3e70f321fb12bf`, the digest that GitHub lists for this asset of the release `cli-v2026.10.1`. Copy `.local/bin/loginprobe-darwin-arm64` and `.local/vaultwarden/cert.pem` from the dev container into the same folder. Then check the forwarded port with `curl -fsS --cacert cert.pem https://localhost:8087/alive` and define the call:

```sh
probe() { ./loginprobe-darwin-arm64 -bw "$PWD/bw" -server https://localhost:8087 -ca "$PWD/cert.pem"; }
echoed() { stty -a | tr ' ;' '\n\n' | grep -x -- '-\{0,1\}echo'; }
```

Run case 1 and case 3 of the table above. The other three cases do not depend on the platform and are not repeated (maintainer decision of 15 November 2026). Do not run the macOS cases when the login fails in the dev container.

**What to record.** For each run: the system, the case, every line the tool printed, the exit status, the output of `echoed`, which prompts appeared, and anything unexpected on the terminal. Once per system: the output of `bw --version`, the SHA-256 of the resolved `bw` file (`sha256sum "$(readlink -f "$(command -v bw)")"` in the dev container, `shasum -a 256 bw` on macOS), `go version`, the operating system version (`uname -a`, `sw_vers` on macOS), and whether the profile location is empty after the last run (`ls -A ~/.cache/bw-takeout/profiles` in the dev container, `ls -A ~/Library/Caches/bw-takeout/profiles` on macOS). Once for the server: the Vaultwarden version, the web vault version and, if the web vault shows them, the number of device entries of the account after all runs. Never record the session key, the password, the code, the TOTP seed or text that `bw` wrote to the terminal beyond the prompt names.

**9. Clean up.** Stop the server with Ctrl-C, remove `.local/vaultwarden` and `.local/bin/loginprobe*` in the dev container, remove the folder on the Mac, and delete the entry of the synthetic account from the authenticator app.

### Login Exercise of 15 November 2026

The maintainer ran the five cases in the dev container (Linux arm64, npm `bw` 2026.10.1) and on macOS 26.7.3 arm64 (official single-file `bw` 2026.10.1) the successful login, the interrupt at the password prompt and, in addition, a wrong password, against Vaultwarden 1.37.4 in the dev container. Every case gave the expected failure or success, and the terminal echoed afterwards in every run. An interrupt at the password prompt failed the login with the class `output`, because `bw` ended with exit code 0 and no session key and no signal reached the Go parent. Versions, results, what the logout left in the profile and what was not tested are in the [report](../research/login-exercise-2026-11-15.md). Evidence level: manual end-to-end run with a synthetic account, Vaultwarden only.

### Spec Scenario Coverage of bw-process-adapter

Every scenario of the two specs of the change bw-process-adapter has one test of the same name. The mapping was checked on 15 November 2026 and is complete: 42 scenarios of bw-invocation in `internal/bwcli` and 12 of bw-profile in `internal/profile`. The tests of `internal/bwcli` run against the fake `bw`, those of `internal/profile` against temporary directories. `TestInterruptAtThePasswordPrompt` is built on Unix only, because it sends an interrupt signal to the fake. `TestPermissionsOnLinuxAndMacOS` is skipped on Windows. The scenarios "Login with a terminal" and "Interrupt at the password prompt" were also run by hand with the real `bw`, see the login exercise above.

| Spec | Requirement | Scenario | Test |
|---|---|---|---|
| bw-invocation | Child environment from an allowlist | Exit code override in the calling environment | `bwcli.TestExitCodeOverrideInTheCallingEnvironment` |
|  |  | Runtime options in the calling environment | `bwcli.TestRuntimeOptionsInTheCallingEnvironment` |
|  |  | Certificate authority file configured | `bwcli.TestCertificateAuthorityFileConfigured` |
|  |  | Certificate authority file only in the calling environment | `bwcli.TestCertificateAuthorityFileOnlyInTheCallingEnvironment` |
|  | Every process uses the run's data directory | Version query | `bwcli.TestVersionQuery` |
|  |  | bw uses another data directory | `bwcli.TestBwUsesAnotherDataDirectory` |
|  | No interaction outside the login | Locked vault | `bwcli.TestLockedVault` |
|  |  | Command does not end | `bwcli.TestCommandDoesNotEnd` |
|  |  | Run is cancelled | `bwcli.TestRunIsCancelled` |
|  | Session key stays in the child environment | Command after login | `bwcli.TestCommandAfterLogin` |
|  |  | Command before login | `bwcli.TestCommandBeforeLogin` |
|  |  | Command after logout | `bwcli.TestCommandAfterLogout` |
|  |  | Failure with a session key in use | `bwcli.TestFailureWithASessionKeyInUse` |
|  | Interactive login needs a terminal | Login without a terminal | `bwcli.TestLoginWithoutATerminal` |
|  |  | Login with a terminal | `bwcli.TestLoginWithATerminal` |
|  | Session key form decides the login result | Exit code 0 with empty output | `bwcli.TestExitCode0WithEmptyOutput` |
|  |  | Valid session key | `bwcli.TestValidSessionKey` |
|  |  | Session key followed by a line ending | `bwcli.TestSessionKeyFollowedByALineEnding` |
|  |  | Other text next to the key | `bwcli.TestOtherTextNextToTheKey` |
|  |  | Non-zero exit code with a key | `bwcli.TestNonZeroExitCodeWithAKey` |
|  | Interrupted login leaves a usable terminal | Interrupt at the password prompt | `bwcli.TestInterruptAtThePasswordPrompt` |
|  | Output stays in memory | Export | `bwcli.TestExport` |
|  |  | bw writes a file | `bwcli.TestBwWritesAFile` |
|  |  | Output exceeds the limit | `bwcli.TestOutputExceedsTheLimit` |
|  | Results are read from the output | Status of a profile without login | `bwcli.TestStatusOfAProfileWithoutLogin` |
|  |  | Status without a sync time | `bwcli.TestStatusWithoutASyncTime` |
|  |  | Status document does not parse | `bwcli.TestStatusDocumentDoesNotParse` |
|  |  | Unknown status value | `bwcli.TestUnknownStatusValue` |
|  | Failures carry a class and no CLI text | Failed download with a name in the error text | `bwcli.TestFailedDownloadWithANameInTheErrorText` |
|  |  | Not logged in | `bwcli.TestNotLoggedIn` |
|  |  | Not-logged-in text with an unlocked vault | `bwcli.TestNotLoggedInTextWithAnUnlockedVault` |
|  |  | Status query after a failure fails | `bwcli.TestStatusQueryAfterAFailureFails` |
|  |  | Premium required | `bwcli.TestPremiumRequired` |
|  |  | Unrecognized error text | `bwcli.TestUnrecognizedErrorText` |
|  | Decryption error lines are counted | Export with decryption errors | `bwcli.TestExportWithDecryptionErrors` |
|  |  | Export without error lines | `bwcli.TestExportWithoutErrorLines` |
|  | Identity of bw is reported and never judged | Version unknown to this project | `bwcli.TestVersionUnknownToThisProject` |
|  |  | Version output is empty | `bwcli.TestVersionOutputIsEmpty` |
|  | Attachments are requested by ID | Download by ID pair | `bwcli.TestDownloadByIDPair` |
|  |  | ID that starts with a dash | `bwcli.TestIDThatStartsWithADash` |
|  | Server address is checked before use | Address without HTTPS | `bwcli.TestAddressWithoutHTTPS` |
|  |  | HTTPS address | `bwcli.TestHTTPSAddress` |
| bw-profile | Fresh profile for every run | Two runs in the same second | `profile.TestTwoRunsInTheSameSecond` |
|  |  | Chosen name already exists | `profile.TestChosenNameAlreadyExists` |
|  |  | Profile location is missing | `profile.TestProfileLocationIsMissing` |
|  | Profile is readable by the owner only | Permissions on Linux and macOS | `profile.TestPermissionsOnLinuxAndMacOS` |
|  | Separate data and working directories | New profile | `profile.TestNewProfile` |
|  | Teardown removes the profile | Teardown after a successful run | `profile.TestTeardownAfterASuccessfulRun` |
|  |  | Logout fails | `profile.TestLogoutFails` |
|  |  | Removal fails | `profile.TestRemovalFails` |
|  |  | Teardown without login | `profile.TestTeardownWithoutLogin` |
|  | Leftover profiles are reported and never removed | Leftover from a killed run | `profile.TestLeftoverFromAKilledRun` |
|  |  | Own profile | `profile.TestOwnProfile` |
|  |  | Unrelated entries in the profile location | `profile.TestUnrelatedEntriesInTheProfileLocation` |

### Test Run of 15 November 2026, bw-process-adapter

In the dev container on Debian 12, Linux 6.8 aarch64, with Go 1.27.3 and OpenSpec 2.0.0: `gofmt -l .` printed nothing, `go vet ./...` passed, `go test -count=1 -json ./... | go run ./internal/tools/skipreport` passed in all packages with only expected skips, and `go test -race ./...` passed. The skips were `TestLeaveGenerationForOfflineExercise`, `TestKillHelper` and the five `TestRealBW…` tests, each with its reason. `GOOS=darwin` and `GOOS=windows` builds and `go vet` passed for `./...`. Evidence level: mock, against the fake `bw`.

Not run in the dev container: the tests on macOS, Windows and Linux amd64, which ran in CI afterwards, see [CI run of bw-process-adapter](#ci-run-of-15-november-2026-bw-process-adapter); the `TestRealBW…` tests in this run, which ran separately on the same day (see above); any automated test with a login; Bitwarden Cloud.

### Test Run of 16 November 2026, archived-item-list

In the dev container on Debian 12, Linux 6.8 aarch64, with Go 1.27.3 and OpenSpec 2.0.0: `gofmt -l .` printed nothing, `go vet ./...` passed, `go test -count=1 -json ./... | go run ./internal/tools/skipreport` passed in all packages with only expected skips, and `go test -race ./...` passed. The skips were `TestLeaveGenerationForOfflineExercise`, `TestKillHelper` and the five `TestRealBW…` tests, each with its reason. `TestRecoveryWithStandardTools` ran with the `age` and `tar` commands and unpacked the four documents byte for byte. `GOOS=darwin` and `GOOS=windows` `go vet ./...` passed. The key-material search printed nothing. Evidence level: mock.

Not run in the dev container: the tests on macOS, Windows and Linux amd64, which ran in CI afterwards, see [CI run of archived-item-list](#ci-run-of-16-november-2026-archived-item-list); any test with a real capture, an account that may not archive, the trash, or an archived item with an attachment.

## Skip Report

A skipped test is not a passed test. The program `internal/tools/skipreport` reads the output of `go test -json`, echoes it, and lists every skipped test with its reason. It exits with status 1 when a test failed or when a test was skipped that the list of expected skips does not allow for the platform. Run it the same way locally and in CI:

```sh
go test -count=1 -json ./... | go run ./internal/tools/skipreport
```

Use `bash` with `-o pipefail`, so that a failing `go test` fails the pipe. The table goes to the job summary when `GITHUB_STEP_SUMMARY` is set and to standard output otherwise. Only the standard library is used, and no other package imports the program.

`.github/expected-skips.txt` lists the allowed skips, one per line: platform (`linux`, `darwin`, `windows` or `*`), package, test name, then `#` and the reason. An entry allows a skip and does not require it; entries that did not skip are shown as information. An entry also covers the subtests of its test. A malformed line or a line without reason stops the program with status 2.

Rule for a new entry: it needs a reason that holds in review, for example a platform limit named in a spec or a non-goal. A test that should run, such as `TestRecoveryWithStandardTools`, stays off the list, so that a missing tool fails the run instead of hiding.

## CI Run of 15 November 2026

The workflow `.github/workflows/ci.yml` ran on GitHub Actions after the commit `7d13507`. The maintainer reports that all four jobs passed: `test` on `ubuntu-24.04`, `macos-26` and `windows-2025`, and `checks` on `ubuntu-24.04`. A passing `test` job means that vet, the tests with the skip report and the race tests all passed on that runner, and that every skip was on the list of expected skips. The first run on Windows failed in four tests, which were fixed in the tests and not in product code, see the change repository-ci-setup.

Evidence level: mock, on three platforms. The runner image versions, the Go and age versions, the durations and the runner minutes were not recorded, because the maintainer did not need them. This page therefore names the platforms by runner label only.

CI covers the tests on Linux amd64, macOS arm64 and Windows amd64 with the file systems of the runners (ext4, APFS and NTFS, as far as the images use them). It does not cover Linux arm64, which stays with the dev container, nor FAT or exFAT media, network shares, the manual offline exercise on a second system, any live capture or the real `bw` binary. CI passing is not a statement of platform support.

The run on FAT or exFAT media is deferred to the change `interactive-backup-run` (maintainer decision of 15 November 2026). Until a backup command exists, no user writes to such a target. A failure there would show as an error with a partial generation left behind, not as a backup that only looks successful. The weak guarantees of these file systems after a power loss are stated in the [limitations](../user/limitations.md) and are beyond what a test run shows. The run must pass before a first release.

## CI Run of 15 November 2026, bw-process-adapter

The workflow ran on the branch `bw-process-adapter` after the commit `e522570`. It was the first run with the tests of `internal/bwcli`, `internal/profile` and `internal/tools/loginprobe`, and with the fake `bw` built on each runner. The maintainer reports that it passed on all three platforms, `ubuntu-24.04`, `macos-26` and `windows-2025`.

The run before it, on the commit `2ff6abb`, passed on Linux and failed in two tests, which were fixed in the tests and not in product code:

- `TestUnrelatedEntriesInTheProfileLocation` failed on macOS and Windows. It created a file and a directory whose names differ only in case, and these collide on the case-insensitive file systems of both runners. The directory now has another random part.
- `TestLoginWithATerminal` failed on macOS. It compared the working directory as a string, while the temporary directory on macOS is reached through the symbolic link `/var` and the fake reports the resolved path. The test now compares the directories by identity.

Both failures were reproduced in the dev container with `TMPDIR` on a symbolic link into the case-insensitive bind mount of the macOS host. The fixed tests and the whole suite with the skip report passed there.

Evidence level: mock, on three platforms, against the fake `bw`. Runner image versions and durations were not recorded. The interactive login is not covered by CI. It was exercised by hand on Linux and macOS, see the [login exercise](#login-exercise-of-15-november-2026).

## CI Run of 16 November 2026, archived-item-list

The workflow ran on the branch `archived-item-list` after the commit `0c78002`. The maintainer reports that it passed on all three platforms, `ubuntu-24.04`, `macos-26` and `windows-2025`. The skip report passed on each runner, so every skip was one that `.github/expected-skips.txt` allows. The change added no skip, and the file was not changed. `TestRecoveryWithStandardTools` is not on the list, so it ran on every runner with the four documents of the new layout.

Evidence level: mock, on three platforms. Runner image versions and durations were not recorded. No test ran against a real capture, an account that may not archive, the trash or an archived item with an attachment.

After this run the workflow was split to save runner minutes: `ci.yml` keeps the test matrix and runs only on changes to Go code, the module files, the list of expected skips or itself, and on pushes to `main` only; the new `checks.yml` runs formatting, module tidiness and the key-material search on every change. The first run after the split failed in `TestWorkflowStructure`, which still expected the checks in `ci.yml`. The test now covers both workflows and the path filters. After the commit `005b30f` the maintainer started both workflows by hand on the branch, and both passed.

## Offline Exercise of 15 November 2026

The maintainer opened a synthetic generation on macOS 26.7.3 on Apple Silicon, following a checklist that repeats the steps of the [recovery page](../user/recovery.md). The generation was left by `TestLeaveGenerationForOfflineExercise` in the dev container, encrypted to a key pair made only for this exercise. `bw-takeout` was built for darwin/arm64 from commit `8b353f4`, and age v1.2.1 for darwin/arm64 from the pinned module. Both were copied with the generation into a folder outside the project. The identity was kept in a separate folder outside the project and outside that folder.

Results, as reported by the maintainer:

- macOS ran both programs without blocking them. `age --version` printed v1.2.1, and `bw-takeout` without arguments ended with status 2.
- `bw-takeout verify` printed the expected generation, source, counts and the verdict verified, and ended with status 0.
- `age -d … | tar -tvf -` listed seven entries, starting with `RECOVERY.txt` and ending with `manifest.json`.
- Unpacking with age and tar produced the documented layout. `RECOVERY.txt` was understandable without further help.
- `sha256sum` exists on this macOS version. The hash of `documents/export.json` matched the manifest.
- Both attachments matched a reference list computed from the fixed attachment contents in the fixture source, not from the archive.
- Nothing was installed or looked up beyond what the exercise folder held.

Evidence level: mock, synthetic data only. Limits of this exercise:

- The Mac is the host of the dev container that wrote the generation. It is a second operating system, not an independent machine.
- The identity was in a separate folder on the same disk, not on removable media.
- The network stayed on, by maintainer decision. The maintainer reports that nothing was fetched.
- The recovery page has shell commands for Unix-like systems only. Recovery on Windows was not exercised.

## Bootstrap Check of 13 November 2026

Docker build and start as user vscode on Linux/arm64 succeeded. Go 1.27.3, bw 2026.10.1, age 1.2.1 and OpenSpec 2.0.0 were run in the image. A synthetic age round trip succeeded without network access. SHA-256 of bw's installed npm entry point (build/bw.js): `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`. Package integrity is recorded separately in the npm lockfile.

OpenSpec init generated six shared skills. The Claude reference for Explore is readable in the container. openspec list reported no active changes. For a temporary change, the proposal instructions contained the project-specific documentation rule. The VS Code UI, the personal Claude feature, Linux/amd64 and authenticated vaults were not tested.
