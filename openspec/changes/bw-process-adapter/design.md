# Design

## Context

See [proposal.md](proposal.md) for motivation. The repository holds the pure packages `internal/capture`, `internal/verify`, `internal/manifest`, `internal/archive` and `internal/publish` and the command `bw-takeout verify`. Nothing starts `bw`. The architecture rule that binds this change: only one component knows `bw`, and everything else works against an interface and is testable with a fake ([architecture](../../../docs/development/architecture.md)).

Decisions the maintainer took on 15 November 2026, before this proposal:

1. A fresh profile per run for the prototype. Reusing a profile is looked at later.
2. The login is part of this change and is exercised by hand on macOS, against a Vaultwarden instance in the dev container. Windows is not exercised now.
3. The `bw` version never stops a run and never produces a message for the user. Tested versions are tracked in the developer documentation only. A message appears only when export fields differ, which is a separate later change.
4. Tests with the real `bw`, with or without login, run only in the dev container and only behind a switch. Automated tests with login wait for a separate explore session on a Vaultwarden test setup.
5. Leftover profiles are reported and not removed.

Evidence this design builds on, with its level:

- **Live probe, bw 2026.10.1 against Vaultwarden 1.37.4, Linux arm64** ([report](../../../docs/research/vaultwarden-live-probe-2026-11-14.md)): `bw login --raw` with standard input and standard error on a terminal and standard output on a pipe printed exactly one base64 string of 88 characters that decodes to 64 bytes, without a line ending. Without a terminal it exited with code 0 and empty output. With `BW_NOINTERACTION=true` the same case exited with code 1. On a decryption failure, export and list exited with code 0 and wrote three `ERROR bitwarden_crypto…` lines to standard error. `get attachment --itemid <item> --raw` by attachment ID returned the exact bytes and wrote no file. bw refuses servers without HTTPS, and a self-signed certificate needs `NODE_EXTRA_CA_CERTS`.
- **Code level, installed 2026.10.1 bundle** (`build/bw.js`, SHA-256 `8a443d11…ee9b`, read on 13 and 15 November 2026, line numbers of that file):
  - The bundle reads these variables: `BW_SESSION`, `BW_NOINTERACTION`, `BW_CLEANEXIT`, `BW_RESPONSE`, `BW_QUIET`, `BW_RAW`, `BW_PRETTY`, `BW_SERVE`, `BW_CLIENTID`, `BW_CLIENTSECRET`, `BITWARDENCLI_APPDATA_DIR`, `BITWARDENCLI_DEBUG`, `ADDITIONAL_REGIONS`, `BITWARDEN_DESKTOP_PROXY_PATH`, `http_proxy`, `https_proxy` (lowercase only), `HOME`, `APPDATA`, `XDG_CONFIG_HOME`.
  - Any non-empty `BW_CLEANEXIT` turns a failure into exit code 0 (32091).
  - A directory `bw-data` next to the executable of the process takes precedence over `BITWARDENCLI_APPDATA_DIR` (96777).
  - Every prompt is written to standard error. A vault command on a locked vault starts the unlock prompt, and tries the desktop application first, unless `BW_NOINTERACTION` is `true`. Then it ends with "Vault is locked." (32219).
  - `lastSync` is written in two places only: after a successful full sync and after a successful "no sync needed" check (72953, 72989). The value is taken before the request is sent.
  - The login runs a full sync without error propagation (39134). A failed sync still leads to a session key. `bw sync --force` propagates the error (45555).
  - An export without `--output` goes to standard output only when `--raw` is set. Otherwise a file with a default name is written (31690).
  - `get attachment` requires premium for an attachment of a personal item (57898 and following).
  - `bw logout` clears local state and calls no server endpoint for the device. The bundle contains a device deactivation call that no command uses. The device ID is global to the profile, independent of account and server.
  - `bw login` refuses while an account is logged in. `bw config server` refuses while an account is logged in (39769).
  - The login can run account migrations after a successful authentication (39147). What they change was not read.
