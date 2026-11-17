# Offline Recovery

A backup generation is a directory in your backup target, for example `bw-takeout-20261114T170000Z-1a2b3c4d`. It holds one file, `vault.tar.age`: a tar archive encrypted with [age](https://age-encryption.org). The archive is described in [ADR 0005](../decisions/0005-archive-format.md) and in the file `RECOVERY.txt` inside it.

A directory name ending in `.partial` belongs to a run that has not finished or was aborted. It is not a backup. A name ending in `.unverified` is a complete backup whose completeness could not be confirmed. Its findings are listed in the manifest.

## What to Keep Outside the Archive

- **The age identity** (secret key) that belongs to one of the recipient keys the backup was encrypted to. Without it the archive cannot be opened. Keep it separately from the archive, and keep more than one identity in different places if you encrypt to several recipients.
- **An age program.** The [reference implementation](https://github.com/FiloSottile/age) and other implementations read the format. This project was tested with age 1.3.2 and, before 17 November 2026, with age 1.2.1. Both read the archives of this project.
- **These steps**, for example printed, in case the documentation is not reachable.

A key pair can be created with `age-keygen -o identity.txt`. The command `age-keygen -y identity.txt` prints the recipient key, which is the only part the backing-up system needs.

## Checking a Generation Without Writing Plaintext

Set `G` to the generation directory, for example:

```sh
G=/media/backup/bw-takeout-20261114T170000Z-1a2b3c4d
```

Check it with this project. The check needs no network and no `bw`, and it writes no file:

```sh
bw-takeout verify --identity identity.txt "$G"
```

It reads the whole archive, compares every entry with the manifest and prints the creation time, the server and account, the number of items and attachments, and the stored verdict. Exit status 0 means intact and verified, 3 intact and unverified, 1 failed. See [usage](usage.md).

Without this project, the age and tar commands list the archive without writing plaintext:

```sh
age -d -i identity.txt "$G/vault.tar.age" | tar -tvf -
```

## Recovering With age and tar Alone

**Warning:** unpacking writes the whole vault in clear text to disk, including every password and passkey private key. Do it only on an encrypted or temporary volume, on a system you trust, and delete the files when you are done.

```sh
mkdir recovered
age -d -i identity.txt "$G/vault.tar.age" | tar -xf - -C recovered
```

The directory `recovered` then holds:

- `RECOVERY.txt`: the layout and these instructions.
- `documents/export.json`: the plaintext JSON export of the vault, as `bw` wrote it. Bitwarden, Vaultwarden and several other password managers can import it.
- `documents/export.encrypted.json`: the account-bound encrypted export. Only the account that made it can read it.
- `documents/items.json`: the default item list from `bw list items`. It leaves archived items out.
- `documents/items.archived.json`: the list of archived items from `bw list items --archived`. Together with `documents/items.json` it is the only source that names the attachments. It is `[]` when the vault holds no archived item.
- `attachments/<item-id>/<attachment-id>`: the attachments. Their original file names are in `manifest.json`.
- `manifest.json`: source, versions, the verification result and the size and SHA-256 of every other file.

To check one file by hand, compare its SHA-256 with the manifest:

```sh
sha256sum recovered/documents/export.json
grep -A 3 '"path": "documents/export.json"' recovered/manifest.json
```

When you are done, delete the plaintext:

```sh
rm -r recovered
```

`rm` does not overwrite the data. On an unencrypted disk the content may stay recoverable, which is why an encrypted or temporary volume is recommended.

## Manual Offline Exercise

The product requirements ask for a test on a second system without network access. Developers can leave a synthetic generation for it, see [testing](../development/testing.md). It contains no real vault data.
