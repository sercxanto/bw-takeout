# Design

## Context

See proposal.md for the motivation. The parts the run orders exist and keep their interfaces:

- `bwcli.Client`: `Version`, `SetServer`, `Login`, `Logout` and the vault commands. Every failure is a `*bwcli.Error` with operation, class and exit code. A command whose context has ended returns the class `cancelled` without starting a process, and `Logout` forgets the session key whatever the outcome.
- `profile.Creator.Create`, `Profile.Teardown(logout)` and `Creator.Leftovers(own)`.
- `collect.Collector.Snapshot` before the target is touched, and `Collector.Attachments` with a receiver.
- `capture.Parse`, `verify.Verify` and `Result.JSON`.
- `archive.NewWriter`, `AddAttachment` and `Finish`.
- `publish.Publisher.Publish(target, write)` and `publish.ListPartial`.

Constraints that shape the design:

- `cmd/bw-takeout` holds only the offline `verify` command. Its test `TestNoNetworkOrProcessPackage` forbids `net`, `os/exec` and `internal/verify` in the binary (design D10 of `archive-generation`). `backup` needs all three.
- `bwcli.Login` installs its own signal handler while `bw login` runs, and restores the terminal afterwards. In the login exercise of 15 November 2026, Ctrl-C at the password prompt reached `bw` as a character, not as a signal, and the login failed with the class `output`.
- On Unix every `bw` command except the login runs in its own process group, so a keyboard interrupt does not reach it. On Windows it shares the console, and the console sends the interrupt to `bw` as well.
- `bw logout` clears local state only and calls no server endpoint (finding 10 of the code notes of 15 November 2026). Removing the profile is what protects the tokens on disk.

## Goals / Non-Goals

**Goals:**

- One package owns the order of the phases and teardown, testable against the fake `bw` with an injected terminal, profile location, clock, signals and logout limit.
- The report and the exit status are computed from one outcome value, so that every path has exactly one exit status.
- The proof that `verify` uses no network and starts no process stays a test, now at package level.

**Non-Goals:**

- No change to the behavior of `collect`, `verify`, `capture`, `manifest`, `publish` or `profile`.
- No core dump limit, no memory locking. Both belong to the hardening change.
- No configuration file, no machine-readable output.

## Decisions

### D1. Packages

```text
cmd/bw-takeout          flags, subcommand dispatch, signal wiring, exit
   |        \
   |         internal/offline     the verify command, moved unchanged
   v
internal/run            phases, teardown, interrupts -> Outcome
   |
internal/report         Outcome -> text on stdout, exit status
   |
internal/printable      replacement of non-printable characters (leaf)
```

- `run` returns an `Outcome` and calls a small progress interface for the lines that appear while the run goes on: the setup, the leftovers, the snapshot, the attachment results once the downloads have ended, and the interrupt message.
- `report` implements that interface and renders the final part of the outcome. It is the only package that writes run output.
- `printable` holds the `clean` function that today lives in `cmd/bw-takeout`, so that `offline` and `report` share it without `offline` importing `report`, which imports `verify`.

(review of 18 November 2026, C3, maintainer decision of the same day) The attachment lines reach the progress interface only after `collect.Attachments` has returned, because `collect` returns the download results at the end and does not change in this change. They are not printed while the downloads run, and not at all when the download phase ends early. The report still shows every result in order with its position, as the spec requires. Progress while the downloads run is a follow-up on the roadmap.

These are the components `run` and `report` that the architecture page has listed as planned. Alternative: everything in `cmd/bw-takeout`. Rejected, because the order and the teardown need tests with injected signals and a fake terminal, and the command package should only wire.

### D2. Flags and usage errors

```text
bw-takeout backup --server <https-url> --target <dir> --recipients-file <file>
                  [--bw <path>] [--ca <file>]
```

The flags come before any argument, as for `verify`, and `backup` takes no argument. Status 2 covers exactly what can be judged from the argument list:

- a missing required flag,
- an unknown flag,
- an extra argument,
- a server address that the existing rule of `bw-invocation` refuses.

That rule is exported from `bwcli` as `ValidServerAddress`, so the check runs before any process starts. Everything that needs the file system gives status 1, as an unreadable identity file does for `verify`. A target on an unmounted drive is then a failed run, not a wrong invocation.

