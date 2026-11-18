# Tasks

## Recommended apply setup

Model Opus, effort high. The change adds three packages, has concurrency between signals, contexts and teardown on three platforms, and a secret boundary in the report. That needs careful reading more than speed, and two review checkpoints split the session.

## 1. Offline command in its own package

- [x] 1.1 Create `internal/printable` with `Clean`, moved from `cmd/bw-takeout`, and its test for control characters, a terminal escape sequence and printable non-ASCII text (design D1). Verify `go test ./internal/printable/` passes and `grep -rn "func clean" cmd internal` prints nothing.
- [x] 1.2 Move `verifyGeneration`, `readIdentities`, `reportReadError` and `storedResult` with their unit tests into `internal/offline`, behind `offline.Run(args []string, stdout, stderr io.Writer) int`. Have `cmd/bw-takeout` call it for `verify` (design D9). Verify that every existing test of `cmd/bw-takeout` passes unchanged, including `TestSystemWithoutBW`, and that the output of `verify` on the test generations is byte-identical before and after the move.
- [x] 1.3 Replace `TestNoNetworkOrProcessPackage` with a test in `internal/offline` that runs `go list -deps bw-takeout/internal/offline`. It fails on `net` and its subpackages, `os/exec`, `internal/verify`, `internal/bwcli`, `internal/collect` and `internal/run` (design D9). Verify the test fails when a temporary import of `os/exec` is added to `internal/offline`, then remove the import again.
- [x] 1.4 Update the row "System without bw" in the scenario coverage table of archive-generation in `docs/development/testing.md`, so that it names the new package-level test, and add one sentence below the table that the binary-level check was replaced by this change (design D9). The spec `offline-verification` does not change. Verify `grep -rn "TestNoNetworkOrProcessPackage" docs cmd internal` prints nothing, or only the new test if it keeps the name.

## 2. Inputs

- [x] 2.1 Add `archive.ParseRecipientFile` and `ErrRecipientFile` (design D10). Verify tests named after the scenarios "Two recipients with a comment" and "Invalid line", plus tests for a file with comments only, trailing whitespace, a CRLF file, and a refusal message that does not contain a sentinel placed in the invalid line.
- [x] 2.2 Export `bwcli.ValidServerAddress`, used by `SetServer` as before (design D2). Verify the existing server address tests of `internal/bwcli` pass and a new test calls the exported function with `http://`, a missing host and user information.

## 3. The run

- [x] 3.1 Create `internal/run` with `Config`, `Deps` (profile function, leftovers function, terminal, standard input and error, signal channel, clock, publisher, logout limit, progress interface), `Outcome` and the package documentation that links this design (designs D1 and D7). Verify `go vet ./internal/run/` passes and `go doc ./internal/run` shows the types.
- [x] 3.2 Write the test setup: `TestMain` builds the fake `bw`, a profile function writes the scenario into each new data directory, a fake terminal, documents and attachments from `capturetest`, a recording progress implementation, and helpers that hash the target and point the temporary directory at an empty directory (design D11). Verify a smoke test of the setup that runs the fake `bw --version` through a profile created by the helper.
- [x] 3.3 Implement the checks before the first process (design D3). Verify tests named after the scenarios "Target missing" and "bw not found", plus tests for a target that is a file, an empty recipients file, a CA path that is a directory and a relative `--bw` path. Each test checks that no record file and no profile directory exist afterwards.
- [x] 3.4 Implement the order of design D4 up to the published generation, with `Wipe` after `capture.Parse` and through `defer`. Verify:
  - a test that runs the intact fixture to a published generation, opens it with `archive` and the test identity, and compares documents, attachments, the manifest source and the `bw` version and hash with the fixture and the fake,
  - the scenario "Stale snapshot" with the target hash unchanged,
  - the scenario "No temporary files",
  - a test that the four document slices hold only zero bytes after the run, both after a published run and after a failure of `NewWriter`.
- [x] 3.5 Implement teardown with `loginStarted`, `teardownCtx` and the five logout outcomes (design D5). Verify:
  - tests named after the scenarios "Login refused", "Failure before the login" and "Logout hangs", the last one with a logout limit of 200 ms and a hanging fake logout,
  - a test that the default limit is 10 seconds,
  - a test that the profile is removed after a failed verdict,
  - a test that a removal failure is in the outcome with its path.
