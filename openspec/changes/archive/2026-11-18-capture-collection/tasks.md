# Tasks

## Recommended apply setup

Model Opus, effort high: the change spans five packages and has a code-reading task with a stop condition. Its spec comparison and secret-handling tests need careful reading more than speed, and three review checkpoints already break the session into parts.

## 1. Evidence note

- [x] 1.1 Read `ListCommand`, `ExportCommand` and `GetCommand` of the installed bundle `build/bw.js` in full, together with the services they call on the way to their output. Check whether any of them starts a sync, and whether the CLI runs a sync outside a command: a timer, a notification or the account migrations of finding 16. Verify the SHA-256 of the bundle matches the one in the package documentation of `internal/capture`. **Stop condition:** if any sync is found, stop and show the maintainer the finding before any task of group 2 starts, because design D3 and the dropped comparison depend on it.
- [x] 1.2 Write `docs/research/bw-cli-code-notes-2026-11-17.md` with the findings of task 1.1: bundle and SHA-256, line numbers, quoted expressions, the evidence level "code reading", and what stays unobserved at run time. Link it from `docs/research/research-summary.md`. Verify every quoted expression with `grep -n` against the bundle, and verify with `git diff --stat docs/research` that only the summary and the new file changed.

## 2. Capture model

- [x] 2.1 Add the struct `DecryptionErrors` with one count per document to `capture.Input` and `capture.Capture`, have `capture.Parse` copy it, and add `DecryptionErrors int` to `capture.Download` (design D9). Verify parser tests show that the counts reach the capture unchanged and that an input without counts parses as before.
- [x] 2.2 Add `FileNames map[AttachmentRef]string` to `capture.ItemList`, filled by `ParseItemList` with the first occurrence winning, and an empty name for an absent or non-string `fileName` (design D5). Verify tests for two attachments with the same name, a missing `fileName`, a numeric `fileName` that does not reject the list, and a duplicate ID pair.
- [x] 2.3 Add `capture.PersonalAttachments(lists ...*ItemList) []AttachmentRef` (design D6). Verify tests for an ID pair in both lists, an organization item, a duplicate within one list, a nil list and the order of the result.
- [x] 2.4 Update the package documentation of `internal/capture`: the counts, the file names as vault content kept for the manifest only, and the link to the note of task 1.2. Verify `go doc ./internal/capture` shows the new fields and the function.

## 3. Verification

- [x] 3.1 Build the expected attachment set in `verify` from `capture.PersonalAttachments` instead of its own loop (design D6). Verify that every existing test of `internal/verify` passes unchanged and that the serialized result of every `capturetest` variant is byte-identical before and after the change.
- [x] 3.2 Add the reasons `document_decryption_errors` (unverified, with document and count) and `attachment_decryption_errors` (warning, with item ID, attachment ID and count), widen the comment on `Finding.Count`, and add both reasons to `verify.Reasons()` (design D9). Verify tests named after the four scenarios of the `capture-verification` delta, one test per document for the unverified finding, and a test for a negative count.
- [x] 3.3 Verify the cross-cutting tests pass for every variant, with and without the reference inventory, including a variant with decryption error counts: no sensitive value in the serialized result, every finding with severity, known reason and IDs, and byte-identical output of two runs.
- [x] 3.4 Add one sentence to `docs/decisions/0004-completeness-verdict.md` saying that decryption error lines on a document are a check that did not pass and lead to unverified. Keep the status proposed. Verify `docs/decisions/README.md` still lists it as proposed.
- [x] 3.5 Update "What a Verdict Can and Cannot Detect" in `docs/user/limitations.md`: decryption error lines on a document lead to unverified, lines on an attachment download only to a warning, and the lines name neither item nor field. Verify the page states both consequences and links the live probe.
- [x] 3.6 Add the scenarios of the `capture-verification` delta to the scenario coverage table in `docs/development/testing.md`. Verify `go test -list` shows a test for every row.
- [x] 3.7 Review checkpoint: show the maintainer the output of `go test -v ./internal/capture/... ./internal/verify/...`, with every skipped test listed separately. Verify the maintainer confirmed before any task of group 4 starts.

## 4. Collection

- [x] 4.1 Create `internal/collect` with the interface `Vault` and a compile-time assertion that `*bwcli.Client` satisfies it, the errors `ErrNotFresh` and `ErrNotUnlocked`, and package documentation that links this change's design (design D1). Verify `go vet ./internal/collect/...` passes and `go doc ./internal/collect` shows the interface.
- [x] 4.2 Write the in-memory `Vault` of the unit tests: answers from a queue per method, recorded calls, a fixed clock and a wait that records its durations and returns at once (design D11). Verify a test of the test double itself fails when a call has no queued answer left.
- [x] 4.3 Implement `Snapshot`: `t0` truncated to milliseconds before the forced sync, the status check, the four documents in the order of design D4, the counts, the source and the plan with file names (designs D2 to D6). Verify unit tests named after the scenarios of the requirements "Collection starts with a forced sync", "Snapshot must be fresh", "Source comes from the status", "Four documents are collected" and "Attachments of personal items are downloaded once". Also cover a `lastSync` equal to `t0`, a `lastSync` one millisecond before `t0`, and a failure of each of the four document commands, with the recorded calls showing that nothing after the failure was requested.
- [x] 4.4 Implement `(*Snapshot).Wipe`, which clears the document bytes (design D10). Verify a test that the four slices hold only zero bytes after the call.
- [x] 4.5 Implement `Attachments`: the plan in order, the SHA-256 and the size, `receive`, the clearing of the content after `receive` returns, and one download result per planned pair (designs D2 and D10). Verify unit tests named after the scenarios of the requirements "Attachments are handed on one at a time", "Download results record hash and size" and "Decryption error counts are passed on". Also verify that the slice given to `receive` holds only zero bytes after `receive` returns.
- [x] 4.6 Implement the classification and the retries of designs D7 and D8. Verify one unit test per row of the table in D7, including the number of attempts and the download error class. Also verify unit tests named after the scenarios of "Transient download failures are retried", "Premium failure is a failed download" and "Run-level failures end the collection", a cancellation during a wait that ends the collection, and the recorded waits of 2 and 5 seconds.
- [x] 4.7 Add a "Using bw" paragraph to `docs/user/limitations.md` on the failures that end a run: a failed or stale sync, a failed document command, and a session lost during the downloads, each without a generation. Add the retries of a failed download, and a premium failure as a failed download, so that an account with lapsed premium and personal attachments gets no generation. Add the time limit of 5 minutes per attempt, with the bandwidth a 100 MB and a 500 MB attachment need, and the consequence that an attachment too large for the limit fails in every run. Verify the page names the three retried failure kinds, the premium consequence and the time limit.
- [x] 4.8 Change `bwcli.DefaultAttachmentTimeout` from 30 to 5 minutes and update its comment with a reference to design D12 of this change. Add a test in `internal/bwcli` that a client built without an attachment limit uses 5 minutes, and that a limit the caller sets is kept. Verify `go test ./internal/bwcli/...` passes and `grep -rn "30 \* time.Minute" internal` prints nothing.

