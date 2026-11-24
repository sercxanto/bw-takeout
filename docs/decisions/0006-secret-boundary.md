# Secret Boundary around the bw Process

Status: proposed

## Context

This project starts the unmodified `bw` ([ADR 0001](0001-official-cli.md)) and reads its results. The process boundary is where the known false successes arise, and where secrets cross ([live probe](../research/vaultwarden-live-probe-2026-11-14.md), [code notes](../research/bw-cli-code-notes-2026-11-15.md)):

- `bw login --raw` without a terminal exits with code 0 and prints nothing.
- Any non-empty `BW_CLEANEXIT` in the environment turns a failure into exit code 0. The Node runtime reads further variables such as `NODE_OPTIONS` and `NODE_TLS_REJECT_UNAUTHORIZED`.
- A vault command on a locked vault starts a prompt unless `BW_NOINTERACTION` is `true`, and can then wait forever.
- An export without `--raw` writes a plaintext file into the working directory.
- Error text of `bw` can name items, and the first start prints the full path of the profile. `bw` has no error codes: every failure exits with code 1 and carries only message text.
- The session key unlocks the vault for as long as the profile is logged in.

How these are handled is an essential security assumption of the project. It fixes where the session key may exist, what text of `bw` may reach a user, and which signals decide a result.

## Decision

- **Environment.** Every `bw` process gets an environment built from nothing: a fixed allowlist of variables of the caller, the data directory of the run, `BW_NOINTERACTION=true` for every command except the login, the session key between login and logout, and a configured certificate authority file. No other variable of the caller reaches `bw`.
- **Session key.** It is passed to `bw` only in the environment of a child process, only between a successful login and the logout. It is never an argument, never written to a file, and never part of an error or any output of this project. The client holds it in a byte slice that is overwritten at logout whatever the outcome.
- **Login.** It runs only with a terminal on standard input and standard error, which `bw` uses directly. Standard output goes through a pipe. The login counts as successful only with exit code 0 and exactly one session key of the expected form. The terminal state is saved before and restored after the login, and interrupts are held while `bw` runs.
- **Other commands.** They run with an empty standard input, a time limit and an empty working directory. A file that appears there fails the command. Standard output is held in memory up to a limit and is never written to a file by this project.
- **Failures.** A failure carries the operation, a class from a fixed list, the exit code and, for a download, the two IDs, and for an organization export, the organization ID (change organization-export). Each ID is validated before it becomes an argument. Text of `bw` is never copied into a failure or an output, with one exception: the key path of a warning about an export key unknown to this project may hold key names of an export document that have the form of an identifier, an ASCII letter followed by at most 63 ASCII letters or digits. Any other unknown key is written as `<other>`, and no value of the document is copied (change bw-version-2026-11, design D5, maintainer decision of 24 November 2026; residual risk on the [security page](../user/security.md#key-names-in-the-report)). Standard error is read as a stream: decryption error lines are counted, nothing else is kept, and nothing is forwarded. The classes not logged in and locked are taken from a status query after the failure. Only premium required is recognized from a fixed sentence of `bw`, and only on an attachment download. No class may lead to a more favorable result than a general failure.
- **Crash dumps.** A dump of a `bw` process would carry what it decrypted out of the boundary. On Linux and macOS the run sets the soft and the hard core file size limit of its own process to zero before the first `bw` process starts, and every `bw` process inherits both. On Linux the run also marks its own process as not dumpable. The mark does not reach `bw`, because starting a program resets it. A run that cannot set the limit or the mark, or does not read it back as set, starts no `bw` process. On Windows nothing is limited yet, and the run says so (maintainer decisions, 19 November 2026; design of the change memory-hardening).

The current profile lifetime, a fresh CLI data directory per run that is removed at teardown, is a choice of the prototype and not part of this decision. The maintainer wants to revisit it.

## Alternatives

- **An inherited environment with a list of removed variables.** The list would have to follow every variable that a later `bw` or Node version reads. `BW_CLEANEXIT` shows the cost of one missed entry: a failed command with exit code 0.
- **`--session` as an argument.** Arguments are visible to every user of the system in the process list on Linux and macOS. The environment is readable only by the same user.
- **A pseudo-terminal, with this project relaying the prompts.** It needs a different mechanism on Windows and would put the master password and the second-factor code into the memory of this project. The direct connection works on Linux according to the live probe.
- **Forwarding standard error to the user.** It would show `bw`'s own diagnosis, but it prints the profile path and can carry item names, and it would put English text of one `bw` version into this project's output.

## Consequences

- The session key is visible to processes of the same user that can read the environment of a child process. This is accepted, as the [CLI analysis](../research/bitwarden-cli-analysis-2026-10-31.md) of 31 October 2026 recommended `BW_SESSION` only in the environment of the child that needs it.
- Copies of the session key can stay in the memory of this process. The Go runtime builds the environment of a child from strings, and `os/exec` copies the standard output of the login into the client through an internal buffer. Overwriting the byte slice of the client reaches neither.
- Diagnostic text of `bw` is not available to the user. A failure names the operation, the class and the exit code, so the user has to run `bw` by hand to learn more.
- A crash handler that a piping `core_pattern` names on Linux, such as systemd-coredump, receives the core limit of a crashed `bw` process and decides on its own whether it stores the core. Windows Error Reporting is not limited at all. Both stay outside this boundary until a later change examines them.
- No debugger or process of the same user can attach to a running `bw-takeout` on Linux, a side effect of the not-dumpable mark that is accepted.
- One failure class, premium required, still depends on an English sentence of `bw`. It is therefore not allowed to improve a result, and a changed sentence falls back to a general failure.

The rules in detail are in the design of the change bw-process-adapter, decisions D2 to D7 and D15, and for crash dumps in the design of the change memory-hardening, decisions D1 to D5.
