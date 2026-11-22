# Tasks

## Recommended apply setup

Model Opus, effort high. The change touches almost every package, adds a format version and new verification rules, and extends the secret boundary to three new `bw` commands. A wrong rule would let an incomplete organization backup look verified, so the session must follow the design closely and stop at the marked points instead of working around a deviation.

## 1. Evidence of the spike

- [x] 1.1 Write `docs/research/organization-export-spike-2026-11-21.md` from the explore session of 21 November 2026 (design, Context). It records:
  - **Versions:** Vaultwarden 1.37.4 linux/arm64, with the binary SHA-256 from the cache, and `bw` 2026.10.1 with the `bw.js` hash.
  - **Method:** three and then four synthetic accounts, the organization setup of design D12, two collections, and members assigned to C1 only.
  - **Results:** the table rows of the design marked "Observed".
  - **Code reading:** the rows marked "Code reading", with their bundle lines.
  - **What was not tested:** the Cloud, custom roles, blob items, passkeys in organization items, decryption error lines on organization exports, and a change during a run.
  - **Status of the scripts:** they were lost in a scratchpad cleanup.

  Add a row to `docs/research/research-summary.md`. Verify that every link in both files resolves and that the note holds no password, token, session key, organization key or ciphertext.

## 2. bw invocation

- [x] 2.1 Add `vaultid.ValidOrganizationID` with the UUID rule of item IDs (design D1). Verify with a table test: a valid lowercase and a valid uppercase ID are accepted, and an ID with a dash prefix, `../x`, an empty string and a 35-character string are refused.
- [x] 2.2 Add `ListOrganizations`, `ExportOrganizationJSON` and `ExportOrganizationEncryptedJSON` to `bwcli.Client`, with the operations, arguments and time limits of design D9. The organization ID is validated before the command is built and kept in `bwcli.Error.OrganizationID`. Verify with tests against the fake `bw`:
  - the scenarios of "Organization exports are requested by ID": export by ID, ID with a leading dash with no process started, and a sentinel in the error text that stays out of the failure while the failure names the organization ID;
  - the scenarios of "Organization list is read from its output": two memberships, an entry without a role, and an empty array;
  - an entry with an ID that is not a UUID, which is the class `output`;
  - the raw list output wiped after decoding.
- [x] 2.3 Extend the fake `bw` of `internal/bwcli/bwclitest` with answers for the three argument lists and with a hook that logs the fake out after one answer (design D9). Verify with a test: after an organization export that fails through the hook, the client reports `not_logged_in`, and a following vault command fails as not logged in.
- [x] 2.4 Add the new tests to the `bw-invocation` table of `docs/development/testing.md`. Verify that the test names in the table match the code.

## 3. Capture model

- [x] 3.1 Add the organization input and capture types, the two organization documents and `DocumentError.OrganizationID` to `internal/capture` (design D4). Parse both documents with the existing export parsers. Verify with tests:
  - a plaintext and an encrypted organization export with `collections` instead of `folders` parse;
  - a password-protected organization export is rejected with the organization document and ID;
  - an absent organization document is rejected the same way.
- [x] 3.2 Keep `revisionDate` as a raw string on plaintext, encrypted and listed items (design D4). Verify that existing fixtures still parse unchanged, and that a missing `revisionDate` on a personal item is not an error.
- [x] 3.3 Replace `PersonalAttachments` with `ScopedAttachments(chosen, lists...)` (design D6). Verify with tests: personal attachments come in the same order as before, attachments of a chosen organization are included once even when both lists show them, and attachments of another organization are left out.
- [x] 3.4 Update the package documentation of `internal/capture` for the organization inputs. Verify with `go doc ./internal/capture` that the new types are described.

## 4. Verification

- [x] 4.1 Extend `capturetest` with the intact organization fixture and the fourteen variants of design D13. Each variant declares the places it changes, like the existing ones. Verify that the fixture self-test compares every declared place with the actual difference to the intact fixture.
- [x] 4.2 Implement the organization pass of design D5, steps 1 to 6, the field `organization_id`, the new reasons `organization_item_mismatch` and `organization_changed_during_capture`, the scope with organizations and the limit `organization_exports_separate_server_reads`. Remove `organization_items_excluded` and `OrganizationCount`. Verify with one test per scenario of the `capture-verification` delta and one per variant of 4.1. The variant of an organization that is not chosen must produce no finding and no warning. The intact organization fixture must be verified with the new scope and the new limit. Each new test must fail with its rule disabled.
- [x] 4.3 Extend the reference comparison to the plaintext items of the chosen organizations (design D5). Verify with the scenario "Organization item missing from the capture" and with the intact organization fixture and its reference.
- [x] 4.4 Verify that the sentinel test of "No secrets in results" covers the organization fixture with sentinels in every sensitive field of the organization exports.
- [x] 4.5 Update [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md) in place (design D11): the scope claim with the chosen organizations, the new limit, and organization findings inside the one verdict. Keep its status proposed. Update the `capture-verification` table of `docs/development/testing.md`: remove the rows of "Organization items stay visible", add the new requirements, and add the organization variants to the fixture description. Verify that the test names in the table match the code.