- [x] 3.6 Implement the interrupt handling of design D6. Verify:
  - tests named after the scenarios "Interrupt during the attachments", "Interrupt before the target is touched", "Second interrupt during the logout", "bw ends on its own after the interrupt" and "Terminate signal",
  - a second signal before the logout starts, with the record file showing that no logout process ran and the outcome showing that the session key was forgotten,
  - a signal during the commit, with a published generation,
  - the two messages of D6 on standard error.

  Run the interrupt tests with `-race` and `-count=20` in the dev container and verify they pass every time.
- [x] 3.7 Add a section "Interrupting a Backup" to `docs/user/limitations.md`:
  - the first and second Ctrl-C,
  - the 10-second logout limit,
  - Ctrl-C at a `bw` prompt as a failed login,
  - the commit that finishes after an interrupt,
  - the Windows console close event, after which teardown may not finish,
  - that the real `bw` on Windows is untested.

  Verify each of these points is on the page.
- [x] 3.8 Add the section on swap, hibernation and core dumps to `docs/user/security.md` and rewrite rule 3 in `docs/development/architecture.md` (design D12). Update the sentence of `security.md` that says the backup run does not exist yet. Verify `grep -n "never as a file" docs/development/architecture.md` prints nothing, and the security page names encrypted swap or an encrypted disk and the hardening change.
- [x] 3.9 Review checkpoint: show the maintainer the output of `go test -race -v ./internal/run/`, with every skipped test listed separately. Verify the maintainer confirmed before any task of group 4 starts.

## 4. Report and exit status

- [x] 4.1 Implement `internal/report`: the progress interface of `run`, the final report of design D8 and `ExitStatus`. Verify:
  - a golden test of the full report for the example of D8, built from a synthetic outcome,
  - a table test of `ExitStatus` over every row of D7.
- [x] 4.2 Verify with run tests against the fake `bw`, wired through `report`:
  - tests named after the scenarios "Leftovers present", "One download fails", "Failed verdict" (both requirements), "Profile not removed", "Status without server address", "Verified backup", "Unverified backup", "Published generation, logout failed" and "No generation and profile not removed",
  - that leftover profiles and partial generations are byte-identical after the run.
- [x] 4.3 Add the redaction tests "Sentinels in the vault and in bw output" and "Control characters in the account address". Search both output streams of the run for every sentinel of `capturetest`, for a sentinel that every fake command writes to standard error and for the session key of the fake login. Verify the tests fail when a sentinel is deliberately printed by a temporary change, then revert the change.

## 5. Command

- [x] 5.1 Add the `backup` subcommand to `cmd/bw-takeout`: flags per design D2, the usage text for both subcommands, the signal subscription per platform per design D6, building `bwcli.Config`, `run.Deps` and `report`, and the exit status. Verify tests named after the scenarios "Server flag missing" and "Server address not HTTPS", plus tests for an unknown flag and an extra argument, each with status 2. Also verify a test that a missing target gives status 1 with an empty program search path, which shows that no `bw` was needed.
- [x] 5.2 Write `docs/user/usage.md` "Making a Backup":
  - the command line,
  - each flag,
  - the recipients file with an example of two recipients,
  - what the report shows,
  - the exit status table of D7,
  - a pointer to `verify` for the offline check.

  Update `docs/user/configuration.md`: the flags are the interim configuration, and the format of a configuration file stays open. Verify the documented command line matches the usage text of the command, by a test that compares the flags in both.
- [x] 5.3 Add the scenarios of the `backup-run` delta to a new scenario coverage table in `docs/development/testing.md`, section "Backup Run Tests", with the test setup of design D11 and the evidence level mock. Verify `go test -list` shows a test for every row.

## 6. Manual backup exercise

- [x] 6.1 Add the procedure of the manual exercise to `docs/development/testing.md`:
  - the synthetic account with a passkey, a personal attachment and an archived item with an attachment, created through the web vault of the Vaultwarden instance of the login exercise,
  - the three runs of design D11 and `verify`,
  - what is recorded: platform, `bw` version and hash, exit statuses, logout duration, device entries, and whether the archived attachment is in the generation.

  Verify the procedure names no credential and stores every file under `.local/`.