### D3. Checks before the first bw process

In this order, all before the profile is created:

1. The target is an existing directory (`os.Stat`).
2. The recipients file parses with `archive.ParseRecipientFile` (D10) into at least one recipient.
3. `bw` resolves: the given path, or `exec.LookPath("bw")`. In both cases it is made absolute and must be a regular file after following links.
4. A given CA file opens and is a regular file.
5. `publish.ListPartial(target, now)` for the report.

The target can change between the check and the publishing. `publish.Begin` checks it again, so the check only saves the user a two-step code and is not a guarantee. Alternative: no checks before the login. Rejected, because the cost of a mistake is a login, a new device entry and a two-step code.

### D4. Order of the run

```text
checks (D3)
profile.Create, Creator.Leftovers(own)              report: setup, leftovers
client.Version  -> manifest.BW, report the bw line
client.SetServer(server)
loginStarted = true; client.Login
snap := collector.Snapshot(ctx, client)             target untouched so far
defer snap.Wipe()
publisher.Publish(target, func(w, created) {
    aw := archive.NewWriter(w, recipients, created, snap.Documents())
    downloads := collector.Attachments(ctx, client, snap, aw.AddAttachment)
    c := capture.Parse(snap.Input(downloads))
    snap.Wipe()
    result := verify.Verify(c, nil)
    return aw.Finish(Meta{snap.Source, tool, bw, result.JSON()})
})
teardown (D5)
```

- `Wipe` runs as soon as the capture is parsed, and through `defer` on every earlier exit after the snapshot. Calling it twice is harmless. The documents still live through the whole download phase, because `capture.Parse` needs the download results. This is the residual risk that the security page names (D12).
- The tool version comes from `runtime/debug.ReadBuildInfo`, the version of the main module. A local build reports `(devel)`.
- The verification result is kept in the outcome before `Finish`, so that the report can show it for a failed verdict as well, when `Publish` removes the partial directory.
- An error from `Publish` after a successful write means a commit step failed and the partial directory stays. The report says so and points to the leftover listing of the next run. `Publish` does not return the name of that directory, and a later listing cannot tell it from the partial of a concurrent run.
- (review of 18 November 2026, C1 and S1) `Publish` returns the generation together with an error when the rename succeeded and only the flush of the target directory failed. The outcome keeps that generation as published, with the error as a warning, so the exit status is 0, 3 or 4 and the report names its path.
- (review of 18 November 2026, C2 and S3) When the abort of the partial directory fails after a failed verdict or a failed write, `Publish` joins both errors. The outcome records the failure in the commit phase, or in the phase of the write, and that the partial directory stays. The report prints the line about the partial directory only then, and not after a successful abort.
- (review of 18 November 2026, C4) A panic in a phase is recovered and recorded as an internal error without its value, which could hold vault content. A panic inside the write function becomes an error of that function, so that `Publish` aborts the partial directory.

### D5. Teardown

```text
logout := nil
if loginStarted { logout = func() error { return client.Logout(teardownCtx) } }
logoutErr, removeErr := profile.Teardown(logout)
```

- `loginStarted` is set before `Login` is called, so a login that fails in any way is followed by a logout, as `loginprobe` already does.
- `teardownCtx` is derived from `context.Background()`, not from the run context, with a deadline of `LogoutLimit` (default 10 seconds, maintainer decision of 18 November 2026). D6 can cancel it.
- The logout outcome is one of:
  - not needed: no login started,
  - ok,
  - failed: with the class,
  - stopped at the limit: the deadline of `teardownCtx` passed,
  - skipped: an interrupt cancelled `teardownCtx`.

  The run tells the last two apart by the cause of the context error, not by the class of the `bwcli` failure, which is `cancelled` in both cases.
- The profile is removed after the logout whatever its outcome. The removal takes no context and is not interrupted.
- (review of 18 November 2026, C4) Teardown runs from a `defer` once the profile exists, so a panic in a phase does not skip the logout or the removal.
- (review of 18 November 2026, C5) Whenever the logout failed, the cause of `teardownCtx` decides between skipped and stopped at the limit, whatever the class. On Windows the console also sends the interrupt to `bw logout`, which can end with a non-zero exit code before the context ends.
- (review of 18 November 2026, second round, R2-C2) The cause is taken when the logout returns, not after the removal of the profile, and a signal after the logout no longer announces that the logout is skipped. The Windows case of C5 is covered as long as the signal is handled before `bw logout` has returned.
- The report shows the duration of the logout, which gives the manual exercise its measurement.

