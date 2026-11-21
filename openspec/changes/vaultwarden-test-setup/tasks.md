# Tasks

## Recommended apply setup

Model Opus, effort medium. The design fixes the hard parts and a wrong key derivation fails loudly at the login of `bw`, so little open reasoning is left, but the session must recognize the three stop points of tasks 3.3, 4.3 and 5.2 instead of working around a deviation, and keep the output hygiene of design D9, which calls for judgment rather than depth.

## 1. Evidence of the spike

- [x] 1.1 Write `docs/research/vaultwarden-registration-spike-2026-11-21.md` from the results of the explore session of 21 November 2026 (design, Context): versions and digests (Vaultwarden 1.37.4 arm64 and amd64 manifests and binary hashes, `bw` 2026.10.1 npm with the `bw.js` hash), method (registration in two calls, PBKDF2 with 600 000 iterations, seeding through `bw`), results (status codes, session key form, counts of both lists and the plaintext export, legacy format, premium reported for every user by code reading), and what was not tested (amd64 at run time, a pseudo-terminal, the product itself, tampering, Bitwarden Cloud). Add a row to `docs/research/research-summary.md`. Verify that every link in both files resolves and that the note holds no password, token, session key or ciphertext.

## 2. Server lifecycle

- [x] 2.1 Create `internal/e2e` with a package comment and the build constraint `//go:build vaultwarden && linux` on every file, all of them `_test.go` files (design D1). Verify that `go build ./...`, `go vet ./...` and `go test ./...` pass without the tag and do not list the package with an error, and that `go vet -tags vaultwarden ./internal/e2e/` passes.
- [x] 2.2 Implement the fetch of design D2: anonymous token, the pinned index digest, manifest selection by `GOARCH` against the table of D2, SHA-256 check of every blob, unpacking of `vaultwarden` only, and the digest-keyed cache under `.local/vaultwarden-cache/`. Verify with unit tests on an `httptest` registry: a blob with a wrong digest fails, a manifest that differs from the table fails with a message naming the architecture, an unknown architecture fails, a damaged cache file is fetched anew, and a valid cache needs no request. Verify against Docker Hub in the dev container that the unpacked binary has the arm64 SHA-256 of D2.
- [x] 2.3 Implement the server start and stop of design D3: certificate created in Go, free port, the environment of the manual exercise with the web vault off, the wait for `/alive` with a limit of 30 seconds, and the cleanup (SIGTERM, then SIGKILL after 5 seconds). Verify with a test that starts the server, gets `/alive` over TLS with the test certificate, and after the cleanup finds the process gone and the port free.

## 3. Account helper

- [x] 3.1 Implement the key derivation and the EncString of design D4. Verify with unit tests: the EncString has the form `2.<iv>|<ct>|<mac>` with a 16-byte IV, and a wrong MAC key fails to open it in the test's own decrypt function; the wrapped user key and the wrapped private key decrypt again with the derived keys; and a known-answer test of PBKDF2 and HKDF for a fixed address and password whose expected values come from a published source named in the test. If no published vector is found, keep only the round trip tests and record that under D4 in design.md.
- [x] 3.2 Implement the registration of design D4 (both calls, flat request form, random address under `example.invalid` and random password in memory). Verify with a unit test on an `httptest` server that the request body has the fields of the context section and no field holds the password, and with a test against the started server that both calls return 200 and the second returns `registerFinish`.
- [x] 3.3 Implement the access token and the cipher `GET` and `PUT` of design D7. Verify against the started server, before anything builds on it, that a `PUT` with a replaced `keyValue` returns success and that a second `GET` returns the new value. If Vaultwarden refuses the `PUT`, stop: record the response status and message in the research note of 1.1 and ask the maintainer whether to take the database fallback of D7.

## 4. Seeding and the pseudo-terminal

- [x] 4.1 Implement the seeding of design D5 in its own seed profile, with the reference inventory in memory, and the removal of the import file and the seed profile. Verify with a test against the started server that the seed `bw` then shows two items in `list items`, one in `list items --archived`, three items and one passkey in the plaintext export, and that the seed profile and the import file no longer exist afterwards.
- [x] 4.2 Implement the pseudo-terminal of design D6 with `golang.org/x/sys/unix` and the prompt reader. Verify with a unit test that runs a small child program built in the test, which prompts on standard error and reads standard input, that the reader answers both known prompts and fails on an unknown prompt and on the time limit, and that the failure message holds the prompt names but not the answers. Verify that `go mod tidy -diff` shows no change.
- [x] 4.3 Implement the backup call of design D6: the binary built in `TestMain`, the age identity and the recipients file, the isolated `XDG_CACHE_HOME`, and the collection of exit status and report. Verify by running it once against a seeded server in the dev container and recording the prompt names `bw` showed. If they differ from `Email address` and `Master password`, update the reader and design D6.

## 5. Scenarios