- [x] 6.2 Ask the maintainer to run the exercise on Linux and, if possible, on macOS. Record the result in `docs/development/testing.md`, and add a research note under `docs/research/` if a run contradicts a code note. Verify the record states what was not run.

## 7. Roadmap

- [x] 7.1 Update `docs/development/architecture.md`:
  - roadmap item 4 C done, with the decisions of the proposal,
  - a new item for memory hardening with the points moved out of C: the core dump limit on Unix, the Windows path, and the correction of the security page,
  - the follow-ups of the proposal under item 5 or 6: `--json`, the exit status for aborted runs, login data from another vault for batch operation, `bw.cmd` on Windows,
  - the component rows of `run` and `report` as implemented, and `offline` and `printable` added,
  - the planned flow.

  Verify every bullet of the old item 4 C is either marked done or found again in the hardening item.
- [x] 7.2 Review checkpoint: show the maintainer the changed roadmap. Verify the maintainer confirmed it.

## 8. Integration check

- [x] 8.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...` and the key-material search from `docs/development/testing.md` in the dev container. Cross-compile `cmd/bw-takeout` for `darwin/arm64` and `windows/amd64`. Verify all pass, the search prints nothing, and `.github/expected-skips.txt` lists every new skip or none was added. Record the run in `docs/development/testing.md` with date, platform, Go version, skipped tests and the evidence level.

## Review fixes

Findings of the independent review of 18 November 2026 (code review C1 to C6, security review S1 to S4).

- [x] 9.1 C1, S1: a generation that `Publish` returns together with an error (target directory not flushed) is kept in the outcome as published, and the error is reported as a warning. Verify a run test with a publisher that returns both, which fails without the fix.
- [x] 9.2 C2, S3: when the abort of a partial generation fails, after a failed verdict or a failed write, the outcome records that the partial directory stays, and the report says so. The line is printed only then. Verify run tests for both cases and a report test, which fail without the fix.
- [x] 9.3 C4: teardown runs from a `defer`, and a panic in a phase is recovered, recorded as an internal error without its value, and followed by teardown. Verify a run test with a panic after the login that checks the logout and the removal of the profile.
- [x] 9.4 C5: the logout outcome is taken from the cause of the teardown context whenever the logout failed, whatever the class. Verify a test of the classification with a failure of the class exit and a skipped context.
- [x] 9.5 S2: on Unix the backup command subscribes to SIGPIPE without acting on it, so a closed standard output cannot end the run before teardown. Verify a command test with a closed pipe as standard output, which ends by the signal without the fix.
- [x] 9.6 S4: the recipients example of the usage page holds placeholders that do not parse. Verify the docs test checks that the example is refused.
- [x] 9.7 C6: README, getting-started and the FAT/exFAT note of testing.md name the backup command. Verify `grep -rn -i "no backup command\|one subcommand" README.md docs` prints nothing.
- [x] 9.8 C3: the attachment lines stay after the downloads (maintainer decision of 18 November 2026). Design D1, the usage page, the comment of `run.Progress` and the roadmap say so. Verify the usage page states that no attachment line appears while the downloads run.

Findings of the second review round of 18 November 2026 (code review R2-C1 to R2-C4, security review without findings), fixed with the maintainer's approval of the same day without a third round.

- [x] 9.9 R2-C1: a failure that ended the run before the first signal is named in the report also when an interrupt followed, for example during teardown. Only a failure after the signal is replaced by the interrupt line (design D8). Verify a run test with a refused login and a signal during a hanging logout, and a report test.
- [x] 9.10 R2-C2: the logout result is taken from the cause of the teardown context as it stood when the logout returned, and a signal after the logout no longer announces that the logout is skipped. Verify a run test with a failing logout and a signal after it returned.
- [x] 9.11 R2-C3: the report prints joined errors with "; " between the parts instead of a replaced line break. Verify a report test with a joined error.
- [x] 9.12 R2-C4: the first sentence of the README, the FAT and exFAT note of the testing page, and the spec scenario "Sentinels in the vault and in bw output", which exempts the login prompts of `bw`. Verify by reading the three places.
