# Live Probe: bw 2026.10.1 against Vaultwarden 1.37.4

Date of the run: 13 and 14 November 2026 (UTC). This is one authenticated end-to-end run of the unmodified, pinned CLI process against a local Vaultwarden instance with synthetic accounts. No product code was written.

**Main result.** The false success from the SDK-level experiment of 31 October 2026 reproduces through the real CLI process. After one encrypted `keyValue` was replaced on the server, `bw export --format json`, `bw list items` and `bw get item` all exited with code 0 and returned the login with **zero** of its two passkeys. The account-bound `encrypted_json` export still showed two credential entries. The only other signal was three error lines on stderr.

## Versions and artifacts

| Component | Value |
|---|---|
| Bitwarden CLI | 2026.10.1, npm package from the dev container. SHA-256 of `build/bw.js`: `8a443d1190643330826e8cc0b27381fba5af5a06c6c3d7c7254033913a51ee9b` (same as the bootstrap check) |
| Node.js | 26.13.1 |
| Host | Dev container, Debian 12, Linux aarch64 |
| Server image | `docker.io/vaultwarden/server:1.37.4-alpine` |
| Image index digest | `sha256:19ea2b669945d842dadc3d500f2d6a795866a003330f59a47b9634658bae98bb` |
| linux/arm64 manifest digest | `sha256:c82bc74bb0a88821fd3fbce48e7867e8aca4e492e512970a309d02a629ab91d4` |
| Image config digest | `sha256:0cf03834d7f2824970b1ec952d540c03351f9e94718feb4f2c599566bc83ded0` |
| Layer with the server binary | `sha256:dbfa9406220c15463c964b4635a81ad518aded8cf750fc79f369e3c84479ab72` |
| Layer with the web vault | `sha256:d7b6e37be08b53480724fb772207fdfc1d75a2d150580c41348a82e7fa52403b` |
| Server binary | `vaultwarden --version` prints 1.37.4. SHA-256: `ae658947f85964e934d7673d9cab4c0edbce8e6167422d6ff3b10008e125421d`. Image label revision: `7c188967e7a467b19b2757bc3326f9cf3ccae327` |
| Web vault | 2026.7.0 according to `vw-version.json` |
| Database | SQLite, created by the server on first start |

## Method

**Server.** The image index, the linux/arm64 manifest, the config and all five layers were fetched from the Docker Hub registry API with curl and an anonymous pull token. The SHA-256 of every blob matched its digest. Only `/vaultwarden` and `/web-vault` were extracted, into the ignored `.local/` directory. The statically linked binary ran directly in the dev container, bound to 127.0.0.1:8087, with open sign-ups, no SMTP and icon downloads disabled. Docker was not used.

**TLS.** The first start used plain HTTP. bw 2026.10.1 refused it: every API call failed with `InsecureUrlNotAllowedError: Insecure URL not allowed. All URLs must use HTTPS.` The check sits in the API service and is skipped only in development mode, which was not used. The server was restarted with Rocket TLS and a self-signed certificate for `localhost` valid for seven days. bw trusted it through `NODE_EXTRA_CA_CERTS`. No account existed before the restart.

**Accounts.** The user registered two synthetic accounts in the web vault and enabled TOTP on the first one (account A). Account B has no second factor.

**CLI profiles.** Each login used its own empty `BITWARDENCLI_APPDATA_DIR` under `.local/`: one for account A, one for account B, and a third, fresh one for account A after the tampering.

**Secrets handling.** The user ran every interactive login in a terminal. The session key went through a pipe into a file with mode 0600 and was passed to later commands in the `BW_SESSION` environment variable. Captured exports were written to 0600 files and inspected only for structure, IDs, counts and SHA-256 fingerprint matches against the seed. No session, password, TOTP seed, passkey value or ciphertext appears in arguments, logs or this report.