- [x] 5.1 Add `TestIntactVaultIsVerified` with the assertions of design D8. Verify that it passes in the dev container on Linux arm64, and that it fails when the test's expected SHA-256 of one attachment is changed on purpose (revert afterwards).
- [x] 5.2 Add `TestTamperedPasskeyFails` with the assertions of design D8. Verify that it passes in the dev container. If the failed finding is not `passkey_count_mismatch` with the counts 1 and 0, stop as design D8 says: record the observation in the research note and ask the maintainer.
- [x] 5.3 Check the output hygiene of design D9. Verify that a `go test -tags vaultwarden -v ./internal/e2e/` log of both scenarios contains neither the password nor the session key nor the address typed at the prompt (the test generates them, so it can search its own log by writing it to a temporary file in a final check, or the implementer searches the saved log by hand and records the result), and that the key-material check of `checks.yml` passes on the working tree.
- [x] 5.4 Add the section "Vaultwarden Tests" to `docs/development/testing.md` (design D11): how to run the suite, the build tag, the cache, the scenarios with what they assert, the evidence level, what it does not cover, and that the manual exercises stay for macOS, TOTP and interrupts. Update the sentence in "bw Process and Profile Tests" that cases with a login wait for an automated setup. Record the first run in the dev container with date, architecture, Go, `bw` and Vaultwarden versions, duration and result. Verify that the documented command runs as written.

## 6. Workflows

- [x] 6.1 Add `.github/workflows/vaultwarden.yml` as design D10 describes, with the header comment on external contacts and no uploads. Extend `TestWorkflowStructure` with a case for it: only `workflow_dispatch`, the common checks, no `setup-go`, and the check for `cache: false` limited to workflows that use `setup-go`. Verify that `go test ./internal/tools/skipreport/` passes, and that it fails when a `pull_request` trigger is added to the new file on purpose (revert afterwards).
- [x] 6.2 Add the step `go vet -tags vaultwarden ./...` to `checks.yml` (design D10). Verify that the step passes locally and that `TestWorkflowStructure` still passes.
- [x] 6.3 Reproduce the CI step of design D10 in the dev container as far as possible: run the suite with `HOME`, `GOCACHE` and `GOMODCACHE` set to empty directories under `/tmp`. Verify that both scenarios pass that way. The run in Docker on the runner is a workflow follow-up.

## 7. Documentation

- [x] 7.1 Update `docs/development/integrations.md`: the Vaultwarden pin table of design D2 with how to update it, the new workflow with its pins and trigger in the CI section, and a sentence in the tested `bw` versions that the suite ran against 2026.10.1 on Linux arm64. Verify that the pins in the page equal those in the code and the workflow.
- [x] 7.2 Update `docs/development/releases.md` with a passing dispatched run of `vaultwarden.yml` on the release commit as a release step, and `docs/development/getting-started.md` with where to start it. Verify that the links resolve.
- [x] 7.3 Update `docs/development/architecture.md`: in item 5 the automated Vaultwarden setup is done with this change, a new follow-up names the second change for two-step login, a deleted or corrupted attachment file, the login cases of `bw-process-adapter` and an interrupt during a live backup, and the statement that blob items, lapsed premium and the archive filter cannot be produced on Vaultwarden 1.37.4. Verify that `grep -n "automated Vaultwarden" docs/development/architecture.md` shows the item as done and the follow-up once.
- [x] 7.4 Compare `docs/user/limitations.md` with the results of 5.1 and 5.2. Change it only where a run contradicts a statement. Verify by recording in the testing page entry of 5.4 whether a change was needed.

## 8. Integration checks

- [x] 8.1 Run `gofmt -l .`, `go vet ./...`, `go vet -tags vaultwarden ./...`, `go test -race ./...`, `GOOS=darwin go vet ./...`, `GOOS=windows go vet ./...`, `go test -count=1 -tags vaultwarden ./internal/e2e/` and `openspec validate vaultwarden-test-setup --strict` in the dev container. Verify that all pass, and record the run in the testing page as earlier changes did.

## Workflow follow-up

- Run the independent code and security reviews required by the apply guidance before reporting completion.
- After the maintainer commits and pushes, dispatch `vaultwarden.yml` on the branch and record the run in the testing page: Linux amd64, the duration, both scenarios, and the first run of the amd64 server binary. Check that the regular `CI` and `Checks` runs pass and that the skip report lists no new skip.
- Archive the change after review. It has no spec delta to sync.
- Start the follow-up change for two-step login and the further cases once the dispatched run has passed.

## Review fixes

- [x] R1 (code review C1) Log the output of a failed `bw-takeout verify` in `TestIntactVaultIsVerified` with the address replaced, as the backup report is (design D9). Verify with a unit test of the redaction helper on text in the form of the `Account:` line of `verify`.
- [x] R2 (code review C2) Require the profile location under the temporary `XDG_CACHE_HOME` to exist and be empty after a backup, so that the check proves the run used the isolated location (design D6, D8). Verify with a unit test: a missing location fails, an empty one passes, one with an entry fails.
- [x] R3 (code review C3) Move the prompt child of the pseudo-terminal test to `internal/e2e/testdata/promptchild/` with the build constraint `vaultwarden && linux`, and build it from there with the tag, so that no run writes a source file into the package directory (design D1). Verify with a test that every Go file of `internal/e2e` outside `testdata` is a `_test.go` file with that constraint, and that the pty tests pass.
- [x] R4 (code review C4) Name `internal/e2e` in the list of existing code in `docs/development/architecture.md`, and the step `go vet -tags vaultwarden ./...` in the description of `Checks` in `docs/development/getting-started.md`.
- [x] R5 (second code review C1) Record in the testing page that the checks and the live suite ran again after R1 to R4, with 26 tests.
- [x] R6 (second code review C2) Describe `TestPackageFilesAreTaggedTests` in the testing page by what it checks; the pseudo-terminal tests are what build the child from `testdata`.
