# Proposal

## Why

The project can judge, store and check a capture, but nothing starts `bw` yet. The process boundary is where the known false successes arise: an exit code 0 without a session key, an exit code that an inherited variable turns into 0, a prompt that makes a command hang, and error text that can carry names. The live probe and a code reading of the installed 2026.10.1 bundle give enough evidence to build this boundary now, against a fake `bw`, before the backup run depends on it.

## What Changes

- Add the package `internal/bwcli`, the only code that starts `bw`. It builds the child environment from an allowlist, keeps the session key in the environment of child processes only, collects standard output in memory, and reports failures by operation, class and exit code without copying CLI text.
- Add typed operations for the commands the backup run needs: version, status, server configuration, login, forced sync, both exports, the item list with and without archived items, the download of one attachment by ID, and logout. Collecting a capture from them stays with the next change.
- Add the interactive login. It runs only with a terminal on standard input and standard error, reads the session key from a pipe and checks its form instead of trusting the exit code.
- Set `BW_NOINTERACTION` and an empty standard input for every command except the login, with a time limit, so that no command can wait on a prompt.
- Report the `bw` version and the SHA-256 of the executable. The version never stops a run and never produces a message for the user (maintainer decision of 15 November 2026).
- Add the package `internal/profile`: a fresh CLI data directory per run, removal at teardown also when the logout fails, and a list of leftovers that are reported and never removed (maintainer decisions of 15 November 2026).
- Add a fake `bw` for the tests, built from this repository, that reproduces the observed behaviors. These tests run on all CI platforms.
- Add tests with the real `bw` without login. They run only when a switch is set and only in the dev container, because a fresh profile contacts Bitwarden Cloud once.
- Add a small development tool and a procedure for a manual login exercise against a Vaultwarden instance in the dev container: five cases in the dev container and the two terminal-dependent cases on macOS. This is the evidence that [ADR 0002](../../../docs/decisions/0002-go.md) asks for on interactive process handling.
- Record the secret boundary around the `bw` process and the handling of `bw` versions as two proposed ADRs.
- Record the code-level findings of 13 and 15 November 2026 as a research note, because none of them is in the repository yet.

This change adds no backup command, no capture collection, no sync freshness rule and no automated authenticated test.

## Capabilities

### New Capabilities

- `bw-invocation`: How the unmodified `bw` is started and how its results are read: environment, session key, interaction, output handling, failure classes and the interactive login.
- `bw-profile`: The CLI data directory of a run: creation, isolation from the user's own CLI profile, teardown and leftovers.

### Modified Capabilities

None. `capture-verification`, `backup-archive`, `generation-publishing` and `offline-verification` are not changed. The manifest already has a field for the `bw` version.

## Impact

**Product requirements addressed** ([requirements](../../../docs/product/requirements.md)):

- The first prototype handles one account interactively with TOTP. This change supplies the login and the process handling for it.
- Errors must stay visible. Exit codes alone are not trusted, and failures are classified.
- Linux, macOS and Windows are target platforms. The non-interactive part is tested on all three in CI with the fake. The interactive login is exercised by hand on Linux and macOS. Windows stays open.

**Code:** new packages `internal/bwcli` and `internal/profile`, a fake `bw` used only by tests, and a development tool under `internal/tools/`. The two ID rules of `internal/archive` move to a small shared package without a change in behavior. New direct dependency `golang.org/x/term` for terminal detection and for restoring the terminal state. `cmd/bw-takeout` is not changed.

**Tests and CI:** `.github/expected-skips.txt` gains entries for the tests that need the real `bw`. The workflow itself is not changed.

**Documentation to update:**

- `docs/development/architecture.md`: the profile per run is now a maintainer decision for the prototype, `bwcli` reports version and hash instead of checking them, component status, and the follow-ups listed below in the roadmap
- `docs/development/testing.md`: the fake, the switch for real-binary tests, scenario coverage, and the record of the manual login exercise
- `docs/development/integrations.md`: the internal list of tested `bw` versions, the Cloud contact of a fresh profile, and the Vaultwarden instance for the manual exercise
- `docs/development/releases.md`: third-party notice for `golang.org/x/term`
- `docs/user/security.md` and `docs/user/limitations.md`: session key and profile handling, one device entry per run on the server, the Cloud contact also for self-hosted servers, and the premium requirement for personal attachments
- `docs/research/`: a note with the code-level findings and a report of the manual login exercise, each with a row in the research overview
- `docs/decisions/`: two new proposed ADRs, an update of ADR 0002, and the index
- `README.md`: project status

**Existing ADRs:**

- [0001 Official bw CLI](../../../docs/decisions/0001-official-cli.md), accepted: binds this change. The adapter uses public commands and options only. It checks that a file exists in the profile directory but never reads the profile.
- [0002 Go](../../../docs/decisions/0002-go.md), proposed: this change gives the first evidence on interactive process handling, for Linux and macOS. The ADR is updated with it and stays proposed, because Windows is not exercised.
- [0004 Completeness verdict](../../../docs/decisions/0004-completeness-verdict.md), proposed: not changed. The count of decryption error lines is only made available here. How it enters a result is decided with the backup run.
- [0003 age](../../../docs/decisions/0003-age.md) and [0005 Archive format](../../../docs/decisions/0005-archive-format.md), proposed: not touched.

**Follow-ups this change records but does not do:**

- `interactive-backup-run`: capture collection, the freshness rule "lastSync is later than the start of the run" before the capture, teardown on abort, and what a premium failure means for the verdict, never more lenient than another failed download.
- A separate explore session on an automated Vaultwarden test setup, including account creation without a person.
- A separate small change that reports export fields unknown to this project at run time.
- Reusing one profile across runs in a logged-out state, so that the server keeps one device entry. To be looked at before a first release.
- Comparing passkey counters with an earlier generation. Reading an earlier generation needs an identity, which ADR 0003 keeps off the backing-up system, so this fits an offline comparison of two generations.

**Open evidence:**

- Everything about `bw` in this change is code-level evidence for 2026.10.1, the Vaultwarden live probe on Linux arm64, or an unauthenticated run in the dev container. Bitwarden Cloud is untested.
- The login from a Go parent process is untested on every platform until the manual exercise has run. Windows is not planned in this change.
- Whether a failed sync during login leaves an empty local state from which an export succeeds with zero items is untested. The fake models it. The check belongs to the backup run.
- The premium requirement for personal attachments is code-level evidence only.
- Whether Bitwarden Cloud limits device entries, throttles repeated logins or sends a message per new device is unknown.
- What `bw logout` leaves in a profile is known from code reading only. The manual exercise records it.
- The login can run account migrations after a successful authentication. What they change on the account was not read. To be examined before the backup run is used with a real account.
- On Windows, `bw` installed through npm is started through a command script. Stopping it on a timeout may leave the Node process running. Untested.
- The SHA-256 of the executable does not cover the separate SDK module file of the npm layout.