Alternative: the short time limit of `bwcli`, 60 seconds, for the logout. Rejected, because a local logout does not need a minute and an interrupted user should not wait that long.

### D6. Interrupts

```text
              first signal                    any later signal
  running  ----------------->  interrupted  -------------------> logout skipped
     |       cancel run ctx         |          cancel teardownCtx
     |       set interrupted        |
     |       print message          v
     +----- run ends ------------> teardown --- signal in teardown --> cancel teardownCtx
```

- `cmd/bw-takeout` subscribes to the signals and passes the channel to `run`. On Unix these are `SIGINT`, `SIGQUIT`, `SIGTERM` and `SIGHUP`. On Windows they are `os.Interrupt` and `syscall.SIGTERM`, which Go reports for the console close, logoff and shutdown events. The system ends a process a few seconds after such an event, so teardown may not finish there. This comes from the Go documentation and was not observed.
- One goroutine in `run` reads the channel:
  - The first signal sets the interrupted flag, cancels the run context and prints the message on standard error.
  - When teardown has already started, the first signal also cancels `teardownCtx`, and the message then says that the logout is skipped.
  - Every later signal cancels `teardownCtx`.
  - A signal that arrives before the logout starts leaves `teardownCtx` ended, so `Logout` returns at once without starting `bw` and still forgets the session key. Teardown thereby calls the logout, as `bw-profile` requires.
- An interrupt during the snapshot or the attachments ends the collection with the class `cancelled`. The write function then fails, and `Publish` aborts the partial directory.
- An interrupt during the login cancels the context of `bwcli.Login`. `bwcli` stops `bw login` and restores the terminal. When `bw` keeps the terminal in raw mode, no signal arrives at all, and the login fails with the class `output`. The report then shows a failed login, not an interrupt.
- `Publish` takes no context. An interrupt during the commit lets the commit finish, and the generation is published.
- The outcome counts as interrupted when the flag is set, whatever failure ended the run. On Windows `bw` can end on its own with a non-zero exit code before the run cancels it.

(review of 18 November 2026, S2) On Unix the command also subscribes to SIGPIPE, without acting on it. Without a subscription Go ends the process on a write to a broken pipe on standard output or standard error, for example when the output goes to `head` or a pager that was closed, and the logout and the removal of the profile would not run. With it, the write fails and the run goes on. A subscription, unlike ignoring the signal, is not inherited by `bw`.

Messages, on standard error:

```text
Interrupted. Logging out, at most 10 s. Press Ctrl-C again to skip the logout.
Interrupted. Skipping the logout.
```

### D7. Outcome and exit status

`run.Outcome` holds:

- the setup: the `bw` path, version and SHA-256, the server, the target and the number of recipients,
- the leftovers,
- the source,
- the number of planned attachments and the download results,
- the verification result, if one was computed,
- the published generation, if any,
- the failure that ended the run: its phase and the `*bwcli.Error` or the error of the step,
- the interrupted flag,
- the logout outcome with its duration,
- the removal error.

`report.ExitStatus(outcome)`:

| Published | Teardown complete | Verdict | Status |
|---|---|---|---:|
| yes | yes | verified | 0 |
| yes | yes | unverified | 3 |
| yes | no | any | 4 |
| no | any | any | 1 |

Teardown is complete when the logout was not needed or ok, and the profile was removed. Usage errors give 2 in `cmd/bw-takeout` before `run` starts. The values 0, 1, 2 and 3 match `verify`. Status 4 is a maintainer decision of 18 November 2026: a generation is in place, but a profile with tokens may be left, or the local logout did not finish, and a script must not book that as a plain success.

### D8. Report format

Line-oriented English text with fixed labels, in the style of the `verify` output. Every value passes through `printable.Clean`.

