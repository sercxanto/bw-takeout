# Design

## Context

See proposal.md for the motivation. The behavior is in the spec delta of `backup-run`.

Current state:

- `run.Run` starts with the checks of design D3 of `interactive-backup-run` (`runner.check`). They start no process and create nothing. A failed check ends the run in the phase `checks` with exit status 1, and the report prints `Failed: checks: <error>`.
- Every `bw` process runs with an empty working directory inside the profile. `bwcli` fails a command when a file appears there, and teardown removes the profile on every path.
- The setup lines of the report (`report.Writer.setup`) are printed after the version query.
- The fake `bw` appends one record per start, with its arguments, environment and working directory, and is built for Linux, macOS and Windows.
- The run tests in `internal/run` call `run.Run` in the test process, and nil dependencies take the real implementation.

Platform facts that shape the design:

- On Linux and macOS a process can always lower its own soft and hard resource limits. A child inherits them across fork and exec, and an unprivileged child cannot raise a hard limit.
- On Linux the kernel ignores the core limit when `core_pattern` starts with `|` and pipes the core to a handler. It passes the soft limit to the handler as `%c`, and the handler decides on its own.
- On Linux a process that is not dumpable gets no core at all, also not through a pipe handler, and its `/proc` files belong to root, so other processes of the same user cannot read its memory or trace it. A plain `execve` resets the mark, so it never reaches `bw`.
- The dev container runs with `core_pattern = core` and an unlimited core limit. A crash of `bw` there would write `core` into the working directory of `bw`.

## Goals / Non-Goals

**Goals:**

- One small package owns the system calls, with seams for the calls themselves, so that a failed call and a wrong read-back can be tested without privileges.
- The run tests stay independent of test order: no test lowers the limits of the shared test process by accident.
- One test shows the real path end to end in a fresh process: limit set, read back, inherited by the fake `bw`.

**Non-Goals:**

- Any change to a system setting, such as `core_pattern`, `suid_dumpable` or a Windows registry key.
- Protection of the `bw` process beyond the inherited core limit.
- Windows. Its stub reports "not limited" and changes nothing.

## Decisions

### D1. A package `internal/crashdump`

`crashdump.Limit() (Protection, error)` sets and reads back what the platform allows. `Protection` tells the report what holds: `CoreLimit` is true on Linux and macOS, `NotDumpable` is true on Linux, and both are false on Windows. Unexported variables hold the system calls (set limit, get limit, set dumpable, get dumpable), and the tests of the package replace them.

The files follow the platforms:

- `crashdump_unix.go` holds the core limit with `unix.Setrlimit` and `unix.Getrlimit` for `RLIMIT_CORE` with `{Cur: 0, Max: 0}`.
- `dumpable_linux.go` holds `unix.Prctl(PR_SET_DUMPABLE, 0)` and `unix.PrctlRetInt(PR_GET_DUMPABLE)`.
- `dumpable_other.go` is for the other Unix systems and does nothing.
- `crashdump_windows.go` returns a zero `Protection` and no error.

Two errors carry no detail beyond what failed: `ErrCoreLimit` ("core file size limit cannot be set to zero") and `ErrDumpable` ("process cannot be marked as not dumpable"). A read-back that differs from the set value gives the same error as a failed set. The report does not need to tell them apart, and the cause is never in user data.

(review of 19 November 2026, C2, also noted by the security review) Rejected: the finding that the run test of "Core limit does not read back as zero" injects the same error as the one of "Core limit cannot be set". That is the consequence of this decision: the run cannot tell the two apart, and the read-back itself is tested in `crashdump.TestCoreLimitErrors`.

Alternatives considered:

- **Inside `internal/run`.** `run` has no platform files today and would get three. A separate package keeps the system calls and their seams out of the order of the run.
- **`syscall.Setrlimit` from the standard library instead of `golang.org/x/sys/unix`.** It works for the core limit, but `prctl` has no wrapper in `syscall`. Using `x/sys/unix` for both keeps one way of calling the system. `x/sys` is already in the module graph, and Dependabot updates it.

### D2. Called first in the checks, through `Deps`

`run.Deps` gets `LimitCrashDumps func() (crashdump.Protection, error)`. Nil takes `crashdump.Limit`, so `cmd/bw-takeout` needs no change. `runner.check` calls it first, before the target is looked at. An error ends the run as `Failure{Phase: checks, Err: err}`, and the protection is stored in `Setup.CrashDumps` for the report.

First, rather than last among the checks: the point is to be done before the first `bw` process, and an earlier call costs nothing. The checks after it then never run with dumps enabled, and the spec scenario "Limit set before the first bw process" holds for every later check.

Alternatives considered:

- **In `main`, before the flags are parsed.** This is the earliest point, but `run` would then rely on an unchecked precondition that its own tests could not show, and a usage error would already have changed the process limits.
- **In `bwcli`, per started process (`SysProcAttr`).** Go offers no limit per child on Unix without a wrapper program. It would also leave the Go process itself unprotected.

### D3. Fail closed on Linux and macOS, continue on Windows

These are the maintainer decisions of 19 November 2026. On Linux and macOS lowering one's own limits needs no privilege, so a failure means something unusual, such as a seccomp filter or a sandbox, and the run cannot keep the promise of the security page. On Windows no mechanism exists yet, so the run states this and continues.

### D4. Hard limit zero as well

A soft limit of zero alone would let `bw`, or a Node option, raise the limit again up to the hard limit. With the hard limit at zero the run gives up nothing it needs: neither `bw-takeout` nor `bw` uses core files. The change is irreversible for the process and its children only, and the process ends with the run.

### D5. Report line

`report.Writer.setup` prints one more line after `Recipients:`:

