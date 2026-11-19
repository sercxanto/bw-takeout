# Tasks

## Recommended apply setup

Model Opus, effort medium. The change is small, one new package and a few edits in `run`, `report` and the fake `bw`, but it makes a security promise with platform-specific system calls, and macOS and Windows are checked only in CI. That calls for care rather than depth.

## 1. Package crashdump

- [x] 1.1 Make `golang.org/x/sys` a direct dependency in `go.mod` by importing `golang.org/x/sys/unix` from the new package and running `go mod tidy` (design D1). Verify that `go build ./...` passes and that `go mod tidy` leaves `go.mod` and `go.sum` unchanged on a second run.
- [x] 1.2 Create `internal/crashdump` with `Protection`, `Limit`, `ErrCoreLimit` and `ErrDumpable`, and the files `crashdump_unix.go`, `dumpable_linux.go`, `dumpable_other.go` and `crashdump_windows.go`. The system calls go behind unexported variables (design D1, D4). Verify with unit tests that replace the calls:
  - a failed core limit set and a failed get give `ErrCoreLimit`,
  - a read-back with a soft limit of 1 and one with a hard limit of 1 give `ErrCoreLimit`,
  - on Linux, a failed dumpable set and a dumpable read-back of 1 give `ErrDumpable`,
  - success gives `CoreLimit` true and, on Linux only, `NotDumpable` true,
  - no error text contains a number or a path.
- [x] 1.3 Add a test that calls the real `Limit` in a re-executed child of the test binary, as in `internal/publish/kill_test.go`. The child prints the core limits it reads afterwards and, on Linux, the dumpable value (design D6). Verify in the dev container that the parent sees `0 0` and dumpable `0`, and that the test process itself keeps its own limits afterwards: `ulimit -c` is unchanged and a `Getrlimit` in the parent after the child still shows the old values. On Windows the test checks a zero `Protection` and no error, in CI.
- [x] 1.4 Add the row `crashdump` to the component table of `docs/development/architecture.md` with its status. Verify the row names the platforms and that the status says Windows reports "not limited".

## 2. Fake bw records the core limit

- [x] 2.1 Add `CoreLimit *[2]uint64` (soft, hard) to `bwclitest.Record`, filled by `fakebw/rlimit_unix.go` on every start and left nil by `fakebw/rlimit_windows.go`. List the new field in the comment of the fake (design D6). Verify that `TestFakeRecordsEachStart` is extended: on Unix the recorded value equals the limits of the test process, and on Windows it is nil. Verify that `GOOS=windows go vet ./internal/bwcli/...` passes.

## 3. The run applies the limit

- [x] 3.1 Add `Deps.LimitCrashDumps`, `Setup.CrashDumps`, and the call as the first step of `runner.check`. A failure is `Failure{Phase: checks}` with the error of `crashdump` (design D2, D3). In the test setup of `internal/run`, inject a recording function by default, so that no in-process test lowers the limits of the test binary. Verify with tests named after the scenarios "Core limit cannot be set", "Core limit does not read back as zero" and "Not-dumpable mark cannot be set on Linux", each with an injected error. Each checks exit status 1 through `report.ExitStatus`, the phase `checks`, the `Failed:` line naming the core limit or the dumpable mark, no record file of the fake and no profile directory. Verify with one more test that combines a missing target with a recording function and checks that the function was called and the run failed on the target.
- [x] 3.2 Add the real-path tests in a re-executed child of the run test binary, on Linux and macOS (design D6):
  - one child runs a full backup of the intact fixture against the fake `bw` with nil `LimitCrashDumps`, and the parent checks that every record holds `CoreLimit` `[0, 0]` (scenario "bw inherits the limit"),
  - on Linux, the child also reports a dumpable value of 0 after the run (scenario "Run process not dumpable"),
  - a second child fails on a missing target and reports soft and hard core limits of 0 (scenario "Limit set before the first bw process").

  Verify that both pass in the dev container and in the macOS CI job, and are skipped with a reason on Windows.