- **Observed without login, dev container, 15 November 2026**: `bw --version` in an empty profile printed the version and a line ending, wrote "Could not find data file, "<full path>"; creating it instead." to standard error, and created `data.json` with mode 0600. The file then held a new device ID and server configuration fetched from `https://api.bitwarden.com`. Running `bw config server <url>` as the first command did not avoid this request. `bw status` printed `{"serverUrl":null,"lastSync":null,"status":"unauthenticated"}` with exit code 0. `sync` and `list items` without login exited with code 1 and "You are not logged in.". With `BW_CLEANEXIT=false` the failing sync exited with code 0. `--version` took 0.8 s and `status` 2.6 s.
- **Not confirmed by any run**: the login started from a Go parent process, on any platform. Bitwarden Cloud. The premium failure. An export after a failed login sync. What `bw logout` leaves in a profile.

## Goals / Non-Goals

**Goals:**

- One package that starts `bw`, with every rule of the `bw-invocation` spec enforced in one place.
- Tests that run offline on Linux, macOS and Windows against a fake `bw` built from this repository.
- A login that works without a pseudo-terminal, shown by hand on Linux and macOS.
- Failures that a later backup run can map to its own messages without ever seeing CLI text.

**Non-Goals:**

- Collecting a capture: merging the two item lists, the download loop, building `capture.Input`.
- The freshness rule, the comparison before and after, teardown on abort of a whole run, user output and exit codes of a backup command.
- A message about `bw` versions or export fields at run time.
- Passing a proxy to `bw`. The bundle reads only the lowercase proxy variables. Whether and how a proxy is configured is decided with the configuration format.
- Finding the `bw` executable. The caller supplies an absolute path.
- Login with an API key, with single sign-on, or with a password from a file or variable.
- Killing a process tree on Windows, and Windows access control lists for the profile.
- Reading or changing any content of the CLI profile.

## Decisions

### D1: Packages

- `internal/bwcli`: the client. It is the only package that imports `os/exec` for `bw`.
- `internal/bwcli/fakebw`: a `main` package, the fake `bw` of D12. No other package imports it.
- `internal/profile`: the profile directories. It knows nothing about `bw` and receives the logout as a function.
- `internal/tools/loginprobe`: a `main` package for the manual exercise of D14. It is not part of the product.
- `internal/vaultid`: the two ID rules that the archive and the client share, see D5.

`internal/bwcli` does not import `internal/capture` or `internal/archive`. The backup run maps the failure classes of D7 to the download error classes of the capture model.

*Alternative considered:* the client in `internal/capture`. Rejected because the capture model and its parsers must stay free of process code.

### D2: One client per profile

A client is created with a configuration: the absolute path of the executable, the data directory, the working directory, an optional certificate authority file, time limits, the output limit and the terminal of D6. It holds the session key after a login. All operations are methods of the client and take a context.

The client keeps the session key as a byte slice that is not exported, overwrites it at logout, and builds the environment of each child from it. The Go runtime can still hold copies, because the environment of a child is built from strings. This is accepted and recorded in the ADR of D15.

### D3: Environment from an allowlist

The child environment is built from nothing, never by removing entries from the inherited one.

