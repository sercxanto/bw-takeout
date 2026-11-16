# Code Notes: bw 2026.10.1 Bundle and Unauthenticated Runs

Dates: code reading on 13 and 15 November 2026, unauthenticated runs on 15 November 2026 (UTC). These notes collect the findings that the change `bw-process-adapter` builds on and that were not yet in the repository. No account, no server of the project and no secret was involved.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package `@bitwarden/cli` in the dev container, started through `/usr/local/bin/bw` |
| Bundle | `build/bw.js`, 97082 lines, SHA-256 `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b`, the same file as in the bootstrap check and the [live probe](vaultwarden-live-probe-2026-11-14.md) |
| Host | Dev container, Debian 12, Linux aarch64 |

## Method

**Code level.** The installed bundle was read with a text editor and `grep`. Line numbers below refer to this file. Each finding was looked up again on 15 November 2026 at the line given. Code reading shows what the code would do. It does not show what happens at run time, which server flags apply, or what other versions do.

**Unauthenticated runs.** `bw` was started in the dev container with an empty directory in `BITWARDENCLI_APPDATA_DIR` and no login. These runs contacted Bitwarden Cloud, see finding 12.

## Findings

Evidence levels: **code** for a reading of the bundle, **run** for an unauthenticated run in the dev container, **probe** for the authenticated live probe of 14 November 2026.

| # | Finding | Evidence | Where |
|---|---|---|---|
| 1 | The bundle reads these environment variables: `BW_SESSION`, `BW_NOINTERACTION`, `BW_CLEANEXIT`, `BW_RESPONSE`, `BW_QUIET`, `BW_RAW`, `BW_PRETTY`, `BW_SERVE`, `BW_CLIENTID`, `BW_CLIENTSECRET`, `BITWARDENCLI_APPDATA_DIR`, `BITWARDENCLI_DEBUG`, `ADDITIONAL_REGIONS`, `BITWARDEN_DESKTOP_PROXY_PATH`, `http_proxy`, `https_proxy`, `HOME`, `APPDATA` and `XDG_CONFIG_HOME`. Only the lowercase proxy variables are read. The Node runtime reads further variables such as `NODE_OPTIONS`, `NODE_TLS_REJECT_UNAUTHORIZED` and `NODE_EXTRA_CA_CERTS`, according to the Node documentation and not to this bundle | code | `process.env.*` throughout; 93803 for the desktop proxy path |
| 2 | Any non-empty `BW_CLEANEXIT` turns a failure into exit code 0, also the value `false` | code, run | 32091 |
| 3 | A directory `bw-data` next to the executable of the process takes precedence over `BITWARDENCLI_APPDATA_DIR`. In the npm layout the executable of the process is the Node binary | code | 96777 |
| 4 | Every prompt is written to standard error. A vault command on a locked vault starts the unlock prompt, and tries the desktop application first, unless `BW_NOINTERACTION` is exactly `true`. Then it ends with "Vault is locked." | code; prompt on standard error also probe | 32219 to 32222 |
| 5 | The recognized failure sentences are fixed strings: "You are not logged in." (32182, 45702), "Vault is locked." (32220, 45798) and "Premium status is required to use this feature." (57933 and other places) | code; "You are not logged in." also run | as given |
| 6 | `lastSync` is written in two places only: after a successful full sync and after a successful "no sync needed" check. The value is taken before the request is sent | code | 72953, 72989 |
| 7 | The login runs a full sync without error propagation, so a failed sync still leads to a session key. `bw sync --force` propagates the error | code | 39134, 45555 |
| 8 | An export without `--output` goes to standard output only when raw output is requested (`--raw` sets `BW_RAW`). Otherwise a file with a default name is written into the working directory | code | 31690 |
| 9 | `get attachment` requires premium for an attachment of a personal item | code | 57898 and following, sentence at 57933 |
| 10 | `bw logout` clears local state and calls no server endpoint for the device. The bundle contains a device deactivation call (`POST /devices/<id>/deactivate`) that no command uses. The device ID is global to the profile, independent of account and server | code | 65103 to 65105 for the unused call |
| 11 | `bw login` refuses while an account is logged in ("You are already logged in as …", 32171). `bw config server` refuses while an account is logged in | code | 32171, 39769 |
| 12 | `bw --version` in an empty profile printed `2026.10.1` and a line ending, wrote `Could not find data file, "<full path>"; creating it instead.` to standard error (94632) and created `data.json` with mode 0600. The file then held a new device ID and server configuration fetched from `https://api.bitwarden.com`. Running `bw config server <url>` as the first command did not avoid this request | run; message also code | 94632 |
| 13 | `bw status` in a profile without login printed `{"serverUrl":null,"lastSync":null,"status":"unauthenticated"}` with exit code 0 | run | |
| 14 | `bw sync` and `bw list items` without login exited with code 1 and "You are not logged in.". With `BW_CLEANEXIT=false` the failing sync exited with code 0 | run | |
| 15 | `--version` took 0.8 s and `status` 2.6 s | run | |
| 16 | The login can run account migrations after a successful authentication. What these migrations change on the account was not read | code | 39147 |
| 17 | `bw` has no error codes. Every failed command exits with code 1, or 0 under `BW_CLEANEXIT` (32091). The error response that `--response` prints has only the fields `success`, `message` and, for an ambiguous lookup, `data` with the matching IDs (31452). No field distinguishes one failure from another, so a failure can be told apart only by its message text or by a later query such as `bw status` | code | 32091, 31452 |

Findings that the change also relies on and that the [live probe](vaultwarden-live-probe-2026-11-14.md) already records: the session key form of `bw login --raw`, exit code 0 without a terminal, decryption error lines on standard error with exit code 0, the attachment download by ID without a file, and the refusal of servers without HTTPS.

## Scope

- Code-level findings hold for this one bundle. A later `bw` version can change any of them.
- The runs were unauthenticated, in the npm layout on Linux arm64. The official single-file `bw` for macOS or Windows was not run.

## Not tested

- The login started from a Go parent process, on any platform.
- Bitwarden Cloud with an account. The only Cloud contact was the anonymous configuration request of finding 12.
- The premium failure of finding 9 at run time.
- An export after a login whose sync failed (finding 7).
- What `bw logout` leaves in a profile (finding 10).
- The effect of the account migrations of finding 16.
- The `bw-data` precedence of finding 3 at run time.
- Whether Bitwarden Cloud limits device entries, throttles repeated logins or sends a message per new device.
