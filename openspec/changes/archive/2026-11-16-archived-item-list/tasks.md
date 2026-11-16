# Tasks

## 1. Evidence note

- [x] 1.1 Write `docs/research/bw-cli-code-notes-2026-11-16.md` with the three statements of the design's Context table: the bundle and its SHA-256, the line numbers, the quoted expressions, the evidence level "code reading" and what was not observed live; verify each quoted expression with `grep -n` against the installed bundle and state in the note that the SHA-256 of the bundle matches the one in the package documentation of `internal/capture`
- [x] 1.2 Link the note from `docs/research/research-summary.md`; verify the link resolves and no existing research report was changed (`git diff --stat docs/research` shows only the summary and the new file)

## 2. Capture model and fixtures

- [x] 2.1 Add `ArchivedItemList` to `capture.Input`, `ArchivedList` to `capture.Capture` and the document name `archived_item_list` (design D1), parsed with the existing list parser; verify parser tests for a valid archived list, the empty list `[]`, invalid JSON, a zero-length input and a duplicate ID, each naming the archived list as the document
- [x] 2.2 Add an archived login with one attachment to the intact fixture of `capturetest`: in both exports with an archive date, only in the archived list, with its attachment in the reference inventory (design D7); verify the fixture tests pass and `Fixture.Sensitive` covers the new item's values and attachment content
- [x] 2.3 Add the eight variants of design D7 and change the variant "exported item not listed" to remove the item from both lists; verify the existing test that compares each variant's declared changes with its actual difference to the intact fixture passes for every variant
- [x] 2.4 Update the package documentation of `internal/capture`: the archived list as a document of its own, what the live probe observed for it, and what comes from code reading only; verify it links the note of task 1.1

## 3. Verification

- [x] 3.1 Build the listed index from both lists, with an ID of both lists entered once, and report duplicates per list with that list as document (design D2); verify tests for the scenarios "Archived item with an attachment", "Archived item in neither list", "Download missing for an archived item", "Duplicate ID within the archived item list", "Archived item list cannot be parsed" (once invalid, once absent) and "Organization item in both lists", each test carrying the scenario's name
- [x] 3.2 Add the reason `listed_item_differs_between_lists` and compare organization ID and attachment ID set of an item in both lists (design D3); verify tests for "Same item with the same attachments", "Same item with different attachments" and "Same item with a different organization", and that a different order of the same attachment IDs produces no finding
- [x] 3.3 Add the warning `archive_filter_not_applied` with the count of shared item IDs (design D4); verify tests for "Archived list repeats the default list", "Disjoint lists" and "Both lists empty", and that the warning leaves the verdict unchanged in a capture that is otherwise failed, unverified and verified
- [x] 3.4 Add both reasons to `verify.Reasons()`; verify the cross-cutting tests pass for every variant with and without the reference inventory: no sensitive value in the serialized result, every finding with severity, known reason and IDs, and byte-identical output of two runs
- [x] 3.5 Check `docs/decisions/0004-completeness-verdict.md` for statements that assume one item list and update them, keeping the status proposed; verify by stating in the task result which sentences changed, or that none did
- [x] 3.6 Update `docs/development/testing.md`: the fixture with four items, the new variants and their count, and the scenario coverage table for the three new requirements; verify every scenario of the `capture-verification` delta has a row with a test that exists (`go test -list` shows each name)
- [x] 3.7 Update "What a Verdict Can and Cannot Detect" in `docs/user/limitations.md`: both lists are read, an account that may not archive gets the warning and keeps archived items as unverified, the case was not observed live, and the trash is in no document and is not backed up; verify the page no longer says that the archived listing "has to be included" and names the trash
- [x] 3.8 Review checkpoint: stop and show the maintainer the output of `go test -v ./internal/capture/... ./internal/verify/...` with every skipped test listed separately; verify the maintainer confirmed before any task of group 4 starts

## 4. Archive format

- [x] 4.1 Add `ArchivedItemList` to `archive.Documents` and the entry `documents/items.archived.json` after `documents/items.json`, and refuse a zero-length document for each of the four before anything is written (design D5); verify tests for the scenarios "Both item lists round trip", "Empty archived item list" and "Archived item list absent", and one refusal test per document that shows the destination received no byte
- [x] 4.2 Accept and require the new entry in the reader; verify a test for the scenario "Archive without the archived item list" with an archive built for the purpose whose manifest lists no such entry, and that the existing scenarios "Entry not listed", "Listed entry missing" and "Entry differs from the manifest" also pass when applied to the new entry
- [x] 4.3 Update `RECOVERY.txt` for both lists (design D6) and extend the layout test to every document name; verify the scenario "Instructions present and free of vault data" still passes
- [x] 4.4 Update `archivetest` and the tests of `cmd/bw-takeout` to four documents; verify `go test ./cmd/... ./internal/archive/... ./internal/publish/...` passes and the recovery test with the `age` and `tar` commands unpacks four documents byte-identical to the captured ones, or reports a skip with a reason where a command is missing
- [x] 4.5 Update `docs/decisions/0005-archive-format.md`: layout with four documents, "four captured documents", and as a consequence that format version 1 was redefined once before any release and that every later layout change needs a new version; keep the status proposed; verify `docs/decisions/README.md` still lists it as proposed
- [x] 4.6 Update `docs/user/recovery.md` where it names the documents, and the scenario coverage table of archive-generation in `docs/development/testing.md`; verify both name `documents/items.archived.json` and every scenario of the `backup-archive` and `offline-verification` deltas has a row with an existing test

## 5. Roadmap

- [x] 5.1 Update `docs/development/architecture.md`: replace roadmap item 4 with the changes A to D and the constraints from the proposal's section "Split of `interactive-backup-run`", keep the obligations of the old item 4 under the change that takes each of them, add the trash to the items not planned, and update the component rows of `capture`, `verify` and `archive`; verify every bullet of the old item 4 and every constraint of the proposal is found on the page
- [x] 5.2 Review checkpoint: show the maintainer the changed roadmap; verify the maintainer confirmed the assignment of obligations to B, C and D

## 6. Integration check

- [x] 6.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...` and the key-material search from `docs/development/testing.md` in the dev container; verify all pass, the search prints nothing, and record the run with date, platform, Go version, skipped tests and the evidence level mock in `docs/development/testing.md`
- [x] 6.2 Compare each requirement and scenario of the three delta specs with the behavior of the code; verify by listing every deviation in the task result, or stating that there is none
- [x] 6.3 Run `openspec validate archived-item-list --strict`; verify it passes
- [x] 6.4 Record the CI run on Linux, macOS and Windows with its skip report as the test strategy describes; verify the run is linked in `docs/development/testing.md` and `.github/expected-skips.txt` matches

## Workflow follow-up

- Archive the change after the maintainer's review.
- Start change B (`capture-collection`) with a short explore session on its open questions.