| Source | Variables |
|---|---|
| Copied from the caller when set | `PATH`, `HOME`, `USERPROFILE`, `SystemRoot`, `windir`, `ComSpec`, `PATHEXT`, `TEMP`, `TMP`, `TMPDIR`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TERM` |
| Always set by the client | `BITWARDENCLI_APPDATA_DIR` |
| Set for every command except the login | `BW_NOINTERACTION=true` |
| Set between login and logout | `BW_SESSION` |
| Set when configured | `NODE_EXTRA_CA_CERTS` |

`PATH` is needed because the npm layout starts `bw` through `env node`. The Windows variables are needed to start a process and a command script. Everything else stays out, including every `BW_*`, `BITWARDEN*` and `NODE_*` variable of the caller. The comparison of names is case-insensitive on Windows.

*Alternative considered:* inherit the environment and remove a list of dangerous variables. Rejected because the list would have to track every variable that a later `bw` or Node version reads. `BW_CLEANEXIT` shows the cost of one missed entry: a failed command with exit code 0.

### D4: Process wiring for commands without interaction

Standard input is empty. Standard output goes into a buffer with the output limit of D8. Standard error is scanned line by line and not stored, see D7. The working directory is the empty working directory of the profile. After every command the client checks that this directory is still empty, and reports the class `stray_file` if not. It does not read or remove the file. Teardown removes it with the profile.

Each command runs under a time limit. Defaults: 60 seconds for version, status, server configuration and logout, 10 minutes for sync, exports and lists, 30 minutes for one attachment. The caller can change them. On a timeout or a cancelled context the process is stopped. On Unix the child gets its own process group and the group is killed. On Windows the process is killed, and reading the pipes ends after a short delay even when another process still holds them.

Every argument is fixed text, a validated ID or a validated server address. No secret is ever an argument.

### D5: Operations

| Operation | Arguments | Result on success |
|---|---|---|
| Version | `--version` | One line of printable ASCII, at most 64 characters, plus the SHA-256 of the executable |
| Status | `status` | `serverUrl`, `userId`, `userEmail`, `status`, `lastSync`. `status` must be `unauthenticated`, `locked` or `unlocked` |
| Server | `config server <url>` | Nothing |
| Login | `login --raw` | The session key, kept in the client, see D6 |
| Sync | `sync --force --raw` | Nothing |
| Plaintext export | `export --format json --raw` | The document as bytes |
| Encrypted export | `export --format encrypted_json --raw` | The document as bytes |
| Item list | `list items --raw`, `list items --archived --raw` | The document as bytes, one call per listing |
| Attachment | `get attachment <attachment-id> --itemid <item-id> --raw` | The content as bytes |
| Logout | `logout` | Nothing. The session key is overwritten whatever the outcome |

Documents are returned unparsed, because parsing is the task of `internal/capture`. Only the status and the version are parsed here. `lastSync` is an RFC 3339 time or missing. A value that does not parse fails the command.

The SHA-256 is computed over the file the path resolves to after following symbolic links. In the npm layout this is `bw.js` and does not cover the SDK module file beside it. The hash is information for the manifest and the developer, see D16.

IDs are validated with the same rules as the archive entry names: UUID form for an item, 1 to 64 ASCII letters and digits for an attachment. The two rules exist today as `ValidItemID` and `ValidAttachmentID` in `internal/archive`. They move to a small package `internal/vaultid` without other imports, so that the client does not depend on the archive and its encryption library. `internal/archive` calls the moved functions, and its behavior does not change. A server address must parse as a URL with scheme `https`, a host, no user information and no line break.

Before the login, the client checks that `data.json` exists in the data directory after the version query. This shows that `bw` uses the directory and that no `bw-data` directory took precedence. Only the existence is checked. The file is never read, in line with [ADR 0001](../../../docs/decisions/0001-official-cli.md).

*Alternative considered:* looking for a `bw-data` directory next to the executable. Rejected because in the npm layout the deciding path is that of the Node binary, which the client does not know.

### D6: Interactive login

The login is refused with the class `no_terminal` unless standard input and standard error of the client are terminals. The check comes first because the probe showed exit code 0 and empty output without a terminal.

The child inherits standard input and standard error. Standard output is a pipe with a limit of 4 KiB. The child stays in the process group of the caller, so that it can read from the terminal. `BW_NOINTERACTION` and `BW_SESSION` are not set. There is no time limit, because a person is typing. A cancelled context stops the process.

Before the start, the client saves the terminal state. While the child runs, the client receives interrupt signals instead of ending, because the terminal sends the interrupt to the child and to the caller alike. It waits for the child, restores the terminal state and then reports the result. Without this, an interrupt at the password prompt can leave the terminal without echo.

The result follows the spec: exit code 0 and exactly one session key, optionally followed by `\n` or `\r\n`. The probe saw no line ending, and the code writes one when standard output is a terminal on a system other than Windows, so one line ending is accepted and nothing else.

The terminal is an interface with three functions: is this a terminal, save the state, restore the state. The implementation uses `golang.org/x/term`. Tests supply a fake terminal, because `go test` has none.

*Alternative considered:* a pseudo-terminal, with the client relaying the prompts. Rejected for now. It needs a different mechanism on Windows and would put the master password and the code into the memory of this project. The probe showed that the direct connection works on Linux.

*Alternative considered:* detecting a terminal with the file mode from the standard library. Rejected because `/dev/null` is a character device and would pass.

### D7: Failure classes and standard error

A failure carries the operation, a class, the exit code and, for a download, the two IDs. Its text is built from these values only.

| Class | Meaning |
|---|---|
| `invalid_argument` | An ID or a server address was refused before any start |
| `no_terminal` | Login without a terminal |
| `start` | The process could not be started |
| `timeout`, `cancelled` | The process was stopped |
| `not_logged_in`, `locked`, `premium_required` | Non-zero exit code and a recognized sentence on standard error |
| `exit` | Non-zero exit code without a recognized sentence |
| `output` | Exit code 0 with output of an unexpected form, or output beyond the limit |
| `stray_file` | A file appeared in the working directory |
| `profile_not_used` | `bw` did not use the data directory |

The three recognized sentences are the fixed strings of the 2026.10.1 bundle: "You are not logged in.", "Vault is locked." and "Premium status is required to use this feature.". Colour codes are removed before the comparison. A sentence that changes in a later version falls back to `exit`, which is still a failure.

Standard error is read as a stream. Lines that contain `ERROR bitwarden_crypto::` are counted. Nothing else is kept. The count is returned with every result, also with a successful one. The probe showed that these lines name neither item nor field, so the count is a signal and not a classification.

Standard error is never forwarded, because the first start prints the full path of the profile, and error messages can contain item names.

### D8: Output limit

Standard output is held in memory once. The default limit is 1 GiB and the caller can change it. Output beyond the limit stops the process with the class `output`. The archive already holds each attachment in memory once ([ADR 0005](../../../docs/decisions/0005-archive-format.md)), so this adds no new bound.

### D9: Profile directories

The profile location is `bw-takeout/profiles` under the user cache directory of the system. A profile is a directory `profile-<YYYYMMDD>T<HHMMSS>Z-<8 hex digits>` with the subdirectories `data` and `work`. All three are created with mode 0700, the profile itself with a call that fails on an existing name. On a collision another random part is drawn, for a bounded number of attempts. Location, clock and random source can be replaced in tests.

Leftovers are the other directories in the location whose names have this form. Their age comes from the time in the name. They are listed and never touched, for the reason given for partial generations in ADR 0005: without a lock, a leftover of a dead run cannot be told from the profile of a running one.

Teardown takes the logout as a function, calls it if a login succeeded, then removes the profile directory in any case and returns both errors. The package does not know `bw`.

*Alternative considered:* the temporary directory of the system. It is often cleaned automatically and may be in memory, which is good for secrets. Rejected because leftovers would then appear and vanish without the user learning about them, and because `/tmp` is shared between users on Linux.

*Alternative considered:* one profile per server and account, kept logged out between runs. The server would then keep one device entry. Deferred by the maintainer. It needs a lock against parallel runs and a cleanup after an abort.

### D10: What a fresh profile costs

Each run creates a new device ID, so the server gets one device entry per run, and `bw` offers no command to remove it. The first start of a fresh profile requests configuration from Bitwarden Cloud, also when the user's server is self-hosted. Both are stated in the user limitations. Neither can be avoided with public commands.

### D11: Tests against the fake cover every scenario

Each scenario of both specs gets a test of the same name, as in the earlier changes. The tests of `internal/bwcli` run against the fake of D12. The tests of `internal/profile` use temporary directories and an injected logout function.

### D12: The fake bw

The fake is a small Go program. The tests build it once per package run into a temporary directory. This works on all three CI platforms, needs no shell and no Node.

The fake reads a scenario file from the data directory it is given in `BITWARDENCLI_APPDATA_DIR`. This also proves that the variable arrives. For every start it appends a record to a file in the same directory: arguments, the full environment, the working directory and whether standard input was empty. The tests read the records to check the allowlist, the session key rule and the arguments.

A scenario states per command: exit code, standard output, standard error, a delay, a file to write into the working directory, and whether to create `data.json`. The fake reproduces these behaviors, each from the evidence above:

- login with exit code 0 and empty output, with a valid key, with a key and a line ending, with extra text;
- login that writes a prompt to standard error and reads a line from standard input;
- exit code 0 instead of 1 when `BW_CLEANEXIT` is present, and a failure when `BW_NOINTERACTION` is missing on a command other than login;
- "You are not logged in.", "Vault is locked." and the premium sentence with exit code 1;
- success with three decryption error lines;
- a status with each state, with and without `lastSync`, and with an unknown state;
- an error text that carries a sentinel value;
- a command that does not end, output beyond the limit, a file in the working directory, a data directory left empty.

*Alternative considered:* the test binary as its own fake, selected by a variable, as the kill test of `internal/publish` does. Rejected because the client removes every variable that is not on the allowlist, and a test-only way around the allowlist would weaken the rule under test.

*Alternative considered:* a shell script. Rejected because it does not run on Windows.

### D13: Tests with the real bw behind a switch

Tests named `TestRealBW…` run only when `BW_TAKEOUT_REAL_BW` holds the path of a `bw` executable. Otherwise they report a skip with this reason, and `.github/expected-skips.txt` lists them for every platform. They need no account. They cover: version and hash, `data.json` created in the given directory, status `unauthenticated`, the class `not_logged_in` for sync, list and export, `BW_CLEANEXIT` in the calling environment without effect, and the refused login without a terminal.

They are run by hand in the dev container. Each fresh profile sends one anonymous configuration request to Bitwarden Cloud, so these tests are not offline. CI does not run them, because CI has no `bw`.

### D14: Manual login exercise

`internal/tools/loginprobe` takes the path of `bw`, a server address and an optional certificate authority file. It creates a profile, queries the version, sets the server, logs in, reads the status, syncs, reads the status again, logs out and tears down. It prints the steps with their result: the form of the session key as valid or invalid, the status values, whether `lastSync` is set and later than the start, the failure class of a failed step, and the number of entries in the data file whose name starts with `user_` after the logout. It never prints the session key. The last value is read for research only, in this tool and nowhere in the product, to learn what a logout leaves behind.

The procedure is written into the test strategy so that it can be repeated, for example on Windows later:

1. Start Vaultwarden 1.37.4 in the dev container as in the live probe, with TLS and a certificate for `localhost`, and forward the port to the host.
2. The maintainer registers a synthetic account with TOTP in the web vault.
3. Run the tool in a terminal of the dev container (Linux arm64) with all five cases: a successful login with TOTP, a wrong password, an interrupt at the password prompt, standard input from `/dev/null`, and standard output of the tool redirected to a file.
4. Run the tool on macOS with the official `bw` 2026.10.1 for macOS and the tool built for darwin/arm64, with the two cases that depend on the terminal: the successful login with TOTP and the interrupt at the password prompt.

The dev container run comes first, because it is cheap and is the first run of the login from a Go parent at all. macOS differs in two ways that matter here: `bw` is the official single-file program instead of the npm package under Node, and saving and restoring the terminal state uses other system calls than on Linux. The other three cases do not depend on the platform and are not repeated (maintainer decision of 15 November 2026).

The results go into a research report with version, method and scope, and into ADR 0002. The integration notes suggest running Vaultwarden outside the container. The probe ran it inside, on loopback. With the port forwarded, the instance is reachable from the host only.

### D15: A proposed ADR on the secret boundary

How the session key, the environment, the output and the profile are handled is an essential security assumption, so it meets the ADR criteria in CONTRIBUTING. The architecture page already expects this ADR. The new ADR `0006` starts as proposed and covers D3, D4, D6, D7 and the session key rule of D2. Context: the false successes at the process boundary and the secrets that cross it. Alternatives: an inherited environment, `--session` as an argument, a pseudo-terminal, forwarding standard error. Consequences: the session key is visible to processes of the same user that can read the environment of a child, copies of it can stay in memory, and diagnostic text of `bw` is not available to the user.

The profile lifetime of D9 is named there as a current choice, not as part of the decision, because the maintainer wants to revisit it.

### D16: A proposed ADR on bw versions

The architecture page expects an ADR on how `bw` versions are qualified, and it names "check version and hash" as a task of the adapter. The maintainer decided otherwise: no gate and no message at run time. This changes how the project relates to its integration boundary and what a user can expect after a `bw` update, so it meets the ADR criteria. The new ADR `0007` starts as proposed. Decision: any `bw` version runs, the version goes into the manifest, the tested versions are listed for developers, and differences in the export fields are reported at run time by a later change. Alternatives: refuse unknown versions, warn on unknown versions, pin a hash. Consequences: a behavior change of `bw` that keeps the document forms is not noticed at run time, a tampered `bw` is not detected by this project, and no release of this project is needed for a new `bw` version.

### D17: Dependency

`go.mod` gains `golang.org/x/term` as a direct dependency. It is the only maintained way to query and restore the terminal state on all three platforms. It may raise the version of `golang.org/x/sys`, which is already an indirect dependency. Its license is the three-clause BSD license of the other dependencies, and the release notes on third-party notices are extended.

## Risks / Trade-offs

- [The login from a Go parent may behave differently from the shell pipeline of the probe] → The manual exercise of D14 runs before the change is archived. If it fails on macOS, the pseudo-terminal alternative of D6 is reopened and ADR 0002 records the result.
- [Windows is not exercised] → ADR 0002 stays proposed with this point open. The procedure of D14 is written so that it can be repeated there.
- [On Windows a timeout may leave the Node process of an npm-installed `bw` running] → Recorded as open evidence. The official single-file `bw` has no intermediate process.
- [The fake drifts from the real `bw`] → Every fake behavior names its evidence. The switch-gated tests of D13 compare the cases that need no login. Cases with login wait for the Vaultwarden test setup.
- [The recognized sentences change with a `bw` version] → They fall back to the class `exit`. A failure never becomes a success.
- [The allowlist breaks a setup that needs another variable, for example a proxy] → Stated as a non-goal and in the limitations. Adding a configured value is a small later change.
- [The check for `data.json` depends on observed behavior] → If a later version creates the file lazily, the check fails closed with `profile_not_used` before any login.
- [The session key is in the environment of child processes] → Readable by processes of the same user. Accepted, as the CLI analysis of October did. Recorded in ADR 0006.
- [A killed run leaves a profile with tokens and the encrypted vault cache] → Reported at the next run with its path. The user limitations and the security page say how to remove it.
- [A login can change the account through migrations] → Not examined. Recorded as open evidence for the backup run.
- [Real-binary tests contact Bitwarden Cloud] → They run only behind the switch, and the test strategy says so.
- [Each attachment download costs seconds of start time] → Accepted for the prototype. `bw serve` would avoid it and is a different integration boundary.

## Open Questions

- Does the SHA-256 of the executable go into the manifest, next to the version? The manifest reader ignores unknown keys, so it can be added with the backup run.
- Where does the path of `bw` come from: the search path or a configuration value? Decided with the configuration format.
- Is the default output limit of 1 GiB right once real attachments are seen?
- Should the profile location be configurable? Not needed for the prototype.