## 5. Collection

- [x] 5.1 Add the role check of design D2 to `Collector.Snapshot`, with `collect.OrganizationError` and `collect.ErrOrganization`, and extend the `Vault` interface. Verify with tests against the fake `bw`:
  - every scenario of "Chosen organizations are checked before the documents", that is admin, user role, not in the list, not confirmed, custom role, no organization chosen with no list command started, and a failed list;
  - an owner, who is accepted;
  - for every refused case, that no export command was started.
- [x] 5.2 Collect the organization documents in the order of design D3, with their decryption error counts and the names for the source (D8). Extend `Snapshot.Wipe` to them. Verify with tests:
  - the order of commands for two organizations;
  - the scenario "Organization export fails";
  - the scenario "Session lost on an organization export" through the logout hook of 2.3, with no further vault command started;
  - the scenario "Organization export with decryption errors";
  - the scenario "Source with two organizations";
  - a test that `Wipe` zeroes the organization documents.
- [x] 5.3 Build the plan with `ScopedAttachments`. Verify with the scenarios of "Attachments in scope are downloaded once" against the fake `bw`, including that an attachment of an organization that is not chosen produces no download request.
- [x] 5.4 Update the `capture-collection` table of `docs/development/testing.md`. Verify that the test names in the table match the code.

## 6. Archive, manifest and offline verification

- [x] 6.1 Add the required `organizations` section after `counts` to `internal/manifest`, with format version 1 unchanged (design D7). Verify with tests:
  - a manifest with and without organizations round-trips byte for byte;
  - a manifest without the key `organizations` is refused as invalid;
  - version 2 is refused as unsupported.
- [x] 6.2 Extend `archive.Documents`, `Meta`, `NewWriter` and `kindOf` to the organization documents of design D7, and count the items per organization. Verify with tests:
  - the scenarios "Organization documents round trip", "Organization document absent", "Organization ID that is not a UUID", "Chosen organization in the manifest" and "No organization chosen";
  - a refused duplicate organization ID;
  - the recovery test with the `age` and `tar` commands unpacking the organization documents byte for byte.
- [x] 6.3 Extend the reader and `internal/offline` to the rules of "Organization entries follow the manifest" and "The manifest lists the organizations", and to the output of "The stored verdict is reported". Verify with one test per scenario. Also verify that an archive built in the test from the layout of ADR 0005 before this change is refused, and that a sentinel organization name does not appear in the output.
- [x] 6.4 Update `RECOVERY.txt` for the organization documents (design D7). Verify that the recovery test still finds no vault data in it, and that the steps in `docs/user/recovery.md` run as written against a generation with an organization left by the opt-in test.
- [x] 6.5 Update [ADR 0005](../../../docs/decisions/0005-archive-format.md) in place with the layout and the second redefinition of format version 1 before any release, next to the first one in its consequences. Keep its status proposed. Update `docs/user/recovery.md` and the `backup-archive` and `offline-verification` tables of `docs/development/testing.md`. Verify that every link resolves and the test names match the code.

## 7. Run, report and command

- [x] 7.1 Add the repeatable `--organization` flag to `cmd/bw-takeout` and `run.Config.Organizations` (design D1). Verify with the scenarios of "Organizations are chosen by flag": two organizations, `../x`, the same ID twice with different case, and no flag. In each usage error, no `bw` process was started and no profile was created.
- [x] 7.2 Pass the organizations to the collection and extend `report` with the setup line, the failure lines and the finding field of design D10. Verify with tests:
  - the scenarios of the `backup-run` delta: chosen organizations in the setup, a failed organization check, a failed organization export, a target left unchanged for a missing role and for a failed export, and the empty temporary directory with an organization;
  - a sentinel in an organization name;
  - a golden report of a run with one organization;
  - an unchanged golden report of a run without one.