**Seed data.** A generator wrote one unencrypted Bitwarden JSON import file per account. Each holds one login with two passkeys of 13 fields. The keys are four different fresh P-256 keys in PKCS#8, base64url encoded. Counters are 0 and 6 for account A and 3 and 9 for account B. Two files of 4096 random bytes each, both named `probe.bin`, served as attachments. Fingerprints of keys and files were kept as a reference inventory outside the CLI's view.

## Results

### 1. Login with `bw login --raw`

The command ran as `bw login --raw | cat > file`. stdin and stderr were the user's terminal, stdout was a pipe, and no pseudo-terminal was allocated for the child.

| Run | Exit code | stdout |
|---|---:|---|
| Account A, with TOTP prompt | 0 | 88 bytes |
| Account B | 0 | 88 bytes |
| Account A, fresh profile after tampering, with TOTP prompt | 0 | 88 bytes |

In all three runs stdout was exactly one base64 string that decodes to 64 bytes. It had no trailing newline and no other text. The vault was unlocked with this value in `BW_SESSION`. So the login works without a PTY as long as stdin and stderr are a terminal, and stdout carries only the session key.

Two runs without any terminal behaved differently:

| Condition | Exit code | stdout | stderr |
|---|---:|---|---|
| stdin from `/dev/null` | **0** | empty | Prompt `? Email address:`, then a Node `ERR_USE_AFTER_CLOSE` readline crash |
| stdin from `/dev/null`, `BW_NOINTERACTION=true` | 1 | empty | `Email address is required.` |

The first row is an exit code 0 without a session key. The prompt text in that run was written to stderr.

### 2. Seeding

`bw import bitwardenjson <file>` exited with 0 for both accounts. `bw list items` then showed both passkeys per login, each with 13 fields, the seeded counters and key values identical to the seed. `bw create attachment` ran twice on the login of account A and exited with 0 both times. The item then carried two attachments with the same file name `probe.bin` and two different IDs. Both report a size of 4161, which is the encrypted size of the 4096 plaintext bytes.

### 3. Capture of the intact state

All three commands ran with `--raw`, exited with 0 and wrote nothing to stderr.

| Question | `export --format json` | `export --format encrypted_json` | `list items` |
|---|---|---|---|
| Items | 1 | 1 | 1 |
| Passkeys of the login | 2, both key values and counters match the seed | 2 entries, 12 of 13 fields are EncStrings, `creationDate` is plaintext | 2, both match the seed |
| Attachment information | No `attachments` key | No `attachments` key | Both attachment IDs, each with `id`, `fileName`, `size`, `sizeName`, `url` |

**Format.** The item is in the legacy format. The encrypted export has per-field EncStrings, a wrapped item `key`, no `data` key and no `format_version` marker. The server database stores the login the same way, as per-field ciphertext in a JSON column.

**Passkey count.** The account-bound encrypted export shows the passkey count for this legacy item: the length of `login.fido2Credentials` is 2.

**Attachment IDs.** They appear only in `list items` (and `get item`), not in either export.

### 4. Attachment download

| Selector for `get attachment --itemid <item> --raw` | Exit code | stdout |
|---|---:|---|
| First attachment ID | 0 | 4096 bytes, byte-identical to the first seed file |
| Second attachment ID | 0 | 4096 bytes, byte-identical to the second seed file |
| File name `probe.bin` | 1 | empty. stderr: `More than one result was found. Try getting a specific object by id instead.` followed by both IDs |

No file was written to the working directory.

### 5. Freshness

`lastSync` in `bw status` was `2026-11-14T09:04:57.011Z` before and `2026-11-14T09:06:29.835Z` after `bw sync --force`. The new value lies inside the wall-clock window of the sync call. `bw sync --force --raw` exited with 0 and printed nothing. Without `--raw` it printed `Syncing complete.`

### Added case: archived item

The planning session asked for this case during the run. A second synthetic login without passkeys was created in account A and archived with `bw archive item <id>`. The command exited with 0 and printed nothing. Archiving was available for this account on Vaultwarden.

