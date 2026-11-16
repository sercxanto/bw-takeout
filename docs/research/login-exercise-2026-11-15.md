# Manual Login Exercise: bw 2026.10.1 from a Go Parent

Date of the runs: 15 November 2026 (UTC), between 19:50 and 20:07. The maintainer ran the development tool `internal/tools/loginprobe` by hand against a Vaultwarden instance with one synthetic account. The tool uses the client `internal/bwcli` and the profile package `internal/profile` of the change bw-process-adapter. The procedure is in the [test strategy](../development/testing.md#manual-login-exercise).

**Main result.** The interactive login started from a Go parent process works on Linux arm64 and on macOS arm64. No pseudo-terminal is needed. In every successful run the session key had the expected form, the vault was unlocked, and `lastSync` was later than the start of the run. An interrupt at the password prompt did not reach the Go parent as a signal on either system. Instead, `bw` ended with exit code 0 and no session key. The client reported the login as failed through its check of the session key form, and the terminal echoed again afterwards. This is a second false success at the process boundary, after the one without a terminal that the [live probe](vaultwarden-live-probe-2026-11-14.md) found.

## Versions and artifacts

| Component | Dev container | macOS |
|---|---|---|
| System | Debian GNU/Linux 12, Linux 6.8.0-100-generic, aarch64 | macOS 26.7.3 (build 25G342), arm64. This Mac is the host of the dev container |
| `bw` | 2026.10.1, npm package `@bitwarden/cli` under Node 26.13.1. SHA-256 of `build/bw.js`: `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` | 2026.10.1, official single-file program from the release asset `bw-macos-arm64-2026.10.1.zip` of `cli-v2026.10.1`. SHA-256 of the archive: `4e595f9da80ef8b30abca808d0c78a1560d9589466d1bccd2b3e70f321fb12bf`, equal to the digest GitHub lists. SHA-256 of `bw`: `447a511044b8add85059c6f3f43b61e0e3dcfa2e0208e5baf68317552c156369` |
| `loginprobe` | Built with Go 1.27.3 for linux/arm64 | Cross-built in the dev container with Go 1.27.3 for darwin/arm64 |
| Code | `internal/bwcli`, `internal/profile` and `internal/tools/loginprobe` as of commit `038f938` | same |

Server, the same for both systems:

| Component | Value |
|---|---|
| Vaultwarden | 1.37.4 from `docker.io/vaultwarden/server:1.37.4-alpine`, image index `sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb`, linux/arm64 manifest `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4`. SHA-256 of the binary: `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d`, the same as in the live probe |
| Web vault | 2026.7.0 according to `vw-version.json` |
| Network | Bound to 127.0.0.1:8087 inside the dev container, TLS with a self-signed certificate for `localhost` valid for seven days, passed to `bw` through `NODE_EXTRA_CA_CERTS`. VS Code forwarded the port to `localhost:8087` on the Mac |
| Account | One synthetic account with an address under `example.invalid`, two-step login with an authenticator app, registered by the maintainer in the web vault. No items |

## Method

The tool creates a fresh profile, queries the version, sets the server, logs in, reads the status, syncs, reads the status again, logs out and tears down the profile. For each step it prints ok or the failure class. It never prints the session key or any text that `bw` wrote. After the logout it reads the top-level key names of `data.json` and counts those that start with `user_`. Only this count is printed. This reading is for research only and happens nowhere in the product ([ADR 0001](../decisions/0001-official-cli.md)).

The maintainer ran every case in a terminal: on Linux in a terminal attached to the dev container, on macOS in a terminal on the Mac. After each run the shell printed the exit status of the tool and whether the terminal echoed typed characters (`stty -a`). The maintainer passed the output into the development session. The one-time codes visible there are not recorded here.

## Results

| # | Case | System | Tool result | Exit status | Echo afterwards |
|---|---|---|---|---:|---|
| 1 | Successful login with TOTP | Linux | every step ok, session key form valid, status `unlocked`, `lastSync` later than the start after login and after sync | 0 | on |
| 2 | Wrong password | Linux | login failed with class `exit` | 1 | on |
| 3 | Interrupt (Ctrl-C) at the password prompt | Linux | login failed with class `output`, session key form invalid | 1 | on |
| 4 | Standard input from `/dev/null` | Linux | login failed with class `no_terminal`, no prompt appeared | 1 | on |
| 5 | Standard output of the tool redirected to a file | Linux | same as case 1. The prompts appeared on the terminal. The file held the step lines and no line with 80 or more characters of the base64 alphabet | 0 | on |
| 1 | Successful login with TOTP | macOS | same as case 1 on Linux | 0 | on |
| 2 | Wrong password, run in addition | macOS | same as case 2 on Linux | 1 | on |
| 3 | Interrupt (Ctrl-C) at the password prompt | macOS | same as case 3 on Linux | 1 | on |

Each case ran once. In every run the teardown removed the profile, and the profile location was empty after the last run on both systems.

### Login from a Go parent

`bw` prompted for the address, the master password and, in the successful runs, the two-step code, on the terminal that the client passed through. The session key reached the client through the pipe on standard output and passed the form check. Status and sync then worked with this key. This confirms the direct connection of design D6 for both `bw` layouts: the npm package under Node on Linux and the single-file program on macOS.

### Wrong password

`bw` printed an error message on the terminal and ended with a non-zero exit code without asking again. The client reported the class `exit`.

### Interrupt at the password prompt

On both systems the class was `output`, not `cancelled`. In the client, `cancelled` follows when the parent receives an interrupt signal while the child runs. `output` with this result follows only when the child ended with exit code 0 and standard output did not hold a valid session key. So the Go parent received no signal, and `bw` ended with exit code 0 without a session key. The likely reason is that the prompt of `bw` puts the terminal into raw mode, so that Ctrl-C reaches `bw` as a character and not as a signal. This was not checked.

After the interrupt the terminal echoed again on both systems. The prompt line had no line ending, so the next line of the tool appeared behind it. The spec scenario "Interrupt at the password prompt" asks for a restored terminal and a failed login after the process has ended. Both held. Design D6 was extended with this second path.

### Not on a terminal

With standard input from `/dev/null` the client refused the login before starting `bw` for it. With standard output of the tool redirected to a file, the login still worked, because the client checks standard input and standard error only, and no session key reached the file. Both cases ran on Linux only.

### What the logout leaves in the profile

After the logout, `data.json` still held 12 top-level entries whose name starts with `user_`, on both systems. The tool reads key names only, so it is not known whether these entries still hold values or were cleared. Finding 10 of the [code notes](bw-cli-code-notes-2026-11-15.md) says that `bw logout` clears local state. The run shows that it does not remove these entries. The client removes the whole profile at teardown in any case.

### Device entries on the server

After all runs the web vault listed four devices for the account: one web browser, one CLI on macOS and two CLI on Linux. This matches the three successful logins, each from a fresh profile. The runs with a wrong password or an interrupt did not leave a visible entry. The logout did not remove any entry, as finding 10 of the code notes expects. Each successful run therefore leaves one device entry on the server, as the [limitations](../user/limitations.md) state.

## Scope

- Vaultwarden 1.37.4 with SQLite on Linux arm64, reached over loopback and a forwarded port, with a self-signed certificate.
- One synthetic account with TOTP from an authenticator app and an empty vault.
- `bw` 2026.10.1 in the npm layout on Linux arm64 and as the official single-file program on macOS arm64.
- The macOS system is the host of the dev container, not an independent machine.

## Not tested

- Bitwarden Cloud, with any account.
- Windows, with either `bw` layout, and Linux amd64 or macOS on Intel.
- Cases 2, 4 and 5 on macOS beyond the additional run with a wrong password, by maintainer decision.
- An interrupt at the address or two-step code prompt, a cancelled context, and a login that runs into the 4 KiB output limit.
- Other two-step methods (e-mail, WebAuthn, Duo), an account without two-step login, new-device verification by e-mail, single sign-on and API keys.
- A vault with items. The sync transferred an empty vault, so the run says nothing about a failed or partial login sync.
- Whether Ctrl-C reached `bw` as a character. Only the outcome was observed.
- The values of the `user_` entries after the logout, and account migrations that a login may run.
- Repeated logins in quick succession, throttling, and messages to the account about new devices.
- Repetition. Every case ran once.

## Cleanup

After the runs the maintainer stopped the server, removed the exercise folder on the Mac with `bw`, the tool and the certificate, and deleted the entry of the synthetic account from the authenticator app. In the dev container `.local/vaultwarden` was removed with the server binary, web vault, certificate, key, database and the output file of case 5, together with both builds of the tool. The profile locations on both systems were empty. A first attempt to unpack the whole image into the bind mount of the dev container left entries that the container could not remove. When the maintainer checked on the Mac they were already gone, and step 1 of the procedure now unpacks only the server binary and the web vault. Only this report, the procedure in the test strategy and one row in the research overview remain in the repository.