```text
bw:          /usr/local/bin/bw
bw version:  2026.10.1, sha256 3f9a...e1c0
Server:      https://vault.example.org
Target:      /mnt/backup
Recipients:  2
Leftover profile:    <path> (age 3d4h)
Leftover partial:    <path> (age 1h12m)
Sync:        fresh
Account:     user@example.org (8c1e...)
Attachments: 3 planned
  1/3 ok
  2/3 failed (status), item <uuid>, attachment <id>
  3/3 ok
Verdict:     unverified
Scope:       <verify.ScopeStatement>
Limits:      server_completeness_not_checked, ...
Findings:    1
  unverified attachment_download_failed item=<uuid> attachment=<id>
Warnings:    0
Generation:  /mnt/backup/bw-takeout-20261118T101500Z-4f2a91c0.unverified
Logout:      ok (1.4 s)
Profile:     removed
```

- A finding line shows severity, reason and each field of `verify.Finding` that is set: document, item, attachment, passkey, field, expected, found, count and the counts per organization. These are IDs, positions, counts and codes only (design D10 of `capture-verification`).
- When the status holds no server URL, the server line of the source repeats the flag value. Otherwise it shows what the status reported.
- A failure line names phase, operation, class and exit code from `bwcli.Error.Error()`, which carries no `bw` text. An interrupted run prints `Interrupted during <phase>.` instead of the class.
- (review of 18 November 2026, second round, R2-C1) "Instead of the class" applies to a failure recorded after the first signal. A failure that ended the run before the signal, such as a refused login followed by Ctrl-C during the logout, is named with its class as well, followed by the interrupt line.
- (review of 18 November 2026, second round, R2-C3) Joined errors, such as a refused verdict together with a failed abort, are printed on one line with "; " between the parts.
- Findings are listed in full (maintainer decision, 18 November 2026), in the order `verify` sorts them.
- Standard output only. The login prompts of `bw` and the messages of D6 go to standard error. Progress and report then stay together when standard output is redirected.

### D9. The offline command moves into its own package

`verifyGeneration`, `readIdentities`, `reportReadError` and `storedResult` move from `cmd/bw-takeout` to `internal/offline`, together with their unit tests. `cmd/bw-takeout` calls `offline.Run(args, stdout, stderr)` for `verify`.

The dependency test moves along. It runs `go list -deps bw-takeout/internal/offline` and fails on any of the following:

- `net` and its subpackages,
- `os/exec`,
- `internal/verify`, `internal/bwcli`, `internal/collect` and `internal/run`.

`TestSystemWithoutBW` stays in `cmd/bw-takeout` and runs the binary with an empty search path. This replaces the binary-level check of design D10 of `archive-generation` (maintainer decision, 18 November 2026). The guarantee is weaker, because the delivered binary now contains process code. The `verify` path still cannot reach it.

Alternative: two binaries, so that the offline system gets a file without network and process code. Rejected for the prototype: it costs a second command name and more release work. A binary that contains code it never calls is no more dangerous on the offline system, unless it was tampered with, and then no dependency list helps.

### D10. Recipients file

`archive.ParseRecipientFile(io.Reader) ([]string, error)` reads one native age X25519 recipient per line. Empty lines and lines starting with `#` are skipped, and the whitespace at both ends of a line is trimmed. Every other line must parse with `ParseRecipients`. Errors wrap a new `ErrRecipientFile` and name the line number, never the content, like `ParseIdentityFile`. The function returns strings, because `NewWriter` takes strings and parses them itself.

(review of 18 November 2026, S4) The example on the usage page holds placeholders that do not parse, so that a copied example is refused instead of encrypting to keys whose holders are unknown.

### D11. Tests

- **Run against the fake `bw`.**
  - `internal/run` tests build the fake with `bwclitest.Build` in `TestMain`. A profile function wraps `profile.Creator.Create` on a temporary location and writes the scenario into the new data directory.
  - The terminal is a fake `bwcli.Terminal`.
  - Signals are a channel that the test writes to.
  - The logout limit is injected: 200 ms in the test of a hanging logout, with a separate test that the default is 10 s.
  - Documents and attachments come from `capturetest`.
- **Interrupt tests.**
  - A fake command with `DelayMillis` or `Hang` holds the run in the intended phase. The test sends the signal once the record file shows that the command started.
  - Paths: during the snapshot, during an attachment, during a hanging logout, a second signal before the logout, and a signal during the commit. That last one uses a publisher whose write function blocks on a test channel.