- [x] 7.3 Update `docs/user/usage.md` (the flag and the role), `docs/user/configuration.md` (the flag as configuration), and `docs/user/security.md` (organization data in the archive, the role needed, and the export event in the organization's event log). Update the `backup-run` table of `docs/development/testing.md`. Verify that every link resolves and the documented command line matches `bw-takeout backup -h`.

## 8. Vaultwarden scenario

- [x] 8.1 Add the organization setup of design D12 to `internal/e2e`: the organization key, the RSA wrapping of type 4, the collection and member calls, and the seeding through the owner's `bw`. Record the organization key and every new password for the log check. Verify with a setup test that the admin's `bw list items` shows the items of both collections and `bw list organizations` reports the role admin with status confirmed. If an API call is refused for a reason other than a field name, stop and ask the maintainer. If the admin does not see the item of C2, also stop and ask, because the result of the spike would not hold.
- [x] 8.2 Add `TestOrganizationBackupIsVerified` as the admin with `--organization` (design D12). Verify that it passes in the dev container: the generation is verified, it holds both organization documents and both attachments with their SHA-256, the trash item is in neither organization document, and the reference inventory includes both organization items. Record the duration.
- [x] 8.3 Add `TestOrganizationRoleIsRefused` as the user with `--organization`. Verify:
  - the run ends in the snapshot phase with the reason `role` and exit status 1;
  - the target is unchanged;
  - no `export --organizationid` was started;
  - the test fails when the role check is disabled on purpose, because the run then reports `not_logged_in` instead (revert afterwards).
- [x] 8.4 Document the scenario in the "Vaultwarden Tests" section of `docs/development/testing.md`. Remove organizations from "Not covered" there and from the "Not run" list of the backup exercise where it no longer holds. Verify that the test names match the code.

## 9. Decision record and remaining documentation

- [x] 9.1 Write `docs/decisions/0008-organization-export.md` with the status proposed (design D11), and add it to `docs/decisions/README.md`. Verify that it names context, decision, alternatives and consequences, and that every link resolves.
- [x] 9.2 Replace the sentence "Organization items are not checked" in `docs/user/limitations.md` with a section on organizations covering the points of design D11. Update the opening paragraph where it speaks of missing organization permissions. Verify that the section names Vaultwarden 1.37.4 as the only tested server and links the research note of 1.1.
- [x] 9.3 Update `docs/development/architecture.md`: the component sentence, the flow ("Personal and organization views are handled separately"), and roadmap item 6, from which organizations are taken out and marked as done with this change. Verify that "organizations" no longer appears in item 6 as future work.
- [x] 9.4 Check [ADR 0006](../../../docs/decisions/0006-secret-boundary.md) against the three new commands. Change it only if they contradict it, and record the result in the testing page entry of 10.1. Verify that the key-material check of `checks.yml` passes on the working tree.

## 10. Integration checks

- [x] 10.1 Run these checks in the dev container:
  - `gofmt -l .` and `go mod tidy -diff`;
  - `go vet ./...`, `go vet -tags vaultwarden ./...`, `GOOS=darwin go vet ./...` and `GOOS=windows go vet ./...`;
  - `go test -race ./...` and `go test -count=1 -json ./... | go run ./internal/tools/skipreport`;
  - `go test -count=1 -tags vaultwarden -v ./internal/e2e/`;
  - `openspec validate organization-export --strict`.

  Verify that all pass, that `.github/expected-skips.txt` lists every new skip with its reason, and that the log check of `TestMain` found none of the recorded values. Record the run in the testing page as earlier changes did, with the duration of the suite.

## Review fixes

Findings of the code and security review of 21 November 2026. Each is documentation or a stray file, so no regression test applies.

- [x] R1 (S1, C1) Remove the untracked `bw-takeout` binary at the repository root, a build of the dirty working tree that a `git add -A` would have published. Verify that `git status` lists only files of the change. `.gitignore` stays unchanged (maintainer decision of 21 November 2026).
- [x] R2 (C3) Reword the stale sentences on organization roles and live tests in `docs/development/testing.md` and in the row "Freshness and permissions" of `docs/research/research-summary.md`. The original sentence of the row is kept and the new evidence is added after it. Verify that the links resolve.
- [x] R3 (C4) Replace the reference to the removed `capture.PersonalAttachments` in the comment in `internal/verify/archived_test.go` with `capture.ScopedAttachments`. Verify that `go vet ./internal/verify` passes.
- [x] R4 (C2) Narrow the requirement "Organization items outside the scope" in the `capture-verification` delta to the comparisons with the exports and the attachment accounting, and add the requirement "Item lists are checked as documents in every scope" with the scenario "Unchosen organization item in both lists", covered by `TestUnchosenOrganizationItemInBothLists`, to the delta and to the `capture-verification` table of `docs/development/testing.md`, and say it in `docs/user/limitations.md`. No code changes (maintainer decision). Verify with `openspec validate organization-export --strict` and that the test fails when the item is left out of the archive filter count.
- [x] R5 (second round, C1) Reword the sentence on organization roles in `docs/development/testing.md`: owner, admin, user and the other roles are tested against the fake `bw`, admin and user against Vaultwarden; no owner run against Vaultwarden is part of design D12.
- [x] R6 (second round, C2) Name the requirement "Item lists are checked as documents in every scope" in the paragraph above the `capture-verification` table of `docs/development/testing.md`.
- [x] R7 (second round, C3) Extend the clarification of R4: "Organization items outside the scope" excepts the checks of the item lists as documents, and the requirement of R4, renamed "Item lists are checked as documents in every scope", names the duplicate ID within one list as well. Add the scenario "Unchosen organization item that differs between the lists" with `TestUnchosenOrganizationItemThatDiffersBetweenTheLists`, and `TestUnchosenOrganizationItemTwiceInOneList` for the duplicate ID. No code change (maintainer decision on C2). Verify with `openspec validate organization-export --strict` and that both tests pass.
