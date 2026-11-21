# Design

## Context

See [proposal.md](proposal.md) for motivation. The explore session of 21 November 2026 settled the scope and ran a spike. The state this design builds on:

- **Server.** The manual exercises fetch Vaultwarden 1.37.4 by the image index digest `sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb` with curl and jq, and start it on loopback with TLS ([testing](../../../docs/development/testing.md#manual-login-exercise), steps 1 to 3). Only linux/arm64 values are recorded there. The spike read the index: linux/amd64 has the manifest `sha256:7d90670a4e2531e752f014e58f7283b9d197847b6f073bf84e99598f49d80ffd`, its server binary has the SHA-256 `392bc9afa7de1821e372a6d93369c87c1070644c68e26057017a2bf1b9cac3be`, the same revision label `7c188967e7a467b19b2757bc3326f9cf3ccae327` and the same web vault layer as arm64. The amd64 binary was hashed, not run. The server was ready in under one second.
- **Registration.** Vaultwarden 1.37.4 registers in two calls ([identity.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/api/identity.rs#L1051-L1104), [accounts.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/api/core/accounts.rs#L261-L432)). With mail off, `POST /identity/accounts/register/send-verification-email` returns the verification token in its body. `POST /identity/accounts/register/finish` takes the email, the token, the KDF parameters, the master password hash, the user key wrapped with the stretched master key, and the RSA key pair. Sign-ups are allowed and unverified by default. PBKDF2 needs at least 100 000 iterations. The spike registered an account this way with 600 000 iterations, both calls returned 200, and `bw` 2026.10.1 logged in, returned a session key of the expected form, reported `unlocked` and synced.
- **Seeding.** In the spike `bw import bitwardenjson` kept the passkey, and `bw create item`, `bw create attachment` and `bw archive item` exited with 0. The default list then showed two items, the archived list one, the plaintext export three with one passkey, and no command wrote to standard error. All items were legacy: the encrypted export had no `data` key.
- **Premium.** Vaultwarden 1.37.4 reports `"premium": true` for every user ([user.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/db/models/user.rs#L312)).
- **Login.** `bwcli.Login` refuses without a terminal on standard input and standard error ([login.go](../../../internal/bwcli/login.go)). `bw` prompts for the address and the master password, and for a code when two-step login is on ([login exercise](../../../docs/research/login-exercise-2026-11-15.md)). The exercises needed no pseudo-terminal because a person's terminal was there. A test has none.
- **Tests.** `cmd/bw-takeout` builds the binary in `TestMain` and runs it. The real-`bw` tests of `internal/bwcli` are behind the environment variable `BW_TAKEOUT_REAL_BW` and report a skip otherwise, which [expected-skips.txt](../../../.github/expected-skips.txt) lists. `TestWorkflowStructure` in `internal/tools/skipreport` reads every workflow as text and requires, among other things, `cache: false`, `persist-credentials: false` and read-only permissions.
- **Dev container.** The Dockerfile pins its base images by multi-architecture digests, installs `bw` from npm (JavaScript, the same `bw.js` hash on every architecture) and age from the Go module proxy. It runs on arm64 and amd64 as it is.

## Goals / Non-Goals

**Goals:**

- One entry point, `go test -tags vaultwarden ./internal/e2e/`, that needs nothing but the dev container and network access, on Linux arm64 and amd64.
- The product is exercised only through its real interactive path: the built binary, a terminal on standard input and standard error, and `bw login --raw` prompting.
- Every fetched artifact is checked against a digest recorded in the repository.
- No secret and no vault content in the repository, in arguments of the product, in test output or in CI logs.

**Non-Goals:**

- macOS and Windows. Vaultwarden publishes Linux images only.
- Running on every pull request. The workflow is dispatched by hand.
- A general client library for the Vaultwarden API. The helper covers registration, a token and one cipher update, nothing else.
- Blob-format items, two-step login, organizations and the cases listed as out of scope in the proposal.

## Decisions

### D1: One test package behind a build tag

The suite lives in `internal/e2e`. Every file is a `_test.go` file and starts with `//go:build vaultwarden && linux`. Without the tag the package has no files for the compiler, so `go test ./...`, `go vet ./...` and the skip report do not see it, and nothing in it can be imported by product code. `TestMain` builds `cmd/bw-takeout` into a temporary directory, as `cmd/bw-takeout` does.

*Alternative considered:* an environment variable and `t.Skip`, as `BW_TAKEOUT_REAL_BW` does. Rejected because every test would then appear as an expected skip on all three CI platforms, and because the code would compile into every test binary of the package on Windows and macOS, where it cannot run.

The tag needs one guard: code that no regular run compiles can rot. `checks.yml` therefore runs `go vet -tags vaultwarden ./...` (D10).

Review of 21 November 2026, R3: the prompt child of the pseudo-terminal tests was written as a source file into a temporary directory inside the package during the run, so an interrupted run could leave an untagged `package main` that `go vet ./...` would compile. It now lives in `internal/e2e/testdata/promptchild/` with the same build constraint and is built from there with the tag. `TestPackageFilesAreTaggedTests` checks that every Go file outside `testdata` is a tagged test file.

### D2: The server is fetched and checked in Go

A helper in the test package fetches the pinned image index from the Docker Hub registry with an anonymous pull token. It checks the SHA-256 of every blob against its digest, selects the manifest for `linux/<GOARCH>`, and compares it with a table in the code:

| Architecture | Manifest | Server binary SHA-256 |
|---|---|---|
| arm64 | `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4` | `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d` |
| amd64 | `sha256:7d90670a4e2531e752f014e58f7283b9d197847b6f073bf84e99598f49d80ffd` | `392bc9afa7de1821e372a6d93369c87c1070644c68e26057017a2bf1b9cac3be` |

The index digest alone fixes both manifests and every blob. The table repeats what the index implies, so that a reader sees which binary runs and a mismatch is named in plain terms. Any other architecture fails the test with a message. Only the file `vaultwarden` is unpacked. The web vault is not needed and stays off.

Downloads are cached under `.local/vaultwarden-cache/` in the repository root, which `.gitignore` already covers, by blob digest. Every use checks the cached file against its digest again, so a damaged cache fails or is fetched anew, never trusted.

*Alternatives considered:* a shell script with curl and jq, as in the manual exercises. Rejected because it would be a second entry point beside `go test`, its failure paths could not be tested, and CI would have to call two things in order. Docker on the dev container: not available there, and the exercises already avoid it.

### D3: One server per scenario

Each test starts its own server in a temporary data directory, with a certificate the test creates in Go (ECDSA P-256, `localhost` and `127.0.0.1`, valid for one day). The port is taken from a listener on `127.0.0.1:0` that is closed right before the start, because `DOMAIN` must name the port. The environment is that of the manual exercise: sign-ups allowed, no SMTP, icon downloads off, and in addition the web vault off. The test waits for `GET /alive` with a limit of 30 seconds. A cleanup stops the process (SIGTERM, then SIGKILL after 5 seconds) and the temporary directory goes with the test.

A fresh server per scenario keeps the tampering of D7 from leaking into the intact scenario and keeps the device entries of each test apart. The spike saw the server ready in under a second, so the cost is small.

*Alternative considered:* one server for the package with one account per test. Rejected because a failure in one scenario could leave server state that the next one reads, and the saving is about a second.

### D4: The account is registered by a test-only helper

The helper creates a random address under `example.invalid` and a random password per test, kept in memory only. It derives the keys as the Bitwarden clients do: the master key by PBKDF2-SHA256 of the password with the lowercase address as salt and 600 000 iterations, the master password hash by one more PBKDF2 round of the master key with the password as salt, and the stretched key by HKDF-Expand with the infos `enc` and `mac`. It generates a user key of 64 random bytes, wraps it as an EncString of type 2 (AES-256-CBC with PKCS#7 padding, HMAC-SHA256 over IV and ciphertext) with the stretched key, generates an RSA-2048 key pair, and wraps the PKCS#8 private key with the user key. It then makes the two calls of the context section in the flat request form that Vaultwarden 1.37.4 accepts. It uses the standard library only.

This is a second implementation of part of the Bitwarden key derivation, which [ADR 0001](../../../docs/decisions/0001-official-cli.md) avoids for the product. It stays outside that decision for three reasons. The helper is test code that the program never links (D1). It produces server-side input and no result the product relies on. And an error in it cannot produce a false success: `bw` would fail to log in or to unlock, which fails the test loudly. The ADR stays unchanged.

The helper has unit tests of its own: the EncString form, that the wrapped keys decrypt again with the derived keys, and the request body fields. It also has a known-answer test of the PBKDF2 and HKDF steps for a fixed address and password. The expected values for that test come from the [Bitwarden help article on encryption](https://bitwarden.com/help/bitwarden-security-white-paper/) or another published source, and the test names that source. If no published vector is found, the test is limited to the round trip and the design records that.

Apply of 21 November 2026: the known-answer vectors are the unit tests of the Bitwarden SDK, `bitwarden/sdk-internal` at commit `c5c08aebe8830f3c0fa54348b63f895137f2b827` (`test_master_key_derive_pbkdf2`, `test_stretch_kdf_key` and `test_password_hash_pbkdf2` in `crates/bitwarden-crypto`). The test names them. The registration body uses the field `emailVerificationToken`, because `token` is an alias of the organization invite token in Vaultwarden 1.37.4.

*Alternatives considered:* driving the web vault with a headless browser, which needs Node packages and a browser and depends on the markup of the web vault. A committed database with an account registered once by hand, which is an opaque file in a public repository with keys the key-material check would have to exempt, and which is bound to one schema version.

### D5: The vault is seeded through the official bw

Seeding uses the `bw` of the dev container in a seed profile of its own, a temporary `BITWARDENCLI_APPDATA_DIR` with `NODE_EXTRA_CA_CERTS` set to the test certificate and `BW_NOINTERACTION=true`. It sets the server, logs in with `bw login <address> --passwordenv <name> --raw`, imports a Bitwarden JSON file with one login and one passkey, creates two more logins, adds one attachment to each, archives the third item, logs out and removes the profile. The passkey key is a fresh P-256 key generated in Go. The attachments are 64 KiB of random bytes each, so a run stays short. The import file is written with mode 0600 into a temporary directory and removed right after the import.

`--passwordenv` is used only here. It is a seeding step, not the product under test. The product never takes a password and keeps its prompts.

The seed keeps a reference inventory in memory: the item IDs, the attachment IDs, the SHA-256 of each attachment, the credential ID of the passkey and which item is archived. D8 compares the generation with it. This is the first test against a live capture with an inventory known from outside the CLI.

### D6: The backup runs under a pseudo-terminal

The test opens a pseudo-terminal through `/dev/ptmx` with `golang.org/x/sys/unix` (unlock and read the number of the secondary side), and starts the built `bw-takeout backup` with the secondary side as standard input and standard error and as controlling terminal (`Setsid`, `Setctty`). Standard output goes to a pipe and holds the report. The flags are `--server`, `--ca`, `--bw` with the resolved path of the container's `bw`, `--target` and `--recipients-file`. The age identity is generated in the test with the age library, and the recipients file is written beside it. `XDG_CACHE_HOME` points to a temporary directory, so the profile location of the run is isolated and can be checked afterwards.

A reader on the primary side collects the output of `bw` in memory. When it sees `Email address` it writes the address and a carriage return, and when it sees `Master password` it writes the password and a carriage return. A prompt it does not know, or no exit within 3 minutes, fails the test. On failure the test prints only the names of the prompts it saw and the report, never the collected terminal text, because `bw` echoes the address there.

*Alternatives considered:* the package `creack/pty`. It is more convenient, but it would be a new dependency for about 40 lines that `golang.org/x/sys` already makes possible, and the suite runs on Linux only. Running `internal/run` in process with a fake terminal: that is not the binary users run, and the login refuses without a real terminal on its file descriptors anyway.

### D7: The passkey is tampered with through the Vaultwarden API

For the tampered scenario the helper gets an access token with `POST /identity/connect/token` (password grant with the master password hash, scope `api`, and device fields of its own), reads the cipher of the passkey item with `GET /api/ciphers/<id>`, replaces `login.fido2Credentials[0].keyValue` with an EncString of type 2 under a random 64-byte key, and writes the cipher back with `PUT /api/ciphers/<id>`. The ciphertext is well formed and fails its MAC under the item key, which is the case of the [live probe](../../../docs/research/vaultwarden-live-probe-2026-11-14.md) without a second account. The backup then runs as in D6, and its forced sync fetches the changed cipher.

Before the tampered backup runs, the test checks that the change is on the server: a second `GET` returns the new `keyValue`.

*Alternative considered:* editing the SQLite database of the stopped server, as the live probe did. It needs a SQLite driver in Go, such as `modernc.org/sqlite`, a large new dependency, or Python in the test run. It also binds the test to the database schema and needs a restart. It stays the documented fallback if Vaultwarden refuses the `PUT`. Adding the dependency then needs a maintainer decision.

### D8: What the scenarios assert

**Intact vault** (`TestIntactVaultIsVerified`):

- exit status 0, the report line `Verdict:     verified` and the generation line naming a path in the target,
- exactly one generation in the target and no partial generation,
- `bw-takeout verify --identity <file> <generation>` exits with 0,
- the generation, opened with `archive.Read` of `internal/archive` and the test's identity, holds one attachment entry per seeded attachment under `attachments/<item ID>/<attachment ID>` with the seeded SHA-256, the archived item's attachment among them, and a plaintext export with one passkey whose credential ID is the seeded one,
- the profile location under the temporary `XDG_CACHE_HOME` is empty.

**Tampered passkey** (`TestTamperedPasskeyFails`):

- exit status 1 and the report line `Generation:  none published, the verdict is failed`,
- a failed finding with the reason `passkey_count_mismatch` for the passkey item, naming the counts 1 in the encrypted export and 0 in the plaintext export,
- the target is empty and the profile location is empty.

The live probe predicts that `bw` keeps the item and drops its passkey. If the run shows another failed finding instead, for example the whole item missing from the plaintext export, the test is not loosened in place. The implementer records the observation in the research note and asks the maintainer, because it changes what the setup claims to reproduce.

Apply of 21 November 2026: the tampered run showed the predicted finding `failed passkey_count_mismatch` with `expected=1 found=0` as the only failed finding, and in addition `unverified document_decryption_errors` with `count=3` for the plaintext export, the item list and the archived item list. These follow from the decryption error lines of `bw` and do not contradict D8. The test requires the passkey finding to be the only failed one.

Review of 21 November 2026, R2: the empty profile location of both scenarios must also exist. The run creates the location and leaves it in place, so a missing location would mean that the run did not use the isolated `XDG_CACHE_HOME`, and an empty check would pass for the wrong reason. `TestProfileLocationMustExistAndBeEmpty` covers the helper.

### D9: Output and secret hygiene

- The password, the session key, the access token and the age identity exist only in memory and in temporary files with mode 0600 that the test removes.
- The password reaches the seed `bw` through an environment variable, never as an argument. The product gets it only through the pseudo-terminal.
- No output of `bw` is printed. The test logs IDs, counts, exit statuses, verdicts and the report, which holds no vault content ([backup-run spec](../../../openspec/specs/backup-run/spec.md), "Report holds no vault content and no bw text").
- The key-material check of `checks.yml` must keep passing: keys are generated at run time, and the source holds no PEM headers or encoded key prefixes.
- Apply of 21 November 2026: the report names the account, and the address in it is what was typed at the prompt, so the test logs the report with the address replaced by `<address>`. `TestMain` copies the test log on its way to standard output and searches it after the run for every recorded password, address and session key and for any text of the session key form. A hit fails the run and names only the kind of value. This is the final check of task 5.3.
- Review of 21 November 2026, R1: the output of a failed `bw-takeout verify` names the account as well, so the intact scenario logs it with the address replaced, like the backup report. `TestRedactAddressInVerifyOutput` covers the helper; the path itself runs only when `verify` fails against a live generation.
- Review of 21 November 2026, R4: `architecture.md` and `getting-started.md` name the new package and the new step of `Checks`. The second review round of the same day found two documentation gaps on the testing page, fixed as R5 (the rerun after the fixes) and R6 (the description of `TestPackageFilesAreTaggedTests`), and the second security review had no finding. The first security review had no finding. Its two observations were taken as stated: the log check finds a leak only after the log was written, which with synthetic values on a discarded server exposes nothing real, and `verify` inherits the test environment, which holds no secret.

### D10: Workflows

`.github/workflows/vaultwarden.yml` has the single trigger `workflow_dispatch`, one job on `ubuntu-24.04` with `timeout-minutes: 30`, `permissions: contents: read` and a concurrency group of its own. Its steps:

1. Check out with `persist-credentials: false`, pinned by commit SHA as in `ci.yml`.
2. `docker build -t bw-takeout-dev .devcontainer`.
3. `docker run --rm` with the checkout mounted, the runner's user and group IDs, and `HOME`, `GOCACHE` and `GOMODCACHE` pointing to writable directories under `/tmp`, running `go test -count=1 -tags vaultwarden -v ./internal/e2e/`.

The header comment states that the job contacts Docker Hub and `api.bitwarden.com` and that it uploads nothing. Running inside the dev container image means CI tests the same `bw`, Go and age as the dev container, on amd64 instead of arm64.

`TestWorkflowStructure` gets a case for the new file: only `workflow_dispatch` as trigger, no `push` and no `pull_request`, the common checks, and no `setup-go`. The common check for `cache: false` applies only where `setup-go` is used.

`checks.yml` gets a step `go vet -tags vaultwarden ./...` after the formatting check.

*Alternatives considered:* setting up Go and Node on the runner and installing `bw` with npm or as the single-file `bw-linux` asset. Rejected because that is a second environment with its own pins, and the dev container already pins everything. The action `devcontainers/ci`: one more third-party action to pin and review for a `docker build` and a `docker run`. A pull request label as a second trigger: possible later, left out to keep one trigger to reason about.

### D11: Documentation and decisions

- The spike becomes `docs/research/vaultwarden-registration-spike-2026-11-21.md`, written like the other research notes, with version, method, results, scope and what was not tested, and a row in the research overview.
- The testing page gets a section "Vaultwarden Tests" with how to run the suite, the scenarios, the evidence level "automated end-to-end, Vaultwarden only", and what it does not cover. The manual exercises stay for macOS, TOTP and interrupts. The sentence that cases with a login wait for an automated setup is updated.
- The integration notes get the pin table of D2, the workflow and its pins, and a row in the tested `bw` versions.
- The release notes require a passing dispatched run on the release commit.
- The architecture roadmap marks the item as done and adds the follow-up change for TOTP and the further cases.
- No new ADR. The suite is test tooling that can be removed without affecting the product, it moves no integration boundary of the program, and it fixes no security assumption of the product. D4 records why ADR 0001 is not affected.

## Risks / Trade-offs

- [Vaultwarden refuses the `PUT` of D7] → The first task checks the `PUT` against a running server before the scenario is built on it. The fallback of D7 needs a maintainer decision.
- [`bw` reacts to the foreign `keyValue` differently than in the live probe] → D8 stops and asks instead of loosening the test.
- [The prompts of `bw` do not appear as expected on the pseudo-terminal] → The reader names the prompts it saw. The prompt names are taken from the login exercise. Their exact text is checked in the first run.
- [Docker Hub rate limits anonymous pulls, and runners share addresses] → The cache of D2 makes repeated runs in the dev container free. In CI one run fetches about the size of one image. If limits hit, mirroring by the same digest from another registry is an open question below.
- [Running the dev container image in CI with a different user ID] → D10 sets the writable directories explicitly. The first dispatched run is the check, and its result goes into the testing page.
- [The build tag hides compile errors] → `go vet -tags vaultwarden ./...` in `checks.yml` on every change.
- [Each run adds device entries and contacts Bitwarden Cloud] → The server is discarded with its data. The contact is stated in the workflow and the testing page.
- [A Vaultwarden or `bw` update changes the registration API or the prompts] → Both are pinned. An update is a reviewed change that runs the suite.
- [The helper's key derivation diverges from the clients] → The login of `bw` in every scenario is the end-to-end check. The known-answer test of D4 catches drift in the helper itself.

## Open Questions

- Whether the same index digest is available from another registry, for example `ghcr.io/dani-garcia/vaultwarden`, as a fallback if Docker Hub limits get in the way. It changes only where the fetch goes, not the checks.
