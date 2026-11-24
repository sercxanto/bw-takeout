# Tasks

## Recommended apply setup

Model Opus, effort high. The change starts with careful code reading of a large upstream diff that has stop points, and it then adds a parser rule that touches the secret boundary of the report. A missed assumption or a leaking key would weaken the verdict without any test noticing.

## 1. Assumption register for bw 2026.11.1

- [x] 1.1 Fetch the source of `bitwarden/clients` at the tags `cli-v2026.10.1` and `cli-v2026.11.1` into `.local/` (shallow, only the paths that the rows of design D2 name, plus `libs/common/src/models/export/` and `libs/tools/export-vault-core/`). Install `@bitwarden/cli@2026.11.1` with npm into `.local/bw-2026.11.1/`, and record the hash of `build/bw.js` and the Node engine range from its `package.json`. Verify: both tags resolve to the release commits named in the GitHub releases (`28b247ac…` belongs to 2026.11.0, so look up the commit of 2026.11.1), and `.local/bw-2026.11.1/node_modules/.bin/bw --version` prints `2026.11.1` under the Node of the dev container. If the engine range excludes Node 24, stop and ask the maintainer.
- [x] 1.2 Write `docs/research/bw-cli-code-notes-<date>.md` with the table of design D2: one row per assumption, with its source, the code that depends on it, where it was checked in the 2026.11.1 source or bundle, and the result (unchanged, changed or not determinable). Include the literal decryption error marker and the premium sentence as found in the 2026.11.1 bundle, the gated or restricted item filters in personal exports, organization exports and `list items`, the "shared folder" alias, the exit after a command (PM-45308) with how it flushes standard output, and the path of `get attachment` before and after the removal of the flag `pm-28192-cipher-attachment-ops-to-sdk`. For that path, also record whether Vaultwarden 1.37.4 sets the flag (code reading of its config endpoint). Verify: every row of the D2 table has a result and a file reference. If any row reads "changed" in a way that affects the client, stop and ask the maintainer before continuing.
- [x] 1.3 In the same notes, list the keys of each place of design D4 for 2026.10.1 and 2026.11.1, read from the export model classes and the document types, and the keys that 2026.11.1 added or removed. Verify: every place of D4 has a list for both versions.
- [x] 1.4 Add a row for the new notes to `docs/research/research-summary.md` with version, method and scope, and state that it is code reading only. Verify: the link resolves.

## 2. Pin

- [x] 2.1 Change `.devcontainer/package.json` to `@bitwarden/cli` 2026.11.1 and regenerate `.devcontainer/package-lock.json` with `npm install --package-lock-only` in `.devcontainer/`. Verify: the lockfile names `cli-2026.11.1.tgz` with an integrity hash, `openspec` is unchanged, and `npm ci --dry-run` in a copy of the directory succeeds.
- [x] 2.2 Run `npm audit` against the new lockfile and update the known advisories in `docs/development/integrations.md`. Verify: the paragraph names the date and every advisory that `npm audit` reports for the `@bitwarden/cli` tree, or states that there is none.

## 3. Unknown export keys and item types

- [x] 3.1 Add the known key sets of design D4 to `internal/capture`, from the lists of task 1.3, as the union of both versions. Update the package documentation of the export forms in `internal/capture/doc.go`, naming 2026.11.1 next to 2026.10.1 as a source. Verify: a test parses the intact fixtures of `capturetest`, personal and organization, and records no unknown key path and no unknown item type.
- [x] 3.2 Record unknown key paths and their counts while parsing the four export forms, following design D4: the path notation, no descent into the value of an unknown key or into a known key whose value has an unexpected type, and counts per top-level element. Verify with tests: an unknown key on one item (count 1), the same key on three items (one path, count 3), an unknown key in a passkey of an organization plaintext export, an unknown document-level key, an unknown key whose value is an object with further keys (only the outer key is recorded), a blob item with `data` and a legacy item with `key` (nothing recorded), and an item that lacks a known key (nothing recorded).
- [x] 3.3 Apply the identifier rule of design D5 in `internal/capture`: keys that do not match `^[A-Za-z][A-Za-z0-9]{0,63}$` become `<other>` at their place. Verify with a test whose fixture carries unknown keys with a sentinel value that contains spaces and an at sign, a key of 65 letters, a key that starts with a digit, and a key with a non-ASCII letter. Each must give the placeholder path, and the sentinel must not occur in the serialized parse result.
- [x] 3.4 Count unknown integer item types in the plaintext exports, personal and per organization. Verify with tests: type 9 on two items gives one entry with count 2, the known types 1 to 8 give none, and an encrypted export with a type 9 item is still classified as `FormatUnknown`.
- [x] 3.5 Add the reasons `export_key_unknown`, `export_key_unknown_omitted` and `export_item_type_unknown` and the field `ItemType` to `internal/verify` as described in design D6, including `Reasons()` and the table of `result_test.go`. Turn the parse records into warnings, sorted and capped at 32 paths per document. Verify with tests for each scenario of the spec delta: unknown key on an item, same key on several items, unknown key in a passkey of an organization export, document-level key, keys within an unknown value, known keys only, known key absent, new item type, new item type in an organization export, known item types, non-identifier key (with the sentinel check on the serialized result), and 40 distinct keys (32 warnings plus one with count 8). Each test also checks that the verdict is unchanged.
- [x] 3.6 Print `item_type=<n>` in `report.Finding`. Verify with a report test that renders one warning of each new reason and compares the lines exactly.
- [x] 3.7 Add the new spec scenarios to the coverage table of `docs/development/testing.md` ("Verification Tests"). Verify: every scenario of the spec delta has a row whose test names exist in the code.
- [x] 3.8 Document the warnings in `docs/user/limitations.md` ("What a Verdict Can and Cannot Detect": the warning names a key path and a count, it never changes the verdict, the item lists are not checked, and an unknown key is stored but not judged). Document key names in the report and the residual risk of design D5 in `docs/user/security.md`. Update the bullet on export fields in `docs/decisions/0007-bw-versions.md` in place, with the ADR staying proposed. Verify: the three pages link each other where they refer to the warning, and the ADR status line still reads proposed.

