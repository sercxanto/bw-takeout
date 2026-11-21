# Limitations

A successful CLI export can deliver decryptable partial inventories while items or passkeys are missing. Stale local state limits the view further. Organizations are backed up only when chosen, and only by an owner or admin, see [Organizations](#organizations). An independent reference inventory is often unknown. See [evidence levels](../research/research-summary.md).

## What a Verdict Can and Cannot Detect

Every capture is judged failed, unverified or verified ([ADR 0004](../decisions/0004-completeness-verdict.md)). `bw-takeout backup` runs the verification on every capture. Its rules are tested with synthetic data, and since 21 November 2026 by two automated runs against a Vaultwarden server with a synthetic account: an intact vault and a passkey whose key was replaced on the server ([Vaultwarden tests](../development/testing.md#vaultwarden-tests)). Bitwarden Cloud has not been tested.

The verification reads two item lists: the default list of `bw list items` and the list of archived items from `bw list items --archived`. It treats them as one listing. An item that both lists show is accepted when both entries name the same attachments and the same organization.

It detects:

- an item that is in the encrypted export but missing from the plaintext export, and the reverse,
- a passkey missing from an item whose passkey count the encrypted export shows,
- a passkey with a missing field, an empty or unparsable key, or a counter that is not a non-negative number,
- an attachment that failed to download, has no download result, or has a result that the item lists do not explain,
- a personal item that one of the item lists shows and the plaintext export lacks,
- an item that both item lists show with different attachments or a different organization.
- for each chosen organization, the same deviations between its plaintext and its encrypted organization export, an item that does not belong to it, an item the item lists show for it and its plaintext export lacks, and an item whose revision date differs between the exports and the item lists, see [Organizations](#organizations).

It cannot detect:

- **Missing passkeys in blob-format items.** The encrypted export seals the content of such items, so their passkey count is not visible. A blob login item is reported as unverified, never as "no passkeys".
- **A swapped passkey or a wrong key without a reference.** If a passkey is replaced by another valid passkey, or a key is well-formed but wrong, the counts and the structure still match. Every result without a reference inventory lists credential identity and attachment content as not checked.
- **A missing attachment entry.** The decrypted item lists are the only source of attachment metadata. An attachment that is missing from an otherwise intact list entry stays undetected. Every result without a reference inventory lists this limit.
- **Items that neither item list shows.** Such items are reported as unverified, because their attachments are unknown.
- **Archived items of an account that may not archive.** According to the code of `bw` 2026.10.1, `bw` ignores `--archived` without an error when the account has no premium, personal or through an organization, and returns the default list again. Archived items of such an account are in neither list and stay unverified. The result then carries a warning that the archive filter did not apply. This case was not observed with a real account, see the [code notes of 16 November 2026](../research/bw-cli-code-notes-2026-11-16.md).
- **The trash.** Items in the trash are in no captured document: both item lists and both exports leave them out. They are not backed up, and the verification does not see them. Backing up the trash is a [non-goal](../product/non-goals.md). This also comes from reading the code of `bw` 2026.10.1 and was not observed at run time.
- **Which item a decryption error concerns.** `bw` can write decryption error lines and still report success. In the [live probe](../research/vaultwarden-live-probe-2026-11-14.md) such lines appeared on the plaintext export, the item list and attachment downloads. The lines name neither item nor field, so only their number is kept. Lines on the plaintext export, the encrypted export or either item list make the verdict at least unverified, and the result names the document and the number of lines. Lines on a successful attachment download only produce a warning, because the size and SHA-256 of the content are recorded. Whether such lines can come with wrong attachment content was not tested.
- **Completeness against the server.** "Verified" covers the personal items in the local snapshot that the CLI exported during the capture and, for each chosen organization, the items of its organization exports. An omission in a step that both exports share, and items that the CLI leaves out of every document, stay invisible. Every result states this scope and lists it as a limit.


## Organizations

`bw-takeout backup --organization <id>` backs up a chosen organization through `bw export --organizationid`, see [usage](usage.md#organizations) and [ADR 0008](../decisions/0008-organization-export.md). Vaultwarden 1.37.4 with `bw` 2026.10.1 is the only server it was tested with: in the [spike of 21 November 2026](../research/organization-export-spike-2026-11-21.md) and in the automated [Vaultwarden tests](../development/testing.md#vaultwarden-tests). Bitwarden Cloud has not been tested.

- **Separate reads from the server.** Unlike the personal exports and the item lists, which come from the local snapshot of one forced sync, each organization export fetches the organization from the server again. The verification compares the revision date of each item in both exports and in the item lists. A difference fails the capture as `organization_changed_during_capture`, and the remedy is to run again; the run does not retry on its own. A change that leaves every revision date as it was stays invisible. Every result with a chosen organization lists this limit as `organization_exports_separate_server_reads`.
- **The role is checked first.** The account must be owner or admin with a confirmed membership, which the run reads from `bw list organizations` after the login. A wrong ID or a missing role therefore costs one login. On Vaultwarden, an organization export of an account without that role was answered with 401, and `bw` then logged itself out, so every later command failed as not logged in. The run avoids this by its own check. A role that changes between the check and the export still ends the run with the organization export, the class `not_logged_in` and the organization ID.
- **Custom roles are refused**, also one with the export permission, because no evidence shows how `bw list organizations` reports it. On Vaultwarden a manager is reported as a custom role and cannot export.
- **Bitwarden Cloud is unknown.** On the Cloud, whether an admin sees the items of collections the admin is not assigned to depends on an organization setting. On Vaultwarden, an admin assigned to one of two collections saw the items of both and downloaded both attachments. An exported item that the item lists do not show is reported as unverified, because its attachments are unknown, so a smaller local view on the Cloud would lower the verdict rather than go unnoticed. Whether the Cloud also answers a refused export with 401, and whether it limits the number of exports, is unknown.
- **Organizations that are not chosen.** Their items stay in `documents/items.json`, because the item list is stored as `bw` wrote it, inside the encryption. They are not compared with any export, their attachments are not downloaded, and no warning names them. They still count when the two item lists are compared with each other: an item that both lists show must have the same attachments in both, and counts toward the warning that `bw` did not apply the archive filter.
- **What was not observed**: organization items in blob format or with passkeys, decryption error lines on an organization export, an organization that changes during a run, and the managed-collections export, which the CLI does not offer. The trash of an organization is in neither export.

## What Offline Verification Shows

`bw-takeout verify` shows that:

- the identity file holds an identity that opens the archive,
- the archive is complete and unaltered: a truncated file or a changed byte fails the check,
- every entry has a name the format defines and matches the size and SHA-256 in the manifest, and no entry is missing or added,
- the manifest has a format version this program knows.

It does not show:

- **Completeness of the capture.** The stored verdict was computed when the backup was made. The check reports it but does not compute it again from the archived documents, and it carries all limits of that verdict described above.
- **Origin.** Anyone who knows a recipient key can create an archive that passes. See [security](security.md).
- **That the documents import elsewhere.** The plaintext export is stored as `bw` wrote it. Whether another program imports it is not tested.
- **The state of other generations.** Each generation is checked on its own.

## Using bw

The program starts the official `bw` program. The handling of it is tested against a stand-in for `bw`, without a login against `bw` 2026.10.1 on Linux, and with a login against `bw` 2026.10.1 and a Vaultwarden server on Linux. The points below come from reading the code of `bw` 2026.10.1 and from runs without a login, see the [code notes](../research/bw-cli-code-notes-2026-11-15.md).

- **One device entry per run.** Each run uses a fresh CLI profile, and each fresh profile registers as a new device. The server keeps one device entry per run with a successful login. In a manual exercise against Vaultwarden, runs whose login failed left no visible entry. `bw` offers no command that removes it, and the logout does not remove it. Whether Bitwarden Cloud limits device entries, throttles repeated logins or sends a message for each new device is unknown.
- **Contact with Bitwarden Cloud also for a self-hosted server.** The first start of a fresh profile requests configuration from `https://api.bitwarden.com`, before the server address of your own server is set. This request carries no account data. It cannot be avoided with the public commands of `bw`.
- **Attachments of personal items may need premium.** According to the code of `bw` 2026.10.1, downloading an attachment of a personal item requires a premium account. This was not observed at run time. Such a download is not retried and is reported as failed, and the capture is never judged more favorably because of it. A failed download makes the capture fail, so an account whose premium has lapsed and that holds attachments on personal items gets no generation at all.
- **Failures that end a run.** Each run starts with a forced sync and then requires `bw status` to report an unlocked vault and a last sync time at or after the start of that sync. A failed sync, a locked vault, or a missing or older last sync time ends the run before any document is requested. A failure of one of the four document commands ends the run as well. A session lost during the attachment downloads, for example a vault that is locked or logged out, ends the run at that attachment, and no further attachment is requested. None of these runs writes a generation.
- **Retried downloads.** A download that times out or ends with a non-zero exit code is tried up to three times, with waits of 2 and 5 seconds. If the third attempt fails as well, the download is reported as failed and the capture fails. Other attachments are still downloaded.
- **Attachments larger than 1 GiB.** The program holds at most 1 GiB of output per `bw` command. A larger attachment fails once, without a retry, and the capture fails in every run.
- **Time limit of 5 minutes per attempt.** `bw` downloads, decrypts and only then writes an attachment, so the limit covers the whole transfer. Within 5 minutes, an attachment of 100 MB needs about 3 Mbit/s and one of 500 MB about 14 Mbit/s. An attachment too large for the connection fails on every attempt and in every run, so the account gets no generation until the limit can be configured. A hanging download costs at most 15 minutes in all.
- **No proxy support yet.** Proxy variables of your environment are not passed to `bw`, so `bw` connects without a proxy. `bw` itself reads only the lowercase `http_proxy` and `https_proxy`. How a proxy is configured is decided together with the configuration format.

## Interrupting a Backup

`bw-takeout backup` can be interrupted with Ctrl-C. A run always tries to log out and to remove its CLI profile, because the profile holds the tokens of the login.

- **The first Ctrl-C** ends the run. A generation that is not committed yet is removed, so that nothing of it stays in the target. The run then prints `Interrupted. Logging out, at most 10 s. Press Ctrl-C again to skip the logout.` on standard error, logs out and removes the profile.
- **A second Ctrl-C** stops the logout at once and prints `Interrupted. Skipping the logout.` The profile is still removed, so the tokens on disk are gone. The report states that the logout was skipped.
- **The logout has a limit of 10 seconds.** A logout that takes longer is stopped and reported as not successful. The profile is removed afterwards in any case. A run that published a generation then ends with exit status 4.
- **Ctrl-C at a prompt of `bw`** during the login, such as the password or two-step prompt, reaches `bw` as a character, not as an interrupt, as seen in the login exercise of 15 November 2026. The login then fails, and the report shows a failed login instead of an interrupt. The logout and the removal of the profile follow as for any failed login.
- **An interrupt during the commit** does not stop it. When the archive is complete and the generation is being renamed to its final name, the commit finishes and the generation is published.
- **Closing the console window on Windows**, logging off and shutting down count as an interrupt. Windows ends the process a few seconds after such an event, so the logout and the removal of the profile may not finish. A profile left behind is listed as a leftover by the next run and must be removed by hand. This comes from the Go documentation and was not observed.
- **The real `bw` on Windows is untested.** On Windows, the console sends Ctrl-C to `bw` as well. The run decides from its own record that it was interrupted, so the report states an interrupt whatever `bw` did. The automated tests run a stand-in for `bw` on Windows, never the real one.

## Storage Guarantees

A generation directory loses its `.partial` marker only after its archive file was flushed to storage. Where the platform supports it, the generation directory and the target directory are flushed as well. Some guarantees depend on the platform and the file system:

- **Windows** cannot flush a directory. After a power loss, a rename that the program reported as done can be lost, and the generation can reappear under its `.partial` name. The archive file itself was flushed before the rename.
- **FAT and exFAT media** keep no journal. After a power loss or when a drive is pulled without ejecting, a directory entry or the end of a file can be damaged even after a successful flush. These file systems also have no POSIX permissions, so the owner-only modes are not applied there.
- **Network shares** are untested. Their flush and rename semantics depend on the server.

Offline verification detects a damaged or truncated archive but cannot repair it. Keep more than one generation, and on removable media eject the drive before unplugging it.

Directory rename, flushing and permissions were tested on Linux only. macOS and Windows have not been tested yet.
