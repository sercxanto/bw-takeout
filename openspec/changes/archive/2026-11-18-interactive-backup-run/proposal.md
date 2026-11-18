# Proposal

## Why

Every step of a backup exists and is tested on its own: the `bw` process and its profile, the collection of a capture, the verification, the archive and the publishing of a generation. Nothing orders them, so the product still cannot make a backup. Roadmap item 4 C adds the `backup` command and completes the first prototype. The explore session of 18 November 2026 settled its open questions and moved memory hardening into a separate change that follows this one.

## What Changes

- **New `backup` subcommand** of `bw-takeout`. One run backs up one account interactively, with the login and its two-step code typed at the terminal.
- **Inputs as flags only** (maintainer decision, 18 November 2026).
  - Required: `--server <https-url>`, `--target <directory>` and `--recipients-file <file>`.
  - Optional: `--bw <path>`, otherwise `bw` is looked up on the program search path, and `--ca <file>`.
  - `--server` is required so that a fresh profile never falls back to the default cloud server unnoticed. The recipients file has the form of the identity file: one age recipient per line, with empty lines and `#` comments skipped.
  - There is no configuration file. It follows with roadmap item 6.
- **Checks before the login.** Before any `bw` process starts, the run checks that the target is an existing directory, that the recipients file yields at least one recipient, that `bw` can be found and that the CA file can be read. A mistake then costs no two-step code.
- **Order of the run:**
  - profile, version, server and login,
  - the snapshot, while the target is still untouched,
  - the generation, with the attachments, the verification and the archive inside the publishing step,
  - teardown on every path.

  The four documents are wiped as soon as they are parsed.
- **Teardown.** A logout follows every login attempt, also a failed one. The logout has a limit of 10 seconds. An interrupt during teardown ends the logout at once. The profile is always removed afterwards.
- **Interrupts.**
  - The first interrupt ends the run. A generation that is not committed yet is aborted.
  - The run prints a message that the logout follows and that another interrupt skips it.
  - On Unix, SIGTERM and SIGHUP count as interrupts.
  - The run decides from its own record that it was interrupted, not from the class of the `bw` process that ended last. On Windows the console sends the interrupt to `bw` as well.
- **Report.**
  - On standard output, without vault content. The login prompts and the interrupt message stay on standard error.
  - It names the `bw` executable with version and SHA-256, the server, the target, the number of recipients, leftover profiles and leftover partial generations, the account and the progress per attachment, with IDs only for failures.
  - It names the verdict with scope and standing limits, **every** finding and warning, one per line, the generation path and the teardown result.
  - For a failed verdict the report is the only record, because no generation is published (maintainer decision, 18 November 2026).
- **Exit statuses** (maintainer decision, 18 November 2026):

  | Status | Meaning |
  |---:|---|
  | 0 | Generation published, capture verified, teardown complete |
  | 3 | Generation published, capture unverified, teardown complete |
  | 4 | Generation published, but the logout did not succeed or the profile was not removed |
  | 1 | No generation |
  | 2 | Invalid arguments |

  0, 1, 2 and 3 mean the same as for `verify`.
- **One binary.** `bw-takeout` now links `bwcli`, which starts processes, and `verify`. The offline command moves into its own package. The existing dependency test then checks that package instead of the whole binary (maintainer decision, 18 November 2026). The `verify` command and its behavior do not change.
- **Documentation.**
  - Usage page: the `backup` command and its exit statuses.
  - Security page: the residual risk of swap, hibernation and core dumps until the hardening change.
  - Architecture page: rule 3 corrected, the roadmap with the new hardening change.

Not part of this change:

- memory hardening, which disables core dumps on Unix and examines the Windows path, as a separate change after this one,
- a machine-readable report such as `--json`,
- an exit status that tells an aborted run from a failed verdict,
- a configuration file, several accounts, batch operation and login data taken from another vault (noted for roadmap item 6),
- the FAT and exFAT run (item D).

## Capabilities

### New Capabilities

- `backup-run`: the interactive backup of one account from the command line. It covers the inputs and the checks before the login, the order of the phases around the target, the logout after every login attempt, teardown and interrupts, the report and the exit statuses.

### Modified Capabilities

None. The run uses the existing requirements as they stand. Teardown still attempts the logout, as `bw-profile` requires, and calls it with an ended context when a second interrupt came first. The `verify` command keeps the behavior that `offline-verification` requires. Only the test that proves it changes.

## Impact

**Code.**

- New package `internal/run`: phases, teardown and interrupts, against injected profile, terminal, clock and signals.
- New package `internal/report`: text and exit status.
- New leaf package for the replacement of non-printable characters, which `verify` and the report share.
- New package for the offline command, which moves out of `cmd/bw-takeout` together with its tests on dependencies.
- `cmd/bw-takeout`: the subcommand dispatch, the flags and the wiring of signals.
- `internal/archive`: a parser for a recipients file.
- `internal/bwcli`: the check of the server address is exported, so that the check before the login can use it.

`collect`, `verify`, `capture`, `manifest`, `publish` and `profile` do not change.

**Requirements affected.** The following are now exercised by a command, not only by tests:

- `bw-profile`: teardown and leftovers.
- `bw-invocation`: login, logout, server address, version report.
- `capture-collection`: both phases.
- `generation-publishing`: the verdict decides publication, leftover partial generations are reported.
- `offline-verification`: "Verification works offline and without bw". Its proof moves from the binary to a package.

**Documentation.**

- `docs/user/usage.md`.
- `docs/user/security.md`: residual risk, session key, profile.
- `docs/user/limitations.md`: interrupts, no configuration file, one account.
- `docs/user/configuration.md`: the flags as the interim configuration.
- `docs/development/architecture.md`: components `run` and `report`, rule 3, roadmap C done and the new hardening item.
- `docs/development/testing.md`: run tests against the fake `bw`, the manual backup exercise against Vaultwarden.

**ADRs.**

- [ADR 0006](../../../docs/decisions/0006-secret-boundary.md) (proposed) is checked against the report and the interrupt path.
- [ADR 0004](../../../docs/decisions/0004-completeness-verdict.md) (proposed): its scope claim reaches the user through the report.
- [ADR 0003](../../../docs/decisions/0003-age.md) (proposed): the recipients file keeps the backing-up system free of identities.
- No new ADR. The exit statuses, the flags and the logout limit are reversible and move no integration boundary.

**Open evidence.**

- A full run against a real server has not happened. The manual exercise of this change runs one against Vaultwarden, including an archived item with an attachment, which was not observed yet, and measures how long the logout takes.
- How interrupts reach `bw` on Windows is known from documentation only. CI runs the fake `bw` there, never the real one.
- That the login works with a redirected standard output follows from the code, because only standard input and standard error must be a terminal. The manual exercise confirms it.
- Evidence level of the automated tests: mock, with the fake `bw`.