## 4. Vaultwarden suite against 2026.11.1

- [x] 4.1 Extend `TestTamperedPasskeyFails` so that it requires an `unverified document_decryption_errors` finding for the plaintext export with a count of at least 1 (design D3). Extend `TestIntactVaultIsVerified` and the organization scenario so that they require no `export_key_unknown`, `export_key_unknown_omitted` or `export_item_type_unknown` warning. Update the assertion table of the Vaultwarden section in `docs/development/testing.md`. Verify: `go vet -tags vaultwarden ./internal/e2e/` passes, and the suite passes against 2026.10.1 (the `bw` currently in the container) before the pin is used.
- [x] 4.2 Run the suite in the dev container against 2026.11.1 with `PATH` starting at `.local/bw-2026.11.1/node_modules/.bin`, then again after the maintainer has rebuilt the dev container from the new pin. Verify: every scenario passes, and `bw --version` in the rebuilt container prints `2026.11.1`. If the login or the seed fails because the server lacks an endpoint, stop and ask the maintainer (design, Risks). If the intact scenario reports an unknown key, record it and ask before adding it to a set.
- [x] 4.3 Record the run in `docs/development/testing.md` (a new test run section with the versions, the hash of `bw.js`, the duration and every finding of each scenario). Add a row for 2026.11.1 on Linux arm64 in the dev container to the tested versions table in `docs/development/integrations.md`, and update the sentence on the automated runs. Verify: the record names the decryption error counts of the tampered scenario next to those of 2026.10.1.
- [x] 4.4 Update the macOS download instructions of the manual exercises in `docs/development/testing.md` to the pin, with the asset name `bw-bit-macos-arm64-2026.11.1.zip` and the digest that GitHub lists for it. Leave the records of earlier runs unchanged. Verify: the digest matches `gh release view cli-v2026.11.1 -R bitwarden/clients --json assets`.

## 5. Integration

- [x] 5.1 Update roadmap item 5 in `docs/development/architecture.md`: the follow-up on export fields is done with this change, and the pin is 2026.11.1. Verify: no other page still names 2026.10.1 as the current pin (`grep -rn "2026.10.1" docs/development docs/user`, where each remaining hit is a record of an earlier run or a statement of evidence for that version).
- [x] 5.2 Run `gofmt -l .`, `go vet ./...`, `go vet -tags vaultwarden ./internal/e2e/` and `go test -race ./...`, and `openspec validate bw-version-2026-11 --strict`. Verify: no output from gofmt, and every command passes.

## Review fixes

Findings of the code and security review of 24 November 2026. R1 and R2 follow maintainer decisions of the same day. R3 changes code and has a regression test that failed without the fix; the others are documentation and comments.

- [x] R1 (S1, C1) Add a MODIFIED delta for the `backup-run` requirement "Report holds no vault content and no bw text" with a narrow exception for identifier-shaped key names in the key path of a warning, and the scenario "Key path of an unknown export key". Update ADR 0006 in place (it stays proposed), the proposal, design D5 and the security page. Verify: `run.TestKeyPathOfAnUnknownExportKey` passes and fails with the identifier rule disabled, the coverage table of the test strategy has its row, and `openspec validate bw-version-2026-11 --strict` passes.
- [x] R2 (S2) Correct the sentence of design D6 on invisible passkeys and name the case of a passkey list renamed in both exports in the limitations. The verdict rule stays unchanged (maintainer decision). Verify: the limitations name the case, and D6 records the decision.
- [x] R3 (C3) Count every integer plaintext item type that fits into an int, not only those in the range of `intValue`. Verify: `capture.TestUnknownItemTypeOfAnyInteger` fails before the fix and passes after it, and `GOARCH=386 go vet ./internal/capture/` passes.
- [x] R4 (C2) Describe the actual sort order of the warnings in design D6 and in the comment of `verifier.exportKeys`.
- [x] R5 (C4) Correct the comments in `internal/capture/parse.go` and `internal/capture/doc.go` that said the parsers read neither folders nor collections.
- [x] R6 (C5) Write "All three runs" in the record of the Vaultwarden test run of 24 November 2026.
- [x] R7 (C6) Name the code notes of 24 November 2026 in the package documentation of `internal/collect` and `internal/bwcli`.

Findings of the second review round of the same day:

- [x] R8 (S1, second round) Add a MODIFIED delta for the `bw-invocation` requirement "Failures carry a class and no CLI text" with the narrow exception of R1, and update the proposal. Verify: `openspec validate bw-version-2026-11 --strict` passes.
- [x] R9 (C1, second round) Number the generated document-level keys of the fixture run "unknown export keys and item types" so that it crosses the cap, and keep one key that is no identifier. Verify: `verify.TestUnknownExportKeysFixtureCoversEveryReason` fails before the fix and passes after it.
- [x] R10 (C2, second round) Say on the usage page that a warning about an unknown export key names its key path, and link the security page.

## Workflow follow-up

- After the merge, start `vaultwarden.yml` by hand on `main` and record the run on Linux amd64 in the test strategy and the tested versions table, as the earlier changes did.
- Archive the change after the review of the apply guidance and a passing `vaultwarden.yml` run.