- [x] 3.3 Add a test named after the scenario "bw leaves a core file". The fake writes `core` into its working directory on the plaintext export through `WriteFile`. Verify exit status 1, an unchanged target hash, and that the profile directory no longer exists.
- [x] 3.4 Add the setup line of design D5 to `report.Writer.setup` and update the golden test of the report example. Verify a table test with the three `Setup` values of Linux, macOS and Windows (scenarios "Run on Linux" and "Run on macOS" of "Report names the crash dump protection"). Verify that a run test on Windows with the real stub passes the checks, prints `Crash dumps: not limited on this platform` and creates its profile (scenario "Run on Windows"), in the Windows CI job.
- [x] 3.5 Update `docs/user/usage.md`: the new setup line in "What the Report Shows", and in "Making a Backup" the check that limits crash dumps before `bw` starts, with the failure it causes on Linux and macOS (design D8). Verify that the docs test of `cmd/bw-takeout` still passes and that the line in the page matches the line in the golden test.
- [x] 3.6 Rewrite the section "Swap, Hibernation and Core Dumps" of `docs/user/security.md` as design D8 describes: what is limited per platform, the pipe handler that decides itself, the not-dumpable mark and what it also blocks, Windows not limited, and the advice on encrypted swap kept. Verify that `grep -n "does not yet limit core dumps\|planned after the first backup command" docs/user/security.md` prints nothing and that every link in the section resolves.
- [x] 3.7 Update ADR 0006 in place with a decision point "Crash dumps" and the consequence that pipe handlers and Windows stay outside (design D7). Keep the status proposed. Verify that the decision index in `docs/decisions/README.md` still lists 0006 as proposed and that no new ADR file exists.
- [x] 3.8 Update `docs/development/architecture.md`:
  - rule 3 references the core limit,
  - item E is marked done with the decisions of 19 November 2026,
  - item 5 gets the Windows experiment, together with the job object, stopping the process tree and `bw.cmd`,
  - the security page reference in the plaintext bullet of item B is checked.

  Verify that `grep -n "Memory hardening" docs/development/architecture.md` shows E as done and that the Windows experiment appears once in item 5.
- [x] 3.9 Add the section "Crash Dump Tests" to `docs/development/testing.md`, with the seams of `crashdump`, the re-executed children, the record of the fake and a scenario coverage table for every scenario of this change's spec delta. Verify the table against `go test -list . ./internal/crashdump/ ./internal/run/ ./internal/report/` and state the date of the check.

## 4. Evidence and integration

- [x] 4.1 Run the manual check of design D9 on a Linux host with systemd-coredump, once with only the core limit and once with the dumpable mark as well. Record date, distribution, systemd version and the output of `coredumpctl list` for the helper in the testing page. If no such host is available, record in the testing page that the check was not run. Either way, verify that the wording of the security page matches the recorded result.
- [x] 4.2 Run `gofmt -l .`, `go vet ./...`, `go test -race ./...` and `GOOS=darwin go vet ./...` and `GOOS=windows go vet ./...` in the dev container, and `openspec validate memory-hardening --strict`. Verify that all pass, and record the run in the testing page as earlier changes did.
## Review fixes

- [x] R1 (review of 19 November 2026, security finding S1) The rewritten section "Swap, Hibernation and Core Dumps" of `docs/user/security.md` lost the general statement that covered `bw-takeout verify`, which holds the identity and decrypted archive content and limits no crash dumps. Add a sentence that `verify` limits nothing and extend the encrypted-disk advice to systems that open a generation. Verify with `main.TestSecurityPageNamesVerifyCrashDumps`, which fails without the sentence.
- [x] R2 (review of 19 November 2026, code finding C1) `TestCrashDumpHelper` ended with `os.Exit(0)`, so the cleanups of `newEnv` never ran and every real-path test left its temporary directory, with a published generation and the fake's records, behind. The helper now returns. `runHelper` gives the child a temporary directory of its own and checks that it is empty afterwards, which fails with the old exit.
- [x] R3 (review of 19 November 2026, code finding C3) Three scenarios had no test of their name. The subtests of `report.TestCrashDumpLine` are named "Run on Linux", "Run on macOS" and "Run on Windows", and `TestRunProcessNotDumpable` checks the dumpable mark of the real-path child on Linux, with expected skips on macOS and Windows. The coverage table of the testing page names them.

## Workflow follow-up

- Run the independent code and security reviews required by the apply guidance before reporting completion.
- After the maintainer commits and pushes, check the CI run on Linux, macOS and Windows: the real-path tests of 1.3 and 3.2 ran on macOS (not skipped), and the Windows run test of 3.4 ran. Record the CI run in the testing page.
- Archive the change after review, and sync the delta into `openspec/specs/backup-run/`.