- **Target and temporary files.**
  - The target is hashed before and after, with the `snapshot` helper of the existing `cmd/bw-takeout` tests.
  - `TMPDIR`, or `TMP` and `TEMP` on Windows, points at an empty directory that must stay empty.
- **Redaction.** The sentinel values of `capturetest`, a sentinel in the standard error of every fake command and an escape sequence in the status e-mail. Both output streams are searched.
- **Exit status.** A table test of `report.ExitStatus` over every row of D7, and one run test per status.
- **Command.** `cmd/bw-takeout` tests cover the usage errors without any `bw`, and the checks before the first process with an empty search path.
- **Manual exercise.** A synthetic account on the Vaultwarden instance of the login exercise holds:
  - a login item with a passkey,
  - a personal attachment,
  - an archived item with an attachment.

  The maintainer runs `bw-takeout backup` once with standard output on the terminal, once redirected to a file, and once with Ctrl-C during the attachments. Then `bw-takeout verify` runs on the result. The record states the logout duration from the report, the exit statuses and the device entries. It does not include the archive.

### D12. Documentation of the plaintext residual risk

Rule 3 of the architecture page becomes: plaintext exists only in process memory and inside the encryption stream, this program writes none of it to a file, and the operating system can still put process memory on disk. `docs/user/security.md` gains a section on swap, hibernation and core dumps:

- the four documents stay in memory during the whole download phase,
- the advice to use encrypted swap or an encrypted disk,
- the note that the hardening change will disable core dumps.

(review of 18 November 2026, C6) The README, the getting-started page and the deferred FAT and exFAT note of the testing page still said that no backup command exists. They were updated.

(review of 18 November 2026, second round, R2-C4) The first sentence of the README and the FAT and exFAT note were reworded. The spec scenario "Sentinels in the vault and in bw output" now exempts the login prompts of `bw`, which reach the terminal directly by design and are not part of the report.

The second round of the security review found nothing. The fixes of the second round were made with the maintainer's approval without a third review round.

### Not an ADR

The exit statuses, the flags, the logout limit and the package split are reversible, move no integration boundary and leave the security assumptions of ADR 0006 as they are. The dependency proof of D9 changes how a guarantee is checked, not the guarantee, so it stays a design decision of this change.

## Risks / Trade-offs

- [The interrupt path on Windows was never run against the real `bw`] → The run decides from its own flag (D6), and the Windows CI job runs the interrupt tests against the fake. The limitations page states that the real `bw` on Windows is untested.
- [Ctrl-C at a `bw` prompt is a character, not a signal] → The login fails with the class `output`, and teardown runs as for any failed login. The report shows a failed login. This was observed in the login exercise and is acceptable.
- [10 seconds can be too short for a slow Windows system] → The logout is stopped, the profile is still removed, and the status is 4 if a generation was published. The manual exercise measures the duration. The limit is a constant in `run` that a later configuration can expose.
- [Status 4 even when only the local logout was stopped and the profile is gone] → Chosen for one clear rule. The report says which part was incomplete.
- [Long output when a broken export gives one finding per item] → Accepted for the prototype. `--json` is a follow-up.
- [The binary that runs offline now contains process and network code] → The package test of D9 and the end-to-end test with an empty search path keep the proof for the `verify` path.
- [The checks of D3 can pass and the target can disappear afterwards] → `publish.Begin` checks again, and the run then ends without a generation.
- [A `bw` found on the search path can be a different program than intended] → The report names the resolved path and the SHA-256 before the login, and `--bw` pins the path. ADR 0007 keeps both values unjudged.
- [The documents stay in memory through the download phase, decoded strings until the garbage collector frees them] → Documented in the security page (D12). The hardening change follows.

## Migration Plan

No migration. `verify` keeps its command line, output and exit statuses. No generation format changes.

## Open Questions

- On Windows, `npm` installs `bw` as a `bw.cmd` script. Whether starting it through `exec.LookPath` behaves like the single-file `bw.exe` for arguments, the environment and stopping, is untested. The fake is an `.exe` in CI. This can be examined with the hardening change or the first Windows run, without changing this design.
- Whether progress should move to standard error once a machine-readable report exists.