| Platform | Line |
|---|---|
| Linux | `Crash dumps: core limit 0, process not dumpable` |
| macOS | `Crash dumps: core limit 0` |
| Windows | `Crash dumps: not limited on this platform` |

A run that fails the limit prints no setup lines, like every other failed check, and its `Failed:` line names the error of D1.

### D6. Tests

- **`internal/crashdump`.** Unit tests replace the system calls. They cover a failed set, a failed get, a read-back of a non-zero soft or hard limit, a failed dumpable set and a dumpable read-back of 1, each mapped to its error. A last test calls the real `Limit` in a re-executed child of the test binary, following the pattern of `internal/publish/kill_test.go`. The child reports the limits it reads afterwards and, on Linux, the dumpable value, so the test process keeps its own limits.
- **`internal/run`, in process.** The test setup injects a recording `LimitCrashDumps` by default, so no in-process test lowers the limits of the test binary. Tests cover the three failure scenarios through an injected error: no record file, no profile directory, and the phase `checks`. One test shows that the function is called before the target is looked at, by combining a missing target with a recording function.
- **`internal/run`, real path.** A re-executed child runs one full backup against the fake `bw` with the real `crashdump.Limit` on Linux and macOS. The parent reads the fake's records and checks that every start saw a soft and a hard core limit of 0. A second child fails on a missing target and reports its limits afterwards (scenario "Limit set before the first bw process").
- **Fake `bw`.** `bwclitest.Record` gets `CoreLimit *[2]uint64` (soft, hard), filled on Unix through `fakebw/rlimit_unix.go` and left nil by `fakebw/rlimit_windows.go`.
- **Core file in the working directory.** The existing `WriteFile` behavior of the fake writes `core` on the plaintext export. The test checks the run outcome, the unchanged target hash and that the profile directory is gone (scenario "bw leaves a core file").
- **Report.** The three lines of D5 are tested through `report` with synthetic `Setup` values. The Windows statement is checked by the existing Windows CI job against the real stub.
- (review of 19 November 2026, C1) The run helper returns instead of exiting, so that the cleanups of the test environment remove its temporary directories. The parent gives it a temporary directory of its own and checks that it is empty afterwards.
- (review of 19 November 2026, C3) Each scenario has a test of its name: the subtests of the report table test are named after the scenarios, and "Run process not dumpable" has its own real-path test on Linux.

### D7. ADR 0006 is updated in place, no new ADR

The CONTRIBUTING criteria are "hard to reverse", "changes integration boundaries" and "fixes an essential security assumption". The limit is easy to reverse in code and does not change the boundary to `bw`. It does fix an assumption about what may leave the boundary: a dump of a `bw` process. That assumption belongs to the secret boundary that ADR 0006 already holds, which is proposed. As with ADR 0004 in `capture-collection`, the proposed ADR is updated in place: a decision point "Crash dumps" and a consequence that pipe handlers and Windows stay outside. ADR 0006 stays proposed.

### D8. Documentation

- **Security page, section "Swap, Hibernation and Core Dumps".** What the run limits per platform. That a crash handler named by a piping `core_pattern` receives the limit and decides itself, and that this program cannot control it. That `bw-takeout` itself is not dumpable on Linux, and what that also blocks (memory reads and tracing by the same user). That Windows is not limited yet. The advice on encrypted swap stays.
- **Usage page.** The new setup line in "What the Report Shows" and the new failure of the checks.
- **Architecture page.** Rule 3 references the new state. Item E is marked done with the decisions of 19 November 2026. The Windows experiment moves into the follow-ups of item 5, together with the job object, stopping the process tree and `bw.cmd`.
- (review of 19 November 2026, S1) The security page also states that `bw-takeout verify` limits no crash dumps, although it holds the identity and decrypted archive content, and extends the advice on encrypted disks to systems that open a generation. Limiting crash dumps in `verify` is outside this change.
- (review of 19 November 2026, second round, observation of the security review, maintainer decision of the same day) The security page no longer says that a `core_pattern` naming a file writes a core into the working directory: the core limit of zero stops such a core. The stray-file handling of the working directory of `bw` stays as a second line of defense.
- **Testing page.** A section "Crash Dump Tests" with the scenario coverage table of this change, and the manual check of D9.

### D9. Manual check of a pipe handler

The dev container cannot change `core_pattern`. On a Linux host with systemd-coredump, a small helper, a test binary in a re-executed child, lowers the limits like the run and then aborts with SIGABRT. Then `coredumpctl list` shows whether a core was stored. The test is run twice: once with only the core limit, once with the dumpable mark as well. The result goes into the testing page with date, distribution and systemd version. If no such host is available during apply, the testing page records that the check was not run, and the security page keeps its wording that such a handler decides itself.

## Risks / Trade-offs

- [A pipe handler ignores `%c` and stores the core of a `bw` process anyway] → Outside this program's control. The security page says so, D9 checks the most common handler, and the advice on encrypted disks stays.
- [The not-dumpable mark breaks something that reads `/proc/self` in the run] → A process may always read its own `/proc` files, whatever the mark. The full in-process run test runs with a recording function, but the real-path child of D6 runs a full backup with the mark set and would show such a failure.
- [A user cannot attach a debugger to a running `bw-takeout`] → Accepted (maintainer decision, 19 November 2026). Debugging a run under the debugger from the start still works, because the debugger is then the parent.
- [macOS is tested only in CI] → The real-path child test runs in the macOS CI job, and the report line is checked there too.
- [`golang.org/x/sys` becomes a direct dependency] → It is already in the module graph through `golang.org/x/term`, which `bwcli` uses, and Dependabot covers it.

## Migration Plan

None. A run that starts after this change applies the limit. Existing generations and the archive format are not affected. A rollback removes the call and the report line.