## 5. Integration against the fake bw

- [x] 5.1 Write integration tests in `internal/collect` that start a `bwcli.Client` against the fake `bw` built by `bwclitest`, with the documents of the intact `capturetest` fixture as answers (design D11). Run snapshot, `archive.NewWriter`, attachments, `verify` and `publish.Publish` to a published generation. Verify the generation opens with `archive` and the test identity, that its documents and attachments are byte-identical to the fixture, and that the manifest names the source from the fake status.
- [x] 5.2 Add integration tests for a stale `lastSync`, a failing sync, decryption error lines on the plaintext export and on one download, the premium sentence on one download, and a sentinel value in an error text. Verify each test checks the outcome of its spec scenario: no generation and no partial directory for the first two, the verdict unverified with a published `.unverified` generation for the plaintext export, a warning for the download, no published generation for the premium case, and no sentinel in any error value or result.
- [x] 5.3 Add the scenarios of the `capture-collection` delta to the scenario coverage table in `docs/development/testing.md`, each with its unit or integration test, and add a line saying that a retry with a later success is covered at unit level only, because the fake gives the same command one fixed answer. Verify `go test -list` shows a test for every row.

## 6. Roadmap

- [x] 6.1 Update `docs/development/architecture.md`:
  - mark roadmap item 4 B as done with the decisions of the proposal,
  - remove the comparison before and after from the open points, with a link to the note of task 1.2,
  - add a component row for `collect`,
  - update the row of `capture`,
  - name the two phases in "Planned flow".

  Verify that every bullet of the old item 4 B is either marked done or found again under C or among the follow-ups.
- [x] 6.2 Review checkpoint: show the maintainer the changed roadmap. Verify the maintainer confirmed it.

## 7. Integration check

- [x] 7.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and the key-material search from `docs/development/testing.md` in the dev container. Verify all pass and the search prints nothing. Record the run in `docs/development/testing.md` with date, platform, Go version, skipped tests and the evidence level mock.
- [x] 7.2 Compare each requirement and scenario of the two delta specs with the behavior of the code. Verify by listing every deviation in the task result, or by stating that there is none.
- [x] 7.3 Run `openspec validate capture-collection --strict`. Verify it passes.
- [x] 7.4 Record the CI run on Linux, macOS and Windows with its skip report, as the test strategy describes. Verify the run is linked in `docs/development/testing.md` and that `.github/expected-skips.txt` matches.

## 8. Review findings of 18 November 2026

Work through this group before task 7.4.

- [x] 8.1 Make the output buffer of `bwcli` grow by allocating, copying and clearing the old array (design D13). Verify a test in `internal/bwcli` that writes in many small pieces and finds every earlier array cleared, and that `go test ./internal/bwcli/...` passes.
- [x] 8.2 Stop retrying a download that fails with the class `output` and record it as failed with the class `other` (design D7). Verify the row of the D7 table test with one attempt, a unit test named after the scenario "Output limit exceeded", the downloads bullet in the package documentation of `internal/collect`, the retried failure kinds in `docs/user/limitations.md`, and a row for the new scenario in the coverage table of `docs/development/testing.md`.
- [x] 8.3 Wrap a failure of the receiver in `ErrReceive` with fixed text and the receiver's failure as its cause (design D2). Verify a test with a receiver whose failure contains the fixture marker: `errors.Is` finds `ErrReceive` and the receiver's failure, and `err.Error()` holds no marker.
- [x] 8.4 Base the organization check of unexpected downloads and the reference comparison of listed attachments in `verify` on every entry of an item in both lists (design D6). Verify a test with an item that both lists show, once as an organization item and once as a personal item with an attachment, with the reference inventory, and that the serialized result of every `capturetest` variant stays byte-identical.
- [x] 8.5 Reword the plaintext lifetime in the package documentation of `internal/collect` and, where it applies, `internal/capture`, so that it states what is cleared and what is not (design D10). Verify the text against D10.
- [x] 8.6 Run the checks of tasks 7.1 to 7.3 again, record the run in `docs/development/testing.md`, and compare the changed requirements of the `capture-collection` delta with the code. Then continue with task 7.4.

## Workflow follow-up

- Archive the change after the maintainer's review.
- Start change C (`interactive-backup-run`) with a short explore session on its open points: core dumps on Windows, the order of the phases around the target directory, and how the report names attachments by ID.
