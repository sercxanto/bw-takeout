# Proposal

## Why

During a backup, the plaintext of the vault is held by two kinds of processes: `bw-takeout`, which holds the four captured documents until they are parsed, and every `bw` process, which decrypts the whole vault, the master password or one attachment. A crash of any of them can write that memory to a core file, and the [security page](../../../docs/user/security.md#swap-hibernation-and-core-dumps) still says that core dumps are not limited. Roadmap item 4 E was moved out of `interactive-backup-run` for this, and the explore session of 19 November 2026 settled its scope.

## What Changes

- **Core dumps limited on Linux and macOS.** Before the first `bw` process starts, the run sets the soft and the hard core file size limit of its own process to zero. Every `bw` process inherits the limit and cannot raise it.
- **The Go process not dumpable on Linux.** The run also marks its own process as not dumpable. The kernel then writes no core for it, also not through a crash handler that `core_pattern` names, and other processes of the same user can no longer read its memory or trace it. The mark is reset when a `bw` process starts, so it protects only `bw-takeout` itself (maintainer decision, 19 November 2026).
- **Fail closed** (maintainer decision, 19 November 2026). On Linux and macOS a run whose limit or mark cannot be set, or does not hold when read back, ends in the checks phase with no generation, before a profile is created and before any `bw` process starts. No login and no two-step code is used.
- **Windows continues unchanged.** A Windows run is not limited and says so in the report. Windows gets its own experiment, outside this change (maintainer decision, 19 November 2026).
- **Report.** The setup lines name the crash dump protection of the run: the core limit and, on Linux, the mark, or that crash dumps are not limited on this platform.
- **The existing safeguard is stated.** Every `bw` process already runs in an empty working directory, and a file that appears there ends the run, after which the profile is removed. A core file that a relative `core_pattern` puts there is handled the same way. The spec now names this case.
- **Documentation.**
  - Security page: what is limited on which platform, and what stays outside the control of this program, such as a crash handler that ignores the limit.
  - Usage page: the new report line and the new failure of the checks.
  - Architecture page: item E done, Windows moved to the follow-ups next to `bw.cmd`.
  - ADR 0006 updated in place with a point on crash dumps. It stays proposed.

Not part of this change:

- Windows: Windows Error Reporting, job objects, and stopping the process tree, which also concerns `bw.cmd`. Recorded as a follow-up experiment.
- Locking memory against swap, and hibernation (roadmap item 6, platform hardening).
- Clearing decoded strings in Go memory, which stays a documented residual risk.
- A guard against debugger attach on macOS (`PT_DENY_ATTACH`), which does not affect core dumps.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `backup-run`: new requirements for limiting crash dumps before the first `bw` process, failing closed when that is not possible on Linux and macOS, the Windows statement, the report line, and a core file in the working directory of `bw`. Existing requirements stay as they are.

## Impact

**Code.**

- `internal/run`: one more dependency for limiting crash dumps, with the real implementation for Linux and macOS and a stub for Windows. It is called first in the checks. Two new errors of the checks, and a field in `Setup` for the report.
- `internal/report`: one more setup line.
- `internal/bwcli/fakebw` and `bwclitest`: each start also records the core limit the fake sees, so that tests can show the inheritance.
- `go.mod`: `golang.org/x/sys` becomes a direct dependency for `prctl` on Linux. It is already an indirect dependency, and Dependabot covers it.

**Documentation.** [security](../../../docs/user/security.md), [usage](../../../docs/user/usage.md), [architecture](../../../docs/development/architecture.md), [testing](../../../docs/development/testing.md) and [ADR 0006](../../../docs/decisions/0006-secret-boundary.md).

**Existing ADRs.** [ADR 0006](../../../docs/decisions/0006-secret-boundary.md) gets the crash dump point and stays proposed. [ADR 0002](../../../docs/decisions/0002-go.md) is not affected: Windows evidence stays open.

**Open evidence.**

- Whether systemd-coredump, apport and abrt honour a core limit of zero when `core_pattern` pipes to them is checked by hand on a real distribution, not in the dev container, whose `core_pattern` is plain `core`. Until then the security page says only that such a handler decides on its own.
- macOS is tested in CI only, with the fake `bw`.
