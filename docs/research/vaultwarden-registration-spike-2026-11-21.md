# Spike: Account Registration on Vaultwarden 1.37.4 without a Person

Date of the run: 21 November 2026 (UTC), during the explore session of the change `vaultwarden-test-setup`. The spike asked whether a synthetic account can be registered on the pinned Vaultwarden over HTTP alone, and whether the pinned `bw` can log in to it and seed it. No product code was written, and the product did not run.

**Main result.** Two HTTP calls register an account that `bw` 2026.10.1 accepts. `bw` logged in, returned a session key of the expected form, reported `unlocked` and synced. Seeding through `bw` gave the three items of the [backup exercise of 18 November 2026](../development/testing.md#backup-exercise-of-18-november-2026), with the passkey intact and the archived item in the plaintext export.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package from the dev container. SHA-256 of `build/bw.js`: `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` (same as the [live probe](vaultwarden-live-probe-2026-11-14.md)) |
| Host | Dev container, Linux aarch64 |
| Server image | `docker.io/vaultwarden/server:1.37.4-alpine` |
| Image index digest | `sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb` |
| linux/arm64 manifest digest | `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4` |
| linux/arm64 server binary SHA-256 | `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d` |
| linux/amd64 manifest digest | `sha256:7d90670a4e2531e752f014e58f7283b9d197847b6f073bf84e99598f49d80ffd` |
| linux/amd64 server binary SHA-256 | `392bc9afa7de1821e372a6d93369c87c1070644c68e26057017a2bf1b9cac3be` |
| Image label revision | `7c188967e7a467b19b2757bc3326f9cf3ccae327` on both architectures |
| Web vault layer | The same layer on both architectures. The web vault was not used |
| Database | SQLite, created by the server on first start |

The amd64 values were read from the index and the amd64 manifest, and the binary was unpacked and hashed. It was not run.

## Method

**Server.** The image index and the manifest were fetched from the Docker Hub registry with an anonymous pull token, as in the [manual login exercise](../development/testing.md#manual-login-exercise). The SHA-256 of every blob matched its digest. The arm64 binary ran in the dev container on loopback with Rocket TLS and a self-signed certificate that `bw` trusted through `NODE_EXTRA_CA_CERTS`, with sign-ups allowed, no SMTP and icon downloads off. The server answered within one second of the start.

**Registration.** Vaultwarden 1.37.4 registers in two calls ([identity.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/api/identity.rs#L1051-L1104), [accounts.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/api/core/accounts.rs#L261-L432)):

1. `POST /identity/accounts/register/send-verification-email` with the address. With mail off, the response body holds the verification token.
2. `POST /identity/accounts/register/finish` with the address, the token, the KDF parameters, the master password hash, the user key wrapped with the stretched master key, and the RSA key pair, in the flat request form.

Sign-ups are allowed and unverified by default. PBKDF2 needs at least 100 000 iterations. The spike used PBKDF2-SHA256 with 600 000 iterations and derived the keys as the Bitwarden clients do: master key from the password with the lowercase address as salt, master password hash by one more round with the password as salt, stretched key by HKDF-Expand with the infos `enc` and `mac`. The user key was 64 random bytes wrapped as an EncString of type 2, and an RSA-2048 private key in PKCS#8 was wrapped with the user key.

**Login and seeding.** `bw` ran in a profile of its own with `BW_NOINTERACTION=true`. After `bw config server`, the login used `--passwordenv` with the password in an environment variable. `bw import bitwardenjson` imported one login with one passkey. `bw create item` created two more logins, `bw create attachment` added one attachment to each of them, and `bw archive item` archived the third item.

**Secrets handling.** The address was synthetic and the password random. The password, the session key and the account keys stayed in memory or in temporary files that were removed after the run. Outputs were inspected for status codes, counts, key form and structure only. No password, token, session key or ciphertext appears in this note.

## Results

| Step | Result |
|---|---|
| `send-verification-email` | Status 200, body with the verification token |
| `register/finish` | Status 200 |
| `bw login --passwordenv … --raw` | Exit code 0, a session key of the expected form: 88 characters of base64 that decode to 64 bytes, as [login.go](../../internal/bwcli/login.go) checks |
| `bw status` | `unlocked` |
| `bw sync` | Exit code 0 |
| `bw import bitwardenjson` | Exit code 0, the passkey kept |
| `bw create item`, `bw create attachment`, `bw archive item` | Exit code 0 each |
| `bw list items` | 2 items |
| `bw list items --archived` | 1 item |
| `bw export --format json` | 3 items, 1 passkey |
| Standard error of every command above | Empty |

**Format.** All items were in the legacy format: the encrypted export had no `data` key on any item.

**Premium.** Vaultwarden 1.37.4 reports `"premium": true` for every user ([user.rs](https://github.com/dani-garcia/vaultwarden/blob/1.37.4/src/db/models/user.rs#L312)). This is code reading, not a run. A lapsed-premium account and the archive filter of the [code notes of 16 November 2026](bw-cli-code-notes-2026-11-16.md) cannot be produced on this server.

## Scope

- The findings hold for `bw` 2026.10.1 from npm on Linux arm64 against Vaultwarden 1.37.4 with SQLite, for one personal account without two-step login and with legacy items, in one run.
- They show that the account creation the [architecture roadmap](../development/architecture.md#roadmap) asked for needs no person and no web vault on this server version.

## Not tested

- The amd64 server binary at run time.
- Answering the prompts of `bw login` through a pseudo-terminal. The spike used `--passwordenv`.
- The product itself: no `bw-takeout backup` or `verify` ran.
- Tampering with a cipher, through the API or the database.
- Bitwarden Cloud, two-step login, blob-format items, organizations and repetition.

## Cleanup

The server, its data directory, the certificate, the `bw` profile and the seed files were removed after the run. Only this note and one row in the research overview remain in the repository.