| View | Exit code | Archived item ID present |
|---|---:|---|
| `export --format json` | 0 | yes, with `archivedDate` set |
| `export --format encrypted_json` | 0 | yes, with `archivedDate` set |
| `list items` | 0 | **no** |
| `list items --archived` | 0 | yes, and it is the only item returned |

The fresh profile used in step 6 showed the same four answers after its own full sync. This matches the code reading at the pinned commit: the default list leaves archived items out while both exports keep them. An item count taken from `list items` is therefore lower than the item count of either export.

### 6. Foreign ciphertext in the second `keyValue`

The server was stopped. In the SQLite database, the encrypted `keyValue` of the second credential of account A's login was replaced with the encrypted `keyValue` of the second credential of account B's login. Both are type 2 EncStrings of the same length. Nothing else was changed, and the SQLite integrity check passed. The server was restarted.

The three captures were taken twice: from a fresh profile after a new login, and from the existing profile of account A after `bw sync --force`. Both gave the same result.

| Command | Exit code | Passkeys of the tampered login | stderr |
|---|---:|---:|---|
| `export --format json --raw` | **0** | **0** (`fido2Credentials` is an empty array) | 3 error lines |
| `export --format encrypted_json --raw` | 0 | 2 | empty |
| `list items --raw` | **0** | **0** | 3 error lines |
| `get item <id> --raw` (fresh profile only) | **0** | **0** | 3 error lines |

The untouched first passkey is missing as well. User name, password and URI of the same login were byte-identical to the intact export. Both exports still contained both items.

The stderr lines were one `ERROR bitwarden_crypto::store::context: error=The decryption operation failed` and two `ERROR bitwarden_crypto::traits::decryptable: error=The decryption operation failed`. They name neither the item nor the field.

In the encrypted export, the first credential was identical to the intact capture and the second differed only in `keyValue`. The export passes the stored ciphertext through without decrypting it.

Both attachments were still downloaded byte-exactly by ID with exit code 0, with the same three error lines on stderr.

## Scope

- The findings hold for bw 2026.10.1 from npm on Linux arm64 against Vaultwarden 1.37.4 with SQLite, for personal vault items in the legacy format, in one run with two logins in account A.
- They confirm the 31 October SDK and export service experiment at the level of the real CLI process and its exit code. The outcome matches the tolerant row of that experiment: the item succeeds and all its passkeys are missing.
- In this legacy case a comparison of passkey counts per item between the two exports detects the loss: 2 in the encrypted export against 0 in the JSON export. Equal counts would still not prove equal content.
- Bitwarden Cloud was not involved. Vaultwarden and Bitwarden Cloud must be reported separately, and feature flags delivered by the server can differ between them.
- Contrary to the suggestion in the integration notes, the instance ran inside the development container, on loopback only.

## Not tested

- Bitwarden Cloud, linux/amd64 and other server or database backends.
- Blob-format items. No item in this run carried a `format_version` envelope, so the passkey count for blob items remains unconfirmed.
- Organizations, collections, organization attachments and restricted roles.
- Password-protected export, ZIP export and CSV export.
- A failing or interrupted attachment download, and large attachments.
- A corrupted item key, a missing credential, a credential without key value, and a swapped but decryptable credential with an equal count.
- Whether an exported passkey works for a WebAuthn login. Restore or re-import of the captured exports.
- Archiving on an account that is not allowed to archive, and the trash.
- Stale or incomplete sync, concurrent runs and changes during an export. An advancing `lastSync` does not certify the set of server items.
- `BW_CLEANEXIT`, `bw unlock`, API key login, and login with stdin as a pipe that delivers the answers.
- Development mode of the CLI and plain HTTP beyond the refusal described above.
- Repetition. Every case ran once, except the step 3 captures after tampering, which ran on two profiles.

## Cleanup

The server was stopped after the run and `.local/` was removed with the binary, web vault, certificate, database, CLI profiles, session files, TOTP seed, seed files, captures and helper scripts. Only this report and one row in the research overview remain in the repository.
